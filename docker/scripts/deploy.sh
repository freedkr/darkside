#!/bin/bash

# ===========================================
# 统一部署脚本
# ===========================================

set -euo pipefail

# 默认配置
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCKER_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$DOCKER_DIR")"

# 默认值
ENVIRONMENT="local"
SERVICES="all"
ACTION="up"
PROFILES=""
DRY_RUN=false
VERBOSE=false
FORCE_RECREATE=false
BUILD=false
NO_CACHE=false

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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
Moonshot Docker 统一部署脚本

用法:
    $0 [选项]

选项:
    -e, --env ENVIRONMENT       环境 (local|staging|prod) [默认: local]
    -s, --services SERVICES     服务列表 (all|base|api|workers|pdf|llm|monitoring|vector) [默认: all]
    -a, --action ACTION         操作 (up|down|restart|logs|ps|build) [默认: up]
    -p, --profiles PROFILES     Docker Compose profiles
    -f, --force                 强制重新创建容器
    -b, --build                 构建镜像
    --no-cache                  构建时不使用缓存
    -d, --dry-run              仅显示将要执行的命令
    -v, --verbose              详细输出
    -h, --help                 显示帮助信息

示例:
    # 启动本地开发环境
    $0 --env local --services all

    # 启动生产环境的基础服务
    $0 --env prod --services base --profiles prod

    # 查看测试环境日志
    $0 --env staging --action logs --services api

    # 构建并启动开发环境
    $0 --env local --build --profiles dev
    
    # 强制无缓存重新构建特定服务
    $0 --env local --action build --services rule-worker --no-cache
    
    # 启动LLM服务开发模式（需要Go源码热重载）
    $0 --env local --profiles dev --services llm

    # 重启特定服务
    $0 --env local --action restart --services workers

环境说明:
    local     本地开发环境 (所有端口暴露, 开发工具启用)
    staging   测试环境 (启用监控, 资源限制)
    prod      生产环境 (安全配置, 高可用, 不暴露内部端口)

服务组合:
    all       所有服务
    base      基础服务 (postgres, redis, minio)
    api       API服务器
    workers   工作节点 (rule-worker)
    pdf       PDF验证服务
    llm       LLM服务 (llm-service)
    monitoring 监控栈 (prometheus, grafana, loki)
    vector    向量数据库 (milvus, etcd)
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
            -s|--services)
                SERVICES="$2"
                shift 2
                ;;
            -a|--action)
                ACTION="$2"
                shift 2
                ;;
            -p|--profiles)
                PROFILES="$2"
                shift 2
                ;;
            -f|--force)
                FORCE_RECREATE=true
                shift
                ;;
            -b|--build)
                BUILD=true
                shift
                ;;
            --no-cache)
                NO_CACHE=true
                shift
                ;;
            -d|--dry-run)
                DRY_RUN=true
                shift
                ;;
            -v|--verbose)
                VERBOSE=true
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

# 验证环境
validate_environment() {
    case $ENVIRONMENT in
        local|staging|prod)
            ;;
        *)
            log_error "无效的环境: $ENVIRONMENT"
            exit 1
            ;;
    esac

    # 检查环境文件
    local env_file="$DOCKER_DIR/environments/.env.$ENVIRONMENT"
    if [[ ! -f "$env_file" ]]; then
        log_error "环境配置文件不存在: $env_file"
        exit 1
    fi
}

# 设置环境变量
setup_environment() {
    local env_file="$DOCKER_DIR/environments/.env.$ENVIRONMENT"
    
    log_info "加载环境配置: $env_file"
    
    # 导出环境变量
    set -a
    source "$env_file"
    set +a
    
    # 设置默认的 COMPOSE_PROFILES，但优先使用 .env 文件中的设置
    if [[ -z "$PROFILES" ]]; then
        if [[ -n "$COMPOSE_PROFILES" ]]; then
            PROFILES="$COMPOSE_PROFILES"
        else
            case $ENVIRONMENT in
                local)
                    PROFILES="simple"
                    ;;
                staging)
                    PROFILES="simple,monitoring"
                    ;;
                prod)
                    PROFILES="prod,monitoring"
                    ;;
            esac
        fi
    fi
    
    export COMPOSE_PROFILES="$PROFILES"
    
    if [[ "$VERBOSE" == "true" ]]; then
        log_info "环境变量:"
        log_info "  COMPOSE_PROJECT_NAME=$COMPOSE_PROJECT_NAME"
        log_info "  COMPOSE_PROFILES=$COMPOSE_PROFILES"
        log_info "  NETWORK_NAME=$NETWORK_NAME"
    fi
}

# 构建 Docker Compose 命令
build_compose_command() {
    local compose_file="$DOCKER_DIR/profiles/docker-compose.$ENVIRONMENT.yml"
    local env_file="$DOCKER_DIR/environments/.env.$ENVIRONMENT"
    if [[ ! -f "$compose_file" ]]; then
        log_error "Docker Compose 配置文件不存在: $compose_file"
        exit 1
    fi
    
    local cmd="docker compose -f $compose_file --env-file $env_file"
    
    # 构建服务过滤器
    local service_filter=""
    case $SERVICES in
        all)
            service_filter=""
            ;;
        base)
            service_filter="postgres redis minio"
            ;;
        api)
            service_filter="api-server api-server-dev"
            ;;
        workers)
            service_filter="rule-worker rule-worker-dev"
            ;;
        pdf)
            service_filter="pdf-validator-api pdf-validator-worker pdf-validator-flower"
            ;;
        llm)
            service_filter="llm-service llm-service-dev"
            ;;
        monitoring)
            service_filter="prometheus grafana loki promtail"
            ;;
        vector)
            service_filter="etcd minio-vector milvus"
            ;;
        *)
            # 自定义服务列表
            service_filter="$SERVICES"
            ;;
    esac
    
    # 构建完整命令
    case $ACTION in
        up)
            cmd="$cmd up"
            if [[ "$BUILD" == "true" ]]; then
                cmd="$cmd --build"
                [[ "$NO_CACHE" == "true" ]] && cmd="$cmd --no-cache"
            fi
            [[ "$FORCE_RECREATE" == "true" ]] && cmd="$cmd --force-recreate"
            cmd="$cmd -d"
            ;;
        down)
            cmd="$cmd down"
            ;;
        restart)
            cmd="$cmd restart"
            ;;
        logs)
            cmd="$cmd logs -f --tail=100"
            ;;
        ps)
            cmd="$cmd ps"
            ;;
        build)
            cmd="$cmd build"
            [[ "$NO_CACHE" == "true" ]] && cmd="$cmd --no-cache"
            ;;
        *)
            log_error "无效的操作: $ACTION"
            exit 1
            ;;
    esac
    
    # 添加服务过滤器
    if [[ -n "$service_filter" ]]; then
        cmd="$cmd $service_filter"
    fi
    
    echo "$cmd"
}

# 执行前检查
pre_flight_check() {
    log_info "执行部署前检查..."
    
    # 检查 Docker 是否运行
    if ! docker info >/dev/null 2>&1; then
        log_error "Docker 未运行或无权限访问"
        exit 1
    fi
    
    # 检查 Docker Compose 版本
    local compose_version
    if compose_version=$(docker compose version 2>/dev/null); then
        log_info "Docker Compose 版本: $compose_version"
    else
        log_error "Docker Compose 不可用"
        exit 1
    fi
    
    # 网络将由 Docker Compose 自动管理
    
    # 检查磁盘空间
    local available_space
    available_space=$(df . | tail -1 | awk '{print $4}')
    if [[ "$available_space" -lt 1048576 ]]; then  # 1GB in KB
        log_warn "可用磁盘空间不足 1GB，可能影响容器运行"
    fi
    
    log_success "部署前检查完成"
}

# 执行命令
execute_command() {
    local cmd="$1"
    
    log_info "执行环境: $ENVIRONMENT"
    log_info "目标服务: $SERVICES"
    log_info "执行操作: $ACTION"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN - 将要执行的命令:"
        echo "$cmd"
        return 0
    fi
    
    log_info "执行命令: $cmd"
    
    cd "$DOCKER_DIR"
    
    # 执行命令
    if eval "$cmd"; then
        log_success "操作完成: $ACTION"
        
        # 显示状态信息
        if [[ "$ACTION" == "up" ]]; then
            echo ""
            log_info "服务状态:"
            docker compose -f "profiles/docker-compose.$ENVIRONMENT.yml" ps
            
            # 显示访问信息
            show_access_info
        fi
    else
        log_error "操作失败: $ACTION"
        exit 1
    fi
}

# 显示访问信息
show_access_info() {
    local project_name="${COMPOSE_PROJECT_NAME:-moonshot-$ENVIRONMENT}"
    
    echo ""
    log_info "服务访问信息:"
    
    case $ENVIRONMENT in
        local)
            echo "  API服务器:      http://localhost:${API_EXTERNAL_PORT:-8080}"
            echo "  PostgreSQL:     localhost:${POSTGRES_EXTERNAL_PORT:-5432}"
            echo "  Redis:          localhost:${REDIS_EXTERNAL_PORT:-6379}"
            echo "  MinIO:          http://localhost:${MINIO_EXTERNAL_PORT:-29000}"
            echo "  MinIO控制台:     http://localhost:${MINIO_CONSOLE_EXTERNAL_PORT:-29001}"
            echo "  PDF验证服务:     http://localhost:${PDF_VALIDATOR_EXTERNAL_PORT:-8001}"
            echo "  LLM服务:        http://localhost:${LLM_SERVICE_EXTERNAL_PORT:-8090}"
            echo "  Celery Flower:  http://localhost:${FLOWER_EXTERNAL_PORT:-5555}"
            echo "  数据库管理:      http://localhost:8080 (Adminer)"
            echo "  Redis管理:      http://localhost:8081 (Redis Commander)"
            ;;
        staging|prod)
            if [[ "${GRAFANA_EXTERNAL_PORT:-}" ]]; then
                echo "  监控面板:       http://localhost:${GRAFANA_EXTERNAL_PORT:-3000}"
            fi
            if [[ "${PROMETHEUS_EXTERNAL_PORT:-}" ]]; then
                echo "  Prometheus:     http://localhost:${PROMETHEUS_EXTERNAL_PORT:-9090}"
            fi
            ;;
    esac
    
    echo ""
    log_info "常用命令:"
    echo "  查看日志:       $0 --env $ENVIRONMENT --action logs"
    echo "  查看状态:       $0 --env $ENVIRONMENT --action ps"
    echo "  重启服务:       $0 --env $ENVIRONMENT --action restart"
    echo "  停止服务:       $0 --env $ENVIRONMENT --action down"
}

# 主函数
main() {
    log_info "Moonshot Docker 统一部署脚本启动"
    
    parse_args "$@"
    validate_environment
    setup_environment
    pre_flight_check
    
    local compose_command
    compose_command=$(build_compose_command)
    
    execute_command "$compose_command"
    
    log_success "部署脚本执行完成"
}

# 脚本入口
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi