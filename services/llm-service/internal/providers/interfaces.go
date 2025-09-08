// Package providers 定义LLM提供商接口和管理器
package providers

import (
	"context"
	"time"

	"github.com/freedkr/moonshot/services/llm-service/internal/models"
)

// Provider LLM提供商统一接口
type Provider interface {
	// 基本信息
	Name() string
	IsAvailable(ctx context.Context) bool
	GetModels() []Model
	GetLimits() RateLimit
	GetPricing() Pricing

	// 核心处理方法
	Process(ctx context.Context, task *models.LLMTask) (*models.LLMResult, error)
	ProcessStream(ctx context.Context, task *models.LLMTask) (<-chan *models.StreamResult, error)
	ProcessBatch(ctx context.Context, tasks []*models.LLMTask) ([]*models.LLMResult, error)

	// 健康检查
	HealthCheck(ctx context.Context) error

	// 生命周期管理
	Initialize(config ProviderConfig) error
	Close() error
}

// ProviderManager 提供商管理器
type ProviderManager interface {
	// 提供商管理
	RegisterProvider(name string, provider Provider) error
	GetProvider(name string) (Provider, error)
	ListProviders() []string

	// 智能路由
	SelectProvider(ctx context.Context, task *models.LLMTask) (Provider, error)

	// 监控
	GetProviderStatus(name string) (*ProviderStatus, error)
	GetAllProvidersStatus() map[string]*ProviderStatus

	// 生命周期
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Model LLM模型信息
type Model struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Provider       string                 `json:"provider"`
	Type           string                 `json:"type"` // chat, completion, embedding
	MaxTokens      int                    `json:"max_tokens"`
	SupportsBatch  bool                   `json:"supports_batch"`
	SupportsStream bool                   `json:"supports_stream"`
	Pricing        *ModelPricing          `json:"pricing,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// RateLimit 速率限制
type RateLimit struct {
	RequestsPerMinute  int           `json:"requests_per_minute"`
	RequestsPerHour    int           `json:"requests_per_hour"`
	RequestsPerDay     int           `json:"requests_per_day"`
	ConcurrentRequests int           `json:"concurrent_requests"`
	TokensPerMinute    int           `json:"tokens_per_minute"`
	ResetInterval      time.Duration `json:"reset_interval"`
}

// Pricing 定价信息
type Pricing struct {
	PromptTokenPrice     float64 `json:"prompt_token_price"`     // 每1k prompt tokens的价格
	CompletionTokenPrice float64 `json:"completion_token_price"` // 每1k completion tokens的价格
	Currency             string  `json:"currency"`               // 货币单位，如"USD"
}

// ModelPricing 模型定价
type ModelPricing struct {
	InputPrice  float64 `json:"input_price"`  // 输入价格/1k tokens
	OutputPrice float64 `json:"output_price"` // 输出价格/1k tokens
	Currency    string  `json:"currency"`
}

// ProviderConfig 提供商配置
type ProviderConfig struct {
	Name       string                 `json:"name"`
	Type       string                 `json:"type"` // kimi, openai, qwen等
	Enabled    bool                   `json:"enabled"`
	APIKey     string                 `json:"api_key"`
	BaseURL    string                 `json:"base_url,omitempty"`
	Models     []string               `json:"models,omitempty"`
	RateLimit  RateLimit              `json:"rate_limit,omitempty"`
	Timeout    time.Duration          `json:"timeout,omitempty"`
	MaxRetries int                    `json:"max_retries,omitempty"`
	Settings   map[string]interface{} `json:"settings,omitempty"`
}

// ProviderStatus 提供商状态
type ProviderStatus struct {
	Name          string                 `json:"name"`
	Available     bool                   `json:"available"`
	LastCheck     time.Time              `json:"last_check"`
	ResponseTime  time.Duration          `json:"response_time"`
	ErrorCount    int                    `json:"error_count"`
	SuccessCount  int                    `json:"success_count"`
	RateLimitHits int                    `json:"rate_limit_hits"`
	CurrentLoad   int                    `json:"current_load"` // 当前并发请求数
	Models        []Model                `json:"models"`
	Metrics       map[string]interface{} `json:"metrics,omitempty"`
}

// RoutingRule 路由规则
type RoutingRule struct {
	TaskType      models.LLMTaskType `json:"task_type"`
	Providers     []string           `json:"providers"`      // 按优先级排序
	CostWeight    float64            `json:"cost_weight"`    // 成本权重
	SpeedWeight   float64            `json:"speed_weight"`   // 速度权重
	QualityWeight float64            `json:"quality_weight"` // 质量权重
	Conditions    []Condition        `json:"conditions,omitempty"`
}

// Condition 路由条件
type Condition struct {
	Field    string      `json:"field"`    // 字段名，如"data_size", "priority"等
	Operator string      `json:"operator"` // 操作符：eq, gt, lt, in等
	Value    interface{} `json:"value"`    // 比较值
}

// ProviderMetrics 提供商指标
type ProviderMetrics struct {
	RequestCount        int64         `json:"request_count"`
	SuccessCount        int64         `json:"success_count"`
	ErrorCount          int64         `json:"error_count"`
	TotalTokens         int64         `json:"total_tokens"`
	AverageLatency      time.Duration `json:"average_latency"`
	AverageTokensPerSec float64       `json:"average_tokens_per_sec"`
	TotalCost           float64       `json:"total_cost"`
	LastRequestTime     time.Time     `json:"last_request_time"`

	// 按时间窗口的统计
	HourlyStats map[string]*HourlyStats `json:"hourly_stats,omitempty"`
	DailyStats  map[string]*DailyStats  `json:"daily_stats,omitempty"`
}

// HourlyStats 小时统计
type HourlyStats struct {
	Hour         string  `json:"hour"` // 格式：2024-01-01T15
	RequestCount int64   `json:"request_count"`
	SuccessCount int64   `json:"success_count"`
	ErrorCount   int64   `json:"error_count"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCost    float64 `json:"total_cost"`
}

// DailyStats 日统计
type DailyStats struct {
	Date         string  `json:"date"` // 格式：2024-01-01
	RequestCount int64   `json:"request_count"`
	SuccessCount int64   `json:"success_count"`
	ErrorCount   int64   `json:"error_count"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCost    float64 `json:"total_cost"`
}

// ProviderError 提供商错误
type ProviderError struct {
	Provider  string `json:"provider"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Cause     error  `json:"-"`
}

func (e *ProviderError) Error() string {
	if e.Cause != nil {
		return e.Provider + ": " + e.Message + " (" + e.Cause.Error() + ")"
	}
	return e.Provider + ": " + e.Message
}

// 常见错误代码
const (
	ErrCodeRateLimit          = "RATE_LIMIT"
	ErrCodeInvalidRequest     = "INVALID_REQUEST"
	ErrCodeInvalidAPIKey      = "INVALID_API_KEY"
	ErrCodeModelNotFound      = "MODEL_NOT_FOUND"
	ErrCodeTimeout            = "TIMEOUT"
	ErrCodeServerError        = "SERVER_ERROR"
	ErrCodeQuotaExceeded      = "QUOTA_EXCEEDED"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
)

// 工厂方法

// ProviderFactory 提供商工厂
type ProviderFactory func(config ProviderConfig) (Provider, error)

// 全局提供商工厂注册表
var providerFactories = make(map[string]ProviderFactory)

// RegisterProviderFactory 注册提供商工厂
func RegisterProviderFactory(providerType string, factory ProviderFactory) {
	providerFactories[providerType] = factory
}

// CreateProvider 创建提供商实例
func CreateProvider(config ProviderConfig) (Provider, error) {
	factory, exists := providerFactories[config.Type]
	if !exists {
		return nil, &ProviderError{
			Provider: config.Name,
			Code:     "UNKNOWN_PROVIDER_TYPE",
			Message:  "未知的提供商类型: " + config.Type,
		}
	}

	return factory(config)
}
