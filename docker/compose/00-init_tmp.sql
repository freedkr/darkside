-- =========================================
-- 数据库初始化主文件
-- 整合所有必要的表结构和数据
-- =========================================

-- 1. 启用必要的扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 2. 创建 moonshot schema
CREATE SCHEMA IF NOT EXISTS "moonshot";

-- 3. 创建所有表结构
-- \i /docker-entrypoint-initdb.d/moonshot.sql
DROP TABLE IF EXISTS "moonshot"."file_records";
-- Table Definition
CREATE TABLE "moonshot"."file_records" (
    "id" uuid NOT NULL,
    "original_name" varchar(255) NOT NULL,
    "storage_path" text NOT NULL,
    "file_size" int8 NOT NULL,
    "content_type" varchar(255) NOT NULL,
    "md5_hash" varchar(32) NOT NULL,
    "created_at" timestamptz NOT NULL DEFAULT now(),
    "task_id" uuid,
    PRIMARY KEY ("id")
);

DROP TABLE IF EXISTS "moonshot"."processing_stats";
-- Table Definition
CREATE TABLE "moonshot"."processing_stats" (
    "id" uuid NOT NULL DEFAULT uuid_generate_v4(),
    "task_id" uuid NOT NULL,
    "total_records" int8 NOT NULL DEFAULT 0,
    "processed_records" int8 NOT NULL DEFAULT 0,
    "skipped_records" int8 NOT NULL DEFAULT 0,
    "error_records" int8 NOT NULL DEFAULT 0,
    "processing_time_ms" int8 NOT NULL DEFAULT 0,
    "memory_usage_mb" numeric(10,2) NOT NULL DEFAULT 0,
    "created_at" timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY ("id")
);

DROP TABLE IF EXISTS "moonshot"."pdf_results";
-- Sequence and defined type
CREATE SEQUENCE IF NOT EXISTS pdf_results_id_seq;

-- Table Definition
CREATE TABLE "moonshot"."pdf_results" (
    "id" int8 NOT NULL DEFAULT nextval('pdf_results_id_seq'::regclass),
    "task_id" uuid,
    "page" int8 NOT NULL,
    "text" text,
    "bounding_box" _float8,
    "created_at" timestamptz,
    PRIMARY KEY ("id")
);

DROP TABLE IF EXISTS "moonshot"."task_records";
-- Table Definition
CREATE TABLE "moonshot"."task_records" (
    "id" uuid NOT NULL,
    "type" varchar(50) NOT NULL,
    "status" varchar(50) NOT NULL,
    "priority" int8 NOT NULL DEFAULT 0,
    "input_path" text NOT NULL,
    "output_path" text NOT NULL,
    "pdf_path" text,
    "config" jsonb,
    "result" jsonb,
    "error_msg" text,
    "retry_count" int8 NOT NULL DEFAULT 0,
    "created_at" timestamptz NOT NULL DEFAULT now(),
    "updated_at" timestamptz NOT NULL DEFAULT now(),
    "processed_at" timestamptz,
    "created_by" varchar(255),
    "processing_log" text,
    "upload_batch_id" uuid,
    PRIMARY KEY ("id")
);

DROP TABLE IF EXISTS "moonshot"."pdf_blocks";
-- Sequence and defined type
CREATE SEQUENCE IF NOT EXISTS pdf_blocks_id_seq;

-- Table Definition
CREATE TABLE "moonshot"."pdf_blocks" (
    "id" int4 NOT NULL DEFAULT nextval('pdf_blocks_id_seq'::regclass),
    "task_id" varchar(50) NOT NULL,
    "page_num" int4 NOT NULL,
    "block_num" int4 NOT NULL,
    "text" text NOT NULL,
    "x0" float8 NOT NULL,
    "y0" float8 NOT NULL,
    "x1" float8 NOT NULL,
    "y1" float8 NOT NULL,
    "width" float8 NOT NULL,
    "height" float8 NOT NULL,
    "center_x" float8 NOT NULL,
    "center_y" float8 NOT NULL,
    "font" varchar(100),
    "font_size" float8,
    "font_flags" int4,
    "font_color" int4,
    "hierarchy_level" int4,
    "indentation" float8,
    "is_bold" bool DEFAULT false,
    "is_italic" bool DEFAULT false,
    "parent_block_id" int4,
    "occupation_code" varchar(50),
    "occupation_name" varchar(255),
    "confidence" float8,
    "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY ("id")
);

-- Column Comment
COMMENT ON COLUMN "moonshot"."pdf_blocks"."task_id" IS '关联的任务ID';
COMMENT ON COLUMN "moonshot"."pdf_blocks"."page_num" IS '页码（从1开始）';
COMMENT ON COLUMN "moonshot"."pdf_blocks"."block_num" IS '块编号（页内顺序）';
COMMENT ON COLUMN "moonshot"."pdf_blocks"."text" IS '块的文本内容';
COMMENT ON COLUMN "moonshot"."pdf_blocks"."x0" IS '边界框左边界';
COMMENT ON COLUMN "moonshot"."pdf_blocks"."y0" IS '边界框上边界';
COMMENT ON COLUMN "moonshot"."pdf_blocks"."x1" IS '边界框右边界';
COMMENT ON COLUMN "moonshot"."pdf_blocks"."y1" IS '边界框下边界';
COMMENT ON COLUMN "moonshot"."pdf_blocks"."font" IS '字体名称';
COMMENT ON COLUMN "moonshot"."pdf_blocks"."font_size" IS '字号大小';
COMMENT ON COLUMN "moonshot"."pdf_blocks"."font_flags" IS '字体样式标志位';
COMMENT ON COLUMN "moonshot"."pdf_blocks"."hierarchy_level" IS '层级（1-4，1为最高级）';
COMMENT ON COLUMN "moonshot"."pdf_blocks"."occupation_code" IS '职业编码（如果识别出）';
COMMENT ON COLUMN "moonshot"."pdf_blocks"."occupation_name" IS '职业名称（如果匹配到）';
COMMENT ON COLUMN "moonshot"."pdf_blocks"."confidence" IS '匹配置信度';

DROP TABLE IF EXISTS "moonshot"."pdf_validation_tasks";
-- Sequence and defined type
CREATE SEQUENCE IF NOT EXISTS pdf_validation_tasks_id_seq;

-- Table Definition
CREATE TABLE "moonshot"."pdf_validation_tasks" (
    "id" int4 NOT NULL DEFAULT nextval('pdf_validation_tasks_id_seq'::regclass),
    "task_id" varchar(50) NOT NULL,
    "pdf_file_path" varchar(500) NOT NULL,
    "validation_type" varchar(50) NOT NULL DEFAULT 'standard'::character varying,
    "status" varchar(20) NOT NULL DEFAULT 'pending'::character varying,
    "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
    "started_at" timestamp,
    "completed_at" timestamp,
    "priority" int4 DEFAULT 0,
    "retry_count" int4 DEFAULT 0,
    "max_retries" int4 DEFAULT 3,
    "result_summary" jsonb,
    "error_message" text,
    "requester_service" varchar(100),
    "correlation_id" varchar(100),
    PRIMARY KEY ("id")
);

DROP TABLE IF EXISTS "moonshot"."pdf_validation_results";
-- Sequence and defined type
CREATE SEQUENCE IF NOT EXISTS pdf_validation_results_id_seq;

-- Table Definition
CREATE TABLE "moonshot"."pdf_validation_results" (
    "id" int4 NOT NULL DEFAULT nextval('pdf_validation_results_id_seq'::regclass),
    "task_id" varchar(50) NOT NULL,
    "pdf_path" varchar(500) NOT NULL,
    "validation_type" varchar(50) NOT NULL,
    "is_valid" bool NOT NULL,
    "page_count" int4 NOT NULL DEFAULT 0,
    "file_size" int4 NOT NULL DEFAULT 0,
    "processing_time" float8 NOT NULL DEFAULT 0.0,
    "extracted_text" text,
    "extracted_metadata" jsonb,
    "pages_info" jsonb,
    "images_info" jsonb,
    "document_structure" jsonb,
    "quality_checks" jsonb,
    "errors" jsonb,
    "warnings" jsonb,
    "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY ("id")
);



-- Indices
CREATE INDEX idx_file_records_task_id ON moonshot.file_records USING btree (task_id);


-- Indices
CREATE INDEX idx_processing_stats_task_id ON moonshot.processing_stats USING btree (task_id);


-- Indices
CREATE INDEX idx_pdf_results_task_id ON moonshot.pdf_results USING btree (task_id);


-- Indices
CREATE INDEX idx_task_records_status ON moonshot.task_records USING btree (status);
ALTER TABLE "moonshot"."pdf_blocks" ADD FOREIGN KEY ("parent_block_id") REFERENCES "moonshot"."pdf_blocks"("id");


-- Comments
COMMENT ON TABLE "moonshot"."pdf_blocks" IS 'PDF文档块信息表，存储详细的版式和位置信息';


-- Indices
CREATE UNIQUE INDEX idx_pdf_blocks_task_id_page_num ON moonshot.pdf_blocks USING btree (task_id, page_num, block_num);
CREATE INDEX idx_pdf_blocks_task_id ON moonshot.pdf_blocks USING btree (task_id);
CREATE INDEX idx_pdf_blocks_page_num ON moonshot.pdf_blocks USING btree (page_num);
CREATE INDEX idx_pdf_blocks_occupation_code ON moonshot.pdf_blocks USING btree (occupation_code);
CREATE INDEX idx_pdf_blocks_hierarchy_level ON moonshot.pdf_blocks USING btree (hierarchy_level);


-- Indices
CREATE UNIQUE INDEX pdf_validation_tasks_task_id_key ON moonshot.pdf_validation_tasks USING btree (task_id);
CREATE INDEX idx_pdf_validation_tasks_task_id ON moonshot.pdf_validation_tasks USING btree (task_id);
CREATE INDEX idx_pdf_validation_tasks_status ON moonshot.pdf_validation_tasks USING btree (status);


-- Indices
CREATE INDEX idx_pdf_validation_results_task_id ON moonshot.pdf_validation_results USING btree (task_id);


-- 4. 创建 categories 表
DROP TABLE IF EXISTS "moonshot"."categories";
CREATE TABLE "moonshot"."categories" (
    "id" int8 NOT NULL,
    "task_id" uuid,
    "code" varchar(50) NOT NULL,
    "name" varchar(255) NOT NULL,
    "level" varchar(20) NOT NULL,
    "parent_code" varchar(50),
    "status" varchar(50),
    "data_source" varchar(50),
    "pdf_info" jsonb,
    "llm_enhancements" jsonb,
    "created_at" timestamptz DEFAULT now(),
    "updated_at" timestamptz DEFAULT now(),
    "upload_batch_id" uuid,
    "upload_timestamp" timestamp,
    "is_current" bool DEFAULT true,
    PRIMARY KEY ("id")
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_categories_code ON "moonshot"."categories" ("code");
CREATE INDEX IF NOT EXISTS idx_categories_level ON "moonshot"."categories" ("level");
CREATE INDEX IF NOT EXISTS idx_categories_parent_code ON "moonshot"."categories" ("parent_code");

-- 5. 加载分类数据（处理重复键）
-- 先清空表
TRUNCATE TABLE "moonshot"."categories" RESTART IDENTITY CASCADE;

\i /docker-entrypoint-initdb.d/categories.sql

-- 初始化完成
SELECT 'Database initialization completed!' as status;