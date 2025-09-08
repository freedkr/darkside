"""
增强版PDF处理器 - 提取详细的块信息和版式数据
"""
import re
import logging
from datetime import datetime
from pathlib import Path
from typing import Dict, Any, List, Optional, Tuple

import fitz  # PyMuPDF
from sqlalchemy.orm import Session

from app.core.database import SessionLocal
from app.models.validation_task import PDFBlockInfo, ValidationResult
from app.services.page_snapshot_service import PageSnapshotService
from app.utils.exceptions import PDFValidationError

logger = logging.getLogger(__name__)


class EnhancedPDFProcessor:
    """增强版PDF处理器 - 提取并保存详细的块信息"""
    
    def __init__(self):
        self.xixi_pattern = re.compile(r'\b(\d-\d{2}-\d{2}-\d{2})\b')  # 职业编码模式
        self._snapshot_service = None  # 延迟初始化页面快照服务
    
    @property
    def snapshot_service(self):
        """延迟创建页面快照服务"""
        if self._snapshot_service is None:
            self._snapshot_service = PageSnapshotService()
        return self._snapshot_service
        
    def process_pdf_with_blocks(
        self,
        task_id: str,
        file_path: str,
        save_to_db: bool = True,
        extract_snapshots: bool = True
    ) -> Dict[str, Any]:
        """
        处理PDF并提取详细的块信息
        
        Args:
            task_id: 任务ID
            file_path: PDF文件路径
            save_to_db: 是否保存到数据库
            extract_snapshots: 是否提取页面快照
            
        Returns:
            包含块信息的处理结果
        """
        start_time = datetime.utcnow()
        
        try:
            # 打开PDF文档
            doc = fitz.open(file_path)
            
            # 提取所有块信息
            all_blocks = []
            all_text = []
            occupation_codes = []
            page_count = len(doc)
            
            for page_num, page in enumerate(doc):
                # 获取页面的详细文本信息
                page_blocks = self._extract_page_blocks(page, page_num, task_id)
                all_blocks.extend(page_blocks)
                
                # 收集文本
                page_text = page.get_text()
                all_text.append(page_text)
                
                # 提取职业编码
                codes = self.xixi_pattern.findall(page_text)
                occupation_codes.extend(codes)
            
            # 提取页面快照（在关闭文档之前）
            snapshots_info = []
            if extract_snapshots:
                try:
                    logger.info(f"Extracting page snapshots for task {task_id}")
                    snapshots_info = self.snapshot_service.extract_page_snapshots(
                        task_id=task_id,
                        doc=doc,
                        dpi=150,
                        save_thumbnail=True
                    )
                    logger.info(f"Successfully extracted {len(snapshots_info)} page snapshots")
                except Exception as e:
                    logger.error(f"Failed to extract page snapshots: {str(e)}")
                    # 不中断主流程，记录错误继续
            
            doc.close()
            
            # 分析层级关系
            self._analyze_hierarchy(all_blocks)
            
            # 匹配职业编码和名称
            self._match_occupation_info(all_blocks)
            
            # 保存到数据库
            if save_to_db:
                self._save_blocks_to_db(task_id, all_blocks)
            
            # 统计信息
            processing_time = (datetime.utcnow() - start_time).total_seconds()
            
            result = {
                "is_valid": True,
                "page_count": page_count,
                "total_blocks": len(all_blocks),
                "unique_occupation_codes": len(set(occupation_codes)),
                "extracted_text": "\n".join(all_text),
                "processing_time": processing_time,
                "blocks_summary": {
                    "total": len(all_blocks),
                    "with_occupation_code": sum(1 for b in all_blocks if b.get("occupation_code")),
                    "hierarchy_levels": self._get_hierarchy_distribution(all_blocks),
                    "fonts_used": list(set(b.get("font", "") for b in all_blocks if b.get("font")))
                },
                "page_snapshots": {
                    "extracted": len(snapshots_info),
                    "total_size": sum(s.get("image_size", 0) for s in snapshots_info),
                    "with_thumbnails": sum(1 for s in snapshots_info if s.get("thumbnail_path"))
                }
            }
            
            logger.info(f"PDF处理完成 - 任务ID: {task_id}, 块数: {len(all_blocks)}")
            return result
            
        except Exception as e:
            logger.error(f"PDF处理失败 - 任务ID: {task_id}, 错误: {str(e)}")
            raise PDFValidationError(f"PDF处理失败: {str(e)}")
    
    def _extract_page_blocks(self, page, page_num: int, task_id: str) -> List[Dict]:
        """提取页面的所有块信息"""
        blocks = []
        block_num = 0
        
        # 获取页面的详细字典信息
        page_dict = page.get_text("dict")
        
        for block in page_dict.get("blocks", []):
            if block.get("type") != 0:  # 只处理文本块
                continue
                
            for line in block.get("lines", []):
                for span in line.get("spans", []):
                    text = span.get("text", "").strip()
                    if not text:
                        continue
                    
                    bbox = span.get("bbox", [0, 0, 0, 0])
                    
                    # 构建块信息
                    block_info = {
                        "task_id": task_id,
                        "page_num": page_num + 1,
                        "block_num": block_num,
                        "text": text,
                        # 位置信息
                        "x0": bbox[0],
                        "y0": bbox[1],
                        "x1": bbox[2],
                        "y1": bbox[3],
                        "width": bbox[2] - bbox[0],
                        "height": bbox[3] - bbox[1],
                        "center_x": (bbox[0] + bbox[2]) / 2,
                        "center_y": (bbox[1] + bbox[3]) / 2,
                        # 字体信息
                        "font": span.get("font", ""),
                        "font_size": span.get("size", 0),
                        "font_flags": span.get("flags", 0),
                        "font_color": span.get("color", 0),
                        # 样式判断
                        "is_bold": bool(span.get("flags", 0) & 2**4),
                        "is_italic": bool(span.get("flags", 0) & 2**1),
                        # 缩进
                        "indentation": bbox[0],
                    }
                    
                    # 检查是否包含职业编码
                    code_matches = self.xixi_pattern.findall(text)
                    if code_matches:
                        block_info["occupation_code"] = code_matches[0]
                    
                    blocks.append(block_info)
                    block_num += 1
        
        return blocks
    
    def _analyze_hierarchy(self, blocks: List[Dict]):
        """分析块的层级关系"""
        if not blocks:
            return
        
        # 收集字号信息
        font_sizes = sorted(set(b["font_size"] for b in blocks if b.get("font_size", 0) > 0))
        
        # 收集缩进信息
        indentations = sorted(set(b["indentation"] for b in blocks))
        indent_levels = self._get_indent_levels(indentations)
        
        for block in blocks:
            # 基于字号判断层级
            size = block.get("font_size", 0)
            indent = block.get("indentation", 0)
            is_bold = block.get("is_bold", False)
            
            # 层级判断规则
            level = 4  # 默认最低级
            
            if size > 0 and font_sizes:
                if size >= font_sizes[-1] if len(font_sizes) >= 1 else 14:  # 最大字号
                    level = 1
                elif len(font_sizes) >= 2 and size >= font_sizes[-2]:
                    level = 2
                elif len(font_sizes) >= 3 and size >= font_sizes[-3]:
                    level = 3
            
            # 缩进调整
            indent_level = self._get_indent_level(indent, indent_levels)
            if indent_level > 0:
                level = min(level + indent_level, 4)
            
            # 粗体优先级提升
            if is_bold and level > 1:
                level -= 1
            
            block["hierarchy_level"] = level
    
    def _get_indent_levels(self, indentations: List[float]) -> List[float]:
        """识别缩进级别"""
        if not indentations:
            return []
        
        levels = [indentations[0]]
        for indent in indentations[1:]:
            if indent - levels[-1] > 15:  # 15px为最小缩进差
                levels.append(indent)
        
        return levels[:4]  # 最多4个级别
    
    def _get_indent_level(self, indent: float, levels: List[float]) -> int:
        """获取缩进级别"""
        for i, level in enumerate(levels):
            if abs(indent - level) < 10:
                return i
        return len(levels) if indent > levels[-1] else 0
    
    def _match_occupation_info(self, blocks: List[Dict]):
        """匹配职业编码与名称"""
        for i, block in enumerate(blocks):
            if not block.get("occupation_code"):
                continue
            
            # 查找后续块中的职业名称
            for j in range(i + 1, min(i + 5, len(blocks))):
                candidate = blocks[j]
                
                # 判断是否在同一行或相邻行
                if abs(candidate["center_y"] - block["center_y"]) < block["height"]:
                    # 可能是同行的职业名称
                    if self._is_occupation_name(candidate["text"]):
                        block["occupation_name"] = candidate["text"]
                        block["confidence"] = 0.9
                        break
                elif candidate["center_y"] > block["center_y"] and \
                     candidate["center_y"] - block["center_y"] < block["height"] * 2:
                    # 可能是下一行的职业名称
                    if self._is_occupation_name(candidate["text"]):
                        block["occupation_name"] = candidate["text"]
                        block["confidence"] = 0.7
                        break
    
    def _is_occupation_name(self, text: str) -> bool:
        """判断文本是否可能是职业名称"""
        # 排除纯数字、编码等
        if re.match(r'^[\d\s\-\.]+$', text):
            return False
        
        # 包含职业相关关键词
        job_keywords = ['员', '师', '工', '长', '家', '人员', '技术', '管理', '操作', '专员', '主管']
        if any(keyword in text for keyword in job_keywords):
            return True
        
        # 中文占比超过50%
        chinese_chars = len(re.findall(r'[\u4e00-\u9fa5]', text))
        return chinese_chars / len(text) > 0.5 if text else False
    
    def _save_blocks_to_db(self, task_id: str, blocks: List[Dict]):
        """保存块信息到数据库"""
        db = SessionLocal()
        try:
            # 批量创建块记录
            for block in blocks:
                pdf_block = PDFBlockInfo(
                    task_id=task_id,
                    page_num=block["page_num"],
                    block_num=block["block_num"],
                    text=block["text"],
                    x0=block["x0"],
                    y0=block["y0"],
                    x1=block["x1"],
                    y1=block["y1"],
                    width=block["width"],
                    height=block["height"],
                    center_x=block["center_x"],
                    center_y=block["center_y"],
                    font=block.get("font"),
                    font_size=block.get("font_size"),
                    font_flags=block.get("font_flags"),
                    font_color=block.get("font_color"),
                    hierarchy_level=block.get("hierarchy_level"),
                    indentation=block.get("indentation"),
                    is_bold=block.get("is_bold", False),
                    is_italic=block.get("is_italic", False),
                    occupation_code=block.get("occupation_code"),
                    occupation_name=block.get("occupation_name"),
                    confidence=block.get("confidence")
                )
                db.add(pdf_block)
            
            db.commit()
            logger.info(f"成功保存 {len(blocks)} 个块信息到数据库")
            
        except Exception as e:
            db.rollback()
            logger.error(f"保存块信息失败: {str(e)}")
            raise
        finally:
            db.close()
    
    def _get_hierarchy_distribution(self, blocks: List[Dict]) -> Dict[int, int]:
        """获取层级分布"""
        distribution = {1: 0, 2: 0, 3: 0, 4: 0}
        for block in blocks:
            level = block.get("hierarchy_level", 4)
            distribution[level] = distribution.get(level, 0) + 1
        return distribution
    
    def query_blocks(self, task_id: str, filters: Optional[Dict] = None) -> List[PDFBlockInfo]:
        """查询块信息"""
        db = SessionLocal()
        try:
            query = db.query(PDFBlockInfo).filter(PDFBlockInfo.task_id == task_id)
            
            if filters:
                if "page_num" in filters:
                    query = query.filter(PDFBlockInfo.page_num == filters["page_num"])
                if "hierarchy_level" in filters:
                    query = query.filter(PDFBlockInfo.hierarchy_level == filters["hierarchy_level"])
                if "has_occupation_code" in filters and filters["has_occupation_code"]:
                    query = query.filter(PDFBlockInfo.occupation_code.isnot(None))
            
            return query.all()
            
        finally:
            db.close()