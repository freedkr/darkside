"""
PDF Validation API Routes
"""
import uuid
import logging
from typing import Optional, Dict, Any
from datetime import datetime

from fastapi import APIRouter, UploadFile, File, HTTPException, BackgroundTasks, Query
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field

from app.core.database import SessionLocal
from app.models.validation_task import ValidationTask, TaskStatus
from app.services.storage_service import StorageService
from app.workers.pdf_validator import validate_pdf_task

logger = logging.getLogger(__name__)

router = APIRouter()


class ValidationRequest(BaseModel):
    """PDF验证请求模型"""
    pdf_path: str = Field(..., description="PDF文件在对象存储中的路径")
    validation_type: str = Field(default="standard", description="验证类型")
    metadata: Optional[Dict[str, Any]] = Field(default=None, description="附加元数据")


class ValidationResponse(BaseModel):
    """PDF验证响应模型"""
    task_id: str = Field(..., description="任务ID")
    status: str = Field(..., description="任务状态")
    created_at: datetime = Field(..., description="创建时间")
    message: str = Field(default="Task created successfully", description="消息")


@router.post("/validate", response_model=ValidationResponse)
async def validate_pdf(
    request: ValidationRequest,
    background_tasks: BackgroundTasks,
    async_mode: bool = Query(default=True, description="是否异步处理")
):
    """
    创建PDF验证任务
    
    Args:
        request: 验证请求
        background_tasks: FastAPI后台任务
        async_mode: 是否异步处理
        
    Returns:
        ValidationResponse: 任务创建响应
    """
    # 生成任务ID
    task_id = str(uuid.uuid4())
    
    # 创建数据库记录
    db = SessionLocal()
    try:
        task = ValidationTask(
            task_id=task_id,
            pdf_file_path=request.pdf_path,
            validation_type=request.validation_type,
            status=TaskStatus.PENDING,
            created_at=datetime.utcnow(),
            requester_service="api"
        )
        db.add(task)
        db.commit()
        
        # 触发异步任务
        if async_mode:
            # 使用Celery异步任务
            validate_pdf_task.delay(
                task_id,
                request.pdf_path,
                request.validation_type,
                request.metadata
            )
        else:
            # 同步处理（用于测试）
            background_tasks.add_task(
                validate_pdf_task,
                task_id,
                request.pdf_path,
                request.validation_type,
                request.metadata
            )
        
        return ValidationResponse(
            task_id=task_id,
            status=TaskStatus.PENDING,
            created_at=task.created_at,
            message="Validation task created successfully"
        )
        
    except Exception as e:
        logger.error(f"Failed to create validation task: {e}")
        db.rollback()
        raise HTTPException(status_code=500, detail=str(e))
    finally:
        db.close()


@router.post("/upload-and-validate")
async def upload_and_validate_pdf(
    background_tasks: BackgroundTasks,
    file: UploadFile = File(...),
    validation_type: str = Query(default="standard", description="验证类型")
):
    """
    上传PDF文件并验证
    
    Args:
        background_tasks: FastAPI后台任务
        file: 上传的PDF文件
        validation_type: 验证类型
        
    Returns:
        ValidationResponse: 任务创建响应
    """
    # 验证文件类型
    if not file.filename.lower().endswith('.pdf'):
        raise HTTPException(status_code=400, detail="Only PDF files are accepted")
    
    # 生成任务ID和文件路径
    task_id = str(uuid.uuid4())
    object_name = f"uploads/{task_id}/{file.filename}"
    
    try:
        # 上传文件到对象存储
        storage = StorageService()
        content = await file.read()
        from io import BytesIO
        storage.upload_file(
            object_name,
            BytesIO(content),
            content_type="application/pdf"
        )
        
        # 创建验证任务
        db = SessionLocal()
        try:
            task = ValidationTask(
                task_id=task_id,
                pdf_file_path=object_name,
                validation_type=validation_type,
                status=TaskStatus.PENDING,
                created_at=datetime.utcnow(),
                requester_service="api-upload"
            )
            db.add(task)
            db.commit()
            
            # 触发异步验证任务
            validate_pdf_task.delay(
                task_id,
                object_name,
                validation_type,
                {"original_filename": file.filename, "size": len(content)}
            )
            
            return ValidationResponse(
                task_id=task_id,
                status=TaskStatus.PENDING,
                created_at=task.created_at,
                message=f"File '{file.filename}' uploaded and validation task created"
            )
            
        finally:
            db.close()
            
    except Exception as e:
        logger.error(f"Failed to upload and validate PDF: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/status/{task_id}")
async def get_task_status(task_id: str):
    """
    获取任务状态
    
    Args:
        task_id: 任务ID
        
    Returns:
        任务状态信息
    """
    db = SessionLocal()
    try:
        task = db.query(ValidationTask).filter(
            ValidationTask.task_id == task_id
        ).first()
        
        if not task:
            raise HTTPException(status_code=404, detail="Task not found")
        
        response = {
            "task_id": task.task_id,
            "status": task.status,
            "validation_type": task.validation_type,
            "created_at": task.created_at.isoformat() if task.created_at else None,
            "started_at": task.started_at.isoformat() if task.started_at else None,
            "completed_at": task.completed_at.isoformat() if task.completed_at else None,
        }
        
        if task.result_summary:
            response["result"] = task.result_summary
            
        if task.error_message:
            response["error"] = task.error_message
        
        return JSONResponse(status_code=200, content=response)
        
    finally:
        db.close()


@router.get("/tasks")
async def list_tasks(
    status: Optional[str] = Query(None, description="按状态筛选"),
    limit: int = Query(10, ge=1, le=100, description="返回数量限制"),
    offset: int = Query(0, ge=0, description="偏移量")
):
    """
    列出验证任务
    
    Args:
        status: 状态筛选
        limit: 返回数量限制
        offset: 偏移量
        
    Returns:
        任务列表
    """
    db = SessionLocal()
    try:
        query = db.query(ValidationTask)
        
        if status:
            query = query.filter(ValidationTask.status == status)
        
        total = query.count()
        tasks = query.order_by(ValidationTask.created_at.desc()).offset(offset).limit(limit).all()
        
        return JSONResponse(
            status_code=200,
            content={
                "total": total,
                "limit": limit,
                "offset": offset,
                "tasks": [
                    {
                        "task_id": task.task_id,
                        "status": task.status,
                        "validation_type": task.validation_type,
                        "created_at": task.created_at.isoformat(),
                        "completed_at": task.completed_at.isoformat() if task.completed_at else None
                    }
                    for task in tasks
                ]
            }
        )
        
    finally:
        db.close()