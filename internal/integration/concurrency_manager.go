package integration

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ConcurrencyManager 并发管理接口
type ConcurrencyManager interface {
	GetOptimalConcurrency(taskType string) int
	AcquirePermit(ctx context.Context, taskType string) error
	ReleasePermit(taskType string)
	UpdateMetrics(taskType string, metrics TaskMetrics)
	GetStatus() ConcurrencyStatus
}

// QuotaAwareConcurrencyManager 配额感知的并发管理器
type QuotaAwareConcurrencyManager struct {
	config        ConcurrencyConfig
	requestTimers map[string]*RequestTimer
	semaphores    map[string]chan struct{}
	metrics       map[string]*AdaptiveMetrics
	mutex         sync.RWMutex
}

// RequestTimer 请求时间控制器
type RequestTimer struct {
	interval    time.Duration
	lastRequest time.Time
	mutex       sync.Mutex
}

// ConcurrencyConfig 并发配置
type ConcurrencyConfig struct {
	// 全局配额约束
	GlobalQuotas struct {
		MaxRPM        int `yaml:"max_rpm"`        // 500 RPM
		MaxConcurrent int `yaml:"max_concurrent"` // 100
		MaxTPM        int `yaml:"max_tpm"`        // 128,000
	} `yaml:"global_quotas"`

	// 任务类型配额分配
	TaskAllocations map[string]TaskAllocation `yaml:"task_allocations"`

	// 自适应参数
	Adaptive struct {
		EnableAdaptive     bool          `yaml:"enable_adaptive"`
		AdjustmentInterval time.Duration `yaml:"adjustment_interval"`
		MinSuccessRate     float64       `yaml:"min_success_rate"`
		MaxErrorRate       float64       `yaml:"max_error_rate"`
	} `yaml:"adaptive"`
}

// TaskAllocation 任务配额分配
type TaskAllocation struct {
	RPMPercent      float64       `yaml:"rpm_percent"`      // RPM配额百分比
	MaxConcurrent   int           `yaml:"max_concurrent"`   // 最大并发数
	RequestInterval time.Duration `yaml:"request_interval"` // 请求间隔
	Priority        string        `yaml:"priority"`         // 优先级
	AdaptiveRange   AdaptiveRange `yaml:"adaptive_range"`   // 自适应范围
}

// AdaptiveRange 自适应调整范围
type AdaptiveRange struct {
	MinConcurrency     int     `yaml:"min_concurrency"`
	MaxConcurrency     int     `yaml:"max_concurrency"`
	ScaleUpThreshold   float64 `yaml:"scale_up_threshold"`   // 成功率阈值
	ScaleDownThreshold float64 `yaml:"scale_down_threshold"` // 错误率阈值
}

// TaskMetrics 任务执行指标
type TaskMetrics struct {
	Duration    time.Duration
	Success     bool
	ErrorType   string
	TokensUsed  int
	RequestSize int
}

// AdaptiveMetrics 自适应指标
type AdaptiveMetrics struct {
	SuccessRate        float64       `json:"success_rate"`
	ErrorRate          float64       `json:"error_rate"`
	AvgDuration        time.Duration `json:"avg_duration"`
	CurrentConcurrency int           `json:"current_concurrency"`
	TotalRequests      int64         `json:"total_requests"`
	RecentRequests     []TaskMetrics `json:"-"` // 滑动窗口
	LastAdjustment     time.Time     `json:"last_adjustment"`
	mutex              sync.RWMutex  `json:"-"`
}

// ConcurrencyStatus 并发状态
type ConcurrencyStatus struct {
	TaskStatuses map[string]TaskStatus `json:"task_statuses"`
	GlobalStatus GlobalStatus          `json:"global_status"`
}

type TaskStatus struct {
	CurrentConcurrency int           `json:"current_concurrency"`
	MaxConcurrency     int           `json:"max_concurrency"`
	SuccessRate        float64       `json:"success_rate"`
	AvgDuration        time.Duration `json:"avg_duration"`
	RPMUsage           float64       `json:"rpm_usage"`
}

type GlobalStatus struct {
	TotalRPMUsage    float64 `json:"total_rpm_usage"`
	TotalConcurrency int     `json:"total_concurrency"`
	MaxConcurrency   int     `json:"max_concurrency"`
}

// NewQuotaAwareConcurrencyManager 创建配额感知并发管理器
func NewQuotaAwareConcurrencyManager(config ConcurrencyConfig) *QuotaAwareConcurrencyManager {
	manager := &QuotaAwareConcurrencyManager{
		config:        config,
		requestTimers: make(map[string]*RequestTimer),
		semaphores:    make(map[string]chan struct{}),
		metrics:       make(map[string]*AdaptiveMetrics),
	}

	// 为每个任务类型初始化资源
	for taskType, allocation := range config.TaskAllocations {
		manager.initializeTaskResources(taskType, allocation)
	}

	// 启动自适应调整
	if config.Adaptive.EnableAdaptive {
		go manager.adaptiveAdjustmentLoop()
	}

	return manager
}

// initializeTaskResources 初始化任务资源
func (m *QuotaAwareConcurrencyManager) initializeTaskResources(taskType string, allocation TaskAllocation) {
	// 创建请求时间控制器
	m.requestTimers[taskType] = &RequestTimer{
		interval: allocation.RequestInterval,
	}

	// 创建信号量
	m.semaphores[taskType] = make(chan struct{}, allocation.MaxConcurrent)

	// 初始化指标
	m.metrics[taskType] = &AdaptiveMetrics{
		CurrentConcurrency: allocation.MaxConcurrent,
		RecentRequests:     make([]TaskMetrics, 0, 100), // 滑动窗口大小
		LastAdjustment:     time.Now(),
	}
}

// GetOptimalConcurrency 获取最优并发数
func (m *QuotaAwareConcurrencyManager) GetOptimalConcurrency(taskType string) int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if metrics, exists := m.metrics[taskType]; exists {
		return metrics.CurrentConcurrency
	}

	// 默认配置回退
	if allocation, exists := m.config.TaskAllocations[taskType]; exists {
		return allocation.MaxConcurrent
	}

	return 1 // 最保守的默认值
}

// AcquirePermit 获取执行许可
func (m *QuotaAwareConcurrencyManager) AcquirePermit(ctx context.Context, taskType string) error {
	// 1. 首先等待速率限制
	if timer, exists := m.requestTimers[taskType]; exists {
		if err := timer.WaitIfNeeded(ctx); err != nil {
			return fmt.Errorf("rate limit wait failed: %w", err)
		}
	}

	// 2. 然后获取并发槽位
	if semaphore, exists := m.semaphores[taskType]; exists {
		select {
		case semaphore <- struct{}{}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("task type %s not configured", taskType)
}

// WaitIfNeeded 等待请求间隔
func (t *RequestTimer) WaitIfNeeded(ctx context.Context) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	now := time.Now()
	elapsed := now.Sub(t.lastRequest)

	if elapsed < t.interval {
		waitTime := t.interval - elapsed

		select {
		case <-time.After(waitTime):
			t.lastRequest = time.Now()
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	t.lastRequest = now
	return nil
}

// ReleasePermit 释放执行许可
func (m *QuotaAwareConcurrencyManager) ReleasePermit(taskType string) {
	if semaphore, exists := m.semaphores[taskType]; exists {
		select {
		case <-semaphore:
		default:
			// 防止过度释放
		}
	}
}

// UpdateMetrics 更新任务指标
func (m *QuotaAwareConcurrencyManager) UpdateMetrics(taskType string, taskMetrics TaskMetrics) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	metrics, exists := m.metrics[taskType]
	if !exists {
		return
	}

	metrics.mutex.Lock()
	defer metrics.mutex.Unlock()

	// 更新滑动窗口
	metrics.RecentRequests = append(metrics.RecentRequests, taskMetrics)
	if len(metrics.RecentRequests) > 100 {
		metrics.RecentRequests = metrics.RecentRequests[1:]
	}

	// 重新计算统计指标
	m.recalculateMetrics(metrics)
}

// recalculateMetrics 重新计算指标
func (m *QuotaAwareConcurrencyManager) recalculateMetrics(metrics *AdaptiveMetrics) {
	if len(metrics.RecentRequests) == 0 {
		return
	}

	var successCount, totalCount int64
	var totalDuration time.Duration

	for _, req := range metrics.RecentRequests {
		totalCount++
		totalDuration += req.Duration
		if req.Success {
			successCount++
		}
	}

	metrics.TotalRequests = totalCount
	metrics.SuccessRate = float64(successCount) / float64(totalCount)
	metrics.ErrorRate = 1.0 - metrics.SuccessRate
	metrics.AvgDuration = totalDuration / time.Duration(totalCount)
}

// adaptiveAdjustmentLoop 自适应调整循环
func (m *QuotaAwareConcurrencyManager) adaptiveAdjustmentLoop() {
	ticker := time.NewTicker(m.config.Adaptive.AdjustmentInterval)
	defer ticker.Stop()

	for range ticker.C {
		m.performAdaptiveAdjustment()
	}
}

// performAdaptiveAdjustment 执行自适应调整
func (m *QuotaAwareConcurrencyManager) performAdaptiveAdjustment() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for taskType, metrics := range m.metrics {
		allocation, exists := m.config.TaskAllocations[taskType]
		if !exists {
			continue
		}

		metrics.mutex.Lock()

		// 检查是否需要调整
		if time.Since(metrics.LastAdjustment) < m.config.Adaptive.AdjustmentInterval {
			metrics.mutex.Unlock()
			continue
		}

		oldConcurrency := metrics.CurrentConcurrency
		newConcurrency := m.calculateOptimalConcurrency(metrics, allocation.AdaptiveRange)

		if newConcurrency != oldConcurrency {
			metrics.CurrentConcurrency = newConcurrency
			metrics.LastAdjustment = time.Now()

			// 更新信号量容量
			m.updateSemaphoreCapacity(taskType, newConcurrency)

			fmt.Printf("Adaptive adjustment: %s concurrency %d -> %d (success_rate=%.2f, error_rate=%.2f)\n",
				taskType, oldConcurrency, newConcurrency, metrics.SuccessRate, metrics.ErrorRate)
		}

		metrics.mutex.Unlock()
	}
}

// calculateOptimalConcurrency 计算最优并发数
func (m *QuotaAwareConcurrencyManager) calculateOptimalConcurrency(metrics *AdaptiveMetrics, adaptiveRange AdaptiveRange) int {
	current := metrics.CurrentConcurrency

	// 性能良好，可以扩容
	if metrics.SuccessRate >= adaptiveRange.ScaleUpThreshold && metrics.AvgDuration < 3*time.Second {
		return min(current+1, adaptiveRange.MaxConcurrency)
	}

	// 性能不佳，需要缩容
	if metrics.ErrorRate >= adaptiveRange.ScaleDownThreshold || metrics.AvgDuration > 8*time.Second {
		return max(current-1, adaptiveRange.MinConcurrency)
	}

	return current
}

// updateSemaphoreCapacity 更新信号量容量
func (m *QuotaAwareConcurrencyManager) updateSemaphoreCapacity(taskType string, newCapacity int) {
	// 创建新的信号量
	newSemaphore := make(chan struct{}, newCapacity)

	// 尝试迁移现有的许可证
	oldSemaphore := m.semaphores[taskType]
	for i := 0; i < min(len(oldSemaphore), newCapacity); i++ {
		select {
		case <-oldSemaphore:
			newSemaphore <- struct{}{}
		default:
			break
		}
	}

	m.semaphores[taskType] = newSemaphore
}

// GetStatus 获取并发状态
func (m *QuotaAwareConcurrencyManager) GetStatus() ConcurrencyStatus {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	status := ConcurrencyStatus{
		TaskStatuses: make(map[string]TaskStatus),
	}

	var totalConcurrency int
	var totalRPMUsage float64

	for taskType, metrics := range m.metrics {
		metrics.mutex.RLock()
		allocation := m.config.TaskAllocations[taskType]

		taskStatus := TaskStatus{
			CurrentConcurrency: metrics.CurrentConcurrency,
			MaxConcurrency:     allocation.MaxConcurrent,
			SuccessRate:        metrics.SuccessRate,
			AvgDuration:        metrics.AvgDuration,
			RPMUsage:           allocation.RPMPercent,
		}

		status.TaskStatuses[taskType] = taskStatus
		totalConcurrency += metrics.CurrentConcurrency
		totalRPMUsage += allocation.RPMPercent

		metrics.mutex.RUnlock()
	}

	status.GlobalStatus = GlobalStatus{
		TotalRPMUsage:    totalRPMUsage,
		TotalConcurrency: totalConcurrency,
		MaxConcurrency:   m.config.GlobalQuotas.MaxConcurrent,
	}

	return status
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
