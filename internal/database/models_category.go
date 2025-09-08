package database

import (
	"context"
	"time"
)

// Category 对应于数据库中的 categories 表
type Category struct {
	ID         uint   `gorm:"primarykey;autoIncrement"`
	TaskID     string `gorm:"type:uuid;not null"`         // 任务ID，用于数据隔离
	Code       string `gorm:"type:varchar(255);not null"` // 职业编码
	Name       string `gorm:"type:varchar(255);not null"` // 职业名称
	Level      string `gorm:"type:varchar(50);not null"`  // 层级
	ParentCode string `gorm:"type:varchar(255);index"`    // 父级编码

	// 处理状态追踪字段
	Status          string `gorm:"type:varchar(50);not null;default:'excel_parsed';index"` // 处理状态
	DataSource      string `gorm:"type:varchar(50);not null;default:'excel'"`              // 数据源标识
	PDFInfo         string `gorm:"type:text"`                                              // PDF解析信息(JSON格式)
	LLMEnhancements string `gorm:"type:text"`                                              // LLM增强信息(JSON格式)

	// 版本管理字段
	UploadBatchID   string    `gorm:"type:uuid;not null"`                                // 上传批次ID
	UploadTimestamp time.Time `gorm:"type:timestamp;not null;default:CURRENT_TIMESTAMP"` // 上传时间戳
	IsCurrent       bool      `gorm:"type:boolean;not null;default:true;index"`          // 是否为当前版本

	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

func (Category) TableName() string {
	return "moonshot.categories"
}

// 处理状态常量
const (
	StatusExcelParsed = "excel_parsed" // Excel解析完成
	StatusPDFMerged   = "pdf_merged"   // PDF数据已合并
	StatusLLMCleaned  = "llm_cleaned"  // LLM第一轮清洗完成
	StatusLLMEnhanced = "llm_enhanced" // LLM第二轮增强完成
	StatusCompleted   = "completed"    // 全部处理完成
)

// 数据源常量
const (
	DataSourceExcel  = "excel"  // 来自Excel
	DataSourcePDF    = "pdf"    // 来自PDF
	DataSourceMerged = "merged" // 融合数据
)

// CategoryDBInterface 定义了操作 categories 表的接口
type CategoryDBInterface interface {
	// BatchInsertCategories 批量插入分类数据
	BatchInsertCategories(ctx context.Context, categories []*Category) error
	// GetCategoriesByTaskID 根据任务ID获取所有分类
	GetCategoriesByTaskID(ctx context.Context, taskID string) ([]*Category, error)
	// UpdateCategoryByCode 根据TaskID和Code更新分类信息
	UpdateCategoryByCode(ctx context.Context, taskID, code string, updates map[string]interface{}) error
	// BatchUpdateCategoriesByCode 批量按Code更新分类
	BatchUpdateCategoriesByCode(ctx context.Context, taskID string, updates []CategoryUpdate) error
	// GetCategoriesByStatus 根据任务ID和状态获取分类
	GetCategoriesByStatus(ctx context.Context, taskID string, status string) ([]*Category, error)

	// 版本管理相关方法
	// GetCurrentCategoriesByTaskID 获取任务的当前版本分类数据
	GetCurrentCategoriesByTaskID(ctx context.Context, taskID string) ([]*Category, error)
	// GetCategoriesByBatchID 根据批次ID获取分类数据
	GetCategoriesByBatchID(ctx context.Context, batchID string) ([]*Category, error)
	// BatchInsertCategoriesWithVersion 批量插入分类数据（支持版本管理）
	BatchInsertCategoriesWithVersion(ctx context.Context, taskID, batchID string, categories []*Category) error
	// MarkPreviousVersionsAsOld 将之前的版本标记为非当前版本
	MarkPreviousVersionsAsOld(ctx context.Context, taskID string) error
	// GetCategoryVersionHistory 获取分类的版本历史
	GetCategoryVersionHistory(ctx context.Context, taskID string) ([]*CategoryVersion, error)
}

// CategoryUpdate 用于批量更新的结构
type CategoryUpdate struct {
	Code    string                 `json:"code"`
	Updates map[string]interface{} `json:"updates"`
}

// CategoryVersion 版本历史信息
type CategoryVersion struct {
	UploadBatchID   string    `json:"upload_batch_id"`
	UploadTimestamp time.Time `json:"upload_timestamp"`
	RecordCount     int       `json:"record_count"`
	IsCurrent       bool      `json:"is_current"`
}
