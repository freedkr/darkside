#!/bin/bash

# ===========================================
# 服务健康检查脚本
# ===========================================

set -euo pipefail

# 默认配置
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCKER_DIR="$(dirname "$SCRIPT_DIR")"

ENVIRONMENT="local"
TIMEOUT=30
VERBOSE=false
JSON_OUTPUT=false

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 日志函数
log_info() {
    [[ "$VERBOSE" == "true" ]] && echo -e "${BLUE}[INFO]${NC} $1"
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
Moonshot Docker 健康检查脚本

用法:
    $0 [选项]

选项:
    -e, --env ENVIRONMENT       环境 (local|staging|prod) [默认: local]
    -t, --timeout TIMEOUT       超时时间(秒) [默认: 30]
    -j, --json                  JSON格式输出
    -v, --verbose              详细输出
    -h, --help                 显示帮助信息

示例:
    # 检查本地环境所有服务
    $0 --env local

    # 检查生产环境健康状态，JSON输出
    $0 --env prod --json

    # 详细检查测试环境
    $0 --env staging --verbose --timeout 60
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
            -t|--timeout)
                TIMEOUT="$2"
                shift 2
                ;;
            -j|--json)
                JSON_OUTPUT=true
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

# 检查容器状态
check_container_status() {
    local container_name="$1"
    local status
    
    if status=$(docker inspect --format='{{.State.Status}}' "$container_name" 2>/dev/null); then
        echo "$status"
    else
        echo "not_found"
    fi
}

# 检查容器健康状态
check_container_health() {
    local container_name="$1"
    local health
    
    if health=$(docker inspect --format='{{.State.Health.Status}}' "$container_name" 2>/dev/null); then
        if [[ "$health" == "<no value>" ]]; then
            echo "no_healthcheck"
        else
            echo "$health"
        fi
    else
        echo "unknown"
    fi
}

# 检查端口连通性
check_port_connectivity() {
    local host="$1"
    local port="$2"
    local timeout="${3:-5}"
    
    if timeout "$timeout" bash -c "</dev/tcp/$host/$port" 2>/dev/null; then
        echo "open"
    else
        echo "closed"
    fi
}

# 检查HTTP服务
check_http_service() {
    local url="$1"
    local expected_status="${2:-200}"
    local timeout="${3:-10}"
    
    local response
    if response=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout "$timeout" "$url" 2>/dev/null); then
        if [[ "$response" == "$expected_status" ]]; then
            echo "healthy"
        else
            echo "unhealthy:$response"
        fi
    else
        echo "unreachable"
    fi
}

# 检查数据库连接
check_database() {
    local container_name="${COMPOSE_PROJECT_NAME}-postgres"
    local db_name="${POSTGRES_DB}"
    local db_user="${POSTGRES_USER}"
    
    if docker exec "$container_name" pg_isready -U "$db_user" -d "$db_name" >/dev/null 2>&1; then
        echo "healthy"
    else
        echo "unhealthy"
    fi
}

# 检查Redis连接
check_redis() {
    local container_name="${COMPOSE_PROJECT_NAME}-redis"
    
    if docker exec "$container_name" redis-cli ping >/dev/null 2>&1; then
        echo "healthy"
    else
        echo "unhealthy"
    fi
}

# 检查MinIO服务
check_minio() {
    local container_name="${COMPOSE_PROJECT_NAME}-minio"
    
    if docker exec "$container_name" curl -f http://localhost:9000/minio/health/live >/dev/null 2>&1; then
        echo "healthy"
    else
        echo "unhealthy"
    fi
}

# 获取服务列表
get_services() {
    local compose_file="$DOCKER_DIR/profiles/docker-compose.$ENVIRONMENT.yml"
    
    if [[ -f "$compose_file" ]]; then
        docker compose -f "$compose_file" config --services 2>/dev/null || echo ""
    else
        echo ""
    fi
}

# 执行健康检查
perform_health_check() {
    local results=()
    local overall_status="healthy"
    local project_name="${COMPOSE_PROJECT_NAME}"
    
    log_info "开始健康检查 - 环境: $ENVIRONMENT"
    
    # 检查基础设施服务
    local services=(
        "postgres:5432"
        "redis:6379"
        "minio:9000"
    )
    
    # 检查应用服务
    if [[ "$ENVIRONMENT" == "local" ]]; then
        services+=(
            "api-server:8080"
            "pdf-validator-api:8001"
        )
    fi
    
    for service_info in "${services[@]}"; do
        IFS=':' read -r service port <<< "$service_info"
        local container_name="${project_name}-${service}"
        
        log_info "检查服务: $service"
        
        # 检查容器状态
        local container_status
        container_status=$(check_container_status "$container_name")
        
        # 检查健康状态
        local health_status
        health_status=$(check_container_health "$container_name")
        
        # 检查端口连通性
        local port_status=""
        if [[ "$container_status" == "running" ]]; then
            case $service in
                postgres)
                    port_status=$(check_database)
                    ;;
                redis)
                    port_status=$(check_redis)
                    ;;
                minio)
                    port_status=$(check_minio)
                    ;;
                *)
                    if [[ "$ENVIRONMENT" == "local" && -n "${!service_port:-}" ]]; then
                        local external_port="${!service_port}"
                        port_status=$(check_port_connectivity "localhost" "$external_port")
                    else
                        port_status="not_checked"
                    fi
                    ;;
            esac
        else
            port_status="container_down"
        fi
        
        # 记录结果
        local service_status="healthy"
        if [[ "$container_status" != "running" ]]; then
            service_status="unhealthy"
            overall_status="unhealthy"
        elif [[ "$health_status" == "unhealthy" ]]; then
            service_status="unhealthy"
            overall_status="unhealthy"
        elif [[ "$port_status" =~ ^(closed|unreachable|unhealthy)$ ]]; then
            service_status="degraded"
            [[ "$overall_status" == "healthy" ]] && overall_status="degraded"
        fi
        
        results+=("$service:$service_status:$container_status:$health_status:$port_status")
        
        if [[ "$VERBOSE" == "true" ]]; then
            echo "  容器状态: $container_status"
            echo "  健康检查: $health_status"
            echo "  连通性: $port_status"
            echo "  服务状态: $service_status"
            echo ""
        fi
    done
    
    # 输出结果
    output_results "$overall_status" "${results[@]}"
}

# 输出结果
output_results() {
    local overall_status="$1"
    shift
    local results=("$@")
    
    if [[ "$JSON_OUTPUT" == "true" ]]; then
        output_json_results "$overall_status" "${results[@]}"
    else
        output_text_results "$overall_status" "${results[@]}"
    fi
}

# 输出文本格式结果
output_text_results() {
    local overall_status="$1"
    shift
    local results=("$@")
    
    echo ""
    echo "================================"
    echo "健康检查报告 - $ENVIRONMENT 环境"
    echo "================================"
    echo "检查时间: $(date)"
    echo "总体状态: $overall_status"
    echo ""
    
    printf "%-20s %-10s %-15s %-15s %-15s\n" "服务" "状态" "容器状态" "健康检查" "连通性"
    echo "--------------------------------------------------------------------------------"
    
    for result in "${results[@]}"; do
        IFS=':' read -r service status container_status health_status port_status <<< "$result"
        
        local status_color=""
        case $status in
            healthy) status_color="${GREEN}" ;;
            degraded) status_color="${YELLOW}" ;;
            unhealthy) status_color="${RED}" ;;
        esac
        
        printf "%-20s ${status_color}%-10s${NC} %-15s %-15s %-15s\n" \
            "$service" "$status" "$container_status" "$health_status" "$port_status"
    done
    
    echo ""
    
    case $overall_status in
        healthy)
            log_success "所有服务运行正常"
            ;;
        degraded)
            log_warn "部分服务存在问题"
            ;;
        unhealthy)
            log_error "检测到服务故障"
            ;;
    esac
}

# 输出JSON格式结果
output_json_results() {
    local overall_status="$1"
    shift
    local results=("$@")
    
    echo "{"
    echo "  \"timestamp\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\","
    echo "  \"environment\": \"$ENVIRONMENT\","
    echo "  \"overall_status\": \"$overall_status\","
    echo "  \"services\": {"
    
    local first=true
    for result in "${results[@]}"; do
        IFS=':' read -r service status container_status health_status port_status <<< "$result"
        
        [[ "$first" == "true" ]] && first=false || echo ","
        
        echo -n "    \"$service\": {"
        echo -n "\"status\": \"$status\", "
        echo -n "\"container_status\": \"$container_status\", "
        echo -n "\"health_status\": \"$health_status\", "
        echo -n "\"port_status\": \"$port_status\""
        echo -n "}"
    done
    
    echo ""
    echo "  }"
    echo "}"
}

# 主函数
main() {
    parse_args "$@"
    load_environment
    
    local results=()
    local overall_status="healthy"
    perform_health_check
    
    # 根据整体状态设置退出码
    case $overall_status in
        healthy) exit 0 ;;
        degraded) exit 1 ;;
        unhealthy) exit 2 ;;
    esac
}

# 脚本入口
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi