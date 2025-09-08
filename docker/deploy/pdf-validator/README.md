# PDF Validator Service 部署指南

## 概述

PDF Validator Service 是一个专门用于PDF文档验证和内容提取的微服务。

## 快速开始

### 1. 环境准备

```bash
# 复制环境变量配置
cp .env.example .env

# 编辑配置（如需要）
vim .env
```

### 2. 启动服务

```bash
# 启动所有服务
make up

# 或使用docker-compose
docker-compose up -d
```

### 3. 验证服务

```bash
# 检查服务状态
make status

# 测试API健康检查
make test
```

## 服务组件

| 组件 | 端口 | 说明 |
|-----|------|------|
| PDF Validator API | 8001 | RESTful API服务 |
| Celery Worker | - | 异步任务处理 |
| Flower | 5555 | 任务监控界面 |
| PostgreSQL | 5432 | 数据库 |
| Redis | 6379 | 消息队列 |
| MinIO | 9000/9001 | 对象存储 |

## 常用命令

```bash
# 查看日志
make logs           # 所有服务日志
make api-logs       # API服务日志
make worker-logs    # Worker日志

# 服务管理
make restart        # 重启服务
make down           # 停止服务
make build          # 重建镜像
make clean          # 清理所有数据

# 监控
make flower         # 打开Flower界面
make status         # 查看服务状态
```

## API 使用示例

### 健康检查

```bash
curl http://localhost:8001/health
```

### 上传并验证PDF

```bash
curl -X POST \
  http://localhost:8001/api/v1/upload-and-validate \
  -F "file=@document.pdf" \
  -F "validation_type=standard"
```

### 查询任务状态

```bash
curl http://localhost:8001/api/v1/status/{task_id}
```

## 环境变量说明

| 变量 | 默认值 | 说明 |
|-----|--------|------|
| DEBUG | false | 调试模式 |
| POSTGRES_DB | moonshot | 数据库名 |
| POSTGRES_USER | postgres | 数据库用户 |
| POSTGRES_PASSWORD | password | 数据库密码 |
| MINIO_ROOT_USER | minioadmin | MinIO用户 |
| MINIO_ROOT_PASSWORD | minioadmin | MinIO密码 |

## 故障排查

### 服务无法启动

```bash
# 检查端口占用
lsof -i :8001
lsof -i :5432
lsof -i :6379

# 查看详细日志
docker-compose logs -f
```

### Worker任务失败

```bash
# 查看Worker日志
make worker-logs

# 访问Flower监控
open http://localhost:5555
```

### 数据库连接问题

```bash
# 测试数据库连接
docker-compose exec postgres psql -U postgres -d moonshot

# 重置数据库
make clean
make up
```

## 生产部署

### 使用统一配置

```bash
# 切换到项目根目录
cd ../../../

# 使用统一的docker-compose配置
docker-compose -f docker/profiles/docker-compose.local.yml up -d
```

### 性能优化

1. 调整Worker并发数
2. 配置Redis持久化
3. 设置PostgreSQL连接池
4. 配置MinIO多节点

## 监控和告警

- Flower: http://localhost:5555
- MinIO Console: http://localhost:9001
- Prometheus metrics: http://localhost:8001/metrics

## 联系支持

如有问题，请查看项目文档或提交Issue。