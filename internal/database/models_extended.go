package database

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// ProcessingPipeline 处理流水线记录
type ProcessingPipeline struct {
	ID           string            `json:"id" db:"id"`
	UserTaskID   string            `json:"user_task_id" db:"user_task_id"` // 用户上传的原始任务ID
	Status       PipelineStatus    `json:"status" db:"status"`             // 整体流水线状态
	Steps        ProcessingSteps   `json:"steps" db:"steps"`               // 各步骤状态
	ExcelFileID  string            `json:"excel_file_id" db:"excel_file_id"`
	Results      ProcessingResults `json:"results" db:"results"` // 各阶段结果
	CreatedAt    time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at" db:"updated_at"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty" db:"completed_at"`
	ErrorMessage string            `json:"error_message,omitempty" db:"error_message"`
}

// PipelineStatus 流水线状态
type PipelineStatus string

const (
	PipelineStatusPending    PipelineStatus = "pending"
	PipelineStatusProcessing PipelineStatus = "processing"
	PipelineStatusCompleted  PipelineStatus = "completed"
	PipelineStatusFailed     PipelineStatus = "failed"
)

// ProcessingSteps 处理步骤状态
type ProcessingSteps struct {
	ExcelParsing     StepStatus `json:"excel_parsing"`
	PDFProcessing    StepStatus `json:"pdf_processing"`
	LLMBatchCleaning StepStatus `json:"llm_batch_cleaning"`
	DataMerging      StepStatus `json:"data_merging"`
	SemanticJudgment StepStatus `json:"semantic_judgment"`
}

// StepStatus 步骤状态
type StepStatus struct {
	Status      string     `json:"status"` // pending, processing, completed, failed
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	TaskID      string     `json:"task_id,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// ProcessingResults 处理结果路径
type ProcessingResults struct {
	ExcelResultPath    string `json:"excel_result_path,omitempty"`
	PDFResultPath      string `json:"pdf_result_path,omitempty"`
	LLMBatchResultPath string `json:"llm_batch_result_path,omitempty"`
	LLMFinalCodesPath  string `json:"llm_final_codes_path,omitempty"`
	MergedResultPath   string `json:"merged_result_path,omitempty"`
	SemanticResultPath string `json:"semantic_result_path,omitempty"`
	FinalOutputPath    string `json:"final_output_path,omitempty"`
}

// TaskRelation 任务关系表
type TaskRelation struct {
	ID           string    `json:"id" db:"id"`
	ParentTaskID string    `json:"parent_task_id" db:"parent_task_id"`
	ChildTaskID  string    `json:"child_task_id" db:"child_task_id"`
	Relationship string    `json:"relationship" db:"relationship"` // excel_to_pdf, pdf_to_llm, llm_to_merger, merger_to_semantic
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// ProcessingStatistics 处理统计
type ProcessingStatistics struct {
	ID                 string    `json:"id" db:"id"`
	PipelineID         string    `json:"pipeline_id" db:"pipeline_id"`
	TotalExcelItems    int       `json:"total_excel_items" db:"total_excel_items"`
	TotalPDFItems      int       `json:"total_pdf_items" db:"total_pdf_items"`
	LLMCleanedItems    int       `json:"llm_cleaned_items" db:"llm_cleaned_items"`
	MatchedItems       int       `json:"matched_items" db:"matched_items"`
	NameDiffItems      int       `json:"name_diff_items" db:"name_diff_items"`
	ExcelOnlyItems     int       `json:"excel_only_items" db:"excel_only_items"`
	LLMOnlyItems       int       `json:"llm_only_items" db:"llm_only_items"`
	AutoJudgedItems    int       `json:"auto_judged_items" db:"auto_judged_items"`
	ManualReviewItems  int       `json:"manual_review_items" db:"manual_review_items"`
	ProcessingDuration int64     `json:"processing_duration" db:"processing_duration"` // 总处理时长(秒)
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
}

// GORM扫描支持
func (ps *ProcessingSteps) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into ProcessingSteps", value)
	}

	return json.Unmarshal(bytes, ps)
}

func (ps ProcessingSteps) Value() (driver.Value, error) {
	return json.Marshal(ps)
}

func (pr *ProcessingResults) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into ProcessingResults", value)
	}

	return json.Unmarshal(bytes, pr)
}

func (pr ProcessingResults) Value() (driver.Value, error) {
	return json.Marshal(pr)
}

// DatabaseExtendedInterface 扩展的数据库接口
type DatabaseExtendedInterface interface {
	DatabaseInterface

	// 流水线管理
	CreatePipeline(ctx context.Context, pipeline *ProcessingPipeline) error
	GetPipeline(ctx context.Context, pipelineID string) (*ProcessingPipeline, error)
	UpdatePipeline(ctx context.Context, pipeline *ProcessingPipeline) error
	ListPipelines(ctx context.Context, limit, offset int) ([]*ProcessingPipeline, error)

	// 任务关系管理
	CreateTaskRelation(ctx context.Context, relation *TaskRelation) error
	GetTaskRelations(ctx context.Context, parentTaskID string) ([]*TaskRelation, error)
	GetTaskChildren(ctx context.Context, parentTaskID string) ([]string, error)

	// 统计数据管理
	CreateStatistics(ctx context.Context, stats *ProcessingStatistics) error
	GetStatistics(ctx context.Context, pipelineID string) (*ProcessingStatistics, error)
	UpdateStatistics(ctx context.Context, stats *ProcessingStatistics) error
}
