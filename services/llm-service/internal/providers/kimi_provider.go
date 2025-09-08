// Package providers 独立的Kimi提供商实现，不依赖internal/llm包
package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/freedkr/moonshot/services/llm-service/internal/models"
)

// KimiProvider 独立的Kimi提供商实现
type KimiProvider struct {
	name        string
	config      ProviderConfig
	httpClient  *http.Client
	metrics     *ProviderMetrics
	mutex       sync.RWMutex
	rateLimiter *RateLimiter
}

// KimiAPIRequest Kimi API请求结构
type KimiAPIRequest struct {
	Model          string              `json:"model"`
	Messages       []KimiMessage       `json:"messages"`
	ResponseFormat *KimiResponseFormat `json:"response_format,omitempty"`
	MaxTokens      int                 `json:"max_tokens,omitempty"`
	Temperature    float64             `json:"temperature,omitempty"`
}

// KimiMessage 消息结构
type KimiMessage struct {
	Role    string `json:"role"` // "system", "user", "assistant"
	Content string `json:"content"`
}

// KimiResponseFormat 响应格式
type KimiResponseFormat struct {
	Type string `json:"type"` // "json_object"
}

// KimiAPIResponse Kimi API响应结构
type KimiAPIResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []KimiChoice `json:"choices"`
	Usage   KimiUsage    `json:"usage"`
	Error   *KimiError   `json:"error,omitempty"`
}

// KimiChoice 选择结构
type KimiChoice struct {
	Index        int         `json:"index"`
	Message      KimiMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// KimiUsage 使用统计
type KimiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// KimiError 错误信息
type KimiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// TokenUsage Token使用情况 (兼容models包)
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// NewKimiProvider 创建独立的Kimi提供商实例
func NewKimiProvider(config ProviderConfig) (*KimiProvider, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("Kimi API密钥是必需的")
	}

	if config.BaseURL == "" {
		config.BaseURL = "https://api.moonshot.cn/v1"
	}

	if config.Timeout == 0 {
		config.Timeout = 300 * time.Second
	}

	provider := &KimiProvider{
		name:   config.Name,
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		metrics: &ProviderMetrics{
			HourlyStats: make(map[string]*HourlyStats),
			DailyStats:  make(map[string]*DailyStats),
		},
	}

	// 初始化速率限制器
	if config.RateLimit.RequestsPerMinute > 0 {
		provider.rateLimiter = NewRateLimiter(config.RateLimit)
	}

	return provider, nil
}

// Name 返回提供商名称
func (k *KimiProvider) Name() string {
	return k.name
}

// IsAvailable 检查提供商是否可用
func (k *KimiProvider) IsAvailable(ctx context.Context) bool {
	if !k.config.Enabled {
		return false
	}

	// 执行健康检查
	err := k.HealthCheck(ctx)
	if err == nil {
		return true
	}

	// 如果是限流错误，仍然认为服务可用（只是暂时限流）
	if provErr, ok := err.(*ProviderError); ok {
		if provErr.Code == ErrCodeRateLimit {
			log.Printf("⚠️ [Kimi] 提供商遇到限流但仍可用: %v", provErr.Message)
			return true
		}
	}

	// 其他错误认为服务不可用
	log.Printf("❌ [Kimi] 提供商不可用: %v", err)
	return false
}

// GetModels 获取支持的模型列表
func (k *KimiProvider) GetModels() []Model {
	return []Model{
		{
			ID:             "moonshot-v1-auto",
			Name:           "Moonshot V1 Auto",
			Provider:       k.name,
			Type:           "chat",
			MaxTokens:      8000,
			SupportsBatch:  true,
			SupportsStream: false,
			Pricing: &ModelPricing{
				InputPrice:  0.012,
				OutputPrice: 0.012,
				Currency:    "USD",
			},
		},
		{
			ID:             "moonshot-v1-8k",
			Name:           "Moonshot V1 8K",
			Provider:       k.name,
			Type:           "chat",
			MaxTokens:      8000,
			SupportsBatch:  true,
			SupportsStream: false,
			Pricing: &ModelPricing{
				InputPrice:  0.012,
				OutputPrice: 0.012,
				Currency:    "USD",
			},
		},
		{
			ID:             "moonshot-v1-32k",
			Name:           "Moonshot V1 32K",
			Provider:       k.name,
			Type:           "chat",
			MaxTokens:      32000,
			SupportsBatch:  true,
			SupportsStream: false,
			Pricing: &ModelPricing{
				InputPrice:  0.024,
				OutputPrice: 0.024,
				Currency:    "USD",
			},
		},
		{
			ID:             "moonshot-v1-128k",
			Name:           "Moonshot V1 128K",
			Provider:       k.name,
			Type:           "chat",
			MaxTokens:      128000,
			SupportsBatch:  true,
			SupportsStream: false,
			Pricing: &ModelPricing{
				InputPrice:  0.060,
				OutputPrice: 0.060,
				Currency:    "USD",
			},
		},
	}
}

// GetLimits 获取速率限制 - 根据实际Kimi账号配额配置
func (k *KimiProvider) GetLimits() RateLimit {
	return RateLimit{
		RequestsPerMinute:  500,    // RPM: 500 (实际配额)
		RequestsPerHour:    30000,  // RPH: 500 * 60 = 30,000
		RequestsPerDay:     720000, // RPD: 500 * 60 * 24 = 720,000
		ConcurrentRequests: 100,    // 并发: 100 (实际配额)
		TokensPerMinute:    128000, // TPM: 128,000 (实际配额)
		ResetInterval:      time.Minute,
	}
}

// GetPricing 获取定价信息
func (k *KimiProvider) GetPricing() Pricing {
	return Pricing{
		PromptTokenPrice:     0.012,
		CompletionTokenPrice: 0.012,
		Currency:             "USD",
	}
}

// Process 处理单个LLM任务
func (k *KimiProvider) Process(ctx context.Context, task *models.LLMTask) (*models.LLMResult, error) {
	startTime := time.Now()

	// 速率限制检查
	if k.rateLimiter != nil {
		if err := k.rateLimiter.Wait(ctx); err != nil {
			return nil, &ProviderError{
				Provider:  k.name,
				Code:      ErrCodeRateLimit,
				Message:   "速率限制",
				Retryable: true,
				Cause:     err,
			}
		}
		// 确保释放并发槽位
		defer k.rateLimiter.Release()
	}

	// 记录请求
	k.recordRequest()

	// 处理任务
	result, _, err := k.processTask(ctx, task)

	// 记录结果
	processTime := time.Since(startTime)
	if err != nil {
		k.recordError()
		return nil, err
	}

	k.recordSuccess()

	// 构建结果 - 不包含TokenUsage字段，稍后设置
	llmResult := &models.LLMResult{
		TaskID:      task.ID,
		Type:        task.Type,
		Status:      models.StatusCompleted,
		Data:        result,
		ProcessTime: processTime,
		Provider:    k.name,
		Model:       task.Model,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	return llmResult, nil
}

// ProcessStream 流式处理
func (k *KimiProvider) ProcessStream(ctx context.Context, task *models.LLMTask) (<-chan *models.StreamResult, error) {
	return nil, &ProviderError{
		Provider: k.name,
		Code:     "",
		Message:  "",
	}
}

// ProcessBatch 批量处理
func (k *KimiProvider) ProcessBatch(ctx context.Context, tasks []*models.LLMTask) ([]*models.LLMResult, error) {
	results := make([]*models.LLMResult, 0, len(tasks))

	for _, task := range tasks {
		result, err := k.Process(ctx, task)
		if err != nil {
			result = &models.LLMResult{
				TaskID:    task.ID,
				Type:      task.Type,
				Status:    models.StatusFailed,
				Error:     err.Error(),
				Provider:  k.name,
				Model:     task.Model,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
		}
		results = append(results, result)
	}

	return results, nil
}

// HealthCheck 健康检查
func (k *KimiProvider) HealthCheck(ctx context.Context) error {
	// 使用一个简单的测试请求
	request := &KimiAPIRequest{
		Model: "moonshot-v1-auto",
		Messages: []KimiMessage{
			{Role: "user", Content: "测试连接"},
		},
		MaxTokens:   10,
		Temperature: 0.1,
	}

	_, err := k.callKimiAPI(ctx, request)
	if err != nil {
		return &ProviderError{
			Provider: k.name,
			Code:     ErrCodeServiceUnavailable,
			Message:  "健康检查失败",
			Cause:    err,
		}
	}

	return nil
}

// Initialize 初始化提供商
func (k *KimiProvider) Initialize(config ProviderConfig) error {
	k.config = config
	return nil
}

// Close 关闭提供商
func (k *KimiProvider) Close() error {
	return nil
}

// processTask 处理具体任务 - 确保参数正确传递
func (k *KimiProvider) processTask(ctx context.Context, task *models.LLMTask) (interface{}, *TokenUsage, error) {
	// 构建消息列表
	messages := []KimiMessage{}

	// 添加系统提示词
	if task.SystemPrompt != "" {
		messages = append(messages, KimiMessage{
			Role:    "system",
			Content: task.SystemPrompt,
		})
	}

	// 添加用户提示词
	messages = append(messages, KimiMessage{
		Role:    "user",
		Content: task.Prompt,
	})

	// 构建完整的API请求 - 正确传递所有参数
	request := &KimiAPIRequest{
		Model:    k.selectModel(task),
		Messages: messages,
		ResponseFormat: &KimiResponseFormat{
			Type: "json_object",
		},
		Temperature: k.getTemperature(task), // ✅ 确保参数传递
		MaxTokens:   k.getMaxTokens(task),   // ✅ 确保参数传递
	}

	// 调用API
	response, err := k.callKimiAPI(ctx, request)
	if err != nil {
		return nil, nil, k.wrapError(err)
	}

	if len(response.Choices) == 0 {
		return nil, nil, &ProviderError{
			Provider: k.name,
			Code:     "NO_RESPONSE",
			Message:  "API响应中没有选择项",
		}
	}

	// 构建token使用情况
	tokenUsage := &TokenUsage{
		PromptTokens:     response.Usage.PromptTokens,
		CompletionTokens: response.Usage.CompletionTokens,
		TotalTokens:      response.Usage.TotalTokens,
	}

	// 直接返回原始响应，不进行JSON解析，避免双重编码问题
	rawResponse := response.Choices[0].Message.Content
	fmt.Printf("🔍 DEBUG: Kimi API原始响应: %s\n", rawResponse)
	fmt.Printf("🔍 DEBUG: 直接返回原始字符串，避免双重JSON编码\n")

	return rawResponse, tokenUsage, nil
}

// selectModel 选择合适的模型
func (k *KimiProvider) selectModel(task *models.LLMTask) string {
	if task.Model != "" {
		return task.Model
	}
	return "moonshot-v1-auto" // 默认模型
}

// getTemperature 获取温度参数 - 确保参数传递
func (k *KimiProvider) getTemperature(task *models.LLMTask) float64 {
	// 优先使用任务指定的温度
	if task.Temperature > 0 {
		return task.Temperature
	}

	// 默认温度
	return 0.1
}

// getMaxTokens 获取最大token数 - 确保参数传递
func (k *KimiProvider) getMaxTokens(task *models.LLMTask) int {
	// 检查配置中的max_tokens参数
	if task.Config.MaxTokens > 0 {
		return task.Config.MaxTokens
	}
	return 30000
}

// callKimiAPI 直接调用Kimi API - 完全独立实现
func (k *KimiProvider) callKimiAPI(ctx context.Context, request *KimiAPIRequest) (*KimiAPIResponse, error) {
	// 序列化请求
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// 创建HTTP请求
	url := k.config.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+k.config.APIKey)

	// 发送请求
	resp, err := k.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		// 特殊处理429错误（限流）
		if resp.StatusCode == http.StatusTooManyRequests {
			// 尝试解析错误详情
			var errorResp struct {
				Error struct {
					Message string `json:"message"`
					Type    string `json:"type"`
				} `json:"error"`
			}
			json.Unmarshal(body, &errorResp)

			return nil, &ProviderError{
				Provider:  k.name,
				Code:      ErrCodeRateLimit,
				Message:   fmt.Sprintf("触发速率限制(429): %s", errorResp.Error.Message),
				Retryable: true,
				Cause:     fmt.Errorf("HTTP 429: %s", string(body)),
			}
		}

		// 其他错误
		return nil, fmt.Errorf("API返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var response KimiAPIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应JSON失败: %w", err)
	}

	// 检查API错误
	if response.Error != nil {
		return nil, fmt.Errorf("API返回错误: %s", response.Error.Message)
	}

	return &response, nil
}

// wrapError 包装错误
func (k *KimiProvider) wrapError(err error) error {
	// 如果已经是ProviderError，直接返回
	if provErr, ok := err.(*ProviderError); ok {
		return provErr
	}

	// 否则包装为通用错误
	return &ProviderError{
		Provider:  k.name,
		Code:      ErrCodeServerError,
		Message:   "Kimi API调用失败",
		Retryable: true,
		Cause:     err,
	}
}

// 指标记录方法
func (k *KimiProvider) recordRequest() {
	k.mutex.Lock()
	defer k.mutex.Unlock()
	k.metrics.RequestCount++
	k.metrics.LastRequestTime = time.Now()
}

func (k *KimiProvider) recordSuccess() {
	k.mutex.Lock()
	defer k.mutex.Unlock()
	k.metrics.SuccessCount++
}

func (k *KimiProvider) recordError() {
	k.mutex.Lock()
	defer k.mutex.Unlock()
	k.metrics.ErrorCount++
}

// 初始化时注册Kimi提供商工厂
func init() {
	RegisterProviderFactory("kimi", func(config ProviderConfig) (Provider, error) {
		return NewKimiProvider(config)
	})
}
