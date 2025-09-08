-- PDF块信息表迁移脚本
-- 用于存储PDF文档的详细版式信息

-- 创建pdf_blocks表
CREATE TABLE IF NOT EXISTS pdf_blocks (
    id SERIAL PRIMARY KEY,
    task_id VARCHAR(50) NOT NULL,
    page_num INTEGER NOT NULL,
    block_num INTEGER NOT NULL,
    
    -- 文本内容
    text TEXT NOT NULL,
    
    -- 位置信息 (bbox)
    x0 FLOAT NOT NULL,
    y0 FLOAT NOT NULL,
    x1 FLOAT NOT NULL,
    y1 FLOAT NOT NULL,
    
    -- 计算字段
    width FLOAT NOT NULL,
    height FLOAT NOT NULL,
    center_x FLOAT NOT NULL,
    center_y FLOAT NOT NULL,
    
    -- 字体信息
    font VARCHAR(100),
    font_size FLOAT,
    font_flags INTEGER,
    font_color INTEGER,
    
    -- 层级和样式
    hierarchy_level INTEGER,
    indentation FLOAT,
    is_bold BOOLEAN DEFAULT FALSE,
    is_italic BOOLEAN DEFAULT FALSE,
    
    -- 关联信息
    parent_block_id INTEGER REFERENCES pdf_blocks(id),
    
    -- 职业分类信息
    occupation_code VARCHAR(50),
    occupation_name VARCHAR(255),
    confidence FLOAT,
    
    -- 时间戳
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    -- 索引
    CONSTRAINT idx_pdf_blocks_task_id_page_num 
        UNIQUE(task_id, page_num, block_num)
);

-- 创建索引以提高查询性能
CREATE INDEX idx_pdf_blocks_task_id ON pdf_blocks(task_id);
CREATE INDEX idx_pdf_blocks_page_num ON pdf_blocks(page_num);
CREATE INDEX idx_pdf_blocks_occupation_code ON pdf_blocks(occupation_code);
CREATE INDEX idx_pdf_blocks_hierarchy_level ON pdf_blocks(hierarchy_level);

-- 添加注释
COMMENT ON TABLE pdf_blocks IS 'PDF文档块信息表，存储详细的版式和位置信息';
COMMENT ON COLUMN pdf_blocks.task_id IS '关联的任务ID';
COMMENT ON COLUMN pdf_blocks.page_num IS '页码（从1开始）';
COMMENT ON COLUMN pdf_blocks.block_num IS '块编号（页内顺序）';
COMMENT ON COLUMN pdf_blocks.text IS '块的文本内容';
COMMENT ON COLUMN pdf_blocks.x0 IS '边界框左边界';
COMMENT ON COLUMN pdf_blocks.y0 IS '边界框上边界';
COMMENT ON COLUMN pdf_blocks.x1 IS '边界框右边界';
COMMENT ON COLUMN pdf_blocks.y1 IS '边界框下边界';
COMMENT ON COLUMN pdf_blocks.font IS '字体名称';
COMMENT ON COLUMN pdf_blocks.font_size IS '字号大小';
COMMENT ON COLUMN pdf_blocks.font_flags IS '字体样式标志位';
COMMENT ON COLUMN pdf_blocks.hierarchy_level IS '层级（1-4，1为最高级）';
COMMENT ON COLUMN pdf_blocks.occupation_code IS '职业编码（如果识别出）';
COMMENT ON COLUMN pdf_blocks.occupation_name IS '职业名称（如果匹配到）';
COMMENT ON COLUMN pdf_blocks.confidence IS '匹配置信度';