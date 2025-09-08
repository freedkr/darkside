# PDF页面快照功能测试指南

## 功能概述

PDF页面快照功能会自动：
1. 将PDF每一页渲染为PNG图片（150 DPI）
2. 生成200x280像素的缩略图
3. 保存到MinIO对象存储
4. 记录页面元数据到数据库

## 测试环境准备

### 1. 启动依赖服务

```bash
# 启动Docker服务栈（PostgreSQL, Redis, MinIO）
docker-compose up -d postgres redis minio

# 或使用完整服务栈
make docker-up
```

### 2. 数据库迁移

```bash
# 执行数据库迁移，创建pdf_page_snapshots表
psql -h localhost -U postgres -d moonshot < migrations/002_add_pdf_page_snapshots_table.sql
```

## 测试方法

### 方法1: 使用测试脚本（推荐）

```bash
# 运行测试脚本
make test-snapshots

# 或直接运行
python test_page_snapshots.py
```

测试选项：
- 选项1: 测试页面快照提取
- 选项2: 测试增强处理器集成
- 选项3: 运行所有测试

### 方法2: 使用API测试

#### 步骤1: 启动服务

```bash
# 启动API服务
make dev

# 启动Worker（另一个终端）
make worker
```

#### 步骤2: 上传PDF并验证

```bash
# 上传PDF文件
curl -X POST http://localhost:8001/api/v1/upload-and-validate \
  -F "file=@test.pdf" \
  -F "validation_type=enhanced"
```

响应示例：
```json
{
  "task_id": "task_1234567890",
  "status": "processing",
  "message": "PDF validation task submitted"
}
```

#### 步骤3: 查询页面快照

```bash
# 获取所有页面快照信息
curl http://localhost:8001/api/v1/page-snapshots/{task_id}

# 获取指定页的快照信息
curl http://localhost:8001/api/v1/page-snapshots/{task_id}?page=1

# 获取统计信息
curl http://localhost:8001/api/v1/page-snapshots/{task_id}/statistics
```

#### 步骤4: 获取页面图片

```bash
# 获取第1页的图片
curl http://localhost:8001/api/v1/page-snapshots/{task_id}/page/1/image -o page1.png

# 获取第1页的缩略图
curl "http://localhost:8001/api/v1/page-snapshots/{task_id}/page/1/image?thumbnail=true" -o page1_thumb.png
```

### 方法3: 通过MinIO控制台查看

1. 访问MinIO控制台: http://localhost:9001
2. 登录凭据:
   - 用户名: minioadmin
   - 密码: minioadmin
3. 浏览到 `pdf-files` bucket
4. 查看 `pdf-snapshots/{task_id}/` 目录下的图片文件

## API端点说明

### 1. 获取快照列表
- **端点**: `GET /api/v1/page-snapshots/{task_id}`
- **参数**: 
  - `page` (可选): 指定页码
- **返回**: 页面快照信息列表

### 2. 获取页面图片
- **端点**: `GET /api/v1/page-snapshots/{task_id}/page/{page_num}/image`
- **参数**:
  - `thumbnail` (bool): 是否获取缩略图
- **返回**: PNG图片文件流

### 3. 获取统计信息
- **端点**: `GET /api/v1/page-snapshots/{task_id}/statistics`
- **返回**: 包含存储大小、内容统计、字体分布等信息

### 4. 删除快照
- **端点**: `DELETE /api/v1/page-snapshots/{task_id}`
- **返回**: 删除结果

## 验证数据库记录

```sql
-- 连接数据库
psql -h localhost -U postgres -d moonshot

-- 查询所有快照记录
SELECT * FROM moonshot.pdf_page_snapshots ORDER BY task_id, page_num;

-- 查询特定任务的快照
SELECT 
    page_num,
    minio_path,
    image_width,
    image_height,
    image_size,
    text_blocks_count,
    primary_font
FROM moonshot.pdf_page_snapshots 
WHERE task_id = 'your_task_id'
ORDER BY page_num;

-- 统计信息
SELECT 
    task_id,
    COUNT(*) as total_pages,
    SUM(image_size) as total_size,
    AVG(text_blocks_count) as avg_text_blocks
FROM moonshot.pdf_page_snapshots
GROUP BY task_id;
```

## 配置项

在 `.env` 文件中可配置：

```bash
# 页面快照配置
PDF_EXTRACT_SNAPSHOTS=true    # 启用/禁用页面快照
PDF_SNAPSHOT_DPI=150          # 截图DPI（默认150）
PDF_GENERATE_THUMBNAILS=true  # 生成缩略图
```

## 故障排查

### 1. MinIO连接失败
```bash
# 检查MinIO是否运行
docker ps | grep minio

# 查看MinIO日志
docker logs pdf-validator-minio
```

### 2. 图片生成失败
- 检查PyMuPDF是否正确安装
- 确认PDF文件没有损坏
- 查看worker日志: `docker logs pdf-validator-worker`

### 3. 数据库错误
- 确认表已创建: `\dt moonshot.*`
- 检查数据库连接配置
- 查看API日志: `docker logs pdf-validator-api`

## 性能考虑

- **DPI设置**: 150 DPI适合大多数场景，提高DPI会增加文件大小
- **缩略图**: 启用缩略图会增加处理时间和存储空间
- **大文件处理**: 页数较多的PDF可能需要较长处理时间

## 清理测试数据

```bash
# 删除MinIO中的测试文件
mc rm --recursive --force minio/pdf-files/pdf-snapshots/

# 清理数据库
DELETE FROM moonshot.pdf_page_snapshots WHERE task_id LIKE 'test_%';
```