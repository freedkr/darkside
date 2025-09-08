#!/usr/bin/env python3
"""
测试PDF页面快照功能
"""
import os
import sys
import json
import time
import logging
from pathlib import Path

# 添加项目路径
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from app.services.page_snapshot_service import PageSnapshotService
from app.services.enhanced_pdf_processor import EnhancedPDFProcessor
from app.core.database import SessionLocal, init_database
from app.models.validation_task import PDFPageSnapshot
import fitz  # PyMuPDF

# 配置日志
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


def create_test_pdf(file_path: str = "test_document.pdf"):
    """创建测试PDF文件"""
    doc = fitz.open()
    
    # 创建多个页面，每页不同的内容
    for i in range(3):
        page = doc.new_page(width=595, height=842)  # A4尺寸
        
        # 添加标题
        title_rect = fitz.Rect(50, 50, 545, 100)
        page.insert_textbox(
            title_rect,
            f"测试页面 {i + 1}",
            fontsize=24,
            fontname="helv-Bold",
            align=fitz.TEXT_ALIGN_CENTER
        )
        
        # 添加正文内容
        body_rect = fitz.Rect(50, 150, 545, 750)
        content = f"""
这是第 {i + 1} 页的测试内容。

职业编码示例：
1-01-01-01 - 测试职业1
2-02-02-02 - 测试职业2
3-03-03-03 - 测试职业3

这个页面包含了一些测试文本，用于验证PDF处理功能。
包括文本提取、块信息分析、页面快照生成等功能。

字体测试：
- 正常文本
- 不同大小的文本
- 多行文本内容

页面 {i + 1} / 3
        """
        page.insert_textbox(
            body_rect,
            content,
            fontsize=12,
            fontname="helv"
        )
        
        # 添加页脚
        footer_rect = fitz.Rect(50, 790, 545, 820)
        page.insert_textbox(
            footer_rect,
            f"© 2025 测试文档 - 第 {i + 1} 页",
            fontsize=10,
            fontname="helv",
            align=fitz.TEXT_ALIGN_CENTER
        )
    
    doc.save(file_path)
    doc.close()
    logger.info(f"创建测试PDF文件: {file_path}")
    return file_path


def test_page_snapshot_extraction():
    """测试页面快照提取功能"""
    print("\n" + "="*60)
    print("测试PDF页面快照功能")
    print("="*60)
    
    # 1. 创建测试PDF
    test_pdf_path = "test_document.pdf"
    create_test_pdf(test_pdf_path)
    
    # 2. 初始化服务
    snapshot_service = PageSnapshotService()
    task_id = f"test_task_{int(time.time())}"
    
    print(f"\n任务ID: {task_id}")
    print(f"PDF文件: {test_pdf_path}")
    
    # 3. 打开PDF并提取快照
    try:
        doc = fitz.open(test_pdf_path)
        print(f"PDF页数: {len(doc)}")
        
        # 提取页面快照
        print("\n开始提取页面快照...")
        snapshots = snapshot_service.extract_page_snapshots(
            task_id=task_id,
            doc=doc,
            dpi=150,
            save_thumbnail=True
        )
        
        print(f"\n成功提取 {len(snapshots)} 个页面快照")
        
        # 显示快照信息
        for i, snapshot in enumerate(snapshots):
            print(f"\n页面 {i + 1}:")
            print(f"  - MinIO路径: {snapshot['minio_path']}")
            print(f"  - 缩略图路径: {snapshot.get('thumbnail_path', 'N/A')}")
            print(f"  - 页面尺寸: {snapshot['page_width']:.1f} x {snapshot['page_height']:.1f}")
            print(f"  - 图片尺寸: {snapshot['image_width']} x {snapshot['image_height']}")
            print(f"  - 文件大小: {snapshot['image_size'] / 1024:.1f} KB")
            print(f"  - 文本块数: {snapshot['text_blocks_count']}")
            print(f"  - 主要字体: {snapshot.get('primary_font', 'N/A')}")
            
        doc.close()
        
    except Exception as e:
        logger.error(f"提取页面快照失败: {str(e)}")
        raise
    
    # 4. 从数据库查询快照记录
    print("\n\n从数据库查询快照记录...")
    db = SessionLocal()
    try:
        db_snapshots = db.query(PDFPageSnapshot).filter(
            PDFPageSnapshot.task_id == task_id
        ).order_by(PDFPageSnapshot.page_num).all()
        
        print(f"数据库中找到 {len(db_snapshots)} 条记录")
        
        for snapshot in db_snapshots:
            print(f"\n页面 {snapshot.page_num}:")
            print(f"  - ID: {snapshot.id}")
            print(f"  - MinIO路径: {snapshot.minio_path}")
            print(f"  - 有缩略图: {'是' if snapshot.thumbnail_path else '否'}")
            print(f"  - DPI: {snapshot.dpi}")
            print(f"  - 创建时间: {snapshot.created_at}")
            
    finally:
        db.close()
    
    # 5. 清理测试文件
    if os.path.exists(test_pdf_path):
        os.remove(test_pdf_path)
        print(f"\n已删除测试文件: {test_pdf_path}")
    
    print("\n" + "="*60)
    print("测试完成！")
    print("="*60)


def test_enhanced_processor_with_snapshots():
    """测试增强处理器的快照功能"""
    print("\n" + "="*60)
    print("测试增强处理器快照集成")
    print("="*60)
    
    # 1. 创建测试PDF
    test_pdf_path = "test_enhanced.pdf"
    create_test_pdf(test_pdf_path)
    
    # 2. 初始化处理器
    processor = EnhancedPDFProcessor()
    task_id = f"enhanced_test_{int(time.time())}"
    
    print(f"\n任务ID: {task_id}")
    print(f"PDF文件: {test_pdf_path}")
    
    # 3. 处理PDF（包含快照提取）
    try:
        print("\n开始处理PDF...")
        result = processor.process_pdf_with_blocks(
            task_id=task_id,
            file_path=test_pdf_path,
            save_to_db=True,
            extract_snapshots=True
        )
        
        print("\n处理结果:")
        print(f"  - 有效: {result['is_valid']}")
        print(f"  - 页数: {result['page_count']}")
        print(f"  - 总块数: {result['total_blocks']}")
        print(f"  - 处理时间: {result['processing_time']:.2f}秒")
        
        if 'page_snapshots' in result:
            print(f"\n页面快照信息:")
            print(f"  - 提取数量: {result['page_snapshots']['extracted']}")
            print(f"  - 总大小: {result['page_snapshots']['total_size'] / 1024 / 1024:.2f} MB")
            print(f"  - 有缩略图: {result['page_snapshots']['with_thumbnails']}")
        
    except Exception as e:
        logger.error(f"处理PDF失败: {str(e)}")
        raise
    
    # 4. 清理测试文件
    if os.path.exists(test_pdf_path):
        os.remove(test_pdf_path)
        print(f"\n已删除测试文件: {test_pdf_path}")
    
    print("\n" + "="*60)
    print("测试完成！")
    print("="*60)


def main():
    """主函数"""
    print("\nPDF页面快照功能测试")
    print("请选择测试项目:")
    print("1. 测试页面快照提取")
    print("2. 测试增强处理器集成")
    print("3. 运行所有测试")
    
    choice = input("\n请输入选项 (1-3): ").strip()
    
    if choice == "1":
        test_page_snapshot_extraction()
    elif choice == "2":
        test_enhanced_processor_with_snapshots()
    elif choice == "3":
        test_page_snapshot_extraction()
        print("\n" + "-"*60 + "\n")
        test_enhanced_processor_with_snapshots()
    else:
        print("无效选项")


if __name__ == "__main__":
    main()