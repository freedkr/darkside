// LLM Service 主程序入口
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/freedkr/moonshot/services/llm-service/internal/providers"
	"github.com/freedkr/moonshot/services/llm-service/internal/scheduler"
	"github.com/freedkr/moonshot/services/llm-service/internal/server"
)

func main() {
	log.Println("启动LLM服务...")

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建提供商管理器
	providerManager := createProviderManager()
	if err := providerManager.Start(ctx); err != nil {
		log.Fatalf("启动提供商管理器失败: %v", err)
	}
	defer providerManager.Stop(ctx)

	// 创建任务调度器
	taskScheduler := createTaskScheduler(providerManager)
	if err := taskScheduler.Start(ctx); err != nil {
		log.Fatalf("启动任务调度器失败: %v", err)
	}
	defer taskScheduler.Stop(ctx)

	// 创建HTTP服务器
	httpServer := createHTTPServer(taskScheduler, providerManager)
	if err := httpServer.Start(ctx); err != nil {
		log.Fatalf("启动HTTP服务器失败: %v", err)
	}
	defer httpServer.Stop(ctx)

	// 等待信号
	waitForShutdown()

	log.Println("LLM服务已停止")
}

// createProviderManager 创建提供商管理器
func createProviderManager() providers.ProviderManager {
	// 创建管理器
	config := providers.ManagerConfig{
		HealthCheckInterval:   30 * time.Second,
		MetricsUpdateInterval: 10 * time.Second,
		DefaultTimeout:        30 * time.Second,
		EnableAutoFailover:    true,
	}

	manager := providers.NewProviderManager(config)

	// 注册Kimi提供商
	kimiConfig := providers.ProviderConfig{
		Name:    "kimi",
		Type:    "kimi",
		Enabled: true,
		APIKey:  getEnvOrDefault("KIMI_API_KEY", ""),
		BaseURL: getEnvOrDefault("KIMI_BASE_URL", "https://api.moonshot.cn/v1"),
		RateLimit: providers.RateLimit{
			RequestsPerMinute:  500,    // 实际配额: 500 RPM
			RequestsPerHour:    30000,  // 计算值: 500 * 60
			RequestsPerDay:     720000, // 计算值: 500 * 60 * 24
			ConcurrentRequests: 100,    // 实际配额: 100 并发
			TokensPerMinute:    128000, // 实际配额: 128K TPM
			ResetInterval:      time.Minute,
		},
		Timeout:    500 * time.Second,
		MaxRetries: 2,
	}

	// 调试：打印API Key状态
	if kimiConfig.APIKey == "" {
		log.Printf("❌ 警告: 未设置KIMI_API_KEY环境变量，Kimi提供商将不可用")
		log.Printf("🔍 调试: KIMI_API_KEY环境变量值: [%s]", os.Getenv("KIMI_API_KEY"))
	} else {
		log.Printf("✅ 检测到KIMI_API_KEY: %s...", kimiConfig.APIKey[:10])
		kimiProvider, err := providers.CreateProvider(kimiConfig)
		if err != nil {
			log.Printf("创建Kimi提供商失败: %v", err)
		} else {
			if err := manager.RegisterProvider("kimi", kimiProvider); err != nil {
				log.Printf("❌ 注册Kimi提供商失败: %v", err)
			} else {
				log.Println("✅ 成功注册Kimi提供商")
				// 验证提供商是否可用
				if kimiProvider.IsAvailable(context.Background()) {
					log.Println("✅ Kimi提供商健康检查通过")
				} else {
					log.Println("❌ Kimi提供商健康检查失败")
				}
			}
		}
	}

	// 添加路由规则
	manager.AddRoutingRule(providers.RoutingRule{
		TaskType:      "semantic_analysis",
		Providers:     []string{"kimi"},
		CostWeight:    0.3,
		SpeedWeight:   0.7,
		QualityWeight: 1.0,
	})

	manager.AddRoutingRule(providers.RoutingRule{
		TaskType:      "data_cleaning",
		Providers:     []string{"kimi"},
		CostWeight:    0.5,
		SpeedWeight:   0.5,
		QualityWeight: 1.0,
	})

	return manager
}

// createTaskScheduler 创建任务调度器
func createTaskScheduler(providerManager providers.ProviderManager) scheduler.TaskScheduler {
	config := scheduler.SchedulerConfig{
		MaxWorkers:      getEnvIntOrDefault("LLM_MAX_WORKERS", 50),      // 增加到50个worker以支持高并发
		MaxQueueSize:    getEnvIntOrDefault("LLM_MAX_QUEUE_SIZE", 5000), // 增加队列容量
		TaskTimeout:     getEnvDurationOrDefault("LLM_TASK_TIMEOUT", 5*time.Minute),
		CleanupInterval: getEnvDurationOrDefault("LLM_CLEANUP_INTERVAL", time.Minute),
		StatsInterval:   getEnvDurationOrDefault("LLM_STATS_INTERVAL", 30*time.Second),
		RetryAttempts:   getEnvIntOrDefault("LLM_RETRY_ATTEMPTS", 3),
		RetryDelay:      getEnvDurationOrDefault("LLM_RETRY_DELAY", time.Second),
	}

	return scheduler.NewTaskScheduler(providerManager, config)
}

// createHTTPServer 创建HTTP服务器
func createHTTPServer(
	taskScheduler scheduler.TaskScheduler,
	providerManager providers.ProviderManager,
) *server.LLMServer {
	config := server.ServerConfig{
		Port:            getEnvIntOrDefault("LLM_PORT", 8090),
		ReadTimeout:     getEnvDurationOrDefault("LLM_READ_TIMEOUT", 30*time.Second),
		WriteTimeout:    getEnvDurationOrDefault("LLM_WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:     getEnvDurationOrDefault("LLM_IDLE_TIMEOUT", 60*time.Second),
		MaxRequestSize:  getEnvInt64OrDefault("LLM_MAX_REQUEST_SIZE", 32<<20), // 32MB
		EnableCORS:      getEnvBoolOrDefault("LLM_ENABLE_CORS", true),
		EnableMetrics:   getEnvBoolOrDefault("LLM_ENABLE_METRICS", true),
		EnableWebSocket: getEnvBoolOrDefault("LLM_ENABLE_WEBSOCKET", true),
		AuthToken:       getEnvOrDefault("LLM_AUTH_TOKEN", ""),
	}

	return server.NewLLMServer(taskScheduler, providerManager, config)
}

// waitForShutdown 等待关闭信号
func waitForShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	sig := <-quit
	log.Printf("收到信号 %v，正在关闭服务...", sig)

	// 给服务一些时间来优雅关闭
	time.Sleep(2 * time.Second)
}

// 环境变量辅助函数

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed := parseInt(value); parsed > 0 {
			return parsed
		}
	}
	return defaultValue
}

func getEnvInt64OrDefault(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if parsed := parseInt64(value); parsed > 0 {
			return parsed
		}
	}
	return defaultValue
}

func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}

func getEnvDurationOrDefault(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func parseInt(s string) int {
	val, err := strconv.Atoi(s)
	if err != nil {
		return 0 // 或者返回一个错误
	}
	return val
}

func parseInt64(s string) int64 {
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0 // 或者返回一个错误
	}
	return val
}
