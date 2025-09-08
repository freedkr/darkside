// Package providers 提供商管理器实现
package providers

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/freedkr/moonshot/services/llm-service/internal/models"
)

// DefaultProviderManager 默认提供商管理器
type DefaultProviderManager struct {
	providers    map[string]Provider
	routingRules []RoutingRule
	mutex        sync.RWMutex
	
	// 监控相关
	status       map[string]*ProviderStatus
	statusMutex  sync.RWMutex
	
	// 配置
	config       ManagerConfig
	
	// 生命周期
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// ManagerConfig 管理器配置
type ManagerConfig struct {
	HealthCheckInterval   time.Duration `json:"health_check_interval"`
	MetricsUpdateInterval time.Duration `json:"metrics_update_interval"`
	DefaultTimeout        time.Duration `json:"default_timeout"`
	EnableAutoFailover    bool          `json:"enable_auto_failover"`
}

// NewProviderManager 创建新的提供商管理器
func NewProviderManager(config ManagerConfig) *DefaultProviderManager {
	// 设置默认值
	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = 30 * time.Second
	}
	if config.MetricsUpdateInterval == 0 {
		config.MetricsUpdateInterval = 10 * time.Second
	}
	if config.DefaultTimeout == 0 {
		config.DefaultTimeout = 30 * time.Second
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	return &DefaultProviderManager{
		providers:    make(map[string]Provider),
		routingRules: make([]RoutingRule, 0),
		status:       make(map[string]*ProviderStatus),
		config:       config,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// RegisterProvider 注册提供商
func (m *DefaultProviderManager) RegisterProvider(name string, provider Provider) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	if _, exists := m.providers[name]; exists {
		return fmt.Errorf("提供商 %s 已存在", name)
	}
	
	m.providers[name] = provider
	
	// 初始化状态
	m.statusMutex.Lock()
	m.status[name] = &ProviderStatus{
		Name:        name,
		Available:   false,
		LastCheck:   time.Now(),
		Models:      provider.GetModels(),
		Metrics:     make(map[string]interface{}),
	}
	m.statusMutex.Unlock()
	
	return nil
}

// GetProvider 获取提供商
func (m *DefaultProviderManager) GetProvider(name string) (Provider, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	provider, exists := m.providers[name]
	if !exists {
		return nil, fmt.Errorf("提供商 %s 不存在", name)
	}
	
	return provider, nil
}

// ListProviders 列出所有提供商名称
func (m *DefaultProviderManager) ListProviders() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}
	
	sort.Strings(names)
	return names
}

// SelectProvider 智能选择提供商
func (m *DefaultProviderManager) SelectProvider(ctx context.Context, task *models.LLMTask) (Provider, error) {
	// 如果任务指定了提供商，直接使用
	if task.Provider != "" && task.Provider != "auto" {
		return m.GetProvider(task.Provider)
	}
	
	// 根据路由规则选择
	return m.selectByRules(ctx, task)
}

// selectByRules 根据路由规则选择提供商
func (m *DefaultProviderManager) selectByRules(ctx context.Context, task *models.LLMTask) (Provider, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	// 查找匹配的路由规则
	log.Printf("🔍 [SelectProvider] 查找任务类型 %s 的路由规则", task.Type)
	var matchedRule *RoutingRule
	for i, rule := range m.routingRules {
		if rule.TaskType == task.Type {
			log.Printf("🔍 [SelectProvider] 找到匹配规则: %+v", rule)
			if m.evaluateConditions(task, rule.Conditions) {
				matchedRule = &m.routingRules[i]
				break
			}
		}
	}
	
	// 如果没有匹配的规则，使用默认策略
	if matchedRule == nil {
		return m.selectDefaultProvider(ctx, task)
	}
	
	// 按优先级尝试提供商
	for _, providerName := range matchedRule.Providers {
		provider, err := m.GetProvider(providerName)
		if err != nil {
			continue
		}
		
		// 检查提供商是否可用
		if provider.IsAvailable(ctx) {
			return provider, nil
		}
	}
	
	// 如果规则中的提供商都不可用，使用默认策略
	return m.selectDefaultProvider(ctx, task)
}

// selectDefaultProvider 默认提供商选择策略
func (m *DefaultProviderManager) selectDefaultProvider(ctx context.Context, task *models.LLMTask) (Provider, error) {
	// 获取所有可用的提供商
	availableProviders := make([]Provider, 0)
	
	log.Printf("🔍 [SelectProvider] 检查提供商可用性，总数: %d", len(m.providers))
	
	for name, provider := range m.providers {
		isAvailable := provider.IsAvailable(ctx)
		log.Printf("🔍 [SelectProvider] 提供商 %s 可用性: %v", name, isAvailable)
		
		if isAvailable {
			availableProviders = append(availableProviders, provider)
		}
	}
	
	if len(availableProviders) == 0 {
		log.Printf("❌ [SelectProvider] 没有可用的提供商！总提供商数: %d", len(m.providers))
		return nil, fmt.Errorf("没有可用的提供商")
	}
	
	// 简单策略：返回第一个可用的提供商
	// 可以扩展为更复杂的负载均衡策略
	return availableProviders[0], nil
}

// evaluateConditions 评估路由条件
func (m *DefaultProviderManager) evaluateConditions(task *models.LLMTask, conditions []Condition) bool {
	if len(conditions) == 0 {
		return true
	}
	
	for _, condition := range conditions {
		if !m.evaluateCondition(task, condition) {
			return false
		}
	}
	
	return true
}

// evaluateCondition 评估单个条件
func (m *DefaultProviderManager) evaluateCondition(task *models.LLMTask, condition Condition) bool {
	var value interface{}
	
	// 获取字段值
	switch condition.Field {
	case "priority":
		value = task.Priority
	case "data_size":
		value = len(task.Data)
	case "temperature":
		value = task.Temperature
	default:
		// 从元数据中获取
		if task.Metadata != nil {
			value = task.Metadata[condition.Field]
		}
	}
	
	// 评估条件
	switch condition.Operator {
	case "eq":
		return value == condition.Value
	case "gt":
		return compareValues(value, condition.Value) > 0
	case "lt":
		return compareValues(value, condition.Value) < 0
	case "gte":
		return compareValues(value, condition.Value) >= 0
	case "lte":
		return compareValues(value, condition.Value) <= 0
	case "in":
		if slice, ok := condition.Value.([]interface{}); ok {
			for _, v := range slice {
				if value == v {
					return true
				}
			}
		}
		return false
	default:
		return false
	}
}

// compareValues 比较值
func compareValues(a, b interface{}) int {
	// 简化的比较实现，实际应该更健壮
	switch av := a.(type) {
	case int:
		if bv, ok := b.(int); ok {
			if av < bv {
				return -1
			} else if av > bv {
				return 1
			}
			return 0
		}
	case float64:
		if bv, ok := b.(float64); ok {
			if av < bv {
				return -1
			} else if av > bv {
				return 1
			}
			return 0
		}
	case string:
		if bv, ok := b.(string); ok {
			if av < bv {
				return -1
			} else if av > bv {
				return 1
			}
			return 0
		}
	}
	return 0
}

// GetProviderStatus 获取提供商状态
func (m *DefaultProviderManager) GetProviderStatus(name string) (*ProviderStatus, error) {
	m.statusMutex.RLock()
	defer m.statusMutex.RUnlock()
	
	status, exists := m.status[name]
	if !exists {
		return nil, fmt.Errorf("提供商 %s 不存在", name)
	}
	
	// 返回副本
	statusCopy := *status
	return &statusCopy, nil
}

// GetAllProvidersStatus 获取所有提供商状态
func (m *DefaultProviderManager) GetAllProvidersStatus() map[string]*ProviderStatus {
	m.statusMutex.RLock()
	defer m.statusMutex.RUnlock()
	
	result := make(map[string]*ProviderStatus)
	for name, status := range m.status {
		statusCopy := *status
		result[name] = &statusCopy
	}
	
	return result
}

// AddRoutingRule 添加路由规则
func (m *DefaultProviderManager) AddRoutingRule(rule RoutingRule) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.routingRules = append(m.routingRules, rule)
}

// Start 启动管理器
func (m *DefaultProviderManager) Start(ctx context.Context) error {
	// 启动健康检查
	m.wg.Add(1)
	go m.healthCheckLoop()
	
	// 启动指标更新
	m.wg.Add(1)
	go m.metricsUpdateLoop()
	
	return nil
}

// Stop 停止管理器
func (m *DefaultProviderManager) Stop(ctx context.Context) error {
	m.cancel()
	m.wg.Wait()
	
	// 关闭所有提供商
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	for _, provider := range m.providers {
		if err := provider.Close(); err != nil {
			// 记录错误但不中断关闭过程
			fmt.Printf("关闭提供商失败: %v\n", err)
		}
	}
	
	return nil
}

// healthCheckLoop 健康检查循环
func (m *DefaultProviderManager) healthCheckLoop() {
	defer m.wg.Done()
	
	ticker := time.NewTicker(m.config.HealthCheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.performHealthCheck()
		}
	}
}

// metricsUpdateLoop 指标更新循环
func (m *DefaultProviderManager) metricsUpdateLoop() {
	defer m.wg.Done()
	
	ticker := time.NewTicker(m.config.MetricsUpdateInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.updateMetrics()
		}
	}
}

// performHealthCheck 执行健康检查
func (m *DefaultProviderManager) performHealthCheck() {
	m.mutex.RLock()
	providers := make(map[string]Provider)
	for name, provider := range m.providers {
		providers[name] = provider
	}
	m.mutex.RUnlock()
	
	for name, provider := range providers {
		go m.checkProviderHealth(name, provider)
	}
}

// checkProviderHealth 检查单个提供商健康状态
func (m *DefaultProviderManager) checkProviderHealth(name string, provider Provider) {
	start := time.Now()
	
	ctx, cancel := context.WithTimeout(m.ctx, m.config.DefaultTimeout)
	defer cancel()
	
	err := provider.HealthCheck(ctx)
	responseTime := time.Since(start)
	
	m.statusMutex.Lock()
	defer m.statusMutex.Unlock()
	
	status := m.status[name]
	if status == nil {
		return
	}
	
	status.LastCheck = time.Now()
	status.ResponseTime = responseTime
	
	if err != nil {
		status.Available = false
		status.ErrorCount++
	} else {
		status.Available = true
		status.SuccessCount++
	}
}

// updateMetrics 更新指标
func (m *DefaultProviderManager) updateMetrics() {
	// 这里可以实现更复杂的指标收集和更新逻辑
	// 比如从各个提供商收集性能指标、成本统计等
}