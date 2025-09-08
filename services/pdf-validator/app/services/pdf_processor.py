"""
PDF Processing Service - 核心PDF处理逻辑
"""
import logging
from datetime import datetime
from pathlib import Path
from typing import Dict, Any, List, Optional, Tuple

import fitz  # PyMuPDF
from prometheus_client import Counter, Histogram, Gauge

from app.core.config import settings
from app.utils.exceptions import PDFValidationError, FileNotFoundError


# Prometheus指标
pdf_processed_total = Counter('pdf_processed_total', 'PDF处理总数', ['validation_type', 'status'])
pdf_processing_duration = Histogram('pdf_processing_duration_seconds', 'PDF处理时间', ['validation_type'])
pdf_page_count = Gauge('pdf_current_page_count', '当前处理PDF页数')
pdf_file_size = Gauge('pdf_current_file_size_bytes', '当前处理PDF文件大小')

logger = logging.getLogger(__name__)


class PDFProcessor:
    """PDF处理器 - 使用PyMuPDF进行PDF验证和内容提取"""
    
    def __init__(self):
        self.max_file_size = settings.PDF_MAX_FILE_SIZE
        self.processing_timeout = settings.PDF_PROCESSING_TIMEOUT
        
    def validate_pdf(
        self, 
        file_path: str, 
        validation_type: str = "standard",
        metadata: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """
        验证PDF文件并提取内容
        
        Args:
            file_path: PDF文件路径
            validation_type: 验证类型 (standard, enhanced, strict)
            metadata: 附加元数据
            
        Returns:
            Dict[str, Any]: 验证结果
        """
        start_time = datetime.utcnow()
        file_path_obj = Path(file_path)
        
        try:
            # 1. 基础文件检查
            self._validate_file_basic(file_path_obj)
            
            # 2. 使用PyMuPDF打开PDF
            doc = self._open_pdf_document(file_path)
            
            # 3. 根据验证类型执行不同级别的验证
            if validation_type == "standard":
                result = self._standard_validation(doc, file_path_obj)
            elif validation_type == "enhanced":
                result = self._enhanced_validation(doc, file_path_obj)
            elif validation_type == "strict":
                result = self._strict_validation(doc, file_path_obj)
            else:
                raise PDFValidationError(f"不支持的验证类型: {validation_type}")
            
            # 4. 添加处理时间和文件信息
            processing_time = (datetime.utcnow() - start_time).total_seconds()
            result.update({
                "processing_time": processing_time,
                "file_size": file_path_obj.stat().st_size,
                "file_name": file_path_obj.name,
                "validation_type": validation_type,
                "processed_at": datetime.utcnow().isoformat()
            })
            
            # 5. 更新指标
            pdf_processed_total.labels(validation_type=validation_type, status="success").inc()
            pdf_processing_duration.labels(validation_type=validation_type).observe(processing_time)
            
            doc.close()
            
            return result
            
        except Exception as e:
            processing_time = (datetime.utcnow() - start_time).total_seconds()
            pdf_processed_total.labels(validation_type=validation_type, status="failed").inc()
            
            logger.error(f"PDF验证失败 - 文件: {file_path}, 错误: {str(e)}")
            
            return {
                "is_valid": False,
                "processing_time": processing_time,
                "errors": [f"PDF处理失败: {str(e)}"],
                "warnings": [],
                "text": "",
                "metadata": {},
                "page_count": 0,
                "file_size": file_path_obj.stat().st_size if file_path_obj.exists() else 0,
                "file_name": file_path_obj.name if file_path_obj.exists() else "",
                "validation_type": validation_type,
                "processed_at": datetime.utcnow().isoformat()
            }
    
    def _validate_file_basic(self, file_path: Path) -> None:
        """基础文件验证"""
        if not file_path.exists():
            raise FileNotFoundError(f"文件不存在: {file_path}")
            
        if not file_path.is_file():
            raise PDFValidationError(f"不是有效文件: {file_path}")
            
        file_size = file_path.stat().st_size
        if file_size > self.max_file_size:
            raise PDFValidationError(
                f"文件过大: {file_size} bytes, 最大允许: {self.max_file_size} bytes"
            )
            
        if file_size == 0:
            raise PDFValidationError("文件为空")
    
    def _open_pdf_document(self, file_path: str) -> fitz.Document:
        """打开PDF文档"""
        try:
            doc = fitz.open(file_path)
            if doc.is_encrypted and not doc.authenticate(""):
                raise PDFValidationError("PDF文档被加密，无法访问")
            return doc
        except fitz.FileDataError as e:
            raise PDFValidationError(f"PDF文件损坏或格式不正确: {str(e)}")
        except Exception as e:
            raise PDFValidationError(f"无法打开PDF文档: {str(e)}")
    
    def _standard_validation(
        self, 
        doc: fitz.Document, 
        file_path: Path
    ) -> Dict[str, Any]:
        """标准验证 - 基础内容提取"""
        result = {
            "is_valid": True,
            "errors": [],
            "warnings": [],
            "page_count": len(doc),
            "text": "",
            "metadata": {}
        }
        
        # 更新指标
        pdf_page_count.set(len(doc))
        pdf_file_size.set(file_path.stat().st_size)
        
        try:
            # 1. 提取文档元数据
            result["metadata"] = dict(doc.metadata)
            
            # 2. 提取文本内容
            text_content = []
            for page_num in range(len(doc)):
                page = doc.load_page(page_num)
                page_text = page.get_text()
                text_content.append(page_text)
            
            result["text"] = "\\n".join(text_content)
            
            # 3. 基础验证检查
            if len(doc) == 0:
                result["warnings"].append("PDF文档无页面")
            
            if not result["text"].strip():
                result["warnings"].append("PDF文档无文本内容")
            
            return result
            
        except Exception as e:
            result["is_valid"] = False
            result["errors"].append(f"标准验证失败: {str(e)}")
            return result
    
    def _enhanced_validation(
        self, 
        doc: fitz.Document, 
        file_path: Path
    ) -> Dict[str, Any]:
        """增强验证 - 深度内容分析"""
        # 先执行标准验证
        result = self._standard_validation(doc, file_path)
        
        try:
            # 1. 页面级别分析
            pages_info = []
            for page_num in range(len(doc)):
                page = doc.load_page(page_num)
                page_info = {
                    "page_number": page_num + 1,
                    "width": page.rect.width,
                    "height": page.rect.height,
                    "rotation": page.rotation,
                    "text_length": len(page.get_text()),
                    "has_images": len(page.get_images()) > 0,
                    "has_links": len(page.get_links()) > 0
                }
                
                # 检测表格
                if settings.PDF_EXTRACT_TABLES:
                    try:
                        tables = page.find_tables()
                        page_info["table_count"] = len(tables)
                    except:
                        page_info["table_count"] = 0
                
                pages_info.append(page_info)
            
            result["pages_info"] = pages_info
            
            # 2. 图像提取（如果启用）
            if settings.PDF_EXTRACT_IMAGES:
                result["images_info"] = self._extract_images_info(doc)
            
            # 3. 文档结构分析
            result["document_structure"] = self._analyze_document_structure(doc)
            
            # 4. 质量检查
            quality_checks = self._perform_quality_checks(doc, result)
            result["quality_checks"] = quality_checks
            
            return result
            
        except Exception as e:
            result["errors"].append(f"增强验证失败: {str(e)}")
            return result
    
    def _strict_validation(
        self, 
        doc: fitz.Document, 
        file_path: Path
    ) -> Dict[str, Any]:
        """严格验证 - 全面检查"""
        # 先执行增强验证
        result = self._enhanced_validation(doc, file_path)
        
        try:
            # 1. PDF/A合规性检查
            result["pdf_a_compliance"] = self._check_pdf_a_compliance(doc)
            
            # 2. 可访问性检查
            result["accessibility_check"] = self._check_accessibility(doc)
            
            # 3. 完整性检查
            result["integrity_check"] = self._check_document_integrity(doc)
            
            # 4. 严格模式错误检查
            strict_errors = []
            
            if not result["text"].strip():
                strict_errors.append("严格模式: PDF必须包含可提取的文本")
            
            if result["page_count"] == 0:
                strict_errors.append("严格模式: PDF必须包含至少一页")
            
            # 检查文档元数据
            metadata = result.get("metadata", {})
            required_metadata = ["Title", "Author", "Creator"]
            for field in required_metadata:
                if not metadata.get(field):
                    result["warnings"].append(f"严格模式建议: 缺少元数据字段 {field}")
            
            if strict_errors:
                result["errors"].extend(strict_errors)
                result["is_valid"] = False
            
            return result
            
        except Exception as e:
            result["errors"].append(f"严格验证失败: {str(e)}")
            return result
    
    def _extract_images_info(self, doc: fitz.Document) -> List[Dict[str, Any]]:
        """提取图像信息"""
        images_info = []
        
        for page_num in range(len(doc)):
            page = doc.load_page(page_num)
            image_list = page.get_images()
            
            for img_index, img in enumerate(image_list):
                try:
                    xref = img[0]
                    pix = fitz.Pixmap(doc, xref)
                    
                    img_info = {
                        "page": page_num + 1,
                        "index": img_index,
                        "xref": xref,
                        "width": pix.width,
                        "height": pix.height,
                        "colorspace": pix.colorspace.name if pix.colorspace else "unknown",
                        "alpha": pix.alpha,
                        "size": pix.samples_len
                    }
                    
                    images_info.append(img_info)
                    pix = None  # 释放内存
                    
                except Exception as e:
                    logger.warning(f"提取图像信息失败 - 页面{page_num + 1}, 图像{img_index}: {str(e)}")
        
        return images_info
    
    def _analyze_document_structure(self, doc: fitz.Document) -> Dict[str, Any]:
        """分析文档结构"""
        try:
            toc = doc.get_toc()  # 获取目录
            
            structure_info = {
                "has_toc": len(toc) > 0,
                "toc_entries": len(toc),
                "max_toc_level": max([entry[0] for entry in toc]) if toc else 0,
                "has_bookmarks": len(doc.get_page_labels()) > 0,
                "form_fields": len(doc.get_page_fonts(0)) if len(doc) > 0 else 0
            }
            
            return structure_info
        except Exception as e:
            logger.warning(f"文档结构分析失败: {str(e)}")
            return {"analysis_failed": str(e)}
    
    def _perform_quality_checks(
        self, 
        doc: fitz.Document, 
        result: Dict[str, Any]
    ) -> Dict[str, Any]:
        """执行质量检查"""
        checks = {
            "text_to_page_ratio": 0.0,
            "average_page_size": 0.0,
            "consistent_page_sizes": True,
            "has_metadata": bool(result.get("metadata", {})),
            "text_extraction_success_rate": 0.0
        }
        
        try:
            # 文本与页面比例
            total_text_length = len(result.get("text", ""))
            page_count = result.get("page_count", 1)
            checks["text_to_page_ratio"] = total_text_length / page_count if page_count > 0 else 0
            
            # 页面大小一致性检查
            if "pages_info" in result:
                page_sizes = [(p["width"], p["height"]) for p in result["pages_info"]]
                checks["consistent_page_sizes"] = len(set(page_sizes)) <= 2  # 允许轻微差异
                
                if page_sizes:
                    avg_area = sum(w * h for w, h in page_sizes) / len(page_sizes)
                    checks["average_page_size"] = avg_area
            
            # 文本提取成功率
            pages_with_text = sum(1 for p in result.get("pages_info", []) if p.get("text_length", 0) > 0)
            checks["text_extraction_success_rate"] = pages_with_text / page_count if page_count > 0 else 0
            
        except Exception as e:
            logger.warning(f"质量检查失败: {str(e)}")
            checks["quality_check_failed"] = str(e)
        
        return checks
    
    def _check_pdf_a_compliance(self, doc: fitz.Document) -> Dict[str, Any]:
        """检查PDF/A合规性"""
        # 简化的PDF/A合规性检查
        compliance = {
            "is_compliant": False,
            "version": None,
            "issues": []
        }
        
        try:
            # 检查元数据中的PDF/A标识
            metadata = dict(doc.metadata)
            
            # 检查字体嵌入
            fonts_embedded = True
            for page_num in range(min(5, len(doc))):  # 检查前5页
                page = doc.load_page(page_num)
                fonts = page.get_fonts()
                for font in fonts:
                    if not font[1]:  # 字体未嵌入
                        fonts_embedded = False
                        compliance["issues"].append(f"页面{page_num + 1}存在未嵌入字体")
            
            compliance["fonts_embedded"] = fonts_embedded
            
        except Exception as e:
            compliance["issues"].append(f"PDF/A合规性检查失败: {str(e)}")
        
        return compliance
    
    def _check_accessibility(self, doc: fitz.Document) -> Dict[str, Any]:
        """检查可访问性"""
        accessibility = {
            "has_structure": False,
            "has_alt_text": False,
            "reading_order_defined": False,
            "issues": []
        }
        
        try:
            # 检查是否有结构化标签
            # 注意: PyMuPDF对可访问性检查的支持有限
            
            # 检查目录结构
            toc = doc.get_toc()
            accessibility["has_structure"] = len(toc) > 0
            
            if not accessibility["has_structure"]:
                accessibility["issues"].append("缺少文档结构/目录")
            
        except Exception as e:
            accessibility["issues"].append(f"可访问性检查失败: {str(e)}")
        
        return accessibility
    
    def _check_document_integrity(self, doc: fitz.Document) -> Dict[str, Any]:
        """检查文档完整性"""
        integrity = {
            "is_complete": True,
            "all_pages_accessible": True,
            "no_corruption": True,
            "issues": []
        }
        
        try:
            # 检查所有页面是否可访问
            for page_num in range(len(doc)):
                try:
                    page = doc.load_page(page_num)
                    # 尝试获取页面内容以验证完整性
                    page.get_text()
                except Exception as e:
                    integrity["all_pages_accessible"] = False
                    integrity["issues"].append(f"页面{page_num + 1}访问失败: {str(e)}")
            
            integrity["is_complete"] = len(integrity["issues"]) == 0
            
        except Exception as e:
            integrity["no_corruption"] = False
            integrity["issues"].append(f"文档完整性检查失败: {str(e)}")
        
        return integrity