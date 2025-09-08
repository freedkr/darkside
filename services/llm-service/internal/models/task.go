// Package models 定义LLM服务的数据模型，基于internal/llm接口扩展
package models

import (
	"encoding/json"
	"time"
)

// Priority 枚举
type Priority string

// TokenUsage Token使用情况
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// LLMTask 通用LLM任务结构，扩展自internal/llm包
type LLMTask struct {
	// 基础任务信息
	ID       string      `json:"id" db:"id"`
	Type     LLMTaskType `json:"type" db:"type"`
	Status   TaskStatus  `json:"status" db:"status"`
	Priority Priority    `json:"priority" db:"priority"`

	// LLM配置
	Provider    string  `json:"provider" db:"provider"`       // 提供商
	Model       string  `json:"model" db:"model"`             // 模型
	Temperature float64 `json:"temperature" db:"temperature"` // 温度参数

	// 任务内容
	Prompt       string          `json:"prompt" db:"prompt"`               // 提示词
	SystemPrompt string          `json:"system_prompt" db:"system_prompt"` // 系统提示词
	Data         json.RawMessage `json:"data" db:"data"`                   // 输入数据(JSON)
	Config       TaskConfig      `json:"config" db:"config"`               // 任务配置

	// 回调配置
	CallbackURL   string `json:"callback_url,omitempty" db:"callback_url"`
	CallbackToken string `json:"callback_token,omitempty" db:"callback_token"`

	// 结果信息
	Result     json.RawMessage `json:"result,omitempty" db:"result"`           // 处理结果(JSON)
	Error      string          `json:"error,omitempty" db:"error"`             // 错误信息
	TokenUsage *TokenUsage     `json:"token_usage,omitempty" db:"token_usage"` // Token使用量

	// 时间戳
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	StartedAt   *time.Time `json:"started_at,omitempty" db:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty" db:"completed_at"`

	// 元数据
	Metadata map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
}

// TaskConfig 任务配置
type TaskConfig struct {
	// 批处理配置
	BatchProcessing bool   `json:"batch_processing,omitempty"`
	BatchSize       int    `json:"batch_size,omitempty"`
	GroupBy         string `json:"group_by,omitempty"` // 分组字段

	// 并发配置
	Concurrency int `json:"concurrency,omitempty"` // 并发数

	// 重试配置
	MaxRetries int    `json:"max_retries,omitempty"`
	RetryDelay string `json:"retry_delay,omitempty"` // 重试延迟，如"1s"

	// 超时配置
	Timeout string `json:"timeout,omitempty"` // 超时时间，如"300s"

	// 缓存配置
	CacheEnabled bool   `json:"cache_enabled,omitempty"`
	CacheTTL     string `json:"cache_ttl,omitempty"` // 缓存TTL，如"1h"

	// 其他配置
	MaxTokens        int     `json:"max_tokens,omitempty"`
	TopP             float64 `json:"top_p,omitempty"`
	FrequencyPenalty float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64 `json:"presence_penalty,omitempty"`
}

// LLMTaskType 任务类型，扩展自internal/llm
type LLMTaskType string

const (
	PriorityLow    Priority = "low"    // 低优先级
	PriorityNormal Priority = "normal" // 普通
	PriorityHigh   Priority = "high"   // 高优先级
	PriorityUrgent Priority = "urgent" // 紧急
)

const (
	// 继承internal/llm的数据清洗任务
	TaskTypeDataCleaning LLMTaskType = "data_cleaning"
	TaskTypeConfirmation LLMTaskType = "confirmation"

	// 新增的通用任务类型
	TaskTypeSemanticAnalysis       LLMTaskType = "semantic_analysis"   // 语义分析
	TaskTypeCategoryMatch          LLMTaskType = "category_match"      // 分类匹配
	TaskTypeTextSummarization      LLMTaskType = "text_summarization"  // 文本摘要
	TaskTypeTranslation            LLMTaskType = "translation"         // 翻译
	TaskTypeCodeGeneration         LLMTaskType = "code_generation"     // 代码生成
	TaskTypeQuestionAnswer         LLMTaskType = "question_answer"     // 问答
	TaskTypeTextClassification     LLMTaskType = "text_classification" // 文本分类
	TaskTypeNamedEntityRecognition LLMTaskType = "ner"                 // 命名实体识别
	TaskTypeSentimentAnalysis      LLMTaskType = "sentiment_analysis"  // 情感分析
	TaskTypeCustom                 LLMTaskType = "custom"              // 自定义任务
)

// TaskStatus 任务状态
type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"   // 待处理
	StatusQueued    TaskStatus = "queued"    // 已入队
	StatusRunning   TaskStatus = "running"   // 运行中
	StatusCompleted TaskStatus = "completed" // 已完成
	StatusFailed    TaskStatus = "failed"    // 失败
	StatusCancelled TaskStatus = "cancelled" // 已取消
	StatusRetrying  TaskStatus = "retrying"  // 重试中
	StatusTimeout   TaskStatus = "timeout"   // 超时
)

// Priority 任务优先级，复用internal/llm定义
// type Priority = llm.Priority

// LLMResult 通用LLM结果结构
type LLMResult struct {
	TaskID      string                 `json:"task_id"`
	Type        LLMTaskType            `json:"type"`
	Status      TaskStatus             `json:"status"`
	Data        interface{}            `json:"data,omitempty"`        // 处理结果数据
	Error       string                 `json:"error,omitempty"`       // 错误信息
	TokenUsage  *TokenUsage            `json:"token_usage,omitempty"` // Token使用量
	ProcessTime time.Duration          `json:"process_time"`          // 处理时间
	Provider    string                 `json:"provider"`              // 使用的提供商
	Model       string                 `json:"model"`                 // 使用的模型
	Confidence  float64                `json:"confidence,omitempty"`  // 置信度
	Metadata    map[string]interface{} `json:"metadata,omitempty"`    // 元数据
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// BatchResult 批处理结果
type BatchResult struct {
	BatchID     string        `json:"batch_id"`
	TotalTasks  int           `json:"total_tasks"`
	Completed   int           `json:"completed"`
	Failed      int           `json:"failed"`
	Results     []*LLMResult  `json:"results"`
	ProcessTime time.Duration `json:"process_time"`
	StartedAt   time.Time     `json:"started_at"`
	CompletedAt *time.Time    `json:"completed_at,omitempty"`
}

// StreamResult 流式处理结果
type StreamResult struct {
	TaskID    string    `json:"task_id"`
	Delta     string    `json:"delta"`    // 增量内容
	Content   string    `json:"content"`  // 完整内容
	Finished  bool      `json:"finished"` // 是否完成
	Timestamp time.Time `json:"timestamp"`
}

// TaskMetrics 任务指标
type TaskMetrics struct {
	TaskID          string        `json:"task_id"`
	QueueTime       time.Duration `json:"queue_time"`        // 排队时间
	ProcessTime     time.Duration `json:"process_time"`      // 处理时间
	TotalTime       time.Duration `json:"total_time"`        // 总时间
	TokensPerSecond float64       `json:"tokens_per_second"` // Token处理速度
	Cost            float64       `json:"cost,omitempty"`    // 成本
	Provider        string        `json:"provider"`
	Model           string        `json:"model"`
}

// CallbackEvent 回调事件
type CallbackEvent struct {
	EventType string                 `json:"event_type"` // started, progress, completed, failed
	TaskID    string                 `json:"task_id"`
	Status    TaskStatus             `json:"status"`
	Progress  float64                `json:"progress,omitempty"` // 进度百分比
	Data      interface{}            `json:"data,omitempty"`     // 事件数据
	Error     string                 `json:"error,omitempty"`    // 错误信息
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// 工具方法

// IsTerminal 判断任务是否已结束
func (t *LLMTask) IsTerminal() bool {
	return t.Status == StatusCompleted || t.Status == StatusFailed || t.Status == StatusCancelled
}

// GetDuration 获取任务持续时间
func (t *LLMTask) GetDuration() time.Duration {
	if t.CompletedAt != nil {
		return t.CompletedAt.Sub(t.CreatedAt)
	}
	return time.Since(t.CreatedAt)
}

// SetResult 设置任务结果
func (t *LLMTask) SetResult(result interface{}) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	t.Result = data
	return nil
}

// GetResult 获取任务结果
func (t *LLMTask) GetResult(result interface{}) error {
	if len(t.Result) == 0 {
		return nil
	}
	return json.Unmarshal(t.Result, result)
}

// SetData 设置输入数据
func (t *LLMTask) SetData(data interface{}) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	t.Data = raw
	return nil
}

// GetData 获取输入数据
func (t *LLMTask) GetData(data interface{}) error {
	if len(t.Data) == 0 {
		return nil
	}
	return json.Unmarshal(t.Data, data)
}

// NewLLMTask 创建新的LLM任务
func NewLLMTask(taskType LLMTaskType, prompt string, data interface{}) (*LLMTask, error) {
	task := &LLMTask{
		ID:          generateTaskID(),
		Type:        taskType,
		Status:      StatusPending,
		Priority:    PriorityNormal,
		Prompt:      prompt,
		Temperature: 0.7,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Metadata:    make(map[string]interface{}),
	}

	if data != nil {
		if err := task.SetData(data); err != nil {
			return nil, err
		}
	}

	return task, nil
}

// generateTaskID 生成任务ID
func generateTaskID() string {
	// 可以使用UUID或其他ID生成策略
	return "task_" + time.Now().Format("20060102150405") + "_" + randomString(8)
}

// randomString 生成随机字符串
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
