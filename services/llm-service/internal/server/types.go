// Package server API请求和响应类型定义
package server

import (
	"time"

	"github.com/freedkr/moonshot/services/llm-service/internal/models"
	"github.com/freedkr/moonshot/services/llm-service/internal/providers"
	"github.com/freedkr/moonshot/services/llm-service/internal/scheduler"
)

// SubmitTaskRequest 提交任务请求
type SubmitTaskRequest struct {
	// 任务基本信息
	Type        models.LLMTaskType `json:"type" binding:"required"`
	Provider    string             `json:"provider,omitempty"`    // 指定提供商，空则自动选择
	Model       string             `json:"model,omitempty"`       // 指定模型
	Temperature float64            `json:"temperature,omitempty"` // 温度参数

	// 提示词
	Prompt       string `json:"prompt" binding:"required"` // 用户提示词
	SystemPrompt string `json:"system_prompt,omitempty"`   // 系统提示词

	// 任务配置
	Priority models.Priority   `json:"priority,omitempty"` // 优先级
	Config   models.TaskConfig `json:"config,omitempty"`   // 任务配置

	// 数据
	Data interface{} `json:"data,omitempty"` // 输入数据

	// 回调
	CallbackURL string `json:"callback_url,omitempty"` // 回调URL

	// 元数据
	Metadata map[string]interface{} `json:"metadata,omitempty"` // 元数据
}

// SubmitTaskResponse 提交任务响应
type SubmitTaskResponse struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// BatchSubmitRequest 批量提交请求
type BatchSubmitRequest struct {
	Tasks []SubmitTaskRequest `json:"tasks" binding:"required,min=1,max=100"`
}

// BatchSubmitResponse 批量提交响应
type BatchSubmitResponse struct {
	Results []SubmitTaskResponse `json:"results"`
}

// SyncProcessRequest 同步处理请求（复用SubmitTaskRequest）
type SyncProcessRequest = SubmitTaskRequest

// SyncProcessResponse 同步处理响应
type SyncProcessResponse struct {
	TaskID      string             `json:"task_id"`
	Status      string             `json:"status"`
	Result      interface{}        `json:"result,omitempty"`
	TokenUsage  *models.TokenUsage `json:"token_usage,omitempty"`
	ProcessTime string             `json:"process_time"`
	Error       string             `json:"error,omitempty"`
}

// StreamProcessRequest 流式处理请求
type StreamProcessRequest = SubmitTaskRequest

// StreamProcessResponse 流式处理响应
type StreamProcessResponse struct {
	TaskID   string `json:"task_id"`
	Delta    string `json:"delta"`    // 增量内容
	Content  string `json:"content"`  // 完整内容
	Finished bool   `json:"finished"` // 是否完成
	Error    string `json:"error,omitempty"`
}

// TaskStatusResponse 任务状态响应（复用LLMTask）
type TaskStatusResponse = models.LLMTask

// ProvidersResponse 提供商列表响应
type ProvidersResponse struct {
	Providers []string `json:"providers"`
}

// ProviderStatusResponse 提供商状态响应
type ProviderStatusResponse = providers.ProviderStatus

// AllProvidersStatusResponse 所有提供商状态响应
type AllProvidersStatusResponse = map[string]*providers.ProviderStatus

// StatsResponse 统计响应
type StatsResponse = scheduler.SchedulerStats

// MetricsResponse 指标响应
type MetricsResponse struct {
	Server    ServerMetrics               `json:"server"`
	Scheduler *scheduler.SchedulerStats   `json:"scheduler"`
	Providers map[string]*ProviderMetrics `json:"providers"`
}

// ServerMetrics 服务器指标
type ServerMetrics struct {
	Uptime            string `json:"uptime"`
	RequestCount      int64  `json:"request_count"`
	ErrorCount        int64  `json:"error_count"`
	ActiveConnections int    `json:"active_connections"`
	MemoryUsage       int64  `json:"memory_usage_bytes"`
	GoroutineCount    int    `json:"goroutine_count"`
}

// ProviderMetrics 提供商指标
type ProviderMetrics struct {
	Name           string  `json:"name"`
	RequestCount   int64   `json:"request_count"`
	SuccessCount   int64   `json:"success_count"`
	ErrorCount     int64   `json:"error_count"`
	SuccessRate    float64 `json:"success_rate"`
	AverageLatency string  `json:"average_latency"`
	TotalCost      float64 `json:"total_cost"`
}

// HealthResponse 健康检查响应
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Service   string `json:"service"`
	Version   string `json:"version"`
}

// ReadyResponse 就绪检查响应
type ReadyResponse struct {
	Status             string `json:"status"`
	Timestamp          string `json:"timestamp"`
	AvailableProviders int    `json:"available_providers"`
	TotalTasks         int64  `json:"total_tasks"`
	RunningTasks       int    `json:"running_tasks"`
	Reason             string `json:"reason,omitempty"`
}

// ErrorResponse 错误响应
type ErrorResponse struct {
	Error     string      `json:"error"`
	Code      string      `json:"code,omitempty"`
	Details   interface{} `json:"details,omitempty"`
	Timestamp string      `json:"timestamp"`
}

// WebSocketMessage WebSocket消息
type WebSocketMessage struct {
	Type string      `json:"type"` // message类型：task_update, system_event等
	Data interface{} `json:"data"` // 消息数据
}

// 常用的API响应辅助函数

// SuccessResponse 成功响应
func SuccessResponse(data interface{}) map[string]interface{} {
	return map[string]interface{}{
		"success": true,
		"data":    data,
	}
}

// ErrorResponseFromError 从错误创建错误响应
func ErrorResponseFromError(err error) ErrorResponse {
	return ErrorResponse{
		Error:     err.Error(),
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

// ValidationErrorResponse 验证错误响应
func ValidationErrorResponse(field, message string) ErrorResponse {
	return ErrorResponse{
		Error:     "validation_error",
		Code:      "VALIDATION_ERROR",
		Details:   map[string]string{field: message},
		Timestamp: time.Now().Format(time.RFC3339),
	}
}
