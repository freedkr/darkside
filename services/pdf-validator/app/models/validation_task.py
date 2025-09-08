"""
PDF Validation Task Models - 数据库模型定义
"""
from datetime import datetime
from enum import Enum
from sqlalchemy import Column, Integer, String, Text, DateTime, Boolean, JSON, Float
from sqlalchemy.dialects.postgresql import UUID
from app.core.database import Base


class TaskStatus(str, Enum):
    """任务状态枚举"""
    PENDING = "pending"
    PROCESSING = "processing"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class ValidationTask(Base):
    """PDF验证任务表"""
    __tablename__ = "pdf_validation_tasks"
    __table_args__ = {'schema': 'moonshot'}
    
    id = Column(Integer, primary_key=True, index=True)
    task_id = Column(String(50), unique=True, index=True, nullable=False)
    pdf_file_path = Column(String(500), nullable=False)
    validation_type = Column(String(50), nullable=False, default="standard")
    status = Column(String(20), nullable=False, default=TaskStatus.PENDING)
    
    # 时间戳
    created_at = Column(DateTime, default=datetime.utcnow)
    started_at = Column(DateTime, nullable=True)
    completed_at = Column(DateTime, nullable=True)
    
    # 任务元数据
    priority = Column(Integer, default=0)
    retry_count = Column(Integer, default=0)
    max_retries = Column(Integer, default=3)
    
    # 结果摘要
    result_summary = Column(JSON, nullable=True)
    error_message = Column(Text, nullable=True)
    
    # 请求来源
    requester_service = Column(String(100), nullable=True)
    correlation_id = Column(String(100), nullable=True)


class ValidationResult(Base):
    """PDF验证结果表"""
    __tablename__ = "pdf_validation_results"
    __table_args__ = {'schema': 'moonshot'}
    
    id = Column(Integer, primary_key=True, index=True)
    task_id = Column(String(50), index=True, nullable=False)
    pdf_path = Column(String(500), nullable=False)
    validation_type = Column(String(50), nullable=False)
    
    # 验证结果
    is_valid = Column(Boolean, nullable=False)
    page_count = Column(Integer, nullable=False, default=0)
    file_size = Column(Integer, nullable=False, default=0)
    processing_time = Column(Float, nullable=False, default=0.0)
    
    # 提取的内容
    extracted_text = Column(Text, nullable=True)
    extracted_metadata = Column(JSON, nullable=True)
    
    # 详细分析结果
    pages_info = Column(JSON, nullable=True)
    images_info = Column(JSON, nullable=True)
    document_structure = Column(JSON, nullable=True)
    quality_checks = Column(JSON, nullable=True)
    
    # 错误和警告
    errors = Column(JSON, nullable=True)
    warnings = Column(JSON, nullable=True)
    
    # 时间戳
    created_at = Column(DateTime, default=datetime.utcnow)


class PDFPageSnapshot(Base):
    """PDF页面快照表 - 存储每页的截图和元数据"""
    __tablename__ = "pdf_page_snapshots"
    __table_args__ = {'schema': 'moonshot'}
    
    id = Column(Integer, primary_key=True, index=True)
    task_id = Column(String(50), index=True, nullable=False)
    page_num = Column(Integer, nullable=False, index=True)
    
    # MinIO存储信息
    minio_path = Column(String(500), nullable=False)  # MinIO中的文件路径
    thumbnail_path = Column(String(500), nullable=True)  # 缩略图路径
    
    # 页面尺寸信息
    page_width = Column(Float, nullable=False)  # 页面宽度（点）
    page_height = Column(Float, nullable=False)  # 页面高度（点）
    dpi = Column(Integer, default=150)  # 截图DPI
    
    # 图片信息
    image_width = Column(Integer, nullable=False)  # 截图宽度（像素）
    image_height = Column(Integer, nullable=False)  # 截图高度（像素）
    image_format = Column(String(10), default='png')  # 图片格式
    image_size = Column(Integer, nullable=False)  # 文件大小（字节）
    
    # 页面内容元数据
    text_blocks_count = Column(Integer, default=0)  # 文本块数量
    images_count = Column(Integer, default=0)  # 图片数量
    tables_count = Column(Integer, default=0)  # 表格数量
    
    # 主要字体信息
    primary_font = Column(String(100), nullable=True)  # 主要字体
    font_sizes = Column(JSON, nullable=True)  # 字体大小分布 {"12": 10, "14": 5, ...}
    
    # 页面布局信息
    has_header = Column(Boolean, default=False)  # 是否有页眉
    has_footer = Column(Boolean, default=False)  # 是否有页脚
    columns_count = Column(Integer, default=1)  # 栏数
    
    # OCR相关（预留）
    ocr_processed = Column(Boolean, default=False)  # 是否已OCR处理
    ocr_confidence = Column(Float, nullable=True)  # OCR置信度
    
    # 时间戳
    created_at = Column(DateTime, default=datetime.utcnow)
    

class PDFBlockInfo(Base):
    """PDF块信息表 - 存储详细的版式信息"""
    __tablename__ = "pdf_blocks"
    __table_args__ = {'schema': 'moonshot'}
    
    id = Column(Integer, primary_key=True, index=True)
    task_id = Column(String(50), index=True, nullable=False)
    page_num = Column(Integer, nullable=False, index=True)
    block_num = Column(Integer, nullable=False)
    
    # 文本内容
    text = Column(Text, nullable=False)
    
    # 位置信息 (bbox: [x0, y0, x1, y1])
    x0 = Column(Float, nullable=False)  # 左边界
    y0 = Column(Float, nullable=False)  # 上边界
    x1 = Column(Float, nullable=False)  # 右边界
    y1 = Column(Float, nullable=False)  # 下边界
    
    # 字体信息
    font = Column(String(100), nullable=True)      # 字体名称
    font_size = Column(Float, nullable=True)       # 字号
    font_flags = Column(Integer, nullable=True)    # 字体样式标志（粗体/斜体等）
    font_color = Column(Integer, nullable=True)    # 颜色值
    
    # 计算字段
    width = Column(Float, nullable=False)          # 宽度 (x1-x0)
    height = Column(Float, nullable=False)         # 高度 (y1-y0)
    center_x = Column(Float, nullable=False)       # 中心X坐标
    center_y = Column(Float, nullable=False)       # 中心Y坐标
    
    # 层级信息（通过版式分析得出）
    hierarchy_level = Column(Integer, nullable=True)      # 层级(1-4)
    indentation = Column(Float, nullable=True)            # 缩进值
    is_bold = Column(Boolean, default=False)              # 是否粗体
    is_italic = Column(Boolean, default=False)            # 是否斜体
    
    # 关联信息
    parent_block_id = Column(Integer, nullable=True)      # 父级块ID
    
    # 职业分类相关（如果识别出）
    occupation_code = Column(String(50), nullable=True, index=True)   # 职业编码
    occupation_name = Column(String(255), nullable=True)              # 职业名称
    confidence = Column(Float, nullable=True)                         # 置信度
    
    # 时间戳
    created_at = Column(DateTime, default=datetime.utcnow)
    
    def to_dict(self):
        """转换为字典格式"""
        return {
            "id": self.id,
            "task_id": self.task_id,
            "pdf_path": self.pdf_path,
            "validation_type": self.validation_type,
            "is_valid": self.is_valid,
            "page_count": self.page_count,
            "file_size": self.file_size,
            "processing_time": self.processing_time,
            "extracted_text": self.extracted_text,
            "extracted_metadata": self.extracted_metadata,
            "pages_info": self.pages_info,
            "images_info": self.images_info,
            "document_structure": self.document_structure,
            "quality_checks": self.quality_checks,
            "errors": self.errors,
            "warnings": self.warnings,
            "created_at": self.created_at.isoformat() if self.created_at else None
        }