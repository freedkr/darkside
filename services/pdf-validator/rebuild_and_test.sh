#!/bin/bash
# 重新构建并测试PDF Validator服务

echo "========================================="
echo "重新构建PDF Validator服务"
echo "========================================="

# 切换到服务目录
cd "$(dirname "$0")"

# 停止现有容器
echo "1. 停止现有容器..."
docker-compose down

# 重新构建镜像
echo "2. 重新构建Docker镜像..."
docker-compose build --no-cache pdf-validator-api pdf-validator-worker pdf-validator-flower

# 启动服务
echo "3. 启动服务..."
docker-compose up -d

# 等待服务启动
echo "4. 等待服务启动..."
sleep 10

# 检查服务状态
echo "5. 检查服务状态..."
docker-compose ps

# 查看日志
echo "6. 查看Flower日志..."
docker-compose logs --tail=20 pdf-validator-flower

echo ""
echo "========================================="
echo "服务状态检查"
echo "========================================="

# 健康检查
echo "API健康检查:"
curl -s http://localhost:8001/health | jq '.' 2>/dev/null || echo "API未响应"

echo ""
echo "Flower监控界面:"
curl -s http://localhost:5555/ > /dev/null && echo "✅ Flower运行正常 (http://localhost:5555)" || echo "❌ Flower未响应"

echo ""
echo "========================================="
echo "完成！"
echo "========================================="
echo ""
echo "访问以下地址："
echo "- API文档: http://localhost:8001/docs"
echo "- Flower监控: http://localhost:5555"
echo "- MinIO控制台: http://localhost:9001"
echo ""
echo "测试页面快照功能："
echo "1. 上传PDF: curl -X POST http://localhost:8001/api/v1/upload-and-validate -F 'file=@test.pdf'"
echo "2. 查看快照: curl http://localhost:8001/api/v1/page-snapshots/{task_id}"