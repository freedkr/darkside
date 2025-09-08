#!/bin/bash

# PDF Validator Service 管理脚本
# 用于在统一的Docker环境中管理PDF Validator服务

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DOCKER_DIR="${PROJECT_ROOT}/docker"
SERVICE_DIR="${PROJECT_ROOT}/services/pdf-validator"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 函数：打印消息
print_message() {
    echo -e "${GREEN}[PDF-Validator]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# 函数：检查服务状态
check_status() {
    print_message "检查PDF Validator服务状态..."
    
    if docker ps --format "table {{.Names}}\t{{.Status}}" | grep -q "pdf-validator"; then
        echo -e "${GREEN}✅ PDF Validator服务运行中${NC}"
        docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" | grep "pdf-validator"
    else
        echo -e "${YELLOW}⚠️ PDF Validator服务未运行${NC}"
    fi
}

# 函数：启动服务
start_service() {
    print_message "启动PDF Validator服务..."
    
    cd "${DOCKER_DIR}"
    
    # 使用本地配置启动
    if [ "$1" == "local" ]; then
        docker-compose -f profiles/docker-compose.local.yml up -d pdf-validator-api pdf-validator-worker pdf-validator-flower
    else
        # 使用独立部署配置
        cd "${DOCKER_DIR}/deploy/pdf-validator"
        docker-compose up -d
    fi
    
    print_message "服务启动完成"
    print_message "API: http://localhost:8001"
    print_message "Flower: http://localhost:5555"
}

# 函数：停止服务
stop_service() {
    print_message "停止PDF Validator服务..."
    
    if [ "$1" == "local" ]; then
        cd "${DOCKER_DIR}"
        docker-compose -f profiles/docker-compose.local.yml stop pdf-validator-api pdf-validator-worker pdf-validator-flower
    else
        cd "${DOCKER_DIR}/deploy/pdf-validator"
        docker-compose down
    fi
    
    print_message "服务已停止"
}

# 函数：重启服务
restart_service() {
    print_message "重启PDF Validator服务..."
    stop_service "$1"
    sleep 2
    start_service "$1"
}

# 函数：查看日志
view_logs() {
    service="${1:-all}"
    
    if [ "$service" == "api" ]; then
        docker logs -f --tail=100 $(docker ps -q -f name=pdf-validator-api)
    elif [ "$service" == "worker" ]; then
        docker logs -f --tail=100 $(docker ps -q -f name=pdf-validator-worker)
    elif [ "$service" == "flower" ]; then
        docker logs -f --tail=100 $(docker ps -q -f name=pdf-validator-flower)
    else
        print_message "查看所有PDF Validator服务日志..."
        docker-compose -f "${DOCKER_DIR}/profiles/docker-compose.local.yml" logs -f --tail=100 pdf-validator-api pdf-validator-worker
    fi
}

# 函数：重建服务
rebuild_service() {
    print_message "重建PDF Validator服务镜像..."
    
    cd "${SERVICE_DIR}"
    docker build -t pdf-validator:latest .
    
    print_message "镜像重建完成"
}

# 函数：运行测试
run_tests() {
    print_message "运行PDF Validator测试..."
    
    # 健康检查
    echo -n "API健康检查: "
    if curl -s http://localhost:8001/health > /dev/null 2>&1; then
        echo -e "${GREEN}✅ 正常${NC}"
    else
        echo -e "${RED}❌ 失败${NC}"
    fi
    
    # Redis连接测试
    echo -n "Redis连接: "
    if docker exec $(docker ps -q -f name=redis) redis-cli ping > /dev/null 2>&1; then
        echo -e "${GREEN}✅ 正常${NC}"
    else
        echo -e "${RED}❌ 失败${NC}"
    fi
    
    # PostgreSQL连接测试
    echo -n "PostgreSQL连接: "
    if docker exec $(docker ps -q -f name=postgres) pg_isready > /dev/null 2>&1; then
        echo -e "${GREEN}✅ 正常${NC}"
    else
        echo -e "${RED}❌ 失败${NC}"
    fi
}

# 函数：显示帮助
show_help() {
    echo "PDF Validator Service 管理脚本"
    echo ""
    echo "用法: $0 [命令] [选项]"
    echo ""
    echo "命令:"
    echo "  start [local]   - 启动服务 (local: 使用统一配置)"
    echo "  stop [local]    - 停止服务"
    echo "  restart [local] - 重启服务"
    echo "  status          - 查看服务状态"
    echo "  logs [service]  - 查看日志 (api/worker/flower/all)"
    echo "  build           - 重建Docker镜像"
    echo "  test            - 运行服务测试"
    echo "  help            - 显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 start              # 使用独立配置启动"
    echo "  $0 start local        # 使用统一配置启动"
    echo "  $0 logs api           # 查看API日志"
    echo "  $0 status             # 查看服务状态"
}

# 主逻辑
case "$1" in
    start)
        start_service "$2"
        ;;
    stop)
        stop_service "$2"
        ;;
    restart)
        restart_service "$2"
        ;;
    status)
        check_status
        ;;
    logs)
        view_logs "$2"
        ;;
    build)
        rebuild_service
        ;;
    test)
        run_tests
        ;;
    help|"")
        show_help
        ;;
    *)
        print_error "未知命令: $1"
        show_help
        exit 1
        ;;
esac