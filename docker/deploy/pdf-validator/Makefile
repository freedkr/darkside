# PDF Validator Service Makefile

.PHONY: help up down restart logs build clean status

# 默认目标
help:
	@echo "PDF Validator Service - 可用命令:"
	@echo ""
	@echo "  make up          - 启动所有服务"
	@echo "  make down        - 停止所有服务"
	@echo "  make restart     - 重启所有服务"
	@echo "  make logs        - 查看服务日志"
	@echo "  make build       - 重新构建镜像"
	@echo "  make clean       - 清理所有容器和卷"
	@echo "  make status      - 查看服务状态"
	@echo "  make test        - 测试PDF验证API"
	@echo ""
	@echo "单个服务操作:"
	@echo "  make api-logs    - 查看API服务日志"
	@echo "  make worker-logs - 查看Worker日志"
	@echo "  make flower      - 打开Flower监控界面"

# 启动服务
up:
	@echo "🚀 启动PDF Validator服务..."
	docker-compose up -d
	@echo "✅ 服务已启动"
	@echo ""
	@echo "📍 服务地址:"
	@echo "  - API: http://localhost:8001"
	@echo "  - Flower: http://localhost:5555"
	@echo "  - MinIO: http://localhost:9001"

# 停止服务
down:
	@echo "⏹️ 停止PDF Validator服务..."
	docker-compose down
	@echo "✅ 服务已停止"

# 重启服务
restart:
	@echo "🔄 重启PDF Validator服务..."
	docker-compose restart
	@echo "✅ 服务已重启"

# 查看日志
logs:
	docker-compose logs -f --tail=100

# 构建镜像
build:
	@echo "🔨 构建PDF Validator镜像..."
	docker-compose build --no-cache
	@echo "✅ 镜像构建完成"

# 清理
clean:
	@echo "🧹 清理PDF Validator服务..."
	docker-compose down -v
	@echo "✅ 清理完成"

# 服务状态
status:
	@echo "📊 PDF Validator服务状态:"
	@docker-compose ps

# API日志
api-logs:
	docker-compose logs -f pdf-validator-api --tail=100

# Worker日志
worker-logs:
	docker-compose logs -f pdf-validator-worker --tail=100

# 打开Flower
flower:
	@echo "🌸 打开Flower监控界面..."
	@open http://localhost:5555 || xdg-open http://localhost:5555

# 测试API
test:
	@echo "🧪 测试PDF Validator API..."
	@curl -s http://localhost:8001/health | jq . || echo "❌ API未响应"

# 初始化数据库
init-db:
	@echo "🗄️ 初始化数据库..."
	docker-compose exec postgres psql -U postgres -d moonshot -c "SELECT 1;"
	@echo "✅ 数据库初始化完成"