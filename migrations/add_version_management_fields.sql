-- 添加版本管理字段支持多次上传
-- 迁移时间: 2024-09-05

-- 1. 添加版本管理字段
ALTER TABLE categories ADD COLUMN upload_batch_id UUID;
ALTER TABLE categories ADD COLUMN upload_timestamp TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE categories ADD COLUMN is_current BOOLEAN DEFAULT TRUE;

-- 2. 为现有数据设置默认值
UPDATE categories SET 
    upload_batch_id = gen_random_uuid(),
    upload_timestamp = CURRENT_TIMESTAMP,
    is_current = TRUE
WHERE upload_batch_id IS NULL;

-- 3. 设置字段为NOT NULL（在设置默认值后）
ALTER TABLE categories ALTER COLUMN upload_batch_id SET NOT NULL;
ALTER TABLE categories ALTER COLUMN upload_timestamp SET NOT NULL;
ALTER TABLE categories ALTER COLUMN is_current SET NOT NULL;

-- 4. 重新创建约束以支持版本管理
-- 删除旧的唯一约束
DROP INDEX IF EXISTS idx_task_code;

-- 创建新的唯一约束：同一批次内task_id+code唯一
CREATE UNIQUE INDEX idx_task_code_batch ON categories(task_id, code, upload_batch_id);

-- 创建约束：同一任务下只能有一个当前版本的记录
CREATE UNIQUE INDEX idx_task_code_current ON categories(task_id, code) WHERE is_current = true;

-- 5. 添加索引提升查询性能
CREATE INDEX idx_categories_upload_batch ON categories(upload_batch_id);
CREATE INDEX idx_categories_upload_timestamp ON categories(upload_timestamp);
CREATE INDEX idx_categories_is_current ON categories(is_current);
CREATE INDEX idx_categories_task_current ON categories(task_id, is_current);

-- 6. 添加注释说明
COMMENT ON COLUMN categories.upload_batch_id IS '上传批次ID，每次上传生成唯一ID';
COMMENT ON COLUMN categories.upload_timestamp IS '上传时间戳';
COMMENT ON COLUMN categories.is_current IS '是否为当前版本，每个task_id+code只能有一个当前版本';

-- 7. 查看修改后的表结构
SELECT 
    column_name, 
    data_type, 
    is_nullable, 
    column_default
FROM information_schema.columns 
WHERE table_name = 'categories' 
ORDER BY ordinal_position;