# PDF Validator Service Makefile

.PHONY: help install dev test lint format docker-build docker-up docker-down logs clean

# 默认目标
help:
	@echo "PDF Validator Service - 可用命令："
	@echo ""
	@echo "开发命令："
	@echo "  install     - 安装依赖"
	@echo "  dev         - 启动开发服务器"
	@echo "  worker      - 启动Celery worker"
	@echo "  flower      - 启动任务监控界面"
	@echo ""
	@echo "测试命令："
	@echo "  test        - 运行测试"
	@echo "  test-cov    - 运行测试并生成覆盖率报告"
	@echo "  test-snapshots - 测试页面快照功能"
	@echo "  lint        - 代码检查"
	@echo "  format      - 代码格式化"
	@echo ""
	@echo "Docker命令："
	@echo "  docker-build - 构建Docker镜像"
	@echo "  docker-up   - 启动完整服务栈"
	@echo "  docker-down - 停止服务栈"
	@echo "  logs        - 查看服务日志"
	@echo ""
	@echo "其他命令："
	@echo "  clean       - 清理临时文件"
	@echo "  init-db     - 初始化数据库"

# 开发环境
install:
	pip install -r requirements.txt

dev:
	uvicorn app.main:app --host 0.0.0.0 --port 8001 --reload --log-level info

worker:
	celery -A app.workers.pdf_validator worker --loglevel=info --concurrency=4 \
		--queues=pdf_validation_tasks,pdf_priority_tasks

flower:
	celery -A app.workers.pdf_validator flower --port=5555

# 测试
test:
	pytest -v

test-cov:
	pytest --cov=app --cov-report=html --cov-report=term-missing

# 代码质量
lint:
	flake8 app tests
	mypy app

format:
	black app tests
	isort app tests

# Docker
docker-build:
	docker build -t pdf-validator-service:latest .

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

logs:
	docker-compose logs -f pdf-validator-api pdf-validator-worker

# 数据库
init-db:
	python -c "from app.core.database import init_database; init_database()"

# 测试页面快照功能
test-snapshots:
	python test_page_snapshots.py

# 清理
clean:
	find . -type f -name "*.pyc" -delete
	find . -type d -name "__pycache__" -delete
	find . -type d -name "*.egg-info" -exec rm -rf {} +
	rm -rf .pytest_cache
	rm -rf htmlcov
	rm -rf dist
	rm -rf build

# 监控和调试
monitor:
	@echo "服务状态检查："
	@echo "API服务: http://localhost:8001/health"
	@echo "任务监控: http://localhost:5555"
	@echo "Prometheus: http://localhost:9090"
	@echo "Grafana: http://localhost:3000"
	@echo "MinIO Console: http://localhost:9001"

# 生产部署
deploy-prod:
	@echo "生产部署步骤："
	@echo "1. 检查环境变量配置"
	@echo "2. 构建生产镜像"
	@echo "3. 运行数据库迁移"
	@echo "4. 启动服务"
	docker build -t pdf-validator-service:prod .
	docker-compose -f docker-compose.prod.yml up -d

# 安全检查
security-check:
	pip-audit
	bandit -r app/