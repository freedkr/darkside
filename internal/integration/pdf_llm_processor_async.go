package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/freedkr/moonshot/internal/model"
)

// LLMTaskRequest LLM任务请求
type LLMTaskRequest struct {
	TaskType   string                 `json:"type"`
	Prompt     string                 `json:"prompt"`
	Model      string                 `json:"model,omitempty"`
	Priority   string                 `json:"priority,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Callback   *CallbackConfig        `json:"callback,omitempty"`
}

// CallbackConfig 回调配置
type CallbackConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// LLMTaskResponse LLM任务响应
type LLMTaskResponse struct {
	TaskID    string    `json:"task_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// LLMTaskStatus LLM任务状态
type LLMTaskStatus struct {
	TaskID      string                 `json:"task_id"`
	Status      string                 `json:"status"`
	Result      interface{}            `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Progress    float64                `json:"progress"`
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// callLLMServiceAsync 异步调用LLM服务
func (p *PDFLLMProcessor) callLLMServiceAsync(ctx context.Context, taskType string, prompt string) (string, error) {
	fmt.Printf("📨 DEBUG: callLLMServiceAsync 开始 - taskType: %s, prompt长度: %d\n", taskType, len(prompt))
	
	// 1. 提交任务到LLM服务
	fmt.Printf("📨 DEBUG: callLLMServiceAsync 步骤1: 提交任务到LLM服务\n")
	taskID, err := p.submitLLMTask(ctx, taskType, prompt)
	if err != nil {
		fmt.Printf("❌ DEBUG: callLLMServiceAsync 提交任务失败: %v\n", err)
		return "", fmt.Errorf("提交LLM任务失败: %w", err)
	}
	fmt.Printf("✅ DEBUG: callLLMServiceAsync 任务提交成功 - taskID: %s\n", taskID)

	// 2. 轮询等待任务完成
	fmt.Printf("📨 DEBUG: callLLMServiceAsync 步骤2: 开始等待任务完成 - taskID: %s\n", taskID)
	result, err := p.waitForLLMResult(ctx, taskID)
	if err != nil {
		fmt.Printf("❌ DEBUG: callLLMServiceAsync 等待结果失败: %v\n", err)
		return "", fmt.Errorf("等待LLM结果失败: %w", err)
	}
	
	fmt.Printf("✅ DEBUG: callLLMServiceAsync 完成 - taskID: %s, 结果长度: %d\n", taskID, len(result))
	return result, nil
}

// submitLLMTask 提交任务到LLM服务
func (p *PDFLLMProcessor) submitLLMTask(ctx context.Context, taskType string, prompt string) (string, error) {
	fmt.Printf("📤 DEBUG: submitLLMTask 开始 - taskType: %s, prompt长度: %d\n", taskType, len(prompt))
	
	reqBody := LLMTaskRequest{
		TaskType: taskType,
		Prompt:   prompt,
		// Model:    "moonshot-v1-128k", // 使用128K token的模型
		Priority: "normal", // 普通优先级（字符串类型）
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Printf("❌ DEBUG: submitLLMTask JSON序列化失败: %v\n", err)
		return "", err
	}

	url := fmt.Sprintf("http://%s/api/v1/tasks", p.llmServiceURL)
	fmt.Printf("📤 DEBUG: submitLLMTask 准备发送POST请求到: %s\n", url)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("❌ DEBUG: submitLLMTask 创建请求失败: %v\n", err)
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	fmt.Printf("📤 DEBUG: submitLLMTask 开始发送HTTP请求\n")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		fmt.Printf("❌ DEBUG: submitLLMTask HTTP请求失败: %v\n", err)
		return "", fmt.Errorf("调用LLM服务失败: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("📤 DEBUG: submitLLMTask 收到响应 - StatusCode: %d\n", resp.StatusCode)
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ DEBUG: submitLLMTask 响应状态错误 %d: %s\n", resp.StatusCode, string(body))
		return "", fmt.Errorf("LLM服务返回错误 %d: %s", resp.StatusCode, string(body))
	}

	var taskResp LLMTaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&taskResp); err != nil {
		fmt.Printf("❌ DEBUG: submitLLMTask 解析响应失败: %v\n", err)
		return "", err
	}

	fmt.Printf("✅ DEBUG: submitLLMTask 成功 - taskID: %s\n", taskResp.TaskID)
	return taskResp.TaskID, nil
}

// waitForLLMResult 等待LLM任务完成
func (p *PDFLLMProcessor) waitForLLMResult(ctx context.Context, taskID string) (string, error) {
	fmt.Printf("⏳ DEBUG: waitForLLMResult 开始等待 - taskID: %s\n", taskID)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// 最长等待5分钟
	timeout := time.After(5 * time.Minute)
	checkCount := 0

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("🚫 DEBUG: waitForLLMResult 上下文取消 - taskID: %s, 检查次数: %d\n", taskID, checkCount)
			return "", ctx.Err()
		case <-timeout:
			fmt.Printf("⏰ DEBUG: waitForLLMResult 超时 - taskID: %s, 检查次数: %d\n", taskID, checkCount)
			return "", fmt.Errorf("等待LLM任务超时")
		case <-ticker.C:
			checkCount++
			fmt.Printf("🔍 DEBUG: waitForLLMResult 第%d次检查状态 - taskID: %s\n", checkCount, taskID)
			status, err := p.checkLLMTaskStatus(ctx, taskID)
			if err != nil {
				fmt.Printf("❌ DEBUG: waitForLLMResult 检查状态失败 - taskID: %s, 错误: %v\n", taskID, err)
				// 检查失败，继续重试
				continue
			}

			fmt.Printf("📊 DEBUG: waitForLLMResult 任务状态 - taskID: %s, 状态: %s, 进度: %.2f\n", taskID, status.Status, status.Progress)

			switch status.Status {
			case "completed", "success":
				// status.Result 已经是字符串格式的JSON，直接转换即可
				var resultStr string
				if status.Result == nil {
					resultStr = "{}"
				} else if str, ok := status.Result.(string); ok {
					// Result 是字符串类型，直接使用
					resultStr = str
				} else {
					// Result 是其他类型，需要序列化（兼容处理）
					resultJSON, err := json.Marshal(status.Result)
					if err != nil {
						fmt.Printf("❌ DEBUG: waitForLLMResult 结果序列化失败 - taskID: %s, 错误: %v\n", taskID, err)
						return "", fmt.Errorf("结果序列化失败: %w", err)
					}
					resultStr = string(resultJSON)
				}
				fmt.Printf("✅ DEBUG: waitForLLMResult 任务完成 - taskID: %s, 结果长度: %d\n", taskID, len(resultStr))
				return resultStr, nil
			case "failed", "error":
				if status.Error != "" {
					fmt.Printf("💥 DEBUG: waitForLLMResult 任务失败 - taskID: %s, 错误: %s\n", taskID, status.Error)
					return "", fmt.Errorf("LLM任务失败: %s", status.Error)
				}
				fmt.Printf("💥 DEBUG: waitForLLMResult 任务失败 - taskID: %s, 未知错误\n", taskID)
				return "", fmt.Errorf("LLM任务失败")
			case "cancelled":
				fmt.Printf("🚫 DEBUG: waitForLLMResult 任务被取消 - taskID: %s\n", taskID)
				return "", fmt.Errorf("LLM任务被取消")
			// pending, queued, processing 状态继续等待
			default:
				fmt.Printf("⏳ DEBUG: waitForLLMResult 任务进行中 - taskID: %s, 状态: %s\n", taskID, status.Status)
				continue
			}
		}
	}
}

// checkLLMTaskStatus 检查LLM任务状态
func (p *PDFLLMProcessor) checkLLMTaskStatus(ctx context.Context, taskID string) (*LLMTaskStatus, error) {
	url := fmt.Sprintf("http://%s/api/v1/tasks/%s", p.llmServiceURL, taskID)
	fmt.Printf("🔍 DEBUG: checkLLMTaskStatus 检查任务状态 - URL: %s\n", url)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		fmt.Printf("❌ DEBUG: checkLLMTaskStatus 创建请求失败: %v\n", err)
		return nil, err
	}

	fmt.Printf("🔍 DEBUG: checkLLMTaskStatus 发送GET请求\n")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		fmt.Printf("❌ DEBUG: checkLLMTaskStatus HTTP请求失败: %v\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	fmt.Printf("🔍 DEBUG: checkLLMTaskStatus 收到响应 - StatusCode: %d\n", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ DEBUG: checkLLMTaskStatus 响应状态错误 %d: %s\n", resp.StatusCode, string(body))
		return nil, fmt.Errorf("获取任务状态失败: %d", resp.StatusCode)
	}

	var status LLMTaskStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		fmt.Printf("❌ DEBUG: checkLLMTaskStatus 解析响应失败: %v\n", err)
		return nil, err
	}

	fmt.Printf("✅ DEBUG: checkLLMTaskStatus 成功 - taskID: %s, 状态: %s\n", taskID, status.Status)
	return &status, nil
}

// callLLMServiceWithRetry 带重试的LLM服务调用
func (p *PDFLLMProcessor) callLLMServiceWithRetry(ctx context.Context, taskType string, prompt string, maxRetries int) (string, error) {
	fmt.Printf("🔄 DEBUG: callLLMServiceWithRetry 开始 - taskType: %s, maxRetries: %d\n", taskType, maxRetries)
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		fmt.Printf("🔄 DEBUG: callLLMServiceWithRetry 第%d次尝试\n", i+1)
		
		if i > 0 {
			// 指数退避
			backoff := time.Duration(i*i) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			fmt.Printf("🔄 DEBUG: callLLMServiceWithRetry 等待 %v 后重试\n", backoff)
			time.Sleep(backoff)
		}

		fmt.Printf("🔄 DEBUG: callLLMServiceWithRetry 调用 callLLMServiceAsync\n")
		result, err := p.callLLMServiceAsync(ctx, taskType, prompt)
		if err == nil {
			fmt.Printf("✅ DEBUG: callLLMServiceWithRetry 成功获取结果, 长度: %d\n", len(result))
			return result, nil
		}

		fmt.Printf("❌ DEBUG: callLLMServiceWithRetry 第%d次失败: %v\n", i+1, err)
		lastErr = err
		// 如果是上下文取消，立即返回
		if ctx.Err() != nil {
			fmt.Printf("🚫 DEBUG: callLLMServiceWithRetry 上下文取消: %v\n", ctx.Err())
			return "", ctx.Err()
		}
	}

	fmt.Printf("💥 DEBUG: callLLMServiceWithRetry 所有重试失败\n")
	return "", fmt.Errorf("LLM服务调用失败（重试%d次）: %w", maxRetries, lastErr)
}

// ProcessWithCallback 带回调的处理
func (p *PDFLLMProcessor) ProcessWithCallback(ctx context.Context, taskID string, excelPath string, categories []*model.Category, callbackURL string) error {
	// 创建一个goroutine处理，立即返回
	go func() {
		// 使用新的context，避免请求context取消影响处理
		processCtx := context.Background()

		err := p.ProcessWithPDFAndLLM(processCtx, taskID, excelPath, categories)

		// 回调通知处理结果
		callbackData := map[string]interface{}{
			"task_id": taskID,
			"status":  "completed",
			"error":   "",
		}

		if err != nil {
			callbackData["status"] = "failed"
			callbackData["error"] = err.Error()
		}

		p.sendCallback(callbackURL, callbackData)
	}()

	return nil
}

// sendCallback 发送回调通知
func (p *PDFLLMProcessor) sendCallback(callbackURL string, data map[string]interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", callbackURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
