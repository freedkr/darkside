package integration

import (
	"fmt"
	"os"
	"time"

	"github.com/freedkr/moonshot/internal/config"
	"gopkg.in/yaml.v3"
)

// LoadProcessingConfig 加载处理配置
func LoadProcessingConfig(cfg *config.Config) *ProcessingConfig {
	processingConfig := &ProcessingConfig{
		Services: struct {
			PDF PDFServiceConfig `yaml:"pdf"`
			LLM LLMServiceConfig `yaml:"llm"`
		}{
			PDF: PDFServiceConfig{
				BaseURL:        getConfigServiceURL("pdf-validator", "8000"),
				Timeout:        180 * time.Second,
				MaxRetries:     3,
				ValidationType: "standard",
			},
			LLM: LLMServiceConfig{
				BaseURL:    getConfigServiceURL("llm-service", "8090"),
				Timeout:    120 * time.Second,
				MaxRetries: 3,
				TaskTypes:  []string{"data_cleaning", "semantic_analysis"},
			},
		},
		Processing: struct {
			PDFTimeout            time.Duration `yaml:"pdf_timeout"`
			LLMTimeout            time.Duration `yaml:"llm_timeout"`
			PersistenceBatchSize  int           `yaml:"persistence_batch_size"`
			MaxRetries            int           `yaml:"max_retries"`
			RetryBackoff          time.Duration `yaml:"retry_backoff"`
		}{
			PDFTimeout:           180 * time.Second,
			LLMTimeout:           120 * time.Second,
			PersistenceBatchSize: 100,
			MaxRetries:           3,
			RetryBackoff:         2 * time.Second,
		},
		TestData: struct {
			PDFFilePath string `yaml:"pdf_file_path"`
		}{
			PDFFilePath: getTestPDFPath(),
		},
		Concurrency: getOptimizedConcurrencyConfig(),
	}

	return processingConfig
}

// getConfigServiceURL 获取服务URL - 配置专用版本
func getConfigServiceURL(serviceName, defaultPort string) string {
	switch serviceName {
	case "llm-service":
		if llmURL := os.Getenv("LLM_SERVICE_URL"); llmURL != "" {
			return llmURL
		}
		return fmt.Sprintf("llm-service:%s", defaultPort)
	case "pdf-validator":
		if pdfURL := os.Getenv("PDF_VALIDATOR_URL"); pdfURL != "" {
			return pdfURL
		}
		return fmt.Sprintf("pdf-validator:%s", defaultPort)
	default:
		return fmt.Sprintf("localhost:%s", defaultPort)
	}
}

// getTestPDFPath 获取测试PDF路径 - 保持原有逻辑
func getTestPDFPath() string {
	if pdfFilePath := os.Getenv("PDF_TEST_FILE_PATH"); pdfFilePath != "" {
		return pdfFilePath
	}

	// 默认路径，容器内使用绝对路径
	pdfFilePath := "/root/testdata/2025042918334715812.pdf"
	if _, err := os.Stat(pdfFilePath); os.IsNotExist(err) {
		// 如果容器内路径不存在，尝试相对路径（用于本地开发）
		pdfFilePath = "testdata/2025042918334715812.pdf"
	}
	
	return pdfFilePath
}

// getOptimizedConcurrencyConfig 获取优化的并发配置
func getOptimizedConcurrencyConfig() ConcurrencyConfig {
	return ConcurrencyConfig{
		GlobalQuotas: struct {
			MaxRPM        int `yaml:"max_rpm"`
			MaxConcurrent int `yaml:"max_concurrent"`
			MaxTPM        int `yaml:"max_tpm"`
		}{
			MaxRPM:        500,    // Kimi 账号限制
			MaxConcurrent: 100,    // Kimi 账号限制
			MaxTPM:        128000, // Kimi 账号限制
		},
		TaskAllocations: map[string]TaskAllocation{
			"data_cleaning": {
				RPMPercent:      0.4, // 40% RPM配额
				MaxConcurrent:   3,   // 优化后的并发数（原来是8）
				RequestInterval: 200 * time.Millisecond,
				Priority:        "high",
				AdaptiveRange: AdaptiveRange{
					MinConcurrency:     1,
					MaxConcurrency:     4,
					ScaleUpThreshold:   0.95, // 成功率95%以上可扩容
					ScaleDownThreshold: 0.1,  // 错误率10%以上需缩容
				},
			},
			"semantic_analysis": {
				RPMPercent:      0.3, // 30% RPM配额
				MaxConcurrent:   2,   // 语义分析相对简单，并发数较小
				RequestInterval: 250 * time.Millisecond,
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
			EnableAdaptive:     true,
			AdjustmentInterval: 30 * time.Second, // 每30秒调整一次
			MinSuccessRate:     0.8,              // 最小成功率80%
			MaxErrorRate:       0.2,              // 最大错误率20%
		},
	}
}

// SaveProcessingConfig 保存处理配置到文件
func SaveProcessingConfig(config *ProcessingConfig, filePath string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config failed: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("write config file failed: %w", err)
	}

	return nil
}

// LoadProcessingConfigFromFile 从文件加载处理配置
func LoadProcessingConfigFromFile(filePath string) (*ProcessingConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read config file failed: %w", err)
	}

	var config ProcessingConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("unmarshal config failed: %w", err)
	}

	return &config, nil
}

// ValidateProcessingConfig 验证处理配置
func ValidateProcessingConfig(config *ProcessingConfig) error {
	// 验证全局配额
	if config.Concurrency.GlobalQuotas.MaxRPM <= 0 {
		return fmt.Errorf("invalid MaxRPM: %d", config.Concurrency.GlobalQuotas.MaxRPM)
	}
	if config.Concurrency.GlobalQuotas.MaxConcurrent <= 0 {
		return fmt.Errorf("invalid MaxConcurrent: %d", config.Concurrency.GlobalQuotas.MaxConcurrent)
	}

	// 验证任务分配总和
	var totalRPMPercent float64
	var totalMaxConcurrent int
	
	for taskType, allocation := range config.Concurrency.TaskAllocations {
		totalRPMPercent += allocation.RPMPercent
		totalMaxConcurrent += allocation.MaxConcurrent
		
		// 验证单个任务配置
		if allocation.RPMPercent <= 0 || allocation.RPMPercent > 1 {
			return fmt.Errorf("invalid RPMPercent for task %s: %f", taskType, allocation.RPMPercent)
		}
		if allocation.MaxConcurrent <= 0 {
			return fmt.Errorf("invalid MaxConcurrent for task %s: %d", taskType, allocation.MaxConcurrent)
		}
		if allocation.RequestInterval <= 0 {
			return fmt.Errorf("invalid RequestInterval for task %s: %v", taskType, allocation.RequestInterval)
		}
	}

	// 验证总配额不超限
	if totalRPMPercent > 1.0 {
		return fmt.Errorf("total RPM percent exceeds 100%%: %f", totalRPMPercent)
	}
	if totalMaxConcurrent > config.Concurrency.GlobalQuotas.MaxConcurrent {
		return fmt.Errorf("total MaxConcurrent exceeds global limit: %d > %d", 
			totalMaxConcurrent, config.Concurrency.GlobalQuotas.MaxConcurrent)
	}

	// 验证服务配置
	if config.Services.PDF.BaseURL == "" {
		return fmt.Errorf("PDF service BaseURL cannot be empty")
	}
	if config.Services.LLM.BaseURL == "" {
		return fmt.Errorf("LLM service BaseURL cannot be empty")
	}

	// 验证测试数据路径
	if config.TestData.PDFFilePath == "" {
		return fmt.Errorf("PDF test file path cannot be empty")
	}

	return nil
}

// ConfigurationReport 生成配置报告
type ConfigurationReport struct {
	GlobalQuotas      string                  `json:"global_quotas"`
	TaskAllocations   map[string]string       `json:"task_allocations"`
	ExpectedRPMUsage  map[string]float64      `json:"expected_rpm_usage"`
	ConcurrencyLimits map[string]int          `json:"concurrency_limits"`
	ServiceEndpoints  map[string]string       `json:"service_endpoints"`
	Warnings          []string                `json:"warnings,omitempty"`
}

// GenerateConfigurationReport 生成配置报告
func GenerateConfigurationReport(config *ProcessingConfig) *ConfigurationReport {
	report := &ConfigurationReport{
		TaskAllocations:   make(map[string]string),
		ExpectedRPMUsage:  make(map[string]float64),
		ConcurrencyLimits: make(map[string]int),
		ServiceEndpoints:  make(map[string]string),
		Warnings:          []string{},
	}

	// 全局配额信息
	report.GlobalQuotas = fmt.Sprintf("RPM:%d, Concurrent:%d, TPM:%d",
		config.Concurrency.GlobalQuotas.MaxRPM,
		config.Concurrency.GlobalQuotas.MaxConcurrent,
		config.Concurrency.GlobalQuotas.MaxTPM)

	// 任务分配信息
	var totalRPMPercent float64
	var totalConcurrent int
	
	for taskType, allocation := range config.Concurrency.TaskAllocations {
		allocatedRPM := float64(config.Concurrency.GlobalQuotas.MaxRPM) * allocation.RPMPercent
		
		report.TaskAllocations[taskType] = fmt.Sprintf("RPM:%.0f(%.0f%%), Concurrent:%d, Interval:%v",
			allocatedRPM, allocation.RPMPercent*100, allocation.MaxConcurrent, allocation.RequestInterval)
		
		report.ExpectedRPMUsage[taskType] = allocatedRPM
		report.ConcurrencyLimits[taskType] = allocation.MaxConcurrent
		
		totalRPMPercent += allocation.RPMPercent
		totalConcurrent += allocation.MaxConcurrent
	}

	// 服务端点信息
	report.ServiceEndpoints["pdf"] = config.Services.PDF.BaseURL
	report.ServiceEndpoints["llm"] = config.Services.LLM.BaseURL

	// 生成警告
	if totalRPMPercent > 0.8 {
		report.Warnings = append(report.Warnings, 
			fmt.Sprintf("High RPM allocation: %.1f%% of total quota", totalRPMPercent*100))
	}
	
	if totalConcurrent > config.Concurrency.GlobalQuotas.MaxConcurrent*8/10 {
		report.Warnings = append(report.Warnings, 
			fmt.Sprintf("High concurrency allocation: %d/%d", totalConcurrent, config.Concurrency.GlobalQuotas.MaxConcurrent))
	}

	for taskType, allocation := range config.Concurrency.TaskAllocations {
		if allocation.RequestInterval < 100*time.Millisecond {
			report.Warnings = append(report.Warnings, 
				fmt.Sprintf("Very short request interval for %s: %v", taskType, allocation.RequestInterval))
		}
	}

	return report
}