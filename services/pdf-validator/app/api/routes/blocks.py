"""
PDF块信息查询API
"""
import logging
from typing import Optional, List, Dict, Any

from fastapi import APIRouter, Query, HTTPException
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field

from app.core.database import SessionLocal
from app.models.validation_task import PDFBlockInfo
from app.services.enhanced_pdf_processor import EnhancedPDFProcessor

logger = logging.getLogger(__name__)

router = APIRouter()


class BlockQueryParams(BaseModel):
    """块查询参数"""
    task_id: str = Field(..., description="任务ID")
    page_num: Optional[int] = Field(None, description="页码筛选")
    hierarchy_level: Optional[int] = Field(None, ge=1, le=4, description="层级筛选")
    has_occupation_code: Optional[bool] = Field(None, description="是否包含职业编码")
    font_size_min: Optional[float] = Field(None, description="最小字号")
    font_size_max: Optional[float] = Field(None, description="最大字号")


class BlockInfoResponse(BaseModel):
    """块信息响应"""
    id: int
    task_id: str
    page_num: int
    block_num: int
    text: str
    
    # 位置信息
    x0: float
    y0: float
    x1: float
    y1: float
    width: float
    height: float
    center_x: float
    center_y: float
    
    # 字体信息
    font: Optional[str]
    font_size: Optional[float]
    is_bold: bool
    is_italic: bool
    
    # 层级信息
    hierarchy_level: Optional[int]
    indentation: Optional[float]
    
    # 职业信息
    occupation_code: Optional[str]
    occupation_name: Optional[str]
    confidence: Optional[float]


@router.get("/blocks/{task_id}", response_model=List[BlockInfoResponse])
async def get_pdf_blocks(
    task_id: str,
    page_num: Optional[int] = Query(None, description="页码筛选"),
    hierarchy_level: Optional[int] = Query(None, ge=1, le=4, description="层级筛选"),
    has_occupation_code: Optional[bool] = Query(None, description="是否包含职业编码"),
    limit: int = Query(100, ge=1, le=1000, description="返回数量限制"),
    offset: int = Query(0, ge=0, description="偏移量")
):
    """
    获取PDF块信息
    
    Args:
        task_id: 任务ID
        page_num: 页码筛选
        hierarchy_level: 层级筛选（1-4）
        has_occupation_code: 是否包含职业编码
        limit: 返回数量限制
        offset: 偏移量
    """
    db = SessionLocal()
    try:
        query = db.query(PDFBlockInfo).filter(PDFBlockInfo.task_id == task_id)
        
        # 应用筛选条件
        if page_num is not None:
            query = query.filter(PDFBlockInfo.page_num == page_num)
        
        if hierarchy_level is not None:
            query = query.filter(PDFBlockInfo.hierarchy_level == hierarchy_level)
        
        if has_occupation_code is not None:
            if has_occupation_code:
                query = query.filter(PDFBlockInfo.occupation_code.isnot(None))
            else:
                query = query.filter(PDFBlockInfo.occupation_code.is_(None))
        
        # 排序：页码 -> 块编号
        query = query.order_by(PDFBlockInfo.page_num, PDFBlockInfo.block_num)
        
        # 分页
        total = query.count()
        blocks = query.offset(offset).limit(limit).all()
        
        # 转换为响应格式
        results = []
        for block in blocks:
            results.append(BlockInfoResponse(
                id=block.id,
                task_id=block.task_id,
                page_num=block.page_num,
                block_num=block.block_num,
                text=block.text,
                x0=block.x0,
                y0=block.y0,
                x1=block.x1,
                y1=block.y1,
                width=block.width,
                height=block.height,
                center_x=block.center_x,
                center_y=block.center_y,
                font=block.font,
                font_size=block.font_size,
                is_bold=block.is_bold,
                is_italic=block.is_italic,
                hierarchy_level=block.hierarchy_level,
                indentation=block.indentation,
                occupation_code=block.occupation_code,
                occupation_name=block.occupation_name,
                confidence=block.confidence
            ))
        
        return results
        
    finally:
        db.close()


@router.get("/blocks/{task_id}/summary")
async def get_blocks_summary(task_id: str):
    """
    获取块信息摘要统计
    
    Args:
        task_id: 任务ID
    """
    db = SessionLocal()
    try:
        # 基础统计
        total_blocks = db.query(PDFBlockInfo).filter(PDFBlockInfo.task_id == task_id).count()
        
        if total_blocks == 0:
            raise HTTPException(status_code=404, detail="Task not found or no blocks processed")
        
        # 页数统计
        page_count = db.query(PDFBlockInfo.page_num).filter(
            PDFBlockInfo.task_id == task_id
        ).distinct().count()
        
        # 层级分布
        hierarchy_dist = db.query(
            PDFBlockInfo.hierarchy_level,
            db.func.count(PDFBlockInfo.id).label('count')
        ).filter(
            PDFBlockInfo.task_id == task_id
        ).group_by(PDFBlockInfo.hierarchy_level).all()
        
        # 职业编码统计
        occupation_count = db.query(PDFBlockInfo).filter(
            PDFBlockInfo.task_id == task_id,
            PDFBlockInfo.occupation_code.isnot(None)
        ).count()
        
        # 字体使用统计
        font_stats = db.query(
            PDFBlockInfo.font,
            db.func.count(PDFBlockInfo.id).label('count')
        ).filter(
            PDFBlockInfo.task_id == task_id,
            PDFBlockInfo.font.isnot(None)
        ).group_by(PDFBlockInfo.font).all()
        
        return JSONResponse({
            "task_id": task_id,
            "summary": {
                "total_blocks": total_blocks,
                "page_count": page_count,
                "occupation_codes_found": occupation_count,
                "hierarchy_distribution": {
                    str(level): count for level, count in hierarchy_dist if level
                },
                "fonts_used": [
                    {"font": font, "count": count} 
                    for font, count in font_stats[:10]  # 前10个最常用字体
                ]
            }
        })
        
    finally:
        db.close()


@router.get("/blocks/{task_id}/occupation-codes")
async def get_occupation_codes(task_id: str):
    """
    获取识别出的职业编码列表
    
    Args:
        task_id: 任务ID
    """
    db = SessionLocal()
    try:
        codes = db.query(
            PDFBlockInfo.occupation_code,
            PDFBlockInfo.occupation_name,
            PDFBlockInfo.confidence,
            PDFBlockInfo.page_num,
            PDFBlockInfo.text
        ).filter(
            PDFBlockInfo.task_id == task_id,
            PDFBlockInfo.occupation_code.isnot(None)
        ).order_by(PDFBlockInfo.page_num, PDFBlockInfo.block_num).all()
        
        results = []
        for code, name, confidence, page_num, text in codes:
            results.append({
                "code": code,
                "name": name,
                "confidence": confidence,
                "page": page_num,
                "context": text
            })
        
        return JSONResponse({
            "task_id": task_id,
            "occupation_codes": results,
            "total_found": len(results)
        })
        
    finally:
        db.close()


@router.get("/blocks/{task_id}/layout-analysis")
async def get_layout_analysis(task_id: str):
    """
    获取版式分析结果
    
    Args:
        task_id: 任务ID
    """
    db = SessionLocal()
    try:
        # 获取所有块
        blocks = db.query(PDFBlockInfo).filter(
            PDFBlockInfo.task_id == task_id
        ).all()
        
        if not blocks:
            raise HTTPException(status_code=404, detail="Task not found or no blocks processed")
        
        # 分析版式特征
        processor = EnhancedPDFProcessor()
        
        # 字号分析
        font_sizes = [b.font_size for b in blocks if b.font_size]
        
        # 缩进分析
        indentations = list(set(b.indentation for b in blocks if b.indentation))
        indentations.sort()
        
        # 层级分析
        hierarchy_blocks = {}
        for block in blocks:
            level = block.hierarchy_level or 4
            if level not in hierarchy_blocks:
                hierarchy_blocks[level] = []
            hierarchy_blocks[level].append({
                "text": block.text[:50] + "..." if len(block.text) > 50 else block.text,
                "page": block.page_num,
                "font_size": block.font_size,
                "is_bold": block.is_bold
            })
        
        return JSONResponse({
            "task_id": task_id,
            "layout_analysis": {
                "font_size_range": [min(font_sizes), max(font_sizes)] if font_sizes else [0, 0],
                "indent_levels": indentations[:5],  # 前5个缩进级别
                "hierarchy_structure": {
                    str(level): blocks[:3]  # 每层级显示前3个示例
                    for level, blocks in hierarchy_blocks.items()
                },
                "total_blocks": len(blocks),
                "pages_processed": len(set(b.page_num for b in blocks))
            }
        })
        
    finally:
        db.close()