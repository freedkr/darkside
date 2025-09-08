"""
API Routes Package
"""
from .health import router as health
from .validation import router as validation
from .blocks import router as blocks

__all__ = ["health", "validation", "blocks"]