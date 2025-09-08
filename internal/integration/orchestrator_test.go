package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/freedkr/moonshot/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ===== Mock 定义 =====

// MockPDFService PDF服务Mock
type MockPDFService struct {
	mock.Mock
}

func (m *MockPDFService) ValidateAndExtract(ctx context.Context, request PDFValidationRequest) (PDFResult, error) {
	args := m.Called(ctx, request)
	return args.Get(0).(PDFResult), args.Error(1)
}

func (m *MockPDFService) GetTaskStatus(ctx context.Context, taskID string) (TaskStatus, error) {
	args := m.Called(ctx, taskID)
	return args.Get(0).(TaskStatus), args.Error(1)
}

func (m *MockPDFService) GetOccupationCodes(ctx context.Context, taskID string) (PDFResult, error) {
	args := m.Called(ctx, taskID)
	return args.Get(0).(PDFResult), args.Error(1)
}

// MockLLMService LLM服务Mock
type MockLLMService struct {
	mock.Mock
}

func (m *MockLLMService) CleanDataConcurrently(ctx context.Context, request LLMCleaningRequest) ([]CleanedDataItem, error) {
	args := m.Called(ctx, request)
	return args.Get(0).([]CleanedDataItem), args.Error(1)
}

func (m *MockLLMService) AnalyzeSemanticsConcurrently(ctx context.Context, request LLMSemanticRequest) ([]FinalResultItem, error) {
	args := m.Called(ctx, request)
	return args.Get(0).([]FinalResultItem), args.Error(1)
}

func (m *MockLLMService) ProcessSingleTask(ctx context.Context, taskType string, prompt string) (string, error) {
	args := m.Called(ctx, taskType, prompt)
	return args.String(0), args.Error(1)
}

// MockDataMapper 数据映射器Mock
type MockDataMapper struct {
	mock.Mock
}

func (m *MockDataMapper) FuseRuleAndPDFData(categories []*model.Category, cleanedData []CleanedDataItem) []SemanticChoice {
	args := m.Called(categories, cleanedData)
	return args.Get(0).([]SemanticChoice)
}

func (m *MockDataMapper) TransformPDFResult(pdfResult PDFResult) []CleanedDataItem {
	args := m.Called(pdfResult)
	return args.Get(0).([]CleanedDataItem)
}

func (m *MockDataMapper) TransformSemanticResult(choices []SemanticChoice, llmResults []string) []FinalResultItem {
	args := m.Called(choices, llmResults)
	return args.Get(0).([]FinalResultItem)
}

// MockProcessingRepository 存储Mock
type MockProcessingRepository struct {
	mock.Mock
}

func (m *MockProcessingRepository) SaveProcessingResults(ctx context.Context, request PersistenceRequest) error {
	args := m.Called(ctx, request)
	return args.Error(0)
}

func (m *MockProcessingRepository) GetProcessingHistory(ctx context.Context, taskID string) ([]ProcessingRecord, error) {
	args := m.Called(ctx, taskID)
	return args.Get(0).([]ProcessingRecord), args.Error(1)
}

func (m *MockProcessingRepository) UpdateTaskStatus(ctx context.Context, taskID string, status string, result interface{}) error {
	args := m.Called(ctx, taskID, status, result)
	return args.Error(0)
}

// MockConcurrencyManager 并发管理器Mock
type MockConcurrencyManager struct {
	mock.Mock
}

func (m *MockConcurrencyManager) GetOptimalConcurrency(taskType string) int {
	args := m.Called(taskType)
	return args.Int(0)
}

func (m *MockConcurrencyManager) AcquirePermit(ctx context.Context, taskType string) error {
	args := m.Called(ctx, taskType)
	return args.Error(0)
}

func (m *MockConcurrencyManager) ReleasePermit(taskType string) {
	m.Called(taskType)
}

func (m *MockConcurrencyManager) UpdateMetrics(taskType string, metrics TaskMetrics) {
	m.Called(taskType, metrics)
}

func (m *MockConcurrencyManager) GetStatus() ConcurrencyStatus {
	args := m.Called()
	return args.Get(0).(ConcurrencyStatus)
}

// MockMetricsCollector 指标收集器Mock
type MockMetricsCollector struct {
	mock.Mock
}

func (m *MockMetricsCollector) RecordProcessingDuration(stage string, duration time.Duration) {
	m.Called(stage, duration)
}

func (m *MockMetricsCollector) RecordSuccess(stage string) {
	m.Called(stage)
}

func (m *MockMetricsCollector) RecordError(stage string, err error) {
	m.Called(stage, err)
}

func (m *MockMetricsCollector) GetMetrics() ProcessingMetrics {
	args := m.Called()
	return args.Get(0).(ProcessingMetrics)
}

func (m *MockMetricsCollector) Reset() {
	m.Called()
}

// ===== 测试用例 =====

// TestProcessingOrchestrator_SuccessfulFlow 测试成功的完整流程
func TestProcessingOrchestrator_SuccessfulFlow(t *testing.T) {
	// 准备测试数据
	ctx := context.Background()
	taskID := "test-task-001"
	excelPath := ""
	categories := createTestCategories()

	// 创建mocks
	pdfService := new(MockPDFService)
	llmService := new(MockLLMService)
	dataMapper := new(MockDataMapper)
	repository := new(MockProcessingRepository)
	concurrency := new(MockConcurrencyManager)
	metrics := new(MockMetricsCollector)

	// 配置expectations
	pdfResult := PDFResult{
		TaskID:     taskID,
		Status:     "completed",
		TotalFound: 2,
		OccupationCodes: []PDFOccupationCode{
			{Code: "1-01-01-01", Name: "测试职业1", Confidence: 0.9},
			{Code: "1-01-01-02", Name: "测试职业2", Confidence: 0.8},
		},
	}

	cleanedData := []CleanedDataItem{
		{Code: "1-01-01-01", Name: "清洗后职业1", Confidence: 0.95},
		{Code: "1-01-01-02", Name: "清洗后职业2", Confidence: 0.85},
	}

	semanticChoices := []SemanticChoice{
		{Code: "1-01-01-01", RuleName: "规则名称1", PDFName: "PDF名称1"},
		{Code: "1-01-01-02", RuleName: "规则名称2", PDFName: "PDF名称2"},
	}

	finalResults := []FinalResultItem{
		{Code: "1-01-01-01", Name: "最终名称1", Level: "细类"},
		{Code: "1-01-01-02", Name: "最终名称2", Level: "细类"},
	}

	// 设置Mock期望
	pdfService.On("ValidateAndExtract", ctx, mock.Anything).Return(pdfResult, nil)

	concurrency.On("GetOptimalConcurrency", "data_cleaning").Return(3)
	llmService.On("CleanDataConcurrently", ctx, mock.Anything).Return(cleanedData, nil)

	dataMapper.On("FuseRuleAndPDFData", categories, cleanedData).Return(semanticChoices)

	concurrency.On("GetOptimalConcurrency", "semantic_analysis").Return(2)
	llmService.On("AnalyzeSemanticsConcurrently", ctx, mock.Anything).Return(finalResults, nil)

	repository.On("SaveProcessingResults", ctx, mock.Anything).Return(nil)

	metrics.On("RecordProcessingDuration", mock.Anything, mock.Anything).Return()
	metrics.On("RecordSuccess", mock.Anything).Return()
	metrics.On("RecordError", mock.Anything, mock.Anything).Return().Maybe()

	// 创建Orchestrator
	orchestrator := &ProcessingOrchestrator{
		pdfService:  pdfService,
		llmService:  llmService,
		dataMapper:  dataMapper,
		repository:  repository,
		concurrency: concurrency,
		metrics:     metrics,
		config: &ProcessingConfig{
			TestData: struct {
				PDFFilePath string `yaml:"pdf_file_path"`
			}{
				PDFFilePath: "testdata/test.pdf",
			},
			Processing: struct {
				PDFTimeout           time.Duration `yaml:"pdf_timeout"`
				LLMTimeout           time.Duration `yaml:"llm_timeout"`
				PersistenceBatchSize int           `yaml:"persistence_batch_size"`
				MaxRetries           int           `yaml:"max_retries"`
				RetryBackoff         time.Duration `yaml:"retry_backoff"`
			}{
				PDFTimeout:           180 * time.Second,
				LLMTimeout:           120 * time.Second,
				PersistenceBatchSize: 100,
			},
		},
	}

	// 执行测试
	err := orchestrator.ProcessWithPDFAndLLM(ctx, taskID, excelPath, categories)

	// 验证结果
	assert.NoError(t, err)

	// 验证所有Mock被正确调用
	pdfService.AssertExpectations(t)
	llmService.AssertExpectations(t)
	dataMapper.AssertExpectations(t)
	repository.AssertExpectations(t)
	metrics.AssertExpectations(t)
}

// TestProcessingOrchestrator_PDFValidationError 测试PDF验证失败
func TestProcessingOrchestrator_PDFValidationError(t *testing.T) {
	ctx := context.Background()
	taskID := "test-task-002"

	// 创建mocks
	pdfService := new(MockPDFService)
	metrics := new(MockMetricsCollector)

	// 设置PDF服务返回错误
	expectedError := errors.New("PDF validation failed")
	pdfService.On("ValidateAndExtract", ctx, mock.Anything).Return(PDFResult{}, expectedError)

	metrics.On("RecordProcessingDuration", mock.Anything, mock.Anything).Return()
	metrics.On("RecordError", "pdf_validation", expectedError).Return()

	orchestrator := &ProcessingOrchestrator{
		pdfService: pdfService,
		metrics:    metrics,
		config: &ProcessingConfig{
			TestData: struct {
				PDFFilePath string `yaml:"pdf_file_path"`
			}{
				PDFFilePath: "testdata/test.pdf",
			},
			Processing: struct {
				PDFTimeout           time.Duration `yaml:"pdf_timeout"`
				LLMTimeout           time.Duration `yaml:"llm_timeout"`
				PersistenceBatchSize int           `yaml:"persistence_batch_size"`
				MaxRetries           int           `yaml:"max_retries"`
				RetryBackoff         time.Duration `yaml:"retry_backoff"`
			}{
				PDFTimeout: 180 * time.Second,
			},
		},
	}

	// 执行测试
	err := orchestrator.ProcessWithPDFAndLLM(ctx, taskID, "", nil)

	// 验证错误
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pdf_validation")

	pdfService.AssertExpectations(t)
	metrics.AssertExpectations(t)
}

// TestProcessingOrchestrator_LLMCleaningError 测试LLM清洗失败
func TestProcessingOrchestrator_LLMCleaningError(t *testing.T) {
	ctx := context.Background()
	taskID := "test-task-003"

	// 创建mocks
	pdfService := new(MockPDFService)
	llmService := new(MockLLMService)
	concurrency := new(MockConcurrencyManager)
	metrics := new(MockMetricsCollector)

	pdfResult := PDFResult{
		TaskID:     taskID,
		Status:     "completed",
		TotalFound: 1,
		OccupationCodes: []PDFOccupationCode{
			{Code: "1-01-01-01", Name: "测试职业", Confidence: 0.9},
		},
	}

	// 设置expectations
	pdfService.On("ValidateAndExtract", ctx, mock.Anything).Return(pdfResult, nil)

	expectedError := errors.New("LLM service unavailable")
	concurrency.On("GetOptimalConcurrency", "data_cleaning").Return(3)
	llmService.On("CleanDataConcurrently", ctx, mock.Anything).Return([]CleanedDataItem{}, expectedError)

	metrics.On("RecordProcessingDuration", mock.Anything, mock.Anything).Return()
	metrics.On("RecordSuccess", "pdf_validation").Return()
	metrics.On("RecordError", "llm_cleaning", expectedError).Return()

	orchestrator := &ProcessingOrchestrator{
		pdfService:  pdfService,
		llmService:  llmService,
		concurrency: concurrency,
		metrics:     metrics,
		config: &ProcessingConfig{
			TestData: struct {
				PDFFilePath string `yaml:"pdf_file_path"`
			}{
				PDFFilePath: "testdata/test.pdf",
			},
			Processing: struct {
				PDFTimeout           time.Duration `yaml:"pdf_timeout"`
				LLMTimeout           time.Duration `yaml:"llm_timeout"`
				PersistenceBatchSize int           `yaml:"persistence_batch_size"`
				MaxRetries           int           `yaml:"max_retries"`
				RetryBackoff         time.Duration `yaml:"retry_backoff"`
			}{
				PDFTimeout: 180 * time.Second,
				LLMTimeout: 120 * time.Second,
			},
		},
	}

	// 执行测试
	err := orchestrator.ProcessWithPDFAndLLM(ctx, taskID, "", nil)

	// 验证错误
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "llm_cleaning")

	pdfService.AssertExpectations(t)
	llmService.AssertExpectations(t)
	metrics.AssertExpectations(t)
}

// TestProcessingOrchestrator_PersistenceError 测试持久化失败
func TestProcessingOrchestrator_PersistenceError(t *testing.T) {
	ctx := context.Background()
	taskID := "test-task-004"
	categories := createTestCategories()

	// 创建所有必要的mocks
	pdfService := new(MockPDFService)
	llmService := new(MockLLMService)
	dataMapper := new(MockDataMapper)
	repository := new(MockProcessingRepository)
	concurrency := new(MockConcurrencyManager)
	metrics := new(MockMetricsCollector)

	// 准备测试数据
	pdfResult := PDFResult{
		TaskID: taskID,
		Status: "completed",
	}
	cleanedData := []CleanedDataItem{}
	semanticChoices := []SemanticChoice{}
	finalResults := []FinalResultItem{}

	// 设置成功的流程直到持久化
	pdfService.On("ValidateAndExtract", ctx, mock.Anything).Return(pdfResult, nil)
	concurrency.On("GetOptimalConcurrency", mock.Anything).Return(3)
	llmService.On("CleanDataConcurrently", ctx, mock.Anything).Return(cleanedData, nil)
	dataMapper.On("FuseRuleAndPDFData", categories, cleanedData).Return(semanticChoices)
	llmService.On("AnalyzeSemanticsConcurrently", ctx, mock.Anything).Return(finalResults, nil)

	// 设置持久化失败
	expectedError := errors.New("database connection failed")
	repository.On("SaveProcessingResults", ctx, mock.Anything).Return(expectedError)

	metrics.On("RecordProcessingDuration", mock.Anything, mock.Anything).Return()
	metrics.On("RecordSuccess", mock.Anything).Return()
	metrics.On("RecordError", "persistence", expectedError).Return()

	orchestrator := &ProcessingOrchestrator{
		pdfService:  pdfService,
		llmService:  llmService,
		dataMapper:  dataMapper,
		repository:  repository,
		concurrency: concurrency,
		metrics:     metrics,
		config:      createTestConfig(),
	}

	// 执行测试
	err := orchestrator.ProcessWithPDFAndLLM(ctx, taskID, "", categories)

	// 验证错误
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "persistence")

	repository.AssertExpectations(t)
	metrics.AssertExpectations(t)
}

// ===== 辅助函数 =====

func createTestCategories() []*model.Category {
	return []*model.Category{
		{
			Code: "1",
			Name: "大类1",
			Children: []*model.Category{
				{
					Code: "1-01",
					Name: "中类1",
					Children: []*model.Category{
						{
							Code: "1-01-01",
							Name: "小类1",
							Children: []*model.Category{
								{Code: "1-01-01-01", Name: "细类1"},
								{Code: "1-01-01-02", Name: "细类2"},
							},
						},
					},
				},
			},
		},
	}
}

func createTestConfig() *ProcessingConfig {
	return &ProcessingConfig{
		TestData: struct {
			PDFFilePath string `yaml:"pdf_file_path"`
		}{
			PDFFilePath: "testdata/test.pdf",
		},
		Processing: struct {
			PDFTimeout           time.Duration `yaml:"pdf_timeout"`
			LLMTimeout           time.Duration `yaml:"llm_timeout"`
			PersistenceBatchSize int           `yaml:"persistence_batch_size"`
			MaxRetries           int           `yaml:"max_retries"`
			RetryBackoff         time.Duration `yaml:"retry_backoff"`
		}{
			PDFTimeout:           180 * time.Second,
			LLMTimeout:           120 * time.Second,
			PersistenceBatchSize: 100,
			MaxRetries:           3,
			RetryBackoff:         2 * time.Second,
		},
	}
}
