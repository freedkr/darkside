#!/bin/bash

# LLM Service 启动脚本
set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的信息
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 获取脚本目录
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
SERVICE_DIR="$( cd "$SCRIPT_DIR/.." &> /dev/null && pwd )"

print_info "LLM Service 启动脚本"
print_info "服务目录: $SERVICE_DIR"

# 检查必要的环境变量
check_env() {
    print_info "检查环境变量..."
    
    if [ -z "$KIMI_API_KEY" ]; then
        print_warning "KIMI_API_KEY 未设置，Kimi提供商将不可用"
    else
        print_success "KIMI_API_KEY 已设置"
    fi
    
    # 设置默认值
    export LLM_PORT=${LLM_PORT:-8080}
    export LLM_MAX_WORKERS=${LLM_MAX_WORKERS:-10}
    export LLM_ENABLE_CORS=${LLM_ENABLE_CORS:-true}
    export LLM_ENABLE_WEBSOCKET=${LLM_ENABLE_WEBSOCKET:-true}
    
    print_info "服务端口: $LLM_PORT"
    print_info "最大工作协程: $LLM_MAX_WORKERS"
}

# 检查端口是否可用
check_port() {
    local port=$1
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
        print_error "端口 $port 已被占用"
        lsof -Pi :$port -sTCP:LISTEN
        return 1
    fi
    return 0
}

# 构建服务
build_service() {
    print_info "构建 LLM Service..."
    
    cd "$SERVICE_DIR"
    
    # 检查是否有Go环境
    if ! command -v go &> /dev/null; then
        print_error "Go 未安装或不在PATH中"
        exit 1
    fi
    
    # 构建
    go build -o llm-service main.go
    
    if [ $? -eq 0 ]; then
        print_success "构建完成"
    else
        print_error "构建失败"
        exit 1
    fi
}

# 启动服务
start_service() {
    print_info "启动 LLM Service..."
    
    cd "$SERVICE_DIR"
    
    # 创建日志目录
    mkdir -p logs
    
    # 启动服务
    if [ "$1" = "--daemon" ] || [ "$1" = "-d" ]; then
        # 后台运行
        nohup ./llm-service > logs/llm-service.log 2>&1 &
        local pid=$!
        echo $pid > llm-service.pid
        print_success "服务已在后台启动 (PID: $pid)"
        print_info "日志文件: $SERVICE_DIR/logs/llm-service.log"
        print_info "PID文件: $SERVICE_DIR/llm-service.pid"
    else
        # 前台运行
        ./llm-service
    fi
}

# Docker 启动
start_docker() {
    print_info "使用 Docker 启动服务..."
    
    cd "$SERVICE_DIR"
    
    # 检查Docker是否可用
    if ! command -v docker &> /dev/null; then
        print_error "Docker 未安装或不在PATH中"
        exit 1
    fi
    
    # 检查docker-compose
    if ! command -v docker-compose &> /dev/null; then
        print_error "docker-compose 未安装或不在PATH中"
        exit 1
    fi
    
    # 创建必要的目录
    mkdir -p logs monitoring/grafana/{dashboards,datasources}
    
    # 启动服务
    docker-compose up --build -d
    
    if [ $? -eq 0 ]; then
        print_success "Docker 服务启动成功"
        print_info "访问 http://localhost:8080/health 检查服务状态"
        print_info "查看日志: docker-compose logs -f llm-service"
    else
        print_error "Docker 服务启动失败"
        exit 1
    fi
}

# 停止服务
stop_service() {
    print_info "停止 LLM Service..."
    
    cd "$SERVICE_DIR"
    
    if [ -f "llm-service.pid" ]; then
        local pid=$(cat llm-service.pid)
        if kill -0 $pid 2>/dev/null; then
            kill $pid
            print_success "服务已停止 (PID: $pid)"
            rm -f llm-service.pid
        else
            print_warning "进程不存在 (PID: $pid)"
            rm -f llm-service.pid
        fi
    else
        print_warning "PID文件不存在，尝试查找进程..."
        local pid=$(pgrep -f llm-service)
        if [ ! -z "$pid" ]; then
            kill $pid
            print_success "服务已停止 (PID: $pid)"
        else
            print_warning "未找到运行中的服务"
        fi
    fi
}

# 重启服务
restart_service() {
    print_info "重启 LLM Service..."
    stop_service
    sleep 2
    start_service "$@"
}

# 显示状态
show_status() {
    print_info "检查 LLM Service 状态..."
    
    cd "$SERVICE_DIR"
    
    if [ -f "llm-service.pid" ]; then
        local pid=$(cat llm-service.pid)
        if kill -0 $pid 2>/dev/null; then
            print_success "服务运行中 (PID: $pid)"
            
            # 检查端口
            if lsof -Pi :${LLM_PORT:-8080} -sTCP:LISTEN -t >/dev/null 2>&1; then
                print_success "端口 ${LLM_PORT:-8080} 正在监听"
                
                # 检查健康状态
                if command -v curl &> /dev/null; then
                    local health=$(curl -s http://localhost:${LLM_PORT:-8080}/health | grep -o '"status":"healthy"' || echo "")
                    if [ ! -z "$health" ]; then
                        print_success "健康检查通过"
                    else
                        print_warning "健康检查失败"
                    fi
                fi
            else
                print_warning "端口 ${LLM_PORT:-8080} 未在监听"
            fi
        else
            print_error "进程不存在 (PID: $pid)"
            rm -f llm-service.pid
        fi
    else
        print_info "服务未运行"
    fi
}

# 显示帮助信息
show_help() {
    echo "LLM Service 管理脚本"
    echo ""
    echo "用法: $0 [命令] [选项]"
    echo ""
    echo "命令:"
    echo "  start [-d|--daemon]  启动服务 (可选后台运行)"
    echo "  stop                 停止服务"
    echo "  restart [-d|--daemon] 重启服务"
    echo "  status               显示服务状态"
    echo "  docker               使用Docker启动服务"
    echo "  build                只构建服务"
    echo "  help                 显示此帮助信息"
    echo ""
    echo "环境变量:"
    echo "  KIMI_API_KEY         Kimi API密钥 (必需)"
    echo "  LLM_PORT             服务端口 (默认: 8080)"
    echo "  LLM_MAX_WORKERS      最大工作协程数 (默认: 10)"
    echo "  LLM_AUTH_TOKEN       API认证令牌 (可选)"
    echo ""
    echo "示例:"
    echo "  $0 start             前台启动服务"
    echo "  $0 start -d          后台启动服务"
    echo "  $0 docker            使用Docker启动"
    echo "  $0 status            检查服务状态"
}

# 主逻辑
case "$1" in
    start)
        check_env
        if ! check_port ${LLM_PORT:-8080}; then
            exit 1
        fi
        build_service
        start_service "$2"
        ;;
    stop)
        stop_service
        ;;
    restart)
        restart_service "$2"
        ;;
    status)
        show_status
        ;;
    docker)
        check_env
        start_docker
        ;;
    build)
        build_service
        ;;
    help|--help|-h)
        show_help
        ;;
    "")
        show_help
        ;;
    *)
        print_error "未知命令: $1"
        show_help
        exit 1
        ;;
esac