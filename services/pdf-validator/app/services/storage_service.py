"""
对象存储服务 - MinIO/S3客户端封装
"""
import logging
from typing import Any, BinaryIO, Optional
from pathlib import Path
from io import BytesIO

from minio import Minio
from minio.error import S3Error

from app.core.config import settings
from app.utils.exceptions import StorageError, FileNotFoundError

logger = logging.getLogger(__name__)


class StorageService:
    """对象存储服务客户端"""
    
    def __init__(self):
        """初始化MinIO客户端"""
        self.client = Minio(
            settings.MINIO_ENDPOINT,
            access_key=settings.MINIO_ACCESS_KEY,
            secret_key=settings.MINIO_SECRET_KEY,
            secure=settings.MINIO_SECURE
        )
        self.bucket_name = settings.MINIO_BUCKET_NAME
        self._bucket_checked = False
    
    def _ensure_bucket_exists(self):
        """确保存储桶存在（延迟执行）"""
        if self._bucket_checked:
            return
            
        try:
            if not self.client.bucket_exists(self.bucket_name):
                self.client.make_bucket(self.bucket_name)
                logger.info(f"Created bucket: {self.bucket_name}")
            else:
                logger.debug(f"Bucket exists: {self.bucket_name}")
            self._bucket_checked = True
        except S3Error as e:
            logger.warning(f"Failed to create/check bucket: {e}")
            # 不抛出异常，允许服务继续运行
        except Exception as e:
            logger.warning(f"MinIO connection error during bucket check: {e}")
            # 不抛出异常，允许服务继续运行
    
    def upload_file(
        self,
        object_name: str,
        file_data: BinaryIO,
        content_type: str = "application/octet-stream",
        metadata: Optional[dict] = None
    ) -> str:
        """
        上传文件到对象存储
        
        Args:
            object_name: 对象名称（路径）
            file_data: 文件数据流
            content_type: 内容类型
            metadata: 元数据
            
        Returns:
            str: 对象名称
        """
        # 延迟检查bucket
        self._ensure_bucket_exists()
        
        try:
            # 获取文件大小
            if hasattr(file_data, 'seek') and hasattr(file_data, 'tell'):
                file_data.seek(0, 2)  # 移动到文件末尾
                size = file_data.tell()
                file_data.seek(0)  # 重置到开头
            else:
                size = len(file_data.getvalue()) if hasattr(file_data, 'getvalue') else -1
            
            self.client.put_object(
                self.bucket_name,
                object_name,
                file_data,
                length=size,
                content_type=content_type,
                metadata=metadata or {}
            )
            
            # logger.info(f"Uploaded file: {object_name}")
            return object_name
            
        except S3Error as e:
            raise StorageError(f"Failed to upload file {object_name}: {e}")
    
    def download_file(self, object_name: str, local_path: Optional[str] = None) -> BytesIO:
        """
        下载文件从对象存储
        
        Args:
            object_name: 对象名称（路径）
            local_path: 本地保存路径（可选）
            
        Returns:
            BytesIO: 文件数据流
        """
        try:
            response = self.client.get_object(self.bucket_name, object_name)
            file_data = BytesIO(response.data)
            
            # 如果指定了本地路径，保存到本地
            if local_path:
                Path(local_path).parent.mkdir(parents=True, exist_ok=True)
                with open(local_path, 'wb') as f:
                    file_data.seek(0)
                    f.write(file_data.read())
                    file_data.seek(0)
            
            logger.info(f"Downloaded file: {object_name}")
            return file_data
            
        except S3Error as e:
            if e.code == 'NoSuchKey':
                raise FileNotFoundError(f"File not found: {object_name}")
            raise StorageError(f"Failed to download file {object_name}: {e}")
        finally:
            if 'response' in locals():
                response.close()
    
    def delete_file(self, object_name: str):
        """
        删除文件
        
        Args:
            object_name: 对象名称（路径）
        """
        try:
            self.client.remove_object(self.bucket_name, object_name)
            logger.info(f"Deleted file: {object_name}")
        except S3Error as e:
            if e.code != 'NoSuchKey':  # 忽略文件不存在的错误
                raise StorageError(f"Failed to delete file {object_name}: {e}")
    
    def file_exists(self, object_name: str) -> bool:
        """
        检查文件是否存在
        
        Args:
            object_name: 对象名称（路径）
            
        Returns:
            bool: 文件是否存在
        """
        try:
            self.client.stat_object(self.bucket_name, object_name)
            return True
        except S3Error as e:
            if e.code == 'NoSuchKey':
                return False
            raise StorageError(f"Failed to check file existence {object_name}: {e}")
    
    def get_file_info(self, object_name: str) -> dict:
        """
        获取文件信息
        
        Args:
            object_name: 对象名称（路径）
            
        Returns:
            dict: 文件信息
        """
        try:
            stat = self.client.stat_object(self.bucket_name, object_name)
            return {
                'size': stat.size,
                'last_modified': stat.last_modified,
                'etag': stat.etag,
                'content_type': stat.content_type,
                'metadata': stat.metadata
            }
        except S3Error as e:
            if e.code == 'NoSuchKey':
                raise FileNotFoundError(f"File not found: {object_name}")
            raise StorageError(f"Failed to get file info {object_name}: {e}")
    
    def list_files(self, prefix: str = "") -> list:
        """
        列出文件
        
        Args:
            prefix: 前缀过滤
            
        Returns:
            list: 文件列表
        """
        try:
            objects = self.client.list_objects(
                self.bucket_name,
                prefix=prefix,
                recursive=True
            )
            return [obj.object_name for obj in objects]
        except S3Error as e:
            raise StorageError(f"Failed to list files: {e}")