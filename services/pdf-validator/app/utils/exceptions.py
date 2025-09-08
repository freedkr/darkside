"""
自定义异常类定义
"""


class PDFValidationError(Exception):
    """PDF验证异常"""
    pass


class FileNotFoundError(Exception):
    """文件未找到异常"""
    pass


class StorageError(Exception):
    """存储服务异常"""
    pass


class ProcessingError(Exception):
    """处理异常"""
    pass


class ConfigurationError(Exception):
    """配置异常"""
    pass


class ValidationError(Exception):
    """验证异常"""
    pass


class TimeoutError(Exception):
    """超时异常"""
    pass


class ServiceUnavailableError(Exception):
    """服务不可用异常"""
    pass