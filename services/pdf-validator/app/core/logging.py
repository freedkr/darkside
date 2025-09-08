"""
Logging configuration for PDF Validator Service
"""
import logging
import sys
from typing import Dict, Any

def setup_logging(level: str = "INFO") -> Dict[str, Any]:
    """
    Setup logging configuration for the application
    
    Args:
        level: Logging level (DEBUG, INFO, WARNING, ERROR)
        
    Returns:
        Dictionary with logging configuration
    """
    # Configure root logger
    logging.basicConfig(
        level=getattr(logging, level.upper()),
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S',
        stream=sys.stdout
    )
    
    # Configure specific loggers
    loggers = {
        'uvicorn': logging.getLogger('uvicorn'),
        'uvicorn.access': logging.getLogger('uvicorn.access'),
        'fastapi': logging.getLogger('fastapi'),
        'sqlalchemy': logging.getLogger('sqlalchemy.engine'),
        'celery': logging.getLogger('celery'),
    }
    
    # Set levels for specific loggers
    loggers['uvicorn'].setLevel(logging.INFO)
    loggers['uvicorn.access'].setLevel(logging.INFO)
    loggers['fastapi'].setLevel(logging.INFO)
    loggers['sqlalchemy'].setLevel(logging.WARNING)  # Reduce SQL noise
    loggers['celery'].setLevel(logging.INFO)
    
    logger = logging.getLogger(__name__)
    logger.info(f"Logging configured with level: {level}")
    
    return {
        'version': 1,
        'disable_existing_loggers': False,
        'formatters': {
            'default': {
                'format': '%(asctime)s - %(name)s - %(levelname)s - %(message)s',
            },
        },
        'handlers': {
            'default': {
                'formatter': 'default',
                'class': 'logging.StreamHandler',
                'stream': 'ext://sys.stdout',
            },
        },
        'root': {
            'level': level.upper(),
            'handlers': ['default'],
        },
    }


def get_logger(name: str) -> logging.Logger:
    """
    Get a logger with the specified name
    
    Args:
        name: Logger name
        
    Returns:
        Logger instance
    """
    return logging.getLogger(name)