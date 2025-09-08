"""
PDF页面快照API路由
"""
import logging
from typing import List, Optional
from fastapi import APIRouter, HTTPException, Query, Path
from fastapi.responses import StreamingResponse, JSONResponse
from io import BytesIO

from app.services.page_snapshot_service import PageSnapshotService
from app.services.storage_service import StorageService
from app.models.validation_task import PDFPageSnapshot
from app.core.database import SessionLocal

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/page-snapshots", tags=["Page Snapshots"])

# 初始化服务
snapshot_service = PageSnapshotService()
storage_service = StorageService()


@router.get("/{task_id}")
async def get_task_snapshots(
    task_id: str = Path(..., description="任务ID"),
    page: Optional[int] = Query(None, description="指定页码")
) -> JSONResponse:
    """
    获取任务的页面快照信息
    
    Args:
        task_id: 任务ID
        page: 可选的页码筛选
        
    Returns:
        页面快照信息列表
    """
    db = SessionLocal()
    try:
        query = db.query(PDFPageSnapshot).filter(PDFPageSnapshot.task_id == task_id)
        
        if page is not None:
            query = query.filter(PDFPageSnapshot.page_num == page)
        
        snapshots = query.order_by(PDFPageSnapshot.page_num).all()
        
        if not snapshots:
            raise HTTPException(status_code=404, detail=f"未找到任务 {task_id} 的页面快照")
        
        result = []
        for snapshot in snapshots:
            result.append({
                "id": snapshot.id,
                "task_id": snapshot.task_id,
                "page_num": snapshot.page_num,
                "minio_path": snapshot.minio_path,
                "thumbnail_path": snapshot.thumbnail_path,
                "page_size": {
                    "width": snapshot.page_width,
                    "height": snapshot.page_height
                },
                "image_info": {
                    "width": snapshot.image_width,
                    "height": snapshot.image_height,
                    "format": snapshot.image_format,
                    "size": snapshot.image_size,
                    "dpi": snapshot.dpi
                },
                "content_stats": {
                    "text_blocks": snapshot.text_blocks_count,
                    "images": snapshot.images_count,
                    "tables": snapshot.tables_count
                },
                "font_info": {
                    "primary": snapshot.primary_font,
                    "sizes": snapshot.font_sizes
                },
                "layout": {
                    "has_header": snapshot.has_header,
                    "has_footer": snapshot.has_footer,
                    "columns": snapshot.columns_count
                },
                "created_at": snapshot.created_at.isoformat() if snapshot.created_at else None
            })
        
        return JSONResponse(content={
            "task_id": task_id,
            "total_pages": len(result),
            "snapshots": result
        })
        
    finally:
        db.close()


@router.get("/{task_id}/page/{page_num}/image")
async def get_page_image(
    task_id: str = Path(..., description="任务ID"),
    page_num: int = Path(..., description="页码", ge=1),
    thumbnail: bool = Query(False, description="获取缩略图")
) -> StreamingResponse:
    """
    获取指定页面的图片
    
    Args:
        task_id: 任务ID
        page_num: 页码（从1开始）
        thumbnail: 是否获取缩略图
        
    Returns:
        图片文件流
    """
    db = SessionLocal()
    try:
        # 查询快照记录
        snapshot = db.query(PDFPageSnapshot).filter(
            PDFPageSnapshot.task_id == task_id,
            PDFPageSnapshot.page_num == page_num
        ).first()
        
        if not snapshot:
            raise HTTPException(
                status_code=404, 
                detail=f"未找到任务 {task_id} 第 {page_num} 页的快照"
            )
        
        # 确定要获取的文件路径
        if thumbnail:
            if not snapshot.thumbnail_path:
                raise HTTPException(
                    status_code=404,
                    detail=f"第 {page_num} 页没有缩略图"
                )
            object_path = snapshot.thumbnail_path
        else:
            object_path = snapshot.minio_path
        
        # 从MinIO获取图片
        try:
            image_data = storage_service.download_file(object_path)
            image_data.seek(0)
            
            return StreamingResponse(
                image_data,
                media_type=f"image/{snapshot.image_format}",
                headers={
                    "Content-Disposition": f"inline; filename=page_{page_num:04d}.{snapshot.image_format}"
                }
            )
            
        except Exception as e:
            logger.error(f"获取图片失败: {str(e)}")
            raise HTTPException(
                status_code=500,
                detail=f"获取图片失败: {str(e)}"
            )
            
    finally:
        db.close()


@router.get("/{task_id}/statistics")
async def get_snapshots_statistics(
    task_id: str = Path(..., description="任务ID")
) -> JSONResponse:
    """
    获取页面快照统计信息
    
    Args:
        task_id: 任务ID
        
    Returns:
        统计信息
    """
    db = SessionLocal()
    try:
        snapshots = db.query(PDFPageSnapshot).filter(
            PDFPageSnapshot.task_id == task_id
        ).all()
        
        if not snapshots:
            raise HTTPException(status_code=404, detail=f"未找到任务 {task_id} 的页面快照")
        
        # 计算统计信息
        total_size = sum(s.image_size for s in snapshots)
        total_text_blocks = sum(s.text_blocks_count for s in snapshots)
        total_images = sum(s.images_count for s in snapshots)
        total_tables = sum(s.tables_count for s in snapshots)
        
        # 字体统计
        font_usage = {}
        for snapshot in snapshots:
            if snapshot.primary_font:
                font_usage[snapshot.primary_font] = font_usage.get(snapshot.primary_font, 0) + 1
        
        # 布局统计
        pages_with_header = sum(1 for s in snapshots if s.has_header)
        pages_with_footer = sum(1 for s in snapshots if s.has_footer)
        
        # 列布局分布
        column_distribution = {}
        for snapshot in snapshots:
            col_count = snapshot.columns_count or 1
            column_distribution[col_count] = column_distribution.get(col_count, 0) + 1
        
        return JSONResponse(content={
            "task_id": task_id,
            "total_pages": len(snapshots),
            "storage": {
                "total_size_bytes": total_size,
                "total_size_mb": round(total_size / 1024 / 1024, 2),
                "average_size_kb": round(total_size / len(snapshots) / 1024, 2) if snapshots else 0
            },
            "content": {
                "total_text_blocks": total_text_blocks,
                "total_images": total_images,
                "total_tables": total_tables,
                "avg_text_blocks_per_page": round(total_text_blocks / len(snapshots), 1) if snapshots else 0,
                "avg_images_per_page": round(total_images / len(snapshots), 1) if snapshots else 0
            },
            "fonts": {
                "unique_fonts": len(font_usage),
                "font_usage": font_usage
            },
            "layout": {
                "pages_with_header": pages_with_header,
                "pages_with_footer": pages_with_footer,
                "column_distribution": column_distribution
            },
            "image_info": {
                "dpi": snapshots[0].dpi if snapshots else None,
                "format": snapshots[0].image_format if snapshots else None
            }
        })
        
    finally:
        db.close()


@router.delete("/{task_id}")
async def delete_task_snapshots(
    task_id: str = Path(..., description="任务ID")
) -> JSONResponse:
    """
    删除任务的所有页面快照
    
    Args:
        task_id: 任务ID
        
    Returns:
        删除结果
    """
    db = SessionLocal()
    try:
        # 查询所有快照
        snapshots = db.query(PDFPageSnapshot).filter(
            PDFPageSnapshot.task_id == task_id
        ).all()
        
        if not snapshots:
            raise HTTPException(status_code=404, detail=f"未找到任务 {task_id} 的页面快照")
        
        # 删除MinIO中的文件
        deleted_files = []
        failed_files = []
        
        for snapshot in snapshots:
            # 删除主图片
            try:
                storage_service.delete_file(snapshot.minio_path)
                deleted_files.append(snapshot.minio_path)
            except Exception as e:
                logger.error(f"删除文件失败 {snapshot.minio_path}: {str(e)}")
                failed_files.append(snapshot.minio_path)
            
            # 删除缩略图
            if snapshot.thumbnail_path:
                try:
                    storage_service.delete_file(snapshot.thumbnail_path)
                    deleted_files.append(snapshot.thumbnail_path)
                except Exception as e:
                    logger.error(f"删除缩略图失败 {snapshot.thumbnail_path}: {str(e)}")
                    failed_files.append(snapshot.thumbnail_path)
        
        # 删除数据库记录
        deleted_count = db.query(PDFPageSnapshot).filter(
            PDFPageSnapshot.task_id == task_id
        ).delete()
        
        db.commit()
        
        return JSONResponse(content={
            "task_id": task_id,
            "deleted_records": deleted_count,
            "deleted_files": len(deleted_files),
            "failed_files": len(failed_files),
            "details": {
                "deleted": deleted_files,
                "failed": failed_files
            }
        })
        
    except Exception as e:
        db.rollback()
        logger.error(f"删除页面快照失败: {str(e)}")
        raise HTTPException(status_code=500, detail=f"删除失败: {str(e)}")
    finally:
        db.close()