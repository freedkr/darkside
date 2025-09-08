// Package scheduler 任务调度器实现
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/freedkr/moonshot/services/llm-service/internal/models"
	"github.com/freedkr/moonshot/services/llm-service/internal/providers"
)

// TaskScheduler 任务调度器接口
type TaskScheduler interface {
	// 提交任务
	SubmitTask(ctx context.Context, task *models.LLMTask) error
	
	// 获取任务状态
	GetTaskStatus(taskID string) (*models.LLMTask, error)
	
	// 取消任务
	CancelTask(taskID string) error
	
	// 获取任务列表
	ListTasks(limit, offset int) ([]*models.LLMTask, int, error)
	
	// 获取调度器统计
	GetStats() *SchedulerStats
	
	// 生命周期管理
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// DefaultTaskScheduler 默认任务调度器
type DefaultTaskScheduler struct {
	// 核心组件
	providerManager providers.ProviderManager
	concurrencyMgr  *ConcurrencyManager
	
	// 任务队列
	taskQueues     map[models.LLMTaskType]*PriorityQueue
	queuesMutex    sync.RWMutex
	
	// 任务存储
	tasks          map[string]*models.LLMTask
	tasksMutex     sync.RWMutex
	
	// 工作协程池
	workers        []*Worker
	workerPool     chan *Worker
	
	// 配置
	config         SchedulerConfig
	
	// 生命周期
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	
	// 统计
	stats          *SchedulerStats
	statsMutex     sync.RWMutex
	
	// 回调处理
	callbackHandler CallbackHandler
}

// SchedulerConfig 调度器配置
type SchedulerConfig struct {
	MaxWorkers       int           `json:"max_workers"`
	MaxQueueSize     int           `json:"max_queue_size"`
	TaskTimeout      time.Duration `json:"task_timeout"`
	CleanupInterval  time.Duration `json:"cleanup_interval"`
	StatsInterval    time.Duration `json:"stats_interval"`
	RetryAttempts    int           `json:"retry_attempts"`
	RetryDelay       time.Duration `json:"retry_delay"`
}

// NewTaskScheduler 创建新的任务调度器
func NewTaskScheduler(providerMgr providers.ProviderManager, config SchedulerConfig) *DefaultTaskScheduler {
	// 设置默认值
	if config.MaxWorkers == 0 {
		config.MaxWorkers = 10
	}
	if config.MaxQueueSize == 0 {
		config.MaxQueueSize = 1000
	}
	if config.TaskTimeout == 0 {
		config.TaskTimeout = 5 * time.Minute
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = time.Minute
	}
	if config.StatsInterval == 0 {
		config.StatsInterval = 30 * time.Second
	}
	if config.RetryAttempts == 0 {
		config.RetryAttempts = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = time.Second
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	scheduler := &DefaultTaskScheduler{
		providerManager: providerMgr,
		concurrencyMgr:  NewConcurrencyManager(),
		taskQueues:      make(map[models.LLMTaskType]*PriorityQueue),
		tasks:           make(map[string]*models.LLMTask),
		workers:         make([]*Worker, 0, config.MaxWorkers),
		workerPool:      make(chan *Worker, config.MaxWorkers),
		config:          config,
		ctx:             ctx,
		cancel:          cancel,
		stats:           &SchedulerStats{},
		callbackHandler: NewDefaultCallbackHandler(),
	}
	
	// 初始化任务队列
	scheduler.initializeQueues()
	
	return scheduler
}

// initializeQueues 初始化任务队列
func (s *DefaultTaskScheduler) initializeQueues() {
	s.queuesMutex.Lock()
	defer s.queuesMutex.Unlock()
	
	// 为每种任务类型创建队列
	taskTypes := []models.LLMTaskType{
		models.TaskTypeSemanticAnalysis,
		models.TaskTypeDataCleaning,
		models.TaskTypeCategoryMatch,
		models.TaskTypeTextSummarization,
		models.TaskTypeTranslation,
		models.TaskTypeCustom,
	}
	
	for _, taskType := range taskTypes {
		s.taskQueues[taskType] = NewPriorityQueue(s.config.MaxQueueSize)
	}
}

// RegisterListener 注册回调监听器到调度器
func (s *DefaultTaskScheduler) RegisterListener(listener CallbackListener) {
	if handler, ok := s.callbackHandler.(*DefaultCallbackHandler); ok {
		handler.RegisterListener(listener)
	}
}

// Start 启动调度器
func (s *DefaultTaskScheduler) Start(ctx context.Context) error {
	// 创建工作协程池
	s.createWorkerPool()
	
	// 启动回调处理器
	if err := s.callbackHandler.Start(); err != nil {
		return fmt.Errorf("启动回调处理器失败: %w", err)
	}
	
	// 启动各种循环
	s.wg.Add(3)
	go s.schedulingLoop()
	go s.cleanupLoop()
	go s.statsLoop()
	
	// 启动工作协程
	for _, worker := range s.workers {
		s.wg.Add(1)
		go s.runWorker(worker)
	}
	
	return nil
}

// Stop 停止调度器
func (s *DefaultTaskScheduler) Stop(ctx context.Context) error {
	// 停止接收新任务
	s.cancel()
	
	// 停止回调处理器
	if err := s.callbackHandler.Stop(); err != nil {
		return fmt.Errorf("停止回调处理器失败: %w", err)
	}
	
	// 等待所有工作协程完成
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("停止调度器超时")
	}
}

// SubmitTask 提交任务
func (s *DefaultTaskScheduler) SubmitTask(ctx context.Context, task *models.LLMTask) error {
	// 设置任务状态
	task.Status = models.StatusQueued
	task.UpdatedAt = time.Now()
	
	// 存储任务
	s.tasksMutex.Lock()
	s.tasks[task.ID] = task
	s.tasksMutex.Unlock()
	
	// 添加到相应的队列
	s.queuesMutex.RLock()
	queue, exists := s.taskQueues[task.Type]
	s.queuesMutex.RUnlock()
	
	if !exists {
		return fmt.Errorf("不支持的任务类型: %s", task.Type)
	}
	
	if err := queue.Push(task); err != nil {
		return fmt.Errorf("任务队列已满: %w", err)
	}
	
	// 更新统计
	s.updateStats(func(stats *SchedulerStats) {
		stats.TotalTasks++
		stats.QueuedTasks++
	})
	
	return nil
}

// GetTaskStatus 获取任务状态
func (s *DefaultTaskScheduler) GetTaskStatus(taskID string) (*models.LLMTask, error) {
	s.tasksMutex.RLock()
	defer s.tasksMutex.RUnlock()
	
	task, exists := s.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("任务不存在: %s", taskID)
	}
	
	return task, nil
}

// ListTasks 获取任务列表
func (s *DefaultTaskScheduler) ListTasks(limit, offset int) ([]*models.LLMTask, int, error) {
	s.tasksMutex.RLock()
	defer s.tasksMutex.RUnlock()
	
	// 获取所有任务
	allTasks := make([]*models.LLMTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		allTasks = append(allTasks, task)
	}
	
	// 按创建时间排序（最新的在前面）
	for i := 0; i < len(allTasks)-1; i++ {
		for j := i + 1; j < len(allTasks); j++ {
			if allTasks[i].CreatedAt.Before(allTasks[j].CreatedAt) {
				allTasks[i], allTasks[j] = allTasks[j], allTasks[i]
			}
		}
	}
	
	total := len(allTasks)
	
	// 应用分页
	if offset >= total {
		return []*models.LLMTask{}, total, nil
	}
	
	end := offset + limit
	if end > total {
		end = total
	}
	
	return allTasks[offset:end], total, nil
}

// CancelTask 取消任务
func (s *DefaultTaskScheduler) CancelTask(taskID string) error {
	s.tasksMutex.Lock()
	defer s.tasksMutex.Unlock()
	
	task, exists := s.tasks[taskID]
	if !exists {
		return fmt.Errorf("任务不存在: %s", taskID)
	}
	
	if task.IsTerminal() {
		return fmt.Errorf("任务已完成，无法取消: %s", taskID)
	}
	
	task.Status = models.StatusCancelled
	task.UpdatedAt = time.Now()
	
	return nil
}

// GetStats 获取调度器统计
func (s *DefaultTaskScheduler) GetStats() *SchedulerStats {
	s.statsMutex.RLock()
	defer s.statsMutex.RUnlock()
	
	// 返回统计副本
	stats := *s.stats
	return &stats
}

// scheduleNext 调度下一个任务
func (s *DefaultTaskScheduler) scheduleNext() {
	// 选择下一个任务
	task := s.selectNextTask()
	if task == nil {
		return
	}
	
	// 获取可用的工作协程
	select {
	case worker := <-s.workerPool:
		// 分配任务给工作协程
		go s.assignTask(worker, task)
	default:
		// 没有可用的工作协程，任务保持在队列中
	}
}

// selectNextTask 选择下一个任务
func (s *DefaultTaskScheduler) selectNextTask() *models.LLMTask {
	s.queuesMutex.RLock()
	defer s.queuesMutex.RUnlock()
	
	var bestTask *models.LLMTask
	var bestQueue *PriorityQueue
	
	// 简单策略：选择优先级最高的任务
	for _, queue := range s.taskQueues {
		task := queue.Peek()
		if task != nil {
			if bestTask == nil || task.Priority > bestTask.Priority {
				bestTask = task
				bestQueue = queue
			}
		}
	}
	
	if bestTask != nil {
		bestQueue.Pop()
	}
	
	return bestTask
}

// assignTask 分配任务给工作协程
func (s *DefaultTaskScheduler) assignTask(worker *Worker, task *models.LLMTask) {
	worker.currentTask = task
	worker.taskChan <- task
}

// schedulingLoop 调度循环
func (s *DefaultTaskScheduler) schedulingLoop() {
	defer s.wg.Done()
	
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.scheduleNext()
		}
	}
}

// cleanupLoop 清理循环
func (s *DefaultTaskScheduler) cleanupLoop() {
	defer s.wg.Done()
	
	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanupCompletedTasks()
		}
	}
}

// statsLoop 统计更新循环
func (s *DefaultTaskScheduler) statsLoop() {
	defer s.wg.Done()
	
	ticker := time.NewTicker(s.config.StatsInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.updateStatsCounts()
		}
	}
}

// cleanupCompletedTasks 清理已完成的任务
func (s *DefaultTaskScheduler) cleanupCompletedTasks() {
	s.tasksMutex.Lock()
	defer s.tasksMutex.Unlock()
	
	// 清理超过一定时间的已完成任务
	cutoff := time.Now().Add(-time.Hour) // 保留1小时内的任务
	
	for taskID, task := range s.tasks {
		if task.IsTerminal() && task.UpdatedAt.Before(cutoff) {
			delete(s.tasks, taskID)
		}
	}
}

// updateStatsCounts 更新统计计数
func (s *DefaultTaskScheduler) updateStatsCounts() {
	s.tasksMutex.RLock()
	defer s.tasksMutex.RUnlock()
	
	var running, queued, completed, failed int
	
	for _, task := range s.tasks {
		switch task.Status {
		case models.StatusRunning:
			running++
		case models.StatusQueued:
			queued++
		case models.StatusCompleted:
			completed++
		case models.StatusFailed:
			failed++
		}
	}
	
	s.updateStats(func(stats *SchedulerStats) {
		stats.RunningTasks = running
		stats.QueuedTasks = queued
		stats.CompletedTasks = completed
		stats.FailedTasks = failed
	})
}

// updateStats 更新统计
func (s *DefaultTaskScheduler) updateStats(updater func(*SchedulerStats)) {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()
	
	updater(s.stats)
}

// runWorker 运行工作协程
func (s *DefaultTaskScheduler) runWorker(worker *Worker) {
	defer s.wg.Done()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case task := <-worker.taskChan:
			s.processTask(worker, task)
			// 将工作协程放回池中
			s.workerPool <- worker
		}
	}
}

// processTask 处理任务
func (s *DefaultTaskScheduler) processTask(worker *Worker, task *models.LLMTask) {
	startTime := time.Now()
	
	// 更新任务状态
	task.Status = models.StatusRunning
	task.UpdatedAt = time.Now()
	task.StartedAt = &startTime
	
	// 发送开始回调
	s.callbackHandler.OnTaskStarted(task)
	
	// 选择提供商
	provider, err := s.providerManager.SelectProvider(s.ctx, task)
	if err != nil {
		s.failTask(task, fmt.Errorf("选择提供商失败: %w", err))
		return
	}
	
	// 执行任务（带重试）
	var result *models.LLMResult
	retryCount := 0
	maxRetries := 3
	
	for retryCount <= maxRetries {
		result, err = provider.Process(s.ctx, task)
		if err == nil {
			break // 成功
		}
		
		// 检查是否是限流错误
		if s.isRateLimitError(err) {
			retryCount++
			if retryCount <= maxRetries {
				// 计算退避时间
				backoff := time.Duration(retryCount) * 30 * time.Second
				log.Printf("⚠️ [任务 %s] 遇到限流错误，%d秒后重试 (第%d/%d次)", 
					task.ID, int(backoff.Seconds()), retryCount, maxRetries)
				
				// 等待退避时间
				select {
				case <-time.After(backoff):
					continue
				case <-s.ctx.Done():
					s.failTask(task, fmt.Errorf("任务被取消: %w", s.ctx.Err()))
					return
				}
			}
		}
		
		// 非限流错误或重试次数用尽
		break
	}
	
	if err != nil {
		s.failTask(task, fmt.Errorf("任务执行失败（重试%d次后）: %w", retryCount, err))
		return
	}
	
	// 任务成功
	s.completeTask(task, result)
}

// completeTask 完成任务
func (s *DefaultTaskScheduler) completeTask(task *models.LLMTask, result *models.LLMResult) {
	now := time.Now()
	task.Status = models.StatusCompleted
	task.UpdatedAt = now
	task.CompletedAt = &now
	
	// 设置结果
	task.SetResult(result.Data)
	task.TokenUsage = result.TokenUsage
	
	// 发送完成回调
	s.callbackHandler.OnTaskCompleted(task)
	
	// 更新统计
	s.updateStats(func(stats *SchedulerStats) {
		stats.CompletedTasks++
		stats.RunningTasks--
	})
}

// failTask 任务失败
func (s *DefaultTaskScheduler) failTask(task *models.LLMTask, err error) {
	now := time.Now()
	task.Status = models.StatusFailed
	task.Error = err.Error()
	task.UpdatedAt = now
	task.CompletedAt = &now
	
	// 发送失败回调
	s.callbackHandler.OnTaskFailed(task, err)
	
	// 更新统计
	s.updateStats(func(stats *SchedulerStats) {
		stats.FailedTasks++
		stats.RunningTasks--
	})
}

// isRateLimitError 检查是否是限流错误
func (s *DefaultTaskScheduler) isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	
	// 检查是否是ProviderError类型
	var provErr *providers.ProviderError
	if errors.As(err, &provErr) {
		return provErr.Code == providers.ErrCodeRateLimit
	}
	
	// 检查错误消息中是否包含限流相关关键词
	errStr := err.Error()
	return strings.Contains(errStr, "429") || 
		strings.Contains(errStr, "rate limit") || 
		strings.Contains(errStr, "限流") ||
		strings.Contains(errStr, "限制")
}

// Worker 工作协程
type Worker struct {
	ID          int
	scheduler   *DefaultTaskScheduler
	currentTask *models.LLMTask
	taskChan    chan *models.LLMTask
}

// createWorkerPool 创建工作协程池
func (s *DefaultTaskScheduler) createWorkerPool() {
	for i := 0; i < s.config.MaxWorkers; i++ {
		worker := &Worker{
			ID:        i,
			scheduler: s,
			taskChan:  make(chan *models.LLMTask, 1), // 初始化任务通道
		}
		s.workers = append(s.workers, worker)
		s.workerPool <- worker
	}
}

// SchedulerStats 调度器统计
type SchedulerStats struct {
	TotalTasks     int64     `json:"total_tasks"`
	RunningTasks   int       `json:"running_tasks"`
	QueuedTasks    int       `json:"queued_tasks"`
	CompletedTasks int       `json:"completed_tasks"`
	FailedTasks    int       `json:"failed_tasks"`
	AvgProcessTime float64   `json:"avg_process_time"`
	Uptime         time.Duration `json:"uptime"`
	LastUpdated    time.Time `json:"last_updated"`
}