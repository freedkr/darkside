-- =============================================
-- Migration: 002_add_pdf_page_snapshots_table
-- Description: 添加PDF页面快照表，用于存储每页的截图和元数据
-- Author: System
-- Date: 2025-09-06
-- =============================================

-- 创建pdf_page_snapshots表
CREATE TABLE IF NOT EXISTS moonshot.pdf_page_snapshots (
    id SERIAL PRIMARY KEY,
    task_id VARCHAR(50) NOT NULL,
    page_num INTEGER NOT NULL,
    
    -- MinIO存储信息
    minio_path VARCHAR(500) NOT NULL,
    thumbnail_path VARCHAR(500),
    
    -- 页面尺寸信息
    page_width FLOAT NOT NULL,
    page_height FLOAT NOT NULL,
    dpi INTEGER DEFAULT 150,
    
    -- 图片信息
    image_width INTEGER NOT NULL,
    image_height INTEGER NOT NULL,
    image_format VARCHAR(10) DEFAULT 'png',
    image_size INTEGER NOT NULL,
    
    -- 页面内容元数据
    text_blocks_count INTEGER DEFAULT 0,
    images_count INTEGER DEFAULT 0,
    tables_count INTEGER DEFAULT 0,
    
    -- 主要字体信息
    primary_font VARCHAR(100),
    font_sizes JSONB,
    
    -- 页面布局信息
    has_header BOOLEAN DEFAULT FALSE,
    has_footer BOOLEAN DEFAULT FALSE,
    columns_count INTEGER DEFAULT 1,
    
    -- OCR相关（预留）
    ocr_processed BOOLEAN DEFAULT FALSE,
    ocr_confidence FLOAT,
    
    -- 时间戳
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引
CREATE INDEX idx_pdf_page_snapshots_task_id ON moonshot.pdf_page_snapshots(task_id);
CREATE INDEX idx_pdf_page_snapshots_page_num ON moonshot.pdf_page_snapshots(page_num);
CREATE UNIQUE INDEX idx_pdf_page_snapshots_task_page ON moonshot.pdf_page_snapshots(task_id, page_num);

-- 添加注释
COMMENT ON TABLE moonshot.pdf_page_snapshots IS 'PDF页面快照表，存储每页的截图和元数据';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.task_id IS '关联的任务ID';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.page_num IS '页码（从1开始）';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.minio_path IS 'MinIO中的页面截图路径';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.thumbnail_path IS '缩略图路径';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.page_width IS '页面宽度（点）';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.page_height IS '页面高度（点）';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.dpi IS '截图DPI';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.image_width IS '截图宽度（像素）';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.image_height IS '截图高度（像素）';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.image_format IS '图片格式';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.image_size IS '文件大小（字节）';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.text_blocks_count IS '文本块数量';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.images_count IS '图片数量';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.tables_count IS '表格数量';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.primary_font IS '主要字体';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.font_sizes IS '字体大小分布';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.has_header IS '是否有页眉';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.has_footer IS '是否有页脚';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.columns_count IS '栏数';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.ocr_processed IS '是否已OCR处理';
COMMENT ON COLUMN moonshot.pdf_page_snapshots.ocr_confidence IS 'OCR置信度';

-- 添加外键约束（如果需要）
-- ALTER TABLE moonshot.pdf_page_snapshots
-- ADD CONSTRAINT fk_pdf_page_snapshots_task
-- FOREIGN KEY (task_id)
-- REFERENCES moonshot.pdf_validation_tasks(task_id)
-- ON DELETE CASCADE;