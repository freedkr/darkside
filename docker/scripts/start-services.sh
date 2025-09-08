#!/bin/bash

# ===========================================
# 服务启动脚本 - 整合PDF验证和LLM语义分析
# ===========================================

set -euo pipefail

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

# 默认值
ACTION="up"
SERVICES="all"
BUILD=false

# 显示帮助
show_help() {
    cat << EOF
Moonshot 服务启动脚本

用法:
    $0 [选项]

选项:
    -a, --action ACTION    操作 (up|down|restart|logs|ps) [默认: up]
    -s, --services SERVICES 服务 (all|base|app|pdf|llm) [默认: all]
    -b, --build            构建镜像
    -h, --help             显示帮助

服务说明:
    all   - 所有服务
    base  - 基础服务 (postgres, redis, minio)
    app   - 应用服务 (api-server, rule-worker, ai-worker)
    pdf   - PDF验证服务
    llm   - LLM服务

示例:
    # 启动所有服务
    $0

    # 仅启动基础服务
    $0 --services base

    # 构建并启动所有服务
    $0 --build

    # 查看日志
    $0 --action logs --services app
EOF
}

# 解析参数
while [[ $# -gt 0 ]]; do
    case $1 in
        -a|--action)
            ACTION="$2"
            shift 2
            ;;
        -s|--services)
            SERVICES="$2"
            shift 2
            ;;
        -b|--build)
            BUILD=true
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

# 切换到项目根目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
cd "$PROJECT_ROOT"

# 检查环境变量
check_env() {
    log_info "检查环境变量..."
    
    if [ -z "${KIMI_API_KEY:-}" ]; then
        log_warn "未设置KIMI_API_KEY，LLM功能将不可用"
        log_warn "请设置: export KIMI_API_KEY=your_api_key"
    fi
    
    # 创建必要的目录
    mkdir -p logs/llm-service
    mkdir -p services/pdf-validator/data
}

# 构建服务映像
build_services() {
    log_info "构建服务镜像..."
    docker-compose build
}

# 启动服务
start_services() {
    local compose_services=""
    
    case $SERVICES in
        all)
            compose_services=""
            ;;
        base)
            compose_services="postgres redis minio"
            ;;
        app)
            compose_services="api-server rule-worker"
            ;;
        pdf)
            compose_services="pdf-validator"
            ;;
        llm)
            compose_services="llm-service"
            ;;
        *)
            log_error "未知服务组: $SERVICES"
            exit 1
            ;;
    esac
    
    log_info "启动服务: ${compose_services:-所有服务}"
    
    if [ "$BUILD" = true ]; then
        docker-compose up -d --build $compose_services
    else
        docker-compose up -d $compose_services
    fi
    
    log_success "服务启动完成"
    
    # 显示服务状态
    sleep 3
    docker-compose ps
    
    # 显示访问信息
    log_info "服务访问地址:"
    echo "  - API Server:    http://localhost:8080"
    echo "  - PDF Validator: http://localhost:8000"
    echo "  - LLM Service:   http://localhost:8090"
    echo "  - MinIO Console: http://localhost:9001 (用户名: minioadmin, 密码: minioadmin123)"
}

# 停止服务
stop_services() {
    log_info "停止服务..."
    docker-compose down
    log_success "服务已停止"
}

# 重启服务
restart_services() {
    stop_services
    sleep 2
    start_services
}

# 查看日志
show_logs() {
    local compose_services=""
    
    case $SERVICES in
        all)
            compose_services=""
            ;;
        base)
            compose_services="postgres redis minio"
            ;;
        app)
            compose_services="api-server rule-worker"
            ;;
        pdf)
            compose_services="pdf-validator"
            ;;
        llm)
            compose_services="llm-service"
            ;;
    esac
    
    docker-compose logs -f $compose_services
}

# 显示服务状态
show_status() {
    docker-compose ps
}

# 初始化MinIO存储桶
init_minio() {
    log_info "初始化MinIO存储桶..."
    
    # 等待MinIO启动
    sleep 5
    
    # 使用mc客户端配置别名
    docker run --rm --network moonshot-network \
        -e MC_HOST_minio="http://minioadmin:minioadmin123@minio:9000" \
        minio/mc mb minio/moonshot --ignore-existing || true
    
    docker run --rm --network moonshot-network \
        -e MC_HOST_minio="http://minioadmin:minioadmin123@minio:9000" \
        minio/mc mb minio/moonshot-pdf --ignore-existing || true
    
    log_success "MinIO存储桶初始化完成"
}

# 主流程
main() {
    check_env
    
    case $ACTION in
        up)
            if [ "$BUILD" = true ]; then
                build_services
            fi
            start_services
            if [ "$SERVICES" = "all" ] || [ "$SERVICES" = "base" ]; then
                init_minio
            fi
            ;;
        down)
            stop_services
            ;;
        restart)
            restart_services
            if [ "$SERVICES" = "all" ] || [ "$SERVICES" = "base" ]; then
                init_minio
            fi
            ;;
        logs)
            show_logs
            ;;
        ps)
            show_status
            ;;
        *)
            log_error "未知操作: $ACTION"
            show_help
            exit 1
            ;;
    esac
}

# 执行主流程
main