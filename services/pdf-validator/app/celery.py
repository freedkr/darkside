"""
Celery Application Configuration
"""
from celery import Celery
from app.core.config import settings, get_celery_config

# 创建Celery应用实例
celery_app = Celery("pdf_validator")

# 应用配置
celery_app.conf.update(get_celery_config())

# 显式导入任务模块以确保注册（避免使用autodiscover）
def register_tasks():
    """手动注册任务以避免导入问题"""
    try:
        from app.workers import pdf_validator
        return True
    except ImportError as e:
        print(f"Failed to import pdf_validator tasks: {e}")
        return False

# 注册任务
register_tasks()

# 为了兼容性，同时导出为 celery
celery = celery_app