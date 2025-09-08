"""
PDF Validator Worker - Celery异步任务处理器
"""
import logging
import tempfile
from datetime import datetime
from typing import Dict, Any, Optional, List
from pathlib import Path

import fitz  # PyMuPDF
from celery import Celery
from celery.exceptions import Retry, WorkerLostError
from sqlalchemy.orm import Session

from app.core.config import get_celery_config, settings
from app.core.database import SessionLocal
from app.models.validation_task import ValidationTask, ValidationResult, TaskStatus
from app.services.storage_service import StorageService
from app.services.pdf_processor import PDFProcessor
from app.services.enhanced_pdf_processor import EnhancedPDFProcessor
from app.utils.exceptions import PDFValidationError, FileNotFoundError


# 导入统一的Celery应用
from app.celery import celery_app

logger = logging.getLogger(__name__)


class PDFValidationWorker:
    """PDF验证工作器"""
    
    def __init__(self):
        self.storage_service = None
        self.pdf_processor = PDFProcessor()
        self.enhanced_processor = EnhancedPDFProcessor()
    
    def _get_storage_service(self):
        """延迟初始化存储服务"""
        if self.storage_service is None:
            self.storage_service = StorageService()
        return self.storage_service
        
    def process_validation_task(
        self, 
        task_id: str,
        pdf_file_path: str,
        validation_type: str = "standard",
        metadata: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """
        处理PDF验证任务
        
        Args:
            task_id: 任务ID
            pdf_file_path: PDF文件路径（MinIO中的路径）
            validation_type: 验证类型（standard, enhanced, strict）
            metadata: 附加元数据
            
        Returns:
            Dict[str, Any]: 验证结果
        """
        db = SessionLocal()
        task = None
        
        try:
            # 1. 更新任务状态为处理中
            task = db.query(ValidationTask).filter(
                ValidationTask.task_id == task_id
            ).first()
            
            if not task:
                raise PDFValidationError(f"Task {task_id} not found")
                
            task.status = TaskStatus.PROCESSING
            task.started_at = datetime.utcnow()
            db.commit()
            
            logger.info(f"开始处理PDF验证任务: {task_id}")
            
            # 2. 处理PDF文件路径（支持本地文件和对象存储）
            if pdf_file_path.startswith('/') or pdf_file_path.startswith('C:'):
                # 本地文件路径，直接使用
                temp_path = Path(pdf_file_path)
                if not temp_path.exists():
                    raise FileNotFoundError(f"Local file not found: {pdf_file_path}")
                logger.info(f"使用本地文件: {pdf_file_path}")
            else:
                # 对象存储路径，需要下载
                with tempfile.NamedTemporaryFile(suffix='.pdf', delete=False) as tmp_file:
                    temp_path = Path(tmp_file.name)
                self._get_storage_service().download_file(pdf_file_path, str(temp_path))
                logger.info(f"从对象存储下载文件: {pdf_file_path}")
            
            # 3. 验证PDF文件
            validation_result = self._validate_pdf_file(
                str(temp_path), 
                validation_type, 
                metadata
            )
            
            # 3.5. 使用增强处理器提取块信息和职业编码，并生成页面快照
            try:
                enhanced_result = self.enhanced_processor.process_pdf_with_blocks(
                    task_id, 
                    str(temp_path), 
                    save_to_db=True,
                    extract_snapshots=True  # 启用页面快照提取
                )
                logger.info(f"增强处理完成: {task_id}, 块数量: {enhanced_result.get('total_blocks', 0)}, 页面快照: {enhanced_result.get('page_snapshots', {}).get('extracted', 0)}")
            except Exception as e:
                logger.warning(f"增强处理失败: {task_id}, 错误: {str(e)}")
            
            # 4. 保存验证结果
            result_record = ValidationResult(
                task_id=task_id,
                pdf_path=pdf_file_path,
                validation_type=validation_type,
                is_valid=validation_result.get("is_valid", False),
                extracted_text=validation_result.get("text", ""),
                extracted_metadata=validation_result.get("metadata", {}),
                page_count=validation_result.get("page_count", 0),
                file_size=validation_result.get("file_size", 0),
                processing_time=validation_result.get("processing_time", 0.0),
                errors=validation_result.get("errors", []),
                warnings=validation_result.get("warnings", [])
            )
            
            db.add(result_record)
            
            # 5. 更新任务状态
            task.status = TaskStatus.COMPLETED
            task.completed_at = datetime.utcnow()
            task.result_summary = {
                "is_valid": validation_result.get("is_valid", False),
                "page_count": validation_result.get("page_count", 0),
                "text_length": len(validation_result.get("text", "")),
                "has_errors": len(validation_result.get("errors", [])) > 0
            }
            
            db.commit()
            
            logger.info(f"PDF验证任务完成: {task_id}, 结果: {task.result_summary}")
            
            # 6. 清理临时文件（仅删除从对象存储下载的临时文件）
            if not (pdf_file_path.startswith('/') or pdf_file_path.startswith('C:')):
                # 只有从对象存储下载的临时文件才需要清理
                if temp_path.exists():
                    temp_path.unlink()
                
            return {
                "task_id": task_id,
                "status": "completed",
                "result": validation_result
            }
            
        except Exception as e:
            logger.error(f"PDF验证任务失败: {task_id}, 错误: {str(e)}")
            
            # 更新任务状态为失败
            if task:
                task.status = TaskStatus.FAILED
                task.completed_at = datetime.utcnow()
                task.error_message = str(e)
                db.commit()
            
            # 清理临时文件（仅删除从对象存储下载的临时文件）
            try:
                if 'temp_path' in locals() and temp_path.exists():
                    if not (pdf_file_path.startswith('/') or pdf_file_path.startswith('C:')):
                        temp_path.unlink()
            except:
                pass
                
            raise
            
        finally:
            db.close()
    
    def _validate_pdf_file(
        self, 
        file_path: str, 
        validation_type: str,
        metadata: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """
        实际的PDF文件验证逻辑
        
        Args:
            file_path: 本地PDF文件路径
            validation_type: 验证类型
            metadata: 附加元数据
            
        Returns:
            Dict[str, Any]: 详细验证结果
        """
        start_time = datetime.utcnow()
        
        try:
            # 使用PDF处理器进行验证
            result = self.pdf_processor.validate_pdf(
                file_path, 
                validation_type,
                metadata
            )
            
            processing_time = (datetime.utcnow() - start_time).total_seconds()
            result["processing_time"] = processing_time
            
            return result
            
        except Exception as e:
            processing_time = (datetime.utcnow() - start_time).total_seconds()
            return {
                "is_valid": False,
                "processing_time": processing_time,
                "errors": [f"PDF验证失败: {str(e)}"],
                "warnings": [],
                "text": "",
                "metadata": {},
                "page_count": 0,
                "file_size": 0
            }


# 创建工作器实例
pdf_worker = PDFValidationWorker()


@celery_app.task(
    bind=True,
    autoretry_for=(Exception,),
    retry_kwargs={'max_retries': settings.MAX_RETRY_ATTEMPTS, 'countdown': 60},
    name="validate_pdf_task"
)
def validate_pdf_task(
    self, 
    task_id: str, 
    pdf_file_path: str,
    validation_type: str = "standard",
    metadata: Optional[Dict[str, Any]] = None
) -> Dict[str, Any]:
    """
    标准PDF验证任务
    
    Args:
        task_id: 任务ID
        pdf_file_path: PDF文件路径
        validation_type: 验证类型
        metadata: 附加元数据
        
    Returns:
        Dict[str, Any]: 验证结果
    """
    try:
        return pdf_worker.process_validation_task(
            task_id, pdf_file_path, validation_type, metadata
        )
    except Exception as exc:
        logger.error(f"PDF验证任务异常: {task_id}, 重试次数: {self.request.retries}")
        
        # 如果达到最大重试次数，标记任务为最终失败
        if self.request.retries >= settings.MAX_RETRY_ATTEMPTS:
            db = SessionLocal()
            try:
                task = db.query(ValidationTask).filter(
                    ValidationTask.task_id == task_id
                ).first()
                if task:
                    task.status = TaskStatus.FAILED
                    task.completed_at = datetime.utcnow()
                    task.error_message = f"任务失败，已达最大重试次数: {str(exc)}"
                    db.commit()
            finally:
                db.close()
        
        raise self.retry(exc=exc)


@celery_app.task(
    bind=True,
    name="priority_validate_pdf_task",
    priority=9  # 高优先级
)
def priority_validate_pdf_task(
    self,
    task_id: str,
    pdf_file_path: str,
    validation_type: str = "enhanced",
    metadata: Optional[Dict[str, Any]] = None
) -> Dict[str, Any]:
    """
    高优先级PDF验证任务
    """
    return validate_pdf_task.apply(
        args=[task_id, pdf_file_path, validation_type, metadata],
        queue=settings.PRIORITY_QUEUE_NAME
    )


@celery_app.task(name="batch_validate_pdf_task")
def batch_validate_pdf_task(
    task_batch: List[Dict[str, Any]]
) -> List[Dict[str, Any]]:
    """
    批量PDF验证任务
    
    Args:
        task_batch: 任务批次列表
        
    Returns:
        List[Dict[str, Any]]: 批量验证结果
    """
    results = []
    
    for task_info in task_batch:
        try:
            result = pdf_worker.process_validation_task(
                task_info["task_id"],
                task_info["pdf_file_path"],
                task_info.get("validation_type", "standard"),
                task_info.get("metadata")
            )
            results.append(result)
        except Exception as e:
            results.append({
                "task_id": task_info["task_id"],
                "status": "failed",
                "error": str(e)
            })
    
    return results


def start_celery_worker():
    """启动Celery Worker"""
    celery_app.worker_main([
        'worker',
        '--loglevel=info',
        '--concurrency=4',
        f'--queues={settings.VALIDATION_QUEUE_NAME},{settings.PRIORITY_QUEUE_NAME}',
        '--prefetch-multiplier=1'
    ])