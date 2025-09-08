package integration

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMetricsCollector_RecordProcessingDuration 测试记录处理时长
func TestMetricsCollector_RecordProcessingDuration(t *testing.T) {
	collector := NewMetricsCollector()

	// 记录多个阶段的处理时长
	collector.RecordProcessingDuration("pdf_validation", 100*time.Millisecond)
	collector.RecordProcessingDuration("pdf_validation", 200*time.Millisecond)
	collector.RecordProcessingDuration("pdf_validation", 150*time.Millisecond)

	collector.RecordProcessingDuration("llm_cleaning", 500*time.Millisecond)
	collector.RecordProcessingDuration("llm_cleaning", 300*time.Millisecond)

	// 获取指标
	metrics := collector.GetMetrics()

	// 验证PDF验证阶段的指标
	pdfMetrics, exists := metrics.StageMetrics["pdf_validation"]
	assert.True(t, exists, "应该包含pdf_validation阶段的指标")
	assert.Equal(t, int64(3), pdfMetrics.Count, "应该有3次记录")
	assert.Equal(t, 100*time.Millisecond, pdfMetrics.MinDuration, "最小时长应该是100ms")
	assert.Equal(t, 200*time.Millisecond, pdfMetrics.MaxDuration, "最大时长应该是200ms")
	assert.Equal(t, 150*time.Millisecond, pdfMetrics.AvgDuration, "平均时长应该是150ms")

	// 验证LLM清洗阶段的指标
	llmMetrics, exists := metrics.StageMetrics["llm_cleaning"]
	assert.True(t, exists, "应该包含llm_cleaning阶段的指标")
	assert.Equal(t, int64(2), llmMetrics.Count, "应该有2次记录")
	assert.Equal(t, 300*time.Millisecond, llmMetrics.MinDuration, "最小时长应该是300ms")
	assert.Equal(t, 500*time.Millisecond, llmMetrics.MaxDuration, "最大时长应该是500ms")
	assert.Equal(t, 400*time.Millisecond, llmMetrics.AvgDuration, "平均时长应该是400ms")
}

// TestMetricsCollector_RecordSuccess 测试记录成功
func TestMetricsCollector_RecordSuccess(t *testing.T) {
	collector := NewMetricsCollector()

	// 记录多个成功
	collector.RecordSuccess("pdf_validation")
	collector.RecordSuccess("llm_cleaning")
	collector.RecordSuccess("semantic_analysis")
	collector.RecordSuccess("persistence")
	collector.RecordSuccess("pdf_validation")

	// 获取指标
	metrics := collector.GetMetrics()

	// 验证总计数和成功率
	assert.Equal(t, int64(5), metrics.TotalProcessed, "总处理数应该是5")
	assert.Equal(t, int64(5), metrics.SuccessCount, "成功数应该是5")
	assert.Equal(t, float64(1.0), metrics.SuccessRate, "成功率应该是100%")
	assert.Equal(t, int64(0), metrics.ErrorCount, "错误数应该是0")

	// 验证阶段指标
	pdfMetrics, exists := metrics.StageMetrics["pdf_validation"]
	assert.True(t, exists)
	assert.Equal(t, int64(2), pdfMetrics.Count, "pdf_validation应该有2次成功")
}

// TestMetricsCollector_RecordError 测试记录错误
func TestMetricsCollector_RecordError(t *testing.T) {
	collector := NewMetricsCollector()

	// 记录成功和错误
	collector.RecordSuccess("pdf_validation")
	collector.RecordError("pdf_validation", errors.New("PDF损坏"))
	collector.RecordSuccess("llm_cleaning")
	collector.RecordError("llm_cleaning", errors.New("LLM服务不可用"))
	collector.RecordError("llm_cleaning", errors.New("LLM服务不可用"))

	// 获取指标
	metrics := collector.GetMetrics()

	// 验证总计数和成功率
	assert.Equal(t, int64(5), metrics.TotalProcessed, "总处理数应该是5")
	assert.Equal(t, int64(2), metrics.SuccessCount, "成功数应该是2")
	assert.Equal(t, int64(3), metrics.ErrorCount, "错误数应该是3")
	assert.Equal(t, float64(0.4), metrics.SuccessRate, "成功率应该是40%")

	// 验证错误分布
	assert.Equal(t, int64(1), metrics.ErrorDistribution["PDF损坏"])
	assert.Equal(t, int64(2), metrics.ErrorDistribution["LLM服务不可用"])

	// 验证阶段错误
	pdfMetrics := metrics.StageMetrics["pdf_validation"]
	assert.Len(t, pdfMetrics.Errors, 1, "pdf_validation应该有1个错误")
	
	llmMetrics := metrics.StageMetrics["llm_cleaning"]
	assert.Len(t, llmMetrics.Errors, 2, "llm_cleaning应该有2个错误")
}

// TestMetricsCollector_RecentActivity 测试最近活动记录
func TestMetricsCollector_RecentActivity(t *testing.T) {
	collector := NewMetricsCollector()

	// 记录一些活动
	collector.RecordSuccess("pdf_validation")
	time.Sleep(10 * time.Millisecond)
	collector.RecordProcessingDuration("llm_cleaning", 100*time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	collector.RecordError("semantic_analysis", errors.New("test error"))

	// 获取指标
	metrics := collector.GetMetrics()

	// 验证最近活动
	assert.Len(t, metrics.RecentActivity, 3, "应该有3条活动记录")

	// 验证活动类型
	assert.Equal(t, "pdf_validation", metrics.RecentActivity[0].Stage)
	assert.Equal(t, "success", metrics.RecentActivity[0].Status)

	assert.Equal(t, "llm_cleaning", metrics.RecentActivity[1].Stage)
	assert.Equal(t, "duration_recorded", metrics.RecentActivity[1].Status)

	assert.Equal(t, "semantic_analysis", metrics.RecentActivity[2].Stage)
	assert.Equal(t, "error", metrics.RecentActivity[2].Status)
	assert.Equal(t, "test error", metrics.RecentActivity[2].Error)
}

// TestMetricsCollector_Reset 测试重置功能
func TestMetricsCollector_Reset(t *testing.T) {
	collector := NewMetricsCollector()

	// 记录一些数据
	collector.RecordSuccess("pdf_validation")
	collector.RecordError("llm_cleaning", errors.New("error"))
	collector.RecordProcessingDuration("semantic_analysis", 100*time.Millisecond)

	// 验证有数据
	metrics := collector.GetMetrics()
	assert.Greater(t, metrics.TotalProcessed, int64(0), "应该有处理记录")
	assert.NotEmpty(t, metrics.StageMetrics, "应该有阶段指标")

	// 重置
	collector.Reset()

	// 验证已清空
	metrics = collector.GetMetrics()
	assert.Equal(t, int64(0), metrics.TotalProcessed, "总处理数应该重置为0")
	assert.Equal(t, int64(0), metrics.SuccessCount, "成功数应该重置为0")
	assert.Equal(t, int64(0), metrics.ErrorCount, "错误数应该重置为0")
	assert.Empty(t, metrics.StageMetrics, "阶段指标应该清空")
	assert.Empty(t, metrics.ErrorDistribution, "错误分布应该清空")
	assert.Empty(t, metrics.RecentActivity, "最近活动应该清空")
}

// TestMetricsCollector_ConcurrentAccess 测试并发访问安全性
func TestMetricsCollector_ConcurrentAccess(t *testing.T) {
	collector := NewMetricsCollector()

	var wg sync.WaitGroup
	// 并发记录不同类型的指标
	for i := 0; i < 10; i++ {
		wg.Add(3)
		
		// 记录成功
		go func(id int) {
			defer wg.Done()
			stage := fmt.Sprintf("stage_%d", id%3)
			collector.RecordSuccess(stage)
		}(i)

		// 记录错误
		go func(id int) {
			defer wg.Done()
			stage := fmt.Sprintf("stage_%d", id%3)
			collector.RecordError(stage, fmt.Errorf("error_%d", id))
		}(i)

		// 记录时长
		go func(id int) {
			defer wg.Done()
			stage := fmt.Sprintf("stage_%d", id%3)
			duration := time.Duration(100+id*10) * time.Millisecond
			collector.RecordProcessingDuration(stage, duration)
		}(i)
	}

	// 并发读取指标
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			metrics := collector.GetMetrics()
			// 简单验证能获取到指标
			assert.NotNil(t, metrics)
		}()
	}

	wg.Wait()

	// 最终验证
	metrics := collector.GetMetrics()
	assert.Equal(t, int64(20), metrics.TotalProcessed, "应该有20次处理（10成功+10错误，时长记录不计入总处理数）")
}

// TestMetricsCollector_ActivityWindowLimit 测试活动窗口限制
func TestMetricsCollector_ActivityWindowLimit(t *testing.T) {
	collector := NewMetricsCollector()

	// 记录超过100条活动
	for i := 0; i < 120; i++ {
		if i%2 == 0 {
			collector.RecordSuccess(fmt.Sprintf("stage_%d", i))
		} else {
			collector.RecordError(fmt.Sprintf("stage_%d", i), errors.New("test"))
		}
	}

	// 获取指标
	metrics := collector.GetMetrics()

	// 验证活动窗口限制
	assert.LessOrEqual(t, len(metrics.RecentActivity), 100, "最近活动不应超过100条")
	
	// 验证保留的是最新的活动
	if len(metrics.RecentActivity) > 0 {
		lastActivity := metrics.RecentActivity[len(metrics.RecentActivity)-1]
		// 最后一条应该是stage_119
		assert.Contains(t, lastActivity.Stage, "119", "应该保留最新的活动")
	}
}

// TestMetricsCollector_Timestamp 测试时间戳更新
func TestMetricsCollector_Timestamp(t *testing.T) {
	collector := NewMetricsCollector()

	// 获取初始指标
	metrics1 := collector.GetMetrics()
	time1 := metrics1.Timestamp

	// 等待一小段时间
	time.Sleep(100 * time.Millisecond)

	// 再次获取指标
	metrics2 := collector.GetMetrics()
	time2 := metrics2.Timestamp

	// 验证时间戳更新
	assert.True(t, time2.After(time1), "时间戳应该更新")
}

// TestMetricsCollector_ComplexScenario 测试复杂场景
func TestMetricsCollector_ComplexScenario(t *testing.T) {
	collector := NewMetricsCollector()

	// 模拟完整的处理流程
	stages := []string{"pdf_validation", "llm_cleaning", "data_fusion", "semantic_analysis", "persistence"}
	
	// 模拟10个任务的处理
	for taskID := 0; taskID < 10; taskID++ {
		for _, stage := range stages {
			// 记录处理时长
			duration := time.Duration(100+taskID*10) * time.Millisecond
			collector.RecordProcessingDuration(stage, duration)

			// 90%成功率
			if taskID < 9 {
				collector.RecordSuccess(stage)
			} else {
				collector.RecordError(stage, fmt.Errorf("%s failed for task %d", stage, taskID))
			}
		}
	}

	// 获取并验证指标
	metrics := collector.GetMetrics()

	// 验证总体指标
	assert.Equal(t, int64(50), metrics.TotalProcessed, "应该有50次处理（10任务*5阶段）")
	assert.Equal(t, int64(45), metrics.SuccessCount, "应该有45次成功（9任务*5阶段）")
	assert.Equal(t, int64(5), metrics.ErrorCount, "应该有5次失败（1任务*5阶段）")
	assert.Equal(t, 0.9, metrics.SuccessRate, "成功率应该是90%")

	// 验证每个阶段的指标
	for _, stage := range stages {
		stageMetrics, exists := metrics.StageMetrics[stage]
		require.True(t, exists, "应该包含%s阶段的指标", stage)
		assert.Equal(t, int64(20), stageMetrics.Count, "%s应该有20次处理（10次时长记录+10次成功/错误记录）", stage)
		assert.Greater(t, stageMetrics.MaxDuration, stageMetrics.MinDuration, 
			"%s的最大时长应该大于最小时长", stage)
	}

	// 验证错误分布
	assert.Len(t, metrics.ErrorDistribution, 5, "应该有5种错误类型")
}

// ===== 辅助函数已在上面的import中引入fmt包 =====