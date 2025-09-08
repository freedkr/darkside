// Package scheduler 并发管理器实现
package scheduler

import (
	"sync"
	"time"

	"github.com/freedkr/moonshot/services/llm-service/internal/models"
)

// ConcurrencyManager 并发管理器
type ConcurrencyManager struct {
	// 全局并发限制
	globalSemaphore chan struct{}
	globalLimit     int
	
	// 按提供商的并发限制
	providerSemaphores map[string]chan struct{}
	providerLimits     map[string]int
	providerMutex      sync.RWMutex
	
	// 按任务类型的并发限制
	taskTypeSemaphores map[models.LLMTaskType]chan struct{}
	taskTypeLimits     map[models.LLMTaskType]int
	taskTypeMutex      sync.RWMutex
	
	// 动态并发控制
	adaptiveController *AdaptiveConcurrencyController
	
	// 统计信息
	stats        *ConcurrencyStats
	statsMutex   sync.RWMutex
}

// ConcurrencyStats 并发统计
type ConcurrencyStats struct {
	GlobalConcurrent     int                               `json:"global_concurrent"`
	ProviderConcurrent   map[string]int                   `json:"provider_concurrent"`
	TaskTypeConcurrent   map[models.LLMTaskType]int       `json:"task_type_concurrent"`
	MaxGlobalConcurrent  int                               `json:"max_global_concurrent"`
	TotalAcquired        int64                             `json:"total_acquired"`
	TotalReleased        int64                             `json:"total_released"`
	AverageWaitTime      time.Duration                     `json:"average_wait_time"`
}

// NewConcurrencyManager 创建新的并发管理器
func NewConcurrencyManager() *ConcurrencyManager {
	return &ConcurrencyManager{
		providerSemaphores: make(map[string]chan struct{}),
		providerLimits:     make(map[string]int),
		taskTypeSemaphores: make(map[models.LLMTaskType]chan struct{}),
		taskTypeLimits:     make(map[models.LLMTaskType]int),
		stats: &ConcurrencyStats{
			ProviderConcurrent: make(map[string]int),
			TaskTypeConcurrent: make(map[models.LLMTaskType]int),
		},
	}
}

// SetGlobalLimit 设置全局并发限制
func (cm *ConcurrencyManager) SetGlobalLimit(limit int) {
	if limit <= 0 {
		cm.globalSemaphore = nil
		cm.globalLimit = 0
		return
	}
	
	cm.globalSemaphore = make(chan struct{}, limit)
	cm.globalLimit = limit
}

// SetProviderLimit 设置提供商并发限制
func (cm *ConcurrencyManager) SetProviderLimit(provider string, limit int) {
	cm.providerMutex.Lock()
	defer cm.providerMutex.Unlock()
	
	if limit <= 0 {
		delete(cm.providerSemaphores, provider)
		delete(cm.providerLimits, provider)
		return
	}
	
	cm.providerSemaphores[provider] = make(chan struct{}, limit)
	cm.providerLimits[provider] = limit
}

// SetTaskTypeLimit 设置任务类型并发限制
func (cm *ConcurrencyManager) SetTaskTypeLimit(taskType models.LLMTaskType, limit int) {
	cm.taskTypeMutex.Lock()
	defer cm.taskTypeMutex.Unlock()
	
	if limit <= 0 {
		delete(cm.taskTypeSemaphores, taskType)
		delete(cm.taskTypeLimits, taskType)
		return
	}
	
	cm.taskTypeSemaphores[taskType] = make(chan struct{}, limit)
	cm.taskTypeLimits[taskType] = limit
}

// Acquire 获取并发许可
func (cm *ConcurrencyManager) Acquire(provider string, taskType models.LLMTaskType) (*ConcurrencyToken, error) {
	startTime := time.Now()
	
	token := &ConcurrencyToken{
		manager:   cm,
		provider:  provider,
		taskType:  taskType,
		acquired:  make([]string, 0, 3),
		startTime: startTime,
	}
	
	// 获取全局许可
	if cm.globalSemaphore != nil {
		select {
		case cm.globalSemaphore <- struct{}{}:
			token.acquired = append(token.acquired, "global")
		default:
			return nil, &ConcurrencyError{
				Type:    "global_limit_exceeded",
				Message: "全局并发限制已达到",
			}
		}
	}
	
	// 获取提供商许可
	if sem := cm.getProviderSemaphore(provider); sem != nil {
		select {
		case sem <- struct{}{}:
			token.acquired = append(token.acquired, "provider:"+provider)
		default:
			token.release()
			return nil, &ConcurrencyError{
				Type:    "provider_limit_exceeded",
				Message: "提供商 " + provider + " 并发限制已达到",
			}
		}
	}
	
	// 获取任务类型许可
	if sem := cm.getTaskTypeSemaphore(taskType); sem != nil {
		select {
		case sem <- struct{}{}:
			token.acquired = append(token.acquired, "task_type:"+string(taskType))
		default:
			token.release()
			return nil, &ConcurrencyError{
				Type:    "task_type_limit_exceeded",
				Message: "任务类型 " + string(taskType) + " 并发限制已达到",
			}
		}
	}
	
	// 更新统计
	cm.updateStats(provider, taskType, true, time.Since(startTime))
	
	return token, nil
}

// getProviderSemaphore 获取提供商信号量
func (cm *ConcurrencyManager) getProviderSemaphore(provider string) chan struct{} {
	cm.providerMutex.RLock()
	defer cm.providerMutex.RUnlock()
	
	return cm.providerSemaphores[provider]
}

// getTaskTypeSemaphore 获取任务类型信号量
func (cm *ConcurrencyManager) getTaskTypeSemaphore(taskType models.LLMTaskType) chan struct{} {
	cm.taskTypeMutex.RLock()
	defer cm.taskTypeMutex.RUnlock()
	
	return cm.taskTypeSemaphores[taskType]
}

// updateStats 更新统计信息
func (cm *ConcurrencyManager) updateStats(provider string, taskType models.LLMTaskType, acquired bool, waitTime time.Duration) {
	cm.statsMutex.Lock()
	defer cm.statsMutex.Unlock()
	
	if acquired {
		cm.stats.GlobalConcurrent++
		if cm.stats.GlobalConcurrent > cm.stats.MaxGlobalConcurrent {
			cm.stats.MaxGlobalConcurrent = cm.stats.GlobalConcurrent
		}
		
		cm.stats.ProviderConcurrent[provider]++
		cm.stats.TaskTypeConcurrent[taskType]++
		cm.stats.TotalAcquired++
		
		// 更新平均等待时间
		if cm.stats.TotalAcquired > 1 {
			cm.stats.AverageWaitTime = (cm.stats.AverageWaitTime*time.Duration(cm.stats.TotalAcquired-1) + waitTime) / time.Duration(cm.stats.TotalAcquired)
		} else {
			cm.stats.AverageWaitTime = waitTime
		}
	} else {
		cm.stats.GlobalConcurrent--
		cm.stats.ProviderConcurrent[provider]--
		cm.stats.TaskTypeConcurrent[taskType]--
		cm.stats.TotalReleased++
	}
}

// GetStats 获取并发统计
func (cm *ConcurrencyManager) GetStats() *ConcurrencyStats {
	cm.statsMutex.RLock()
	defer cm.statsMutex.RUnlock()
	
	// 返回副本
	stats := *cm.stats
	stats.ProviderConcurrent = make(map[string]int)
	stats.TaskTypeConcurrent = make(map[models.LLMTaskType]int)
	
	for k, v := range cm.stats.ProviderConcurrent {
		stats.ProviderConcurrent[k] = v
	}
	for k, v := range cm.stats.TaskTypeConcurrent {
		stats.TaskTypeConcurrent[k] = v
	}
	
	return &stats
}

// ConcurrencyToken 并发令牌
type ConcurrencyToken struct {
	manager   *ConcurrencyManager
	provider  string
	taskType  models.LLMTaskType
	acquired  []string
	startTime time.Time
	released  bool
	mutex     sync.Mutex
}

// Release 释放并发许可
func (ct *ConcurrencyToken) Release() {
	ct.mutex.Lock()
	defer ct.mutex.Unlock()
	
	if ct.released {
		return
	}
	
	ct.release()
	ct.released = true
	
	// 更新统计
	ct.manager.updateStats(ct.provider, ct.taskType, false, 0)
}

// release 内部释放方法
func (ct *ConcurrencyToken) release() {
	for _, acquired := range ct.acquired {
		switch {
		case acquired == "global":
			if ct.manager.globalSemaphore != nil {
				<-ct.manager.globalSemaphore
			}
		case acquired[:9] == "provider:":
			provider := acquired[9:]
			if sem := ct.manager.getProviderSemaphore(provider); sem != nil {
				<-sem
			}
		case acquired[:10] == "task_type:":
			taskType := models.LLMTaskType(acquired[10:])
			if sem := ct.manager.getTaskTypeSemaphore(taskType); sem != nil {
				<-sem
			}
		}
	}
	ct.acquired = ct.acquired[:0]
}

// GetDuration 获取持有令牌的时长
func (ct *ConcurrencyToken) GetDuration() time.Duration {
	return time.Since(ct.startTime)
}

// ConcurrencyError 并发错误
type ConcurrencyError struct {
	Type    string
	Message string
}

func (e *ConcurrencyError) Error() string {
	return e.Message
}

// AdaptiveConcurrencyController 自适应并发控制器
type AdaptiveConcurrencyController struct {
	enabled            bool
	currentLimits      map[string]int
	successRates       map[string]float64
	avgLatencies       map[string]time.Duration
	adjustInterval     time.Duration
	minConcurrency     int
	maxConcurrency     int
	targetSuccessRate  float64
	targetLatency      time.Duration
	
	mutex              sync.RWMutex
	lastAdjustment     time.Time
}

// NewAdaptiveConcurrencyController 创建自适应并发控制器
func NewAdaptiveConcurrencyController(enabled bool) *AdaptiveConcurrencyController {
	return &AdaptiveConcurrencyController{
		enabled:           enabled,
		currentLimits:     make(map[string]int),
		successRates:      make(map[string]float64),
		avgLatencies:      make(map[string]time.Duration),
		adjustInterval:    30 * time.Second,
		minConcurrency:    1,
		maxConcurrency:    50,
		targetSuccessRate: 0.95,
		targetLatency:     5 * time.Second,
	}
}

// AdjustLimits 调整并发限制
func (acc *AdaptiveConcurrencyController) AdjustLimits(metrics map[string]ProviderMetrics) {
	if !acc.enabled {
		return
	}
	
	acc.mutex.Lock()
	defer acc.mutex.Unlock()
	
	now := time.Now()
	if now.Sub(acc.lastAdjustment) < acc.adjustInterval {
		return
	}
	
	for provider, metric := range metrics {
		currentLimit := acc.currentLimits[provider]
		if currentLimit == 0 {
			currentLimit = 2 // 默认起始值
		}
		
		successRate := float64(metric.SuccessCount) / float64(metric.RequestCount)
		avgLatency := metric.AverageLatency
		
		// 存储指标
		acc.successRates[provider] = successRate
		acc.avgLatencies[provider] = avgLatency
		
		// 根据成功率和延迟调整并发数
		newLimit := currentLimit
		
		if successRate < acc.targetSuccessRate {
			// 成功率低，减少并发数
			newLimit = max(acc.minConcurrency, currentLimit-1)
		} else if avgLatency > acc.targetLatency {
			// 延迟高，减少并发数
			newLimit = max(acc.minConcurrency, currentLimit-1)
		} else if successRate > acc.targetSuccessRate && avgLatency < acc.targetLatency {
			// 性能良好，可以增加并发数
			newLimit = min(acc.maxConcurrency, currentLimit+1)
		}
		
		acc.currentLimits[provider] = newLimit
	}
	
	acc.lastAdjustment = now
}

// GetCurrentLimit 获取当前并发限制
func (acc *AdaptiveConcurrencyController) GetCurrentLimit(provider string) int {
	acc.mutex.RLock()
	defer acc.mutex.RUnlock()
	
	return acc.currentLimits[provider]
}

// ProviderMetrics 提供商指标（简化版）
type ProviderMetrics struct {
	RequestCount   int64
	SuccessCount   int64
	AverageLatency time.Duration
}

// 辅助函数
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}