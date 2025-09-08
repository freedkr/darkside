package integration

import (
	"context"
	"time"

	"github.com/freedkr/moonshot/internal/model"
)

// PDFService PDF服务接口
type PDFService interface {
	ValidateAndExtract(ctx context.Context, request PDFValidationRequest) (PDFResult, error)
	GetTaskStatus(ctx context.Context, taskID string) (TaskStatus, error)
	GetOccupationCodes(ctx context.Context, taskID string) (PDFResult, error)
}

// LLMService LLM服务接口
type LLMService interface {
	CleanDataConcurrently(ctx context.Context, request LLMCleaningRequest) ([]CleanedDataItem, error)
	AnalyzeSemanticsConcurrently(ctx context.Context, request LLMSemanticRequest) ([]FinalResultItem, error)
	ProcessSingleTask(ctx context.Context, taskType string, prompt string) (string, error)
}

// DataMapper 数据映射接口
type DataMapper interface {
	FuseRuleAndPDFData(categories []*model.Category, cleanedData []CleanedDataItem) []SemanticChoice
	TransformPDFResult(pdfResult PDFResult) []CleanedDataItem
	TransformSemanticResult(choices []SemanticChoice, llmResults []string) []FinalResultItem
}

// ProcessingRepository 处理结果存储接口
type ProcessingRepository interface {
	SaveProcessingResults(ctx context.Context, request PersistenceRequest) error
	GetProcessingHistory(ctx context.Context, taskID string) ([]ProcessingRecord, error)
	UpdateTaskStatus(ctx context.Context, taskID string, status string, result interface{}) error
}

// MetricsCollector 指标收集接口
type MetricsCollector interface {
	RecordProcessingDuration(stage string, duration time.Duration)
	RecordSuccess(stage string)
	RecordError(stage string, err error)
	GetMetrics() ProcessingMetrics
	Reset()
}

// ===== 数据模型定义 =====

// ProcessingConfig 处理配置
type ProcessingConfig struct {
	Services struct {
		PDF PDFServiceConfig `yaml:"pdf"`
		LLM LLMServiceConfig `yaml:"llm"`
	} `yaml:"services"`
	
	Processing struct {
		PDFTimeout            time.Duration `yaml:"pdf_timeout"`
		LLMTimeout            time.Duration `yaml:"llm_timeout"`
		PersistenceBatchSize  int           `yaml:"persistence_batch_size"`
		MaxRetries            int           `yaml:"max_retries"`
		RetryBackoff          time.Duration `yaml:"retry_backoff"`
	} `yaml:"processing"`
	
	TestData struct {
		PDFFilePath string `yaml:"pdf_file_path"`
	} `yaml:"test_data"`
	
	Concurrency ConcurrencyConfig `yaml:"concurrency"`
}

// PDFServiceConfig PDF服务配置
type PDFServiceConfig struct {
	BaseURL        string        `yaml:"base_url"`
	Timeout        time.Duration `yaml:"timeout"`
	MaxRetries     int           `yaml:"max_retries"`
	ValidationType string        `yaml:"validation_type"`
}

// LLMServiceConfig LLM服务配置
type LLMServiceConfig struct {
	BaseURL     string        `yaml:"base_url"`
	Timeout     time.Duration `yaml:"timeout"`
	MaxRetries  int           `yaml:"max_retries"`
	TaskTypes   []string      `yaml:"task_types"`
}

// PDFValidationRequest PDF验证请求
type PDFValidationRequest struct {
	TaskID         string        `json:"task_id"`
	FilePath       string        `json:"file_path"`
	ValidationType string        `json:"validation_type"`
	Timeout        time.Duration `json:"timeout"`
}

// PDFResult PDF处理结果
type PDFResult struct {
	TaskID          string                 `json:"task_id"`
	Status          string                 `json:"status"`
	TotalFound      int                    `json:"total_found"`
	OccupationCodes []PDFOccupationCode    `json:"occupation_codes"`
	ProcessedAt     time.Time              `json:"processed_at"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// PDFOccupationCode PDF提取的职业编码
type PDFOccupationCode struct {
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Confidence float64   `json:"confidence"`
	Source     string    `json:"source"`
	Level      string    `json:"level,omitempty"`
	Font       string    `json:"font,omitempty"`
	ExtractedAt time.Time `json:"extracted_at"`
}

// LLMCleaningRequest LLM清洗请求
type LLMCleaningRequest struct {
	TaskType    string              `json:"task_type"`
	RawData     []PDFOccupationCode `json:"raw_data"`
	Concurrency int                 `json:"concurrency"`
	Options     CleaningOptions     `json:"options,omitempty"`
}

// CleaningOptions 清洗选项
type CleaningOptions struct {
	RemoveDescriptive bool     `json:"remove_descriptive"`
	FixOCRErrors      bool     `json:"fix_ocr_errors"`
	StandardizeNames  bool     `json:"standardize_names"`
	FilterByLevel     []string `json:"filter_by_level,omitempty"`
}

// CleanedDataItem 清洗后的数据项
type CleanedDataItem struct {
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Level       string    `json:"level"`
	Confidence  float64   `json:"confidence"`
	Source      string    `json:"source"`
	ProcessedBy string    `json:"processed_by"`
	CleanedAt   time.Time `json:"cleaned_at"`
}

// SemanticChoice 语义选择项
type SemanticChoice struct {
	Code            string `json:"code"`
	RuleName        string `json:"rule_name"`
	PDFName         string `json:"pdf_name"`
	ParentHierarchy string `json:"parent_hierarchy"`
	Confidence      struct {
		RuleConfidence float64 `json:"rule_confidence"`
		PDFConfidence  float64 `json:"pdf_confidence"`
	} `json:"confidence"`
	Context struct {
		ParentCode string `json:"parent_code"`
		Level      string `json:"level"`
	} `json:"context"`
}

// LLMSemanticRequest LLM语义分析请求
type LLMSemanticRequest struct {
	TaskType    string           `json:"task_type"`
	Choices     []SemanticChoice `json:"choices"`
	Concurrency int              `json:"concurrency"`
	Options     SemanticOptions  `json:"options,omitempty"`
}

// SemanticOptions 语义分析选项
type SemanticOptions struct {
	PreferComplete       bool     `json:"prefer_complete"`
	ExcludeDescriptive   bool     `json:"exclude_descriptive"`
	ConsiderHierarchy    bool     `json:"consider_hierarchy"`
	RequiredFields       []string `json:"required_fields,omitempty"`
}

// FinalResultItem 最终结果项
type FinalResultItem struct {
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Level      string    `json:"level"`
	ParentCode string    `json:"parent_code,omitempty"`
	Source     string    `json:"source"`
	Confidence float64   `json:"confidence"`
	ProcessedAt time.Time `json:"processed_at"`
	Metadata   struct {
		SelectedFrom    string  `json:"selected_from"`    // "rule" or "pdf"
		AlternativeName string  `json:"alternative_name,omitempty"`
		ProcessingStage string  `json:"processing_stage"`
		QualityScore    float64 `json:"quality_score"`
	} `json:"metadata"`
}

// PersistenceRequest 持久化请求
type PersistenceRequest struct {
	TaskID      string            `json:"task_id"`
	Results     []FinalResultItem `json:"results"`
	BatchSize   int               `json:"batch_size"`
	Options     PersistenceOptions `json:"options,omitempty"`
}

// PersistenceOptions 持久化选项
type PersistenceOptions struct {
	ReplaceExisting bool              `json:"replace_existing"`
	CreateBackup    bool              `json:"create_backup"`
	ValidateData    bool              `json:"validate_data"`
	Tags            map[string]string `json:"tags,omitempty"`
}

// ProcessingRecord 处理记录
type ProcessingRecord struct {
	ID          string                 `json:"id"`
	TaskID      string                 `json:"task_id"`
	Stage       string                 `json:"stage"`
	Status      string                 `json:"status"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     *time.Time             `json:"end_time,omitempty"`
	Duration    *time.Duration         `json:"duration,omitempty"`
	Input       map[string]interface{} `json:"input,omitempty"`
	Output      map[string]interface{} `json:"output,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ProcessingMetrics 处理指标
type ProcessingMetrics struct {
	TotalProcessed    int64                        `json:"total_processed"`
	SuccessCount      int64                        `json:"success_count"`
	ErrorCount        int64                        `json:"error_count"`
	SuccessRate       float64                      `json:"success_rate"`
	AvgProcessingTime time.Duration                `json:"avg_processing_time"`
	StageMetrics      map[string]StageMetrics      `json:"stage_metrics"`
	ErrorDistribution map[string]int64             `json:"error_distribution"`
	RecentActivity    []ActivityRecord             `json:"recent_activity"`
	Timestamp         time.Time                    `json:"timestamp"`
}

// StageMetrics 阶段指标
type StageMetrics struct {
	Count       int64         `json:"count"`
	SuccessRate float64       `json:"success_rate"`
	AvgDuration time.Duration `json:"avg_duration"`
	MinDuration time.Duration `json:"min_duration"`
	MaxDuration time.Duration `json:"max_duration"`
	Errors      []string      `json:"errors,omitempty"`
}

// ActivityRecord 活动记录
type ActivityRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Stage     string    `json:"stage"`
	Status    string    `json:"status"`
	Duration  time.Duration `json:"duration"`
	Error     string    `json:"error,omitempty"`
}