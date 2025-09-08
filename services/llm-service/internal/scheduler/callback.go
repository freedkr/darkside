// Package scheduler 回调处理器实现
package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/freedkr/moonshot/services/llm-service/internal/models"
)

// CallbackHandler 回调处理器接口
type CallbackHandler interface {
	// 任务生命周期回调
	OnTaskStarted(task *models.LLMTask)
	OnTaskProgress(task *models.LLMTask, progress float64, message string)
	OnTaskCompleted(task *models.LLMTask)
	OnTaskFailed(task *models.LLMTask, err error)
	
	// 注册回调监听器
	RegisterListener(listener CallbackListener)
	UnregisterListener(listener CallbackListener)
	
	// 生命周期管理
	Start() error
	Stop() error
}

// CallbackListener 回调监听器接口
type CallbackListener interface {
	OnCallback(event *models.CallbackEvent)
}

// DefaultCallbackHandler 默认回调处理器
type DefaultCallbackHandler struct {
	// HTTP客户端用于Webhook回调
	httpClient *http.Client
	
	// 内存回调监听器
	listeners []CallbackListener
	listenersMutex sync.RWMutex
	
	// 回调队列
	eventQueue chan *models.CallbackEvent
	queueSize  int
	
	// 重试配置
	maxRetries int
	retryDelay time.Duration
	
	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	
	// 统计
	stats      *CallbackStats
	statsMutex sync.RWMutex
}

// CallbackStats 回调统计
type CallbackStats struct {
	TotalEvents        int64     `json:"total_events"`
	SuccessfulWebhooks int64     `json:"successful_webhooks"`
	FailedWebhooks     int64     `json:"failed_webhooks"`
	RetryCount         int64     `json:"retry_count"`
	AverageLatency     time.Duration `json:"average_latency"`
	QueueLength        int       `json:"queue_length"`
	LastProcessed      time.Time `json:"last_processed"`
}

// NewDefaultCallbackHandler 创建默认回调处理器
func NewDefaultCallbackHandler() *DefaultCallbackHandler {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &DefaultCallbackHandler{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		listeners:  make([]CallbackListener, 0),
		eventQueue: make(chan *models.CallbackEvent, 1000), // 默认队列大小
		queueSize:  1000,
		maxRetries: 3,
		retryDelay: time.Second,
		ctx:        ctx,
		cancel:     cancel,
		stats:      &CallbackStats{},
	}
}

// OnTaskStarted 任务开始回调
func (h *DefaultCallbackHandler) OnTaskStarted(task *models.LLMTask) {
	event := &models.CallbackEvent{
		EventType: "started",
		TaskID:    task.ID,
		Status:    task.Status,
		Progress:  0.0,
		Timestamp: time.Now(),
	}
	
	h.sendEvent(event)
}

// OnTaskProgress 任务进度回调
func (h *DefaultCallbackHandler) OnTaskProgress(task *models.LLMTask, progress float64, message string) {
	event := &models.CallbackEvent{
		EventType: "progress",
		TaskID:    task.ID,
		Status:    task.Status,
		Progress:  progress,
		Data:      message,
		Timestamp: time.Now(),
	}
	
	h.sendEvent(event)
}

// OnTaskCompleted 任务完成回调
func (h *DefaultCallbackHandler) OnTaskCompleted(task *models.LLMTask) {
	// 获取结果数据
	var resultData interface{}
	if len(task.Result) > 0 {
		json.Unmarshal(task.Result, &resultData)
	}
	
	event := &models.CallbackEvent{
		EventType: "completed",
		TaskID:    task.ID,
		Status:    task.Status,
		Progress:  100.0,
		Data:      resultData,
		Metadata: map[string]interface{}{
			"token_usage": task.TokenUsage,
			"duration":    task.GetDuration().String(),
		},
		Timestamp: time.Now(),
	}
	
	h.sendEvent(event)
}

// OnTaskFailed 任务失败回调
func (h *DefaultCallbackHandler) OnTaskFailed(task *models.LLMTask, err error) {
	event := &models.CallbackEvent{
		EventType: "failed",
		TaskID:    task.ID,
		Status:    task.Status,
		Error:     err.Error(),
		Timestamp: time.Now(),
	}
	
	h.sendEvent(event)
}

// RegisterListener 注册回调监听器
func (h *DefaultCallbackHandler) RegisterListener(listener CallbackListener) {
	h.listenersMutex.Lock()
	defer h.listenersMutex.Unlock()
	
	h.listeners = append(h.listeners, listener)
}

// UnregisterListener 注销回调监听器
func (h *DefaultCallbackHandler) UnregisterListener(listener CallbackListener) {
	h.listenersMutex.Lock()
	defer h.listenersMutex.Unlock()
	
	for i, l := range h.listeners {
		if l == listener {
			h.listeners = append(h.listeners[:i], h.listeners[i+1:]...)
			break
		}
	}
}

// Start 启动回调处理器
func (h *DefaultCallbackHandler) Start() error {
	// 启动事件处理循环
	h.wg.Add(1)
	go h.eventProcessingLoop()
	
	return nil
}

// Stop 停止回调处理器
func (h *DefaultCallbackHandler) Stop() error {
	h.cancel()
	
	// 等待处理完成
	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("停止回调处理器超时")
	}
}

// sendEvent 发送事件
func (h *DefaultCallbackHandler) sendEvent(event *models.CallbackEvent) {
	select {
	case h.eventQueue <- event:
		h.updateStats(func(stats *CallbackStats) {
			stats.TotalEvents++
			stats.QueueLength = len(h.eventQueue)
		})
	default:
		// 队列满了，记录错误
		log.Printf("回调事件队列已满，丢弃事件: %s", event.TaskID)
	}
}

// eventProcessingLoop 事件处理循环
func (h *DefaultCallbackHandler) eventProcessingLoop() {
	defer h.wg.Done()
	
	for {
		select {
		case <-h.ctx.Done():
			return
		case event := <-h.eventQueue:
			h.processEvent(event)
		}
	}
}

// processEvent 处理事件
func (h *DefaultCallbackHandler) processEvent(event *models.CallbackEvent) {
	startTime := time.Now()
	
	// 通知内存监听器
	h.notifyListeners(event)
	
	// 发送Webhook回调（如果有配置）
	// 这里需要从任务中获取回调URL，简化实现先跳过
	// h.sendWebhook(event)
	
	// 更新统计
	processTime := time.Since(startTime)
	h.updateStats(func(stats *CallbackStats) {
		stats.LastProcessed = time.Now()
		stats.QueueLength = len(h.eventQueue)
		
		// 更新平均延迟
		if stats.TotalEvents > 1 {
			stats.AverageLatency = (stats.AverageLatency*time.Duration(stats.TotalEvents-1) + processTime) / time.Duration(stats.TotalEvents)
		} else {
			stats.AverageLatency = processTime
		}
	})
}

// notifyListeners 通知内存监听器
func (h *DefaultCallbackHandler) notifyListeners(event *models.CallbackEvent) {
	h.listenersMutex.RLock()
	listeners := make([]CallbackListener, len(h.listeners))
	copy(listeners, h.listeners)
	h.listenersMutex.RUnlock()
	
	for _, listener := range listeners {
		go func(l CallbackListener) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("回调监听器panic: %v", r)
				}
			}()
			l.OnCallback(event)
		}(listener)
	}
}

// sendWebhook 发送Webhook回调
func (h *DefaultCallbackHandler) sendWebhook(event *models.CallbackEvent, webhookURL, authToken string) {
	if webhookURL == "" {
		return
	}
	
	// 序列化事件数据
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("序列化回调事件失败: %v", err)
		return
	}
	
	// 尝试发送，包含重试机制
	for retry := 0; retry <= h.maxRetries; retry++ {
		if h.sendWebhookOnce(webhookURL, authToken, data) {
			h.updateStats(func(stats *CallbackStats) {
				stats.SuccessfulWebhooks++
				if retry > 0 {
					stats.RetryCount += int64(retry)
				}
			})
			return
		}
		
		if retry < h.maxRetries {
			time.Sleep(h.retryDelay * time.Duration(retry+1)) // 指数退避
		}
	}
	
	// 所有重试都失败了
	h.updateStats(func(stats *CallbackStats) {
		stats.FailedWebhooks++
		stats.RetryCount += int64(h.maxRetries)
	})
	
	log.Printf("Webhook回调最终失败: %s", webhookURL)
}

// sendWebhookOnce 发送单次Webhook请求
func (h *DefaultCallbackHandler) sendWebhookOnce(url, authToken string, data []byte) bool {
	req, err := http.NewRequestWithContext(h.ctx, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		log.Printf("创建Webhook请求失败: %v", err)
		return false
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LLM-Service-Webhook/1.0")
	
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}
	
	resp, err := h.httpClient.Do(req)
	if err != nil {
		log.Printf("发送Webhook请求失败: %v", err)
		return false
	}
	defer resp.Body.Close()
	
	// 检查响应状态码
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true
	}
	
	log.Printf("Webhook请求返回错误状态码: %d", resp.StatusCode)
	return false
}

// updateStats 更新统计
func (h *DefaultCallbackHandler) updateStats(updater func(*CallbackStats)) {
	h.statsMutex.Lock()
	defer h.statsMutex.Unlock()
	
	updater(h.stats)
}

// GetStats 获取统计信息
func (h *DefaultCallbackHandler) GetStats() *CallbackStats {
	h.statsMutex.RLock()
	defer h.statsMutex.RUnlock()
	
	// 返回副本
	stats := *h.stats
	return &stats
}

// WebSocketCallbackListener WebSocket回调监听器
type WebSocketCallbackListener struct {
	connections map[string]WebSocketConnection
	mutex       sync.RWMutex
}

// WebSocketConnection WebSocket连接接口
type WebSocketConnection interface {
	WriteJSON(v interface{}) error
	Close() error
}

// NewWebSocketCallbackListener 创建WebSocket回调监听器
func NewWebSocketCallbackListener() *WebSocketCallbackListener {
	return &WebSocketCallbackListener{
		connections: make(map[string]WebSocketConnection),
	}
}

// OnCallback 处理回调事件
func (w *WebSocketCallbackListener) OnCallback(event *models.CallbackEvent) {
	w.mutex.RLock()
	connections := make(map[string]WebSocketConnection)
	for k, v := range w.connections {
		connections[k] = v
	}
	w.mutex.RUnlock()
	
	// 广播给所有连接
	for connID, conn := range connections {
		go func(id string, c WebSocketConnection) {
			if err := c.WriteJSON(event); err != nil {
				log.Printf("WebSocket发送失败 [%s]: %v", id, err)
				// 移除失效连接
				w.RemoveConnection(id)
			}
		}(connID, conn)
	}
}

// AddConnection 添加WebSocket连接
func (w *WebSocketCallbackListener) AddConnection(connID string, conn WebSocketConnection) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	
	w.connections[connID] = conn
}

// RemoveConnection 移除WebSocket连接
func (w *WebSocketCallbackListener) RemoveConnection(connID string) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	
	if conn, exists := w.connections[connID]; exists {
		conn.Close()
		delete(w.connections, connID)
	}
}