package database

import (
	"time"

	"github.com/lib/pq"
)

// PDFResult 对应于数据库中的 pdf_results 表，用于存储PDF解析的原始结果
type PDFResult struct {
	ID          uint            `gorm:"primarykey;autoIncrement"`
	TaskID      string          `gorm:"type:uuid;index"`
	Page        int             `gorm:"not null"`
	Text        string          `gorm:"type:text"`
	BoundingBox pq.Float64Array `gorm:"type:float[]"`
	CreatedAt   time.Time       `gorm:"autoCreateTime"`
}

func (PDFResult) TableName() string {
	return "moonshot.pdf_results"
}
