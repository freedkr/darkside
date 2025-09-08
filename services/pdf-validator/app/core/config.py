"""
PDF Validator Service Configuration Management
"""
from typing import List, Optional
from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    """应用配置类"""
    
    # 服务基础配置
    SERVICE_NAME: str = "pdf-validator-service"
    VERSION: str = "1.0.0"
    HOST: str = "0.0.0.0"
    PORT: int = 8001
    DEBUG: bool = False
    WORKER_COUNT: int = 4
    AUTO_START_WORKER: bool = False
    
    # 数据库配置 - 使用独立环境变量
    DATABASE_HOST: str = "localhost"
    DATABASE_PORT: int = 5432
    DATABASE_NAME: str = "moonshot"
    DATABASE_USER: str = "postgres"
    DATABASE_PASSWORD: str = "password"
    DATABASE_SCHEMA: str = "moonshot"
    DATABASE_POOL_SIZE: int = 10
    DATABASE_MAX_OVERFLOW: int = 20
    
    # Redis配置
    REDIS_URL: str = "redis://localhost:6379/0"
    REDIS_MAX_CONNECTIONS: int = 20
    
    # Celery配置
    CELERY_BROKER_URL: str = "redis://localhost:6379/1"
    CELERY_RESULT_BACKEND: str = "redis://localhost:6379/2"
    CELERY_TASK_SERIALIZER: str = "json"
    CELERY_RESULT_SERIALIZER: str = "json"
    CELERY_ACCEPT_CONTENT: List[str] = ["json"]
    CELERY_TIMEZONE: str = "UTC"
    CELERY_ENABLE_UTC: bool = True
    
    # 对象存储配置 (MinIO/S3)
    MINIO_ENDPOINT: str = "localhost:9000"
    MINIO_ACCESS_KEY: str = "minioadmin"
    MINIO_SECRET_KEY: str = "minioadmin"
    MINIO_BUCKET_NAME: str = "pdf-files"
    MINIO_SECURE: bool = False
    
    # PDF处理配置
    PDF_MAX_FILE_SIZE: int = 50 * 1024 * 1024  # 50MB
    PDF_PROCESSING_TIMEOUT: int = 300  # 5分钟
    PDF_EXTRACT_IMAGES: bool = True
    PDF_EXTRACT_TABLES: bool = True
    PDF_EXTRACT_SNAPSHOTS: bool = True  # 提取页面快照
    PDF_SNAPSHOT_DPI: int = 150  # 页面快照DPI
    PDF_GENERATE_THUMBNAILS: bool = True  # 生成缩略图
    
    # 任务队列配置
    VALIDATION_QUEUE_NAME: str = "pdf_validation_tasks"
    PRIORITY_QUEUE_NAME: str = "pdf_priority_tasks"
    DEAD_LETTER_QUEUE_NAME: str = "pdf_failed_tasks"
    MAX_RETRY_ATTEMPTS: int = 3
    TASK_TIMEOUT: int = 600  # 10分钟
    
    # 监控配置
    ENABLE_PROMETHEUS: bool = True
    LOG_LEVEL: str = "INFO"
    LOG_FORMAT: str = "json"
    
    # API配置
    API_V1_PREFIX: str = "/api/v1"
    ALLOWED_HOSTS: List[str] = ["*"]
    
    # Go服务集成配置
    MOONSHOT_SERVICE_URL: str = "http://localhost:8080"
    NOTIFICATION_WEBHOOK_URL: Optional[str] = None
    
    class Config:
        env_file = ".env"
        env_file_encoding = "utf-8"
        case_sensitive = True


# 创建全局配置实例
settings = Settings()


def get_database_url() -> str:
    """获取数据库连接URL"""
    return f"postgresql://{settings.DATABASE_USER}:{settings.DATABASE_PASSWORD}@{settings.DATABASE_HOST}:{settings.DATABASE_PORT}/{settings.DATABASE_NAME}?options=-csearch_path%3D{settings.DATABASE_SCHEMA}"


def get_redis_url() -> str:
    """获取Redis连接URL"""
    return settings.REDIS_URL


def get_celery_config() -> dict:
    """获取Celery配置"""
    return {
        "broker_url": settings.CELERY_BROKER_URL,
        "result_backend": settings.CELERY_RESULT_BACKEND,
        "task_serializer": settings.CELERY_TASK_SERIALIZER,
        "result_serializer": settings.CELERY_RESULT_SERIALIZER,
        "accept_content": settings.CELERY_ACCEPT_CONTENT,
        "timezone": settings.CELERY_TIMEZONE,
        "enable_utc": settings.CELERY_ENABLE_UTC,
        "task_routes": {
            "validate_pdf_task": {
                "queue": settings.VALIDATION_QUEUE_NAME
            },
            "priority_validate_pdf_task": {
                "queue": settings.PRIORITY_QUEUE_NAME
            }
        },
        "task_annotations": {
            "*": {
                "rate_limit": "10/m",
                "time_limit": settings.TASK_TIMEOUT,
                "soft_time_limit": settings.TASK_TIMEOUT - 60,
            }
        }
    }