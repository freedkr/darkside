-- =========================================
-- 数据库初始化主文件
-- 整合所有必要的表结构和数据
-- =========================================

-- 1. 启用必要的扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 2. 创建 moonshot schema
CREATE SCHEMA IF NOT EXISTS "moonshot";

\i /docker-entrypoint-initdb.d/moonshot_zero.sql

-- 初始化完成
SELECT 'Database initialization completed!' as status;