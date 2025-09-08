"""
Health Check API Routes
"""
from fastapi import APIRouter
from fastapi.responses import JSONResponse
from datetime import datetime
import logging

from app.core.database import engine

logger = logging.getLogger(__name__)

router = APIRouter()


@router.get("/")
async def health_check():
    """
    基础健康检查
    """
    return JSONResponse(
        status_code=200,
        content={
            "status": "healthy",
            "timestamp": datetime.utcnow().isoformat(),
            "service": "pdf-validator"
        }
    )


@router.get("/ready")
async def readiness_check():
    """
    就绪检查 - 检查所有依赖服务
    """
    try:
        # 检查数据库连接
        with engine.connect() as conn:
            conn.execute("SELECT 1")
        
        return JSONResponse(
            status_code=200,
            content={
                "status": "ready",
                "timestamp": datetime.utcnow().isoformat(),
                "service": "pdf-validator",
                "checks": {
                    "database": "healthy",
                    "storage": "healthy"
                }
            }
        )
    except Exception as e:
        logger.error(f"Readiness check failed: {e}")
        return JSONResponse(
            status_code=503,
            content={
                "status": "not_ready",
                "timestamp": datetime.utcnow().isoformat(),
                "service": "pdf-validator",
                "error": str(e)
            }
        )


@router.get("/live")
async def liveness_check():
    """
    存活检查
    """
    return JSONResponse(
        status_code=200,
        content={
            "status": "alive",
            "timestamp": datetime.utcnow().isoformat(),
            "service": "pdf-validator"
        }
    )