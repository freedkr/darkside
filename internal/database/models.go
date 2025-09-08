package database

import (
	"time"

	"gorm.io/datatypes"
)

// TaskRecord 任务记录
type TaskRecord struct {
	ID            string         `json:"id" gorm:"primaryKey;type:uuid"`
	Type          string         `json:"type" gorm:"type:varchar(50);not null"`
	Status        string         `json:"status" gorm:"type:varchar(50);not null;index"`
	Priority      int            `json:"priority" gorm:"not null;default:0"`
	InputPath     string         `json:"input_path" gorm:"type:text;not null"`
	OutputPath    string         `json:"output_path" gorm:"type:text;not null"`
	PDFPath       string         `json:"pdf_path,omitempty" gorm:"type:text"`
	Config        datatypes.JSON `json:"config" gorm:"type:jsonb"`           // JSON格式的配置
	Result        datatypes.JSON `json:"result,omitempty" gorm:"type:jsonb"` // JSON格式的结果
	ErrorMsg      string         `json:"error_msg,omitempty" gorm:"type:text"`
	RetryCount    int            `json:"retry_count" gorm:"not null;default:0"`
	CreatedAt     time.Time      `json:"created_at" gorm:"not null;default:now()"`
	UpdatedAt     time.Time      `json:"updated_at" gorm:"not null;default:now()"`
	UploadBatchID string         `json:"upload_batch_id,omitempty" gorm:"type:uuid"`
	ProcessedAt   *time.Time     `json:"processed_at,omitempty"`
	CreatedBy     string         `json:"created_by,omitempty" gorm:"type:varchar(255)"`
	ProcessingLog string         `json:"processing_log,omitempty" gorm:"type:text"`
}

// FileRecord 文件记录
type FileRecord struct {
	ID           string    `json:"id" gorm:"primaryKey;type:uuid"`
	OriginalName string    `json:"original_name" gorm:"type:varchar(255);not null"`
	StoragePath  string    `json:"storage_path" gorm:"type:text;not null"`
	FileSize     int64     `json:"file_size" gorm:"not null"`
	ContentType  string    `json:"content_type" gorm:"type:varchar(255);not null"`
	MD5Hash      string    `json:"md5_hash" gorm:"type:varchar(32);not null"`
	CreatedAt    time.Time `json:"created_at" gorm:"not null;default:now()"`
	TaskID       string    `json:"task_id" gorm:"type:uuid;index"`
}

// ProcessingStats 处理统计
type ProcessingStats struct {
	ID               string    `gorm:"primarykey;type:uuid;default:uuid_generate_v4()"`
	TaskID           string    `json:"task_id" gorm:"type:uuid;not null;index"`
	TotalRecords     int       `json:"total_records" gorm:"not null;default:0"`
	ProcessedRecords int       `json:"processed_records" gorm:"not null;default:0"`
	SkippedRecords   int       `json:"skipped_records" gorm:"not null;default:0"`
	ErrorRecords     int       `json:"error_records" gorm:"not null;default:0"`
	ProcessingTimeMs int64     `json:"processing_time_ms" gorm:"not null;default:0"`
	MemoryUsageMB    float64   `json:"memory_usage_mb" gorm:"type:decimal(10,2);not null;default:0"`
	CreatedAt        time.Time `json:"created_at" gorm:"not null;default:now()"`
}

// TableName 指定表名和schema
func (TaskRecord) TableName() string {
	return "moonshot.task_records"
}

// TableName 指定表名和schema
func (FileRecord) TableName() string {
	return "moonshot.file_records"
}

// TableName 指定表名和schema
func (ProcessingStats) TableName() string {
	return "moonshot.processing_stats"
}
