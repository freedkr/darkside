"""
PDF页面快照服务 - 提取每页截图并保存到MinIO
"""
import logging
from io import BytesIO
from typing import List, Dict, Any, Optional, Tuple
from collections import Counter
import fitz  # PyMuPDF
from PIL import Image

from app.services.storage_service import StorageService
from app.models.validation_task import PDFPageSnapshot, PDFBlockInfo
from app.core.database import SessionLocal

logger = logging.getLogger(__name__)


class PageSnapshotService:
    """PDF页面快照服务"""
    
    def __init__(self):
        """初始化服务"""
        self._storage = None  # 延迟初始化
        self.default_dpi = 150  # 默认DPI
        self.thumbnail_size = (200, 280)  # 缩略图尺寸
    
    @property
    def storage(self):
        """延迟创建StorageService实例"""
        if self._storage is None:
            self._storage = StorageService()
        return self._storage
        
    def extract_page_snapshots(
        self,
        task_id: str,
        doc: fitz.Document,
        dpi: int = None,
        save_thumbnail: bool = True
    ) -> List[Dict[str, Any]]:
        """
        提取所有页面的截图并保存
        
        Args:
            task_id: 任务ID
            doc: PDF文档对象
            dpi: 截图DPI（默认150）
            save_thumbnail: 是否生成缩略图
            
        Returns:
            页面快照信息列表
        """
        if dpi is None:
            dpi = self.default_dpi
            
        snapshots = []
        db = SessionLocal()
        
        try:
            for page_num in range(len(doc)):
                # logger.info(f"Processing page {page_num + 1}/{len(doc)} for task {task_id}")
                
                # 提取单页快照
                snapshot_info = self._extract_single_page(
                    task_id=task_id,
                    doc=doc,
                    page_num=page_num,
                    dpi=dpi,
                    save_thumbnail=save_thumbnail
                )
                
                # 保存到数据库
                snapshot_record = PDFPageSnapshot(
                    task_id=task_id,
                    page_num=page_num + 1,
                    minio_path=snapshot_info['minio_path'],
                    thumbnail_path=snapshot_info.get('thumbnail_path'),
                    page_width=snapshot_info['page_width'],
                    page_height=snapshot_info['page_height'],
                    dpi=dpi,
                    image_width=snapshot_info['image_width'],
                    image_height=snapshot_info['image_height'],
                    image_format=snapshot_info['image_format'],
                    image_size=snapshot_info['image_size'],
                    text_blocks_count=snapshot_info['text_blocks_count'],
                    images_count=snapshot_info['images_count'],
                    tables_count=snapshot_info['tables_count'],
                    primary_font=snapshot_info.get('primary_font'),
                    font_sizes=snapshot_info.get('font_sizes'),
                    has_header=snapshot_info.get('has_header', False),
                    has_footer=snapshot_info.get('has_footer', False),
                    columns_count=snapshot_info.get('columns_count', 1)
                )
                
                db.add(snapshot_record)
                snapshots.append(snapshot_info)
                
            db.commit()
            logger.info(f"Successfully saved {len(snapshots)} page snapshots for task {task_id}")
            
        except Exception as e:
            db.rollback()
            logger.error(f"Failed to extract page snapshots: {str(e)}")
            raise
        finally:
            db.close()
            
        return snapshots
    
    def _extract_single_page(
        self,
        task_id: str,
        doc: fitz.Document,
        page_num: int,
        dpi: int,
        save_thumbnail: bool
    ) -> Dict[str, Any]:
        """
        提取单个页面的快照
        
        Args:
            task_id: 任务ID
            doc: PDF文档对象
            page_num: 页码（从0开始）
            dpi: 截图DPI
            save_thumbnail: 是否生成缩略图
            
        Returns:
            页面快照信息
        """
        page = doc.load_page(page_num)
        
        # 获取页面尺寸
        page_rect = page.rect
        page_width = page_rect.width
        page_height = page_rect.height
        
        # 渲染页面为图片
        mat = fitz.Matrix(dpi/72.0, dpi/72.0)  # 72 DPI是PDF的标准DPI
        pix = page.get_pixmap(matrix=mat, alpha=False)
        
        # 转换为PNG格式
        img_data = pix.tobytes("png")
        img_buffer = BytesIO(img_data)
        
        # 上传到MinIO
        minio_path = f"pdf-snapshots/{task_id}/page_{page_num + 1:04d}.png"
        self.storage.upload_file(
            object_name=minio_path,
            file_data=img_buffer,
            content_type="image/png",
            metadata={
                "task_id": task_id,
                "page_num": str(page_num + 1),
                "dpi": str(dpi)
            }
        )
        
        # 生成缩略图
        thumbnail_path = None
        if save_thumbnail:
            thumbnail_path = self._create_thumbnail(
                task_id=task_id,
                page_num=page_num,
                img_data=img_data
            )
        
        # 分析页面内容
        page_metadata = self._analyze_page_content(page)
        
        # 获取字体信息
        font_info = self._analyze_fonts(page)
        
        # 检测页眉页脚
        layout_info = self._detect_layout(page)
        
        return {
            'minio_path': minio_path,
            'thumbnail_path': thumbnail_path,
            'page_width': page_width,
            'page_height': page_height,
            'image_width': pix.width,
            'image_height': pix.height,
            'image_format': 'png',
            'image_size': len(img_data),
            'text_blocks_count': page_metadata['text_blocks_count'],
            'images_count': page_metadata['images_count'],
            'tables_count': page_metadata['tables_count'],
            'primary_font': font_info['primary_font'],
            'font_sizes': font_info['font_sizes'],
            'has_header': layout_info['has_header'],
            'has_footer': layout_info['has_footer'],
            'columns_count': layout_info['columns_count']
        }
    
    def _create_thumbnail(
        self,
        task_id: str,
        page_num: int,
        img_data: bytes
    ) -> str:
        """
        创建缩略图
        
        Args:
            task_id: 任务ID
            page_num: 页码（从0开始）
            img_data: 原始图片数据
            
        Returns:
            缩略图MinIO路径
        """
        try:
            # 使用PIL创建缩略图
            img = Image.open(BytesIO(img_data))
            img.thumbnail(self.thumbnail_size, Image.Resampling.LANCZOS)
            
            # 保存缩略图
            thumb_buffer = BytesIO()
            img.save(thumb_buffer, format='PNG', optimize=True)
            thumb_buffer.seek(0)
            
            # 上传到MinIO
            thumbnail_path = f"pdf-snapshots/{task_id}/thumbnails/page_{page_num + 1:04d}_thumb.png"
            self.storage.upload_file(
                object_name=thumbnail_path,
                file_data=thumb_buffer,
                content_type="image/png",
                metadata={
                    "task_id": task_id,
                    "page_num": str(page_num + 1),
                    "type": "thumbnail"
                }
            )
            
            return thumbnail_path
            
        except Exception as e:
            logger.warning(f"Failed to create thumbnail for page {page_num + 1}: {str(e)}")
            return None
    
    def _analyze_page_content(self, page: fitz.Page) -> Dict[str, Any]:
        """
        分析页面内容
        
        Args:
            page: PDF页面对象
            
        Returns:
            页面内容统计
        """
        text_blocks = page.get_text("blocks")
        images = page.get_images()
        
        # 简单的表格检测（基于文本块的对齐）
        tables_count = self._detect_tables(text_blocks)
        
        return {
            'text_blocks_count': len(text_blocks),
            'images_count': len(images),
            'tables_count': tables_count
        }
    
    def _analyze_fonts(self, page: fitz.Page) -> Dict[str, Any]:
        """
        分析页面字体信息
        
        Args:
            page: PDF页面对象
            
        Returns:
            字体信息
        """
        text_dict = page.get_text("dict")
        font_counter = Counter()
        size_counter = Counter()
        
        # 遍历所有文本块
        for block in text_dict.get("blocks", []):
            if block.get("type") == 0:  # 文本块
                for line in block.get("lines", []):
                    for span in line.get("spans", []):
                        font = span.get("font", "unknown")
                        size = round(span.get("size", 0))
                        
                        if font:
                            font_counter[font] += 1
                        if size > 0:
                            size_counter[size] += 1
        
        # 获取最常用的字体
        primary_font = None
        if font_counter:
            primary_font = font_counter.most_common(1)[0][0]
        
        # 字体大小分布
        font_sizes = dict(size_counter) if size_counter else None
        
        return {
            'primary_font': primary_font,
            'font_sizes': font_sizes
        }
    
    def _detect_layout(self, page: fitz.Page) -> Dict[str, Any]:
        """
        检测页面布局（页眉、页脚、分栏等）
        
        Args:
            page: PDF页面对象
            
        Returns:
            布局信息
        """
        page_height = page.rect.height
        text_blocks = page.get_text("blocks")
        
        has_header = False
        has_footer = False
        columns_count = 1
        
        if text_blocks:
            # 检测页眉（页面顶部10%区域）
            header_threshold = page_height * 0.1
            header_blocks = [b for b in text_blocks if b[1] < header_threshold]
            has_header = len(header_blocks) > 0
            
            # 检测页脚（页面底部10%区域）
            footer_threshold = page_height * 0.9
            footer_blocks = [b for b in text_blocks if b[1] > footer_threshold]
            has_footer = len(footer_blocks) > 0
            
            # 简单的分栏检测（基于文本块的X坐标分布）
            x_positions = [b[0] for b in text_blocks if header_threshold < b[1] < footer_threshold]
            if x_positions:
                x_positions.sort()
                # 如果文本块在X轴上有明显的分组，可能是多栏
                gaps = []
                for i in range(1, len(x_positions)):
                    gap = x_positions[i] - x_positions[i-1]
                    if gap > page.rect.width * 0.1:  # 间隙大于页面宽度的10%
                        gaps.append(gap)
                
                if gaps and len(gaps) >= 1:
                    columns_count = min(len(gaps) + 1, 3)  # 最多检测3栏
        
        return {
            'has_header': has_header,
            'has_footer': has_footer,
            'columns_count': columns_count
        }
    
    def _detect_tables(self, text_blocks: List) -> int:
        """
        简单的表格检测
        
        Args:
            text_blocks: 文本块列表
            
        Returns:
            检测到的表格数量
        """
        # 这是一个简化的表格检测
        # 实际应用中可能需要更复杂的算法
        tables_count = 0
        
        if len(text_blocks) > 3:
            # 检查文本块是否在垂直和水平方向上对齐
            y_positions = [b[1] for b in text_blocks]
            x_positions = [b[0] for b in text_blocks]
            
            # 统计相同Y坐标的块数
            y_counter = Counter([round(y, 1) for y in y_positions])
            
            # 如果有多个块在同一行，可能是表格
            for count in y_counter.values():
                if count >= 3:  # 至少3个块在同一行
                    tables_count = 1
                    break
        
        return tables_count
    
    def get_page_snapshot(self, task_id: str, page_num: int) -> Optional[PDFPageSnapshot]:
        """
        获取指定页面的快照信息
        
        Args:
            task_id: 任务ID
            page_num: 页码（从1开始）
            
        Returns:
            页面快照记录
        """
        db = SessionLocal()
        try:
            snapshot = db.query(PDFPageSnapshot).filter(
                PDFPageSnapshot.task_id == task_id,
                PDFPageSnapshot.page_num == page_num
            ).first()
            return snapshot
        finally:
            db.close()
    
    def get_all_snapshots(self, task_id: str) -> List[PDFPageSnapshot]:
        """
        获取任务的所有页面快照
        
        Args:
            task_id: 任务ID
            
        Returns:
            页面快照列表
        """
        db = SessionLocal()
        try:
            snapshots = db.query(PDFPageSnapshot).filter(
                PDFPageSnapshot.task_id == task_id
            ).order_by(PDFPageSnapshot.page_num).all()
            return snapshots
        finally:
            db.close()