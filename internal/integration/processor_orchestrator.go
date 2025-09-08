package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/freedkr/moonshot/internal/config"
	"github.com/freedkr/moonshot/internal/database"
	"github.com/freedkr/moonshot/internal/model"
)

// ProcessingOrchestrator 处理编排器 - 核心业务逻辑协调者
type ProcessingOrchestrator struct {
	pdfService  PDFService
	llmService  LLMService
	dataMapper  DataMapper
	repository  ProcessingRepository
	concurrency ConcurrencyManager
	metrics     MetricsCollector
	config      *ProcessingConfig

	// 新增：增量处理器，负责5步增量流程
	incrementalProcessor *IncrementalProcessor
}

// NewProcessingOrchestrator 创建处理编排器
func NewProcessingOrchestrator(
	cfg *config.Config,
	db database.DatabaseInterface,
) *ProcessingOrchestrator {
	processingConfig := LoadProcessingConfig(cfg)

	return &ProcessingOrchestrator{
		pdfService:  NewPDFServiceClient(processingConfig.Services.PDF),
		llmService:  NewLLMServiceClient(processingConfig.Services.LLM),
		dataMapper:  NewDataMapper(),
		repository:  NewProcessingRepository(db),
		concurrency: NewQuotaAwareConcurrencyManager(processingConfig.Concurrency),
		metrics:     NewMetricsCollector(),
		config:      processingConfig,

		// 初始化增量处理器
		incrementalProcessor: NewIncrementalProcessor(cfg, db),
	}
}

// ProcessWithPDFAndLLM 主业务流程编排 - 增量更新的五阶段处理
func (o *ProcessingOrchestrator) ProcessWithPDFAndLLM(
	ctx context.Context,
	taskID string,
	excelPath string,
	categories []*model.Category,
) error {
	// 启动处理指标收集
	processingStart := time.Now()
	defer func() {
		o.metrics.RecordProcessingDuration("full_pipeline", time.Since(processingStart))
	}()

	// 委托给增量处理器执行5步流程
	err := o.incrementalProcessor.ProcessIncrementalFlow(ctx, taskID, excelPath, categories)
	if err != nil {
		return o.wrapError("incremental_processing", err)
	}

	o.metrics.RecordSuccess("full_pipeline")
	return nil
}

// ProcessWithPDFAndLLMLegacy 保留原始的重构版本（用于兼容性测试）
func (o *ProcessingOrchestrator) ProcessWithPDFAndLLMLegacy(
	ctx context.Context,
	taskID string,
	excelPath string,
	categories []*model.Category,
) error {
	// 启动处理指标收集
	processingStart := time.Now()
	defer func() {
		o.metrics.RecordProcessingDuration("full_pipeline", time.Since(processingStart))
	}()

	// 阶段1：PDF验证服务调用
	pdfResult, err := o.executePDFValidation(ctx, taskID)
	if err != nil {
		return o.wrapError("pdf_validation", err)
	}

	// 阶段2：LLM数据清洗（并发优化）
	cleanedData, err := o.executeLLMDataCleaning(ctx, pdfResult)
	if err != nil {
		return o.wrapError("llm_cleaning", err)
	}

	// 阶段3：数据融合
	semanticChoices := o.executeDataFusion(categories, cleanedData)

	// 阶段4：LLM语义分析（配额感知并发）
	finalResult, err := o.executeLLMSemanticAnalysis(ctx, semanticChoices)
	if err != nil {
		return o.wrapError("llm_semantic", err)
	}

	// 阶段5：结果持久化
	err = o.executePersistence(ctx, taskID, finalResult)
	if err != nil {
		return o.wrapError("persistence", err)
	}

	o.metrics.RecordSuccess("full_pipeline")
	return nil
}

// executePDFValidation PDF验证阶段
func (o *ProcessingOrchestrator) executePDFValidation(ctx context.Context, taskID string) (PDFResult, error) {
	request := PDFValidationRequest{
		TaskID:         taskID,
		FilePath:       o.config.TestData.PDFFilePath, // 配置化的固定路径
		ValidationType: "standard",
		Timeout:        o.config.Processing.PDFTimeout,
	}

	result, err := o.pdfService.ValidateAndExtract(ctx, request)
	if err != nil {
		o.metrics.RecordError("pdf_validation", err)
		return PDFResult{}, err
	}

	o.metrics.RecordSuccess("pdf_validation")
	return result, nil
}

// executeLLMDataCleaning LLM数据清洗阶段（优化并发）
func (o *ProcessingOrchestrator) executeLLMDataCleaning(ctx context.Context, pdfResult PDFResult) ([]CleanedDataItem, error) {
	// 使用配额感知的并发处理
	cleaningRequest := LLMCleaningRequest{
		TaskType:    "data_cleaning",
		RawData:     pdfResult.OccupationCodes,
		Concurrency: o.concurrency.GetOptimalConcurrency("data_cleaning"),
	}

	result, err := o.llmService.CleanDataConcurrently(ctx, cleaningRequest)
	if err != nil {
		o.metrics.RecordError("llm_cleaning", err)
		return nil, err
	}

	o.metrics.RecordSuccess("llm_cleaning")
	return result, nil
}

// executeDataFusion 数据融合阶段
func (o *ProcessingOrchestrator) executeDataFusion(categories []*model.Category, cleanedData []CleanedDataItem) []SemanticChoice {
	return o.dataMapper.FuseRuleAndPDFData(categories, cleanedData)
}

// executeLLMSemanticAnalysis LLM语义分析阶段（配额感知）
func (o *ProcessingOrchestrator) executeLLMSemanticAnalysis(ctx context.Context, choices []SemanticChoice) ([]FinalResultItem, error) {
	semanticRequest := LLMSemanticRequest{
		TaskType:    "semantic_analysis",
		Choices:     choices,
		Concurrency: o.concurrency.GetOptimalConcurrency("semantic_analysis"),
	}

	result, err := o.llmService.AnalyzeSemanticsConcurrently(ctx, semanticRequest)
	if err != nil {
		o.metrics.RecordError("llm_semantic", err)
		return nil, err
	}

	o.metrics.RecordSuccess("llm_semantic")
	return result, nil
}

// executePersistence 持久化阶段
func (o *ProcessingOrchestrator) executePersistence(ctx context.Context, taskID string, finalResult []FinalResultItem) error {
	persistenceRequest := PersistenceRequest{
		TaskID:    taskID,
		Results:   finalResult,
		BatchSize: o.config.Processing.PersistenceBatchSize,
	}

	err := o.repository.SaveProcessingResults(ctx, persistenceRequest)
	if err != nil {
		o.metrics.RecordError("persistence", err)
		return err
	}

	o.metrics.RecordSuccess("persistence")
	return nil
}

// wrapError 标准化错误包装
func (o *ProcessingOrchestrator) wrapError(stage string, err error) error {
	return &ProcessingError{
		Stage:     stage,
		Message:   err.Error(),
		Timestamp: time.Now(),
		Retryable: isRetryableError(err),
	}
}

// ProcessingError 标准化处理错误
type ProcessingError struct {
	Stage     string    `json:"stage"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Retryable bool      `json:"retryable"`
}

func (e *ProcessingError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Stage, e.Message)
}

func isRetryableError(_ error) bool {
	// 根据错误类型判断是否可重试
	// 网络错误、超时错误等通常可重试
	// 业务逻辑错误、验证错误等通常不可重试
	return true // 简化实现
}
