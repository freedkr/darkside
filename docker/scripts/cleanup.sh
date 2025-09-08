#!/bin/bash

# ===========================================
# 环境清理脚本
# ===========================================

set -euo pipefail

# 默认配置
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCKER_DIR="$(dirname "$SCRIPT_DIR")"

ENVIRONMENT="local"
FORCE=false
DRY_RUN=false
CLEAN_VOLUMES=false
CLEAN_NETWORKS=false
CLEAN_IMAGES=false
CLEAN_ALL=false

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

# 显示帮助信息
show_help() {
    cat << EOF
Moonshot Docker 环境清理脚本

用法:
    $0 [选项]

选项:
    -e, --env ENVIRONMENT       环境 (local|staging|prod) [默认: local]
    -f, --force                强制执行，不询问确认
    -d, --dry-run              仅显示将要执行的操作
    --volumes                  清理数据卷
    --networks                 清理网络
    --images                   清理镜像
    --all                      清理所有资源 (容器+卷+网络+镜像)
    -h, --help                 显示帮助信息

示例:
    # 停止并清理本地环境容器
    $0 --env local

    # 清理测试环境的所有资源
    $0 --env staging --all --force

    # 仅清理数据卷
    $0 --env local --volumes

    # 预览清理操作
    $0 --env prod --all --dry-run

警告:
    - 清理数据卷将永久删除所有数据
    - 生产环境清理需要额外确认
    - 建议在清理前备份重要数据
EOF
}

# 解析命令行参数
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -e|--env)
                ENVIRONMENT="$2"
                shift 2
                ;;
            -f|--force)
                FORCE=true
                shift
                ;;
            -d|--dry-run)
                DRY_RUN=true
                shift
                ;;
            --volumes)
                CLEAN_VOLUMES=true
                shift
                ;;
            --networks)
                CLEAN_NETWORKS=true
                shift
                ;;
            --images)
                CLEAN_IMAGES=true
                shift
                ;;
            --all)
                CLEAN_ALL=true
                CLEAN_VOLUMES=true
                CLEAN_NETWORKS=true
                CLEAN_IMAGES=true
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                log_error "未知选项: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# 加载环境配置
load_environment() {
    local env_file="$DOCKER_DIR/environments/.env.$ENVIRONMENT"
    
    if [[ ! -f "$env_file" ]]; then
        log_error "环境配置文件不存在: $env_file"
        exit 1
    fi
    
    set -a
    source "$env_file"
    set +a
    
    log_info "已加载环境配置: $ENVIRONMENT"
}

# 确认操作
confirm_operation() {
    if [[ "$FORCE" == "true" || "$DRY_RUN" == "true" ]]; then
        return 0
    fi
    
    echo ""
    log_warn "即将清理 $ENVIRONMENT 环境的以下资源:"
    echo "  - 容器: 是"
    echo "  - 数据卷: $([[ "$CLEAN_VOLUMES" == "true" ]] && echo "是" || echo "否")"
    echo "  - 网络: $([[ "$CLEAN_NETWORKS" == "true" ]] && echo "是" || echo "否")"
    echo "  - 镜像: $([[ "$CLEAN_IMAGES" == "true" ]] && echo "是" || echo "否")"
    
    if [[ "$ENVIRONMENT" == "prod" ]]; then
        log_error "这是生产环境！请输入 'I UNDERSTAND THE RISKS' 继续:"
        read -r confirmation
        if [[ "$confirmation" != "I UNDERSTAND THE RISKS" ]]; then
            log_info "操作已取消"
            exit 0
        fi
    else
        echo ""
        read -p "确认继续？(y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "操作已取消"
            exit 0
        fi
    fi
}

# 停止并删除容器
cleanup_containers() {
    local compose_file="$DOCKER_DIR/profiles/docker-compose.$ENVIRONMENT.yml"
    
    if [[ ! -f "$compose_file" ]]; then
        log_warn "Docker Compose 配置文件不存在: $compose_file"
        return
    fi
    
    log_info "停止并删除容器..."
    
    local cmd="docker compose -f $compose_file down"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: $cmd"
        return
    fi
    
    cd "$DOCKER_DIR"
    if eval "$cmd"; then
        log_success "容器清理完成"
    else
        log_error "容器清理失败"
    fi
}

# 清理数据卷
cleanup_volumes() {
    if [[ "$CLEAN_VOLUMES" != "true" ]]; then
        return
    fi
    
    log_info "清理数据卷..."
    
    local project_name="${COMPOSE_PROJECT_NAME}"
    local volumes
    
    # 获取项目相关的卷
    if volumes=$(docker volume ls -q --filter "name=${project_name}_" 2>/dev/null); then
        if [[ -n "$volumes" ]]; then
            for volume in $volumes; do
                if [[ "$DRY_RUN" == "true" ]]; then
                    log_info "DRY RUN: docker volume rm $volume"
                else
                    log_info "删除数据卷: $volume"
                    docker volume rm "$volume" 2>/dev/null || log_warn "无法删除数据卷: $volume"
                fi
            done
            
            [[ "$DRY_RUN" != "true" ]] && log_success "数据卷清理完成"
        else
            log_info "没有找到相关数据卷"
        fi
    else
        log_warn "无法列出数据卷"
    fi
}

# 清理网络
cleanup_networks() {
    if [[ "$CLEAN_NETWORKS" != "true" ]]; then
        return
    fi
    
    log_info "清理网络..."
    
    local network_name="${NETWORK_NAME}"
    
    if docker network ls --format "{{.Name}}" | grep -q "^${network_name}$"; then
        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "DRY RUN: docker network rm $network_name"
        else
            log_info "删除网络: $network_name"
            docker network rm "$network_name" 2>/dev/null || log_warn "无法删除网络: $network_name"
            log_success "网络清理完成"
        fi
    else
        log_info "网络 $network_name 不存在"
    fi
}

# 清理镜像
cleanup_images() {
    if [[ "$CLEAN_IMAGES" != "true" ]]; then
        return
    fi
    
    log_info "清理镜像..."
    
    # 清理悬空镜像
    local dangling_images
    if dangling_images=$(docker images -q --filter "dangling=true" 2>/dev/null); then
        if [[ -n "$dangling_images" ]]; then
            if [[ "$DRY_RUN" == "true" ]]; then
                log_info "DRY RUN: docker rmi $dangling_images"
            else
                log_info "删除悬空镜像..."
                docker rmi $dangling_images 2>/dev/null || log_warn "部分悬空镜像删除失败"
            fi
        fi
    fi
    
    # 清理项目相关镜像
    local project_images
    if project_images=$(docker images --format "{{.Repository}}:{{.Tag}}" | grep "^moonshot/" 2>/dev/null); then
        for image in $project_images; do
            if [[ "$DRY_RUN" == "true" ]]; then
                log_info "DRY RUN: docker rmi $image"
            else
                log_info "删除项目镜像: $image"
                docker rmi "$image" 2>/dev/null || log_warn "无法删除镜像: $image"
            fi
        done
    fi
    
    [[ "$DRY_RUN" != "true" ]] && log_success "镜像清理完成"
}

# 清理系统资源
cleanup_system() {
    log_info "清理Docker系统资源..."
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: docker system prune -f"
        return
    fi
    
    # 清理未使用的资源
    docker system prune -f >/dev/null 2>&1 || log_warn "系统清理可能未完全成功"
    
    log_success "系统资源清理完成"
}

# 显示清理后状态
show_status() {
    if [[ "$DRY_RUN" == "true" ]]; then
        return
    fi
    
    echo ""
    log_info "清理后状态:"
    
    # 显示剩余容器
    local containers
    if containers=$(docker ps -a --filter "name=${COMPOSE_PROJECT_NAME}" --format "{{.Names}}" 2>/dev/null); then
        if [[ -n "$containers" ]]; then
            log_warn "剩余容器:"
            echo "$containers"
        else
            log_success "所有容器已清理"
        fi
    fi
    
    # 显示剩余数据卷
    if [[ "$CLEAN_VOLUMES" == "true" ]]; then
        local volumes
        if volumes=$(docker volume ls -q --filter "name=${COMPOSE_PROJECT_NAME}_" 2>/dev/null); then
            if [[ -n "$volumes" ]]; then
                log_warn "剩余数据卷:"
                echo "$volumes"
            else
                log_success "所有数据卷已清理"
            fi
        fi
    fi
    
    # 显示剩余网络
    if [[ "$CLEAN_NETWORKS" == "true" ]]; then
        if docker network ls --format "{{.Name}}" | grep -q "^${NETWORK_NAME}$"; then
            log_warn "网络仍然存在: ${NETWORK_NAME}"
        else
            log_success "网络已清理"
        fi
    fi
    
    # 显示磁盘使用情况
    echo ""
    log_info "Docker磁盘使用情况:"
    docker system df
}

# 主函数
main() {
    log_info "Moonshot Docker 环境清理脚本启动"
    
    parse_args "$@"
    load_environment
    confirm_operation
    
    echo ""
    log_info "开始清理 $ENVIRONMENT 环境..."
    
    # 执行清理操作
    cleanup_containers
    cleanup_volumes
    cleanup_networks
    cleanup_images
    
    # 清理系统资源
    if [[ "$CLEAN_ALL" == "true" ]]; then
        cleanup_system
    fi
    
    # 显示状态
    show_status
    
    echo ""
    log_success "环境清理完成"
}

# 脚本入口
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi