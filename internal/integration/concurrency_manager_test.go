package integration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQuotaAwareConcurrencyManager_GetOptimalConcurrency 测试获取最优并发数
func TestQuotaAwareConcurrencyManager_GetOptimalConcurrency(t *testing.T) {
	config := createTestConcurrencyConfig()
	manager := NewQuotaAwareConcurrencyManager(config)

	// 测试已配置的任务类型
	concurrency := manager.GetOptimalConcurrency("data_cleaning")
	assert.Equal(t, 3, concurrency, "应该返回配置的并发数")

	concurrency = manager.GetOptimalConcurrency("semantic_analysis")
	assert.Equal(t, 2, concurrency, "应该返回配置的并发数")

	// 测试未配置的任务类型
	concurrency = manager.GetOptimalConcurrency("unknown_task")
	assert.Equal(t, 1, concurrency, "未配置的任务应该返回1")
}

// TestQuotaAwareConcurrencyManager_AcquireAndRelease 测试获取和释放许可
func TestQuotaAwareConcurrencyManager_AcquireAndRelease(t *testing.T) {
	config := createTestConcurrencyConfig()
	manager := NewQuotaAwareConcurrencyManager(config)

	ctx := context.Background()
	taskType := "data_cleaning"

	// 测试获取许可
	err := manager.AcquirePermit(ctx, taskType)
	require.NoError(t, err, "第一次获取许可应该成功")

	// 测试释放许可
	manager.ReleasePermit(taskType)

	// 测试多次获取许可（最多3个并发）
	for i := 0; i < 3; i++ {
		err = manager.AcquirePermit(ctx, taskType)
		require.NoError(t, err, "应该能获取%d个许可", i+1)
	}

	// 第4个应该被阻塞（使用带超时的context测试）
	ctxTimeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	
	err = manager.AcquirePermit(ctxTimeout, taskType)
	assert.Error(t, err, "超过最大并发数应该被阻塞")

	// 释放一个许可
	manager.ReleasePermit(taskType)

	// 现在应该能获取许可
	err = manager.AcquirePermit(ctx, taskType)
	assert.NoError(t, err, "释放后应该能获取许可")
}

// TestQuotaAwareConcurrencyManager_RateLimiting 测试速率限制
func TestQuotaAwareConcurrencyManager_RateLimiting(t *testing.T) {
	config := createTestConcurrencyConfig()
	// 设置更严格的速率限制用于测试
	allocation := config.TaskAllocations["data_cleaning"]
	allocation.RequestInterval = 50 * time.Millisecond
	config.TaskAllocations["data_cleaning"] = allocation
	
	manager := NewQuotaAwareConcurrencyManager(config)
	ctx := context.Background()
	taskType := "data_cleaning"

	// 记录开始时间
	startTime := time.Now()

	// 快速连续发起3个请求
	for i := 0; i < 3; i++ {
		err := manager.AcquirePermit(ctx, taskType)
		require.NoError(t, err)
		manager.ReleasePermit(taskType)
	}

	// 计算总耗时
	elapsed := time.Since(startTime)

	// 3个请求，间隔50ms，至少需要100ms
	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond, 
		"请求间隔控制应该生效")
}

// TestQuotaAwareConcurrencyManager_ConcurrentAccess 测试并发访问安全性
func TestQuotaAwareConcurrencyManager_ConcurrentAccess(t *testing.T) {
	config := createTestConcurrencyConfig()
	manager := NewQuotaAwareConcurrencyManager(config)

	ctx := context.Background()
	taskType := "data_cleaning"
	maxConcurrent := 3

	// 使用原子计数器追踪并发数
	var currentConcurrent int32
	var maxObservedConcurrent int32

	var wg sync.WaitGroup
	// 启动10个goroutine同时请求许可
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 获取许可
			err := manager.AcquirePermit(ctx, taskType)
			if err != nil {
				return
			}
			defer manager.ReleasePermit(taskType)

			// 增加当前并发数
			current := atomic.AddInt32(&currentConcurrent, 1)
			
			// 更新最大观察到的并发数
			for {
				max := atomic.LoadInt32(&maxObservedConcurrent)
				if current <= max || atomic.CompareAndSwapInt32(&maxObservedConcurrent, max, current) {
					break
				}
			}

			// 模拟处理
			time.Sleep(10 * time.Millisecond)

			// 减少当前并发数
			atomic.AddInt32(&currentConcurrent, -1)
		}(i)
	}

	wg.Wait()

	// 验证最大并发数没有超过限制
	assert.LessOrEqual(t, int(maxObservedConcurrent), maxConcurrent,
		"最大并发数不应超过配置限制")
}

// TestQuotaAwareConcurrencyManager_UpdateMetrics 测试指标更新
func TestQuotaAwareConcurrencyManager_UpdateMetrics(t *testing.T) {
	config := createTestConcurrencyConfig()
	manager := NewQuotaAwareConcurrencyManager(config)

	taskType := "data_cleaning"

	// 更新一些成功的指标
	for i := 0; i < 5; i++ {
		manager.UpdateMetrics(taskType, TaskMetrics{
			Duration:  100 * time.Millisecond,
			Success:   true,
			ErrorType: "",
		})
	}

	// 更新一些失败的指标
	for i := 0; i < 2; i++ {
		manager.UpdateMetrics(taskType, TaskMetrics{
			Duration:  200 * time.Millisecond,
			Success:   false,
			ErrorType: "test error",
		})
	}

	// 获取状态
	status := manager.GetStatus()

	// 验证任务状态
	taskStatus, exists := status.TaskStatuses[taskType]
	assert.True(t, exists, "应该包含任务状态")
	assert.Equal(t, 3, taskStatus.CurrentConcurrency, "当前并发数应该是3")
	assert.Equal(t, 3, taskStatus.MaxConcurrency, "最大并发数应该是3")

	// 验证全局状态
	assert.Equal(t, 100, status.GlobalStatus.MaxConcurrency, "全局最大并发数应该是100")
}

// TestQuotaAwareConcurrencyManager_AdaptiveAdjustment 测试自适应调整
func TestQuotaAwareConcurrencyManager_AdaptiveAdjustment(t *testing.T) {
	config := createTestConcurrencyConfig()
	config.Adaptive.EnableAdaptive = true
	config.Adaptive.AdjustmentInterval = 100 * time.Millisecond // 缩短用于测试

	manager := NewQuotaAwareConcurrencyManager(config)
	taskType := "data_cleaning"

	// 初始并发数应该是3
	initialConcurrency := manager.GetOptimalConcurrency(taskType)
	assert.Equal(t, 3, initialConcurrency)

	// 添加高成功率的指标
	for i := 0; i < 20; i++ {
		manager.UpdateMetrics(taskType, TaskMetrics{
			Duration:  1 * time.Second, // 短响应时间
			Success:   true,
			ErrorType: "",
		})
	}

	// 等待自适应调整
	time.Sleep(150 * time.Millisecond)

	// 并发数应该增加
	newConcurrency := manager.GetOptimalConcurrency(taskType)
	assert.Greater(t, newConcurrency, initialConcurrency, 
		"高成功率应该导致并发数增加")

	// 添加高错误率的指标
	for i := 0; i < 20; i++ {
		manager.UpdateMetrics(taskType, TaskMetrics{
			Duration:  10 * time.Second, // 长响应时间
			Success:   false,
			ErrorType: "test error",
		})
	}

	// 等待自适应调整
	time.Sleep(150 * time.Millisecond)

	// 并发数应该减少
	finalConcurrency := manager.GetOptimalConcurrency(taskType)
	assert.Less(t, finalConcurrency, newConcurrency,
		"高错误率应该导致并发数减少")
}

// TestQuotaAwareConcurrencyManager_ContextCancellation 测试Context取消
func TestQuotaAwareConcurrencyManager_ContextCancellation(t *testing.T) {
	config := createTestConcurrencyConfig()
	manager := NewQuotaAwareConcurrencyManager(config)

	taskType := "data_cleaning"

	// 先占满所有并发槽
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		err := manager.AcquirePermit(ctx, taskType)
		require.NoError(t, err)
	}

	// 创建一个会被取消的context
	ctxCancel, cancel := context.WithCancel(context.Background())

	// 在另一个goroutine中尝试获取许可
	errChan := make(chan error, 1)
	go func() {
		errChan <- manager.AcquirePermit(ctxCancel, taskType)
	}()

	// 等待一小段时间
	time.Sleep(50 * time.Millisecond)

	// 取消context
	cancel()

	// 应该收到context取消错误
	err := <-errChan
	assert.Error(t, err, "应该收到错误")
	assert.Equal(t, context.Canceled, err, "应该是context取消错误")
}

// TestQuotaAwareConcurrencyManager_MultipleTaskTypes 测试多任务类型
func TestQuotaAwareConcurrencyManager_MultipleTaskTypes(t *testing.T) {
	config := createTestConcurrencyConfig()
	manager := NewQuotaAwareConcurrencyManager(config)

	ctx := context.Background()

	// 不同任务类型应该有独立的配额
	err1 := manager.AcquirePermit(ctx, "data_cleaning")
	err2 := manager.AcquirePermit(ctx, "semantic_analysis")

	assert.NoError(t, err1, "data_cleaning应该能获取许可")
	assert.NoError(t, err2, "semantic_analysis应该能获取许可")

	// 验证状态
	status := manager.GetStatus()
	
	assert.Len(t, status.TaskStatuses, 2, "应该有两个任务类型的状态")
	assert.Contains(t, status.TaskStatuses, "data_cleaning")
	assert.Contains(t, status.TaskStatuses, "semantic_analysis")
}

// ===== 辅助函数 =====

func createTestConcurrencyConfig() ConcurrencyConfig {
	return ConcurrencyConfig{
		GlobalQuotas: struct {
			MaxRPM        int `yaml:"max_rpm"`
			MaxConcurrent int `yaml:"max_concurrent"`
			MaxTPM        int `yaml:"max_tpm"`
		}{
			MaxRPM:        500,
			MaxConcurrent: 100,
			MaxTPM:        128000,
		},
		TaskAllocations: map[string]TaskAllocation{
			"data_cleaning": {
				RPMPercent:      0.4,
				MaxConcurrent:   3,
				RequestInterval: 10 * time.Millisecond, // 短间隔用于测试
				Priority:        "high",
				AdaptiveRange: AdaptiveRange{
					MinConcurrency:     1,
					MaxConcurrency:     4,
					ScaleUpThreshold:   0.95,
					ScaleDownThreshold: 0.1,
				},
			},
			"semantic_analysis": {
				RPMPercent:      0.3,
				MaxConcurrent:   2,
				RequestInterval: 10 * time.Millisecond,
				Priority:        "medium",
				AdaptiveRange: AdaptiveRange{
					MinConcurrency:     1,
					MaxConcurrency:     3,
					ScaleUpThreshold:   0.95,
					ScaleDownThreshold: 0.1,
				},
			},
		},
		Adaptive: struct {
			EnableAdaptive     bool          `yaml:"enable_adaptive"`
			AdjustmentInterval time.Duration `yaml:"adjustment_interval"`
			MinSuccessRate     float64       `yaml:"min_success_rate"`
			MaxErrorRate       float64       `yaml:"max_error_rate"`
		}{
			EnableAdaptive:     false, // 默认关闭，特定测试开启
			AdjustmentInterval: 30 * time.Second,
			MinSuccessRate:     0.8,
			MaxErrorRate:       0.2,
		},
	}
}