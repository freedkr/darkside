-- -------------------------------------------------------------
-- TablePlus 6.7.0(634)
--
-- https://tableplus.com/
--
-- Database: moonshot
-- Generation Time: 2025-09-07 18:57:55.7080
-- -------------------------------------------------------------


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

DROP TABLE IF EXISTS "moonshot"."pdf_validation_tasks";
-- Sequence and defined type
CREATE SEQUENCE IF NOT EXISTS moonshot.pdf_validation_tasks_id_seq;

-- Table Definition
CREATE TABLE "moonshot"."pdf_validation_tasks" (
    "id" int4 NOT NULL DEFAULT nextval('moonshot.pdf_validation_tasks_id_seq'::regclass),
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
    "correlation_id" varchar(100)
);

DROP TABLE IF EXISTS "moonshot"."pdf_validation_results";
-- Sequence and defined type
CREATE SEQUENCE IF NOT EXISTS moonshot.pdf_validation_results_id_seq;

-- Table Definition
CREATE TABLE "moonshot"."pdf_validation_results" (
    "id" int4 NOT NULL DEFAULT nextval('moonshot.pdf_validation_results_id_seq'::regclass),
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

DROP TABLE IF EXISTS "moonshot"."categories";
-- Sequence and defined type
CREATE SEQUENCE IF NOT EXISTS moonshot.categories_id_seq;

-- Table Definition
CREATE TABLE "moonshot"."categories" (
    "id" int8 NOT NULL DEFAULT nextval('moonshot.categories_id_seq'::regclass),
    "task_id" uuid,
    "code" varchar(255) NOT NULL,
    "name" varchar(255) NOT NULL,
    "level" varchar(50) NOT NULL,
    "parent_code" varchar(255),
    "status" varchar(50) NOT NULL DEFAULT 'excel_parsed'::character varying,
    "data_source" varchar(50) NOT NULL DEFAULT 'excel'::character varying,
    "pdf_info" text,
    "llm_enhancements" text,
    "created_at" timestamptz,
    "updated_at" timestamptz,
    "upload_batch_id" uuid,
    "upload_timestamp" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "is_current" bool DEFAULT true
);

DROP TABLE IF EXISTS "moonshot"."moonshot";
-- Sequence and defined type
CREATE SEQUENCE IF NOT EXISTS moonshot.moonshot_id_seq;

-- Table Definition
CREATE TABLE "moonshot"."moonshot" (
    "id" int4 NOT NULL DEFAULT nextval('moonshot.moonshot_id_seq'::regclass),
    "task_id" varchar(50) NOT NULL,
    "page_num" int4 NOT NULL,
    "minio_path" varchar(500) NOT NULL,
    "thumbnail_path" varchar(500),
    "page_width" float8 NOT NULL,
    "page_height" float8 NOT NULL,
    "dpi" int4 DEFAULT 150,
    "image_width" int4 NOT NULL,
    "image_height" int4 NOT NULL,
    "image_format" varchar(10) DEFAULT 'png'::character varying,
    "image_size" int4 NOT NULL,
    "text_blocks_count" int4 DEFAULT 0,
    "images_count" int4 DEFAULT 0,
    "tables_count" int4 DEFAULT 0,
    "primary_font" varchar(100),
    "font_sizes" jsonb,
    "has_header" bool DEFAULT false,
    "has_footer" bool DEFAULT false,
    "columns_count" int4 DEFAULT 1,
    "ocr_processed" bool DEFAULT false,
    "ocr_confidence" float8,
    "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY ("id")
);

-- Column Comment
COMMENT ON COLUMN "moonshot"."moonshot"."task_id" IS '关联的任务ID';
COMMENT ON COLUMN "moonshot"."moonshot"."page_num" IS '页码（从1开始）';
COMMENT ON COLUMN "moonshot"."moonshot"."minio_path" IS 'MinIO中的页面截图路径';
COMMENT ON COLUMN "moonshot"."moonshot"."thumbnail_path" IS '缩略图路径';
COMMENT ON COLUMN "moonshot"."moonshot"."page_width" IS '页面宽度（点）';
COMMENT ON COLUMN "moonshot"."moonshot"."page_height" IS '页面高度（点）';
COMMENT ON COLUMN "moonshot"."moonshot"."dpi" IS '截图DPI';
COMMENT ON COLUMN "moonshot"."moonshot"."image_width" IS '截图宽度（像素）';
COMMENT ON COLUMN "moonshot"."moonshot"."image_height" IS '截图高度（像素）';
COMMENT ON COLUMN "moonshot"."moonshot"."image_format" IS '图片格式';
COMMENT ON COLUMN "moonshot"."moonshot"."image_size" IS '文件大小（字节）';
COMMENT ON COLUMN "moonshot"."moonshot"."text_blocks_count" IS '文本块数量';
COMMENT ON COLUMN "moonshot"."moonshot"."images_count" IS '图片数量';
COMMENT ON COLUMN "moonshot"."moonshot"."tables_count" IS '表格数量';
COMMENT ON COLUMN "moonshot"."moonshot"."primary_font" IS '主要字体';
COMMENT ON COLUMN "moonshot"."moonshot"."font_sizes" IS '字体大小分布';
COMMENT ON COLUMN "moonshot"."moonshot"."has_header" IS '是否有页眉';
COMMENT ON COLUMN "moonshot"."moonshot"."has_footer" IS '是否有页脚';
COMMENT ON COLUMN "moonshot"."moonshot"."columns_count" IS '栏数';
COMMENT ON COLUMN "moonshot"."moonshot"."ocr_processed" IS '是否已OCR处理';
COMMENT ON COLUMN "moonshot"."moonshot"."ocr_confidence" IS 'OCR置信度';

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

DROP TABLE IF EXISTS "moonshot"."pdf_page_snapshots";
-- Sequence and defined type
CREATE SEQUENCE IF NOT EXISTS moonshot.pdf_page_snapshots_id_seq;

-- Table Definition
CREATE TABLE "moonshot"."pdf_page_snapshots" (
    "id" int4 NOT NULL DEFAULT nextval('moonshot.pdf_page_snapshots_id_seq'::regclass),
    "task_id" varchar(50) NOT NULL,
    "page_num" int4 NOT NULL,
    "minio_path" varchar(500) NOT NULL,
    "thumbnail_path" varchar(500),
    "page_width" float8 NOT NULL,
    "page_height" float8 NOT NULL,
    "dpi" int4 DEFAULT 150,
    "image_width" int4 NOT NULL,
    "image_height" int4 NOT NULL,
    "image_format" varchar(10) DEFAULT 'png'::character varying,
    "image_size" int4 NOT NULL,
    "text_blocks_count" int4 DEFAULT 0,
    "images_count" int4 DEFAULT 0,
    "tables_count" int4 DEFAULT 0,
    "primary_font" varchar(100),
    "font_sizes" jsonb,
    "has_header" bool DEFAULT false,
    "has_footer" bool DEFAULT false,
    "columns_count" int4 DEFAULT 1,
    "ocr_processed" bool DEFAULT false,
    "ocr_confidence" float8,
    "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY ("id")
);

-- Column Comment
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."task_id" IS '关联的任务ID';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."page_num" IS '页码（从1开始）';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."minio_path" IS 'MinIO中的页面截图路径';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."thumbnail_path" IS '缩略图路径';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."page_width" IS '页面宽度（点）';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."page_height" IS '页面高度（点）';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."dpi" IS '截图DPI';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."image_width" IS '截图宽度（像素）';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."image_height" IS '截图高度（像素）';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."image_format" IS '图片格式';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."image_size" IS '文件大小（字节）';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."text_blocks_count" IS '文本块数量';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."images_count" IS '图片数量';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."tables_count" IS '表格数量';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."primary_font" IS '主要字体';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."font_sizes" IS '字体大小分布';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."has_header" IS '是否有页眉';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."has_footer" IS '是否有页脚';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."columns_count" IS '栏数';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."ocr_processed" IS '是否已OCR处理';
COMMENT ON COLUMN "moonshot"."pdf_page_snapshots"."ocr_confidence" IS 'OCR置信度';



-- Indices
CREATE INDEX idx_file_records_task_id ON moonshot.file_records USING btree (task_id);
CREATE INDEX idx_moonshot_file_records_task_id ON moonshot.file_records USING btree (task_id);


-- Indices
CREATE INDEX idx_processing_stats_task_id ON moonshot.processing_stats USING btree (task_id);
CREATE INDEX idx_moonshot_processing_stats_task_id ON moonshot.processing_stats USING btree (task_id);


-- Indices
CREATE INDEX idx_pdf_results_task_id ON moonshot.pdf_results USING btree (task_id);
CREATE INDEX idx_moonshot_pdf_results_task_id ON moonshot.pdf_results USING btree (task_id);


-- Indices
CREATE INDEX idx_task_records_status ON moonshot.task_records USING btree (status);
CREATE INDEX idx_moonshot_task_records_status ON moonshot.task_records USING btree (status);


-- Indices
CREATE INDEX idx_pdf_validation_tasks_task_id ON moonshot.pdf_validation_tasks USING btree (task_id);
CREATE INDEX idx_pdf_validation_tasks_status ON moonshot.pdf_validation_tasks USING btree (status);


-- Indices
CREATE INDEX idx_moonshot_categories_parent_code ON moonshot.categories USING btree (parent_code);
CREATE INDEX idx_moonshot_categories_status ON moonshot.categories USING btree (status);
CREATE INDEX idx_moonshot_categories_is_current ON moonshot.categories USING btree (is_current);
CREATE INDEX idx_categories_code ON moonshot.categories USING btree (code);
