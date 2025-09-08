"""
PDF Validator Service - Main Application Entry Point
"""
import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from prometheus_client import generate_latest, CONTENT_TYPE_LATEST
from fastapi.responses import Response

from app.core.config import settings
from app.core.logging import setup_logging
from app.api.routes import health, validation, blocks
from app.workers.pdf_validator import start_celery_worker


@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期管理"""
    # 启动时初始化
    setup_logging()
    logging.info("PDF Validator Service starting up...")
    
    # 启动 Celery Worker (if not running separately)
    if settings.AUTO_START_WORKER:
        start_celery_worker()
    
    yield
    
    # 关闭时清理
    logging.info("PDF Validator Service shutting down...")


# 创建FastAPI应用实例
app = FastAPI(
    title="PDF Validator Service",
    description="专职的PDF验证微服务，使用PyMuPDF进行PDF文本提取和验证",
    version="1.0.0",
    lifespan=lifespan,
    docs_url="/docs" if settings.DEBUG else None,
    redoc_url="/redoc" if settings.DEBUG else None,
)

# CORS中间件
if settings.DEBUG:
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

# 注册路由 - health、validation和blocks已经是APIRouter对象，直接使用
app.include_router(health, prefix="/health", tags=["健康检查"])
app.include_router(validation, prefix="/api/v1", tags=["PDF验证"])
app.include_router(blocks, prefix="/api/v1", tags=["块信息查询"])

# 导入并注册页面快照路由
from app.api.routes.page_snapshots import router as page_snapshots_router
app.include_router(page_snapshots_router, prefix="/api/v1", tags=["页面快照"])


# Prometheus指标端点
@app.get("/metrics")
async def metrics():
    """Prometheus监控指标"""
    return Response(
        generate_latest(),
        media_type=CONTENT_TYPE_LATEST
    )


@app.get("/")
async def root():
    """根路径信息"""
    return {
        "service": "PDF Validator Service",
        "version": "1.0.0",
        "description": "专职的PDF验证微服务",
        "health": "/health",
        "docs": "/docs" if settings.DEBUG else "disabled in production",
        "metrics": "/metrics"
    }


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(
        "app.main:app",
        host=settings.HOST,
        port=settings.PORT,
        reload=settings.DEBUG,
        workers=1 if settings.DEBUG else settings.WORKER_COUNT
    )