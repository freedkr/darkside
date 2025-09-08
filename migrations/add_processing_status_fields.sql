-- 创建支持增量处理的categories表
-- 迁移时间: 2024-09-05

-- 删除旧表重新创建（包含正确的数据类型）
DROP TABLE IF EXISTS categories CASCADE;

-- 创建新的categories表结构
CREATE TABLE categories (
    id SERIAL PRIMARY KEY,
    task_id UUID NOT NULL,
    code VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    level VARCHAR(50) NOT NULL,
    parent_code VARCHAR(255),
    
    -- 增量处理状态追踪字段
    status VARCHAR(50) NOT NULL DEFAULT 'excel_parsed',
    data_source VARCHAR(50) NOT NULL DEFAULT 'excel',
    pdf_info TEXT,
    llm_enhancements TEXT,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    -- 创建唯一约束
    CONSTRAINT idx_task_code UNIQUE (task_id, code)
);

-- 添加索引提升查询性能
CREATE INDEX idx_categories_status ON categories(status);
CREATE INDEX idx_categories_task_status ON categories(task_id, status);
CREATE INDEX idx_categories_parent_code ON categories(parent_code);

-- 添加注释说明
COMMENT ON COLUMN categories.status IS '处理状态: excel_parsed, pdf_merged, llm_enhanced, completed';
COMMENT ON COLUMN categories.data_source IS '数据源标识: excel, pdf, merged';
COMMENT ON COLUMN categories.pdf_info IS 'PDF解析信息(JSON格式)';
COMMENT ON COLUMN categories.llm_enhancements IS 'LLM增强信息(JSON格式)';

-- 查看修改后的表结构
SELECT 
    column_name, 
    data_type, 
    is_nullable, 
    column_default
FROM information_schema.columns 
WHERE table_name = 'categories' 
ORDER BY ordinal_position;