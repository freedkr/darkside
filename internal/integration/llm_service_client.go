package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LLMServiceClient LLM服务客户端实现
type LLMServiceClient struct {
	config       LLMServiceConfig
	httpClient   *http.Client
	concurrency  ConcurrencyManager
	metrics      MetricsCollector
}

// NewLLMServiceClient 创建LLM服务客户端
func NewLLMServiceClient(config LLMServiceConfig) LLMService {
	return &LLMServiceClient{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		// concurrency 和 metrics 将在 orchestrator 中注入
	}
}

// SetDependencies 设置依赖（用于依赖注入）
func (c *LLMServiceClient) SetDependencies(concurrency ConcurrencyManager, metrics MetricsCollector) {
	c.concurrency = concurrency
	c.metrics = metrics
}

// CleanDataConcurrently 并发清洗数据
func (c *LLMServiceClient) CleanDataConcurrently(ctx context.Context, request LLMCleaningRequest) ([]CleanedDataItem, error) {
	startTime := time.Now()
	defer func() {
		c.metrics.RecordProcessingDuration("llm_data_cleaning", time.Since(startTime))
	}()

	// 按编码前缀分组数据（1-8大类）
	groups := c.groupPDFDataByPrefix(request.RawData)
	
	if len(groups) == 0 {
		return []CleanedDataItem{}, fmt.Errorf("no data groups found")
	}

	// 使用配额感知的并发处理
	results, err := c.processConcurrentlyWithQuota(ctx, groups, request.TaskType)
	if err != nil {
		c.metrics.RecordError("llm_data_cleaning", err)
		return nil, err
	}

	c.metrics.RecordSuccess("llm_data_cleaning")
	return results, nil
}

// AnalyzeSemanticsConcurrently 并发语义分析
func (c *LLMServiceClient) AnalyzeSemanticsConcurrently(ctx context.Context, request LLMSemanticRequest) ([]FinalResultItem, error) {
	startTime := time.Now()
	defer func() {
		c.metrics.RecordProcessingDuration("llm_semantic_analysis", time.Since(startTime))
	}()

	// 使用任务类型轮询实现并发，避免单一队列瓶颈
	results, err := c.processSemanticChoicesWithTaskRotation(ctx, request.Choices, request.TaskType)
	if err != nil {
		c.metrics.RecordError("llm_semantic_analysis", err)
		return nil, err
	}

	c.metrics.RecordSuccess("llm_semantic_analysis")
	return results, nil
}

// ProcessSingleTask 处理单个任务
func (c *LLMServiceClient) ProcessSingleTask(ctx context.Context, taskType string, prompt string) (string, error) {
	return c.callLLMServiceWithRetry(ctx, taskType, prompt, c.config.MaxRetries)
}

// groupPDFDataByPrefix 按编码前缀分组PDF数据
func (c *LLMServiceClient) groupPDFDataByPrefix(rawData []PDFOccupationCode) map[string][]PDFOccupationCode {
	groups := make(map[string][]PDFOccupationCode)

	for _, item := range rawData {
		prefix := c.getMainCategoryPrefix(item.Code)
		groups[prefix] = append(groups[prefix], item)
	}

	return groups
}

// getMainCategoryPrefix 获取主分类前缀
func (c *LLMServiceClient) getMainCategoryPrefix(code string) string {
	parts := strings.Split(code, "-")
	if len(parts) > 0 {
		return parts[0]
	}
	if len(code) > 0 {
		return string(code[0])
	}
	return "unknown"
}

// processConcurrentlyWithQuota 使用配额感知的并发处理
func (c *LLMServiceClient) processConcurrentlyWithQuota(ctx context.Context, groups map[string][]PDFOccupationCode, taskType string) ([]CleanedDataItem, error) {
	type groupResult struct {
		prefix string
		items  []CleanedDataItem
		err    error
	}

	resultCh := make(chan groupResult, len(groups))
	var wg sync.WaitGroup

	// 为每个分组启动goroutine，使用配额管理器控制并发
	for prefix, groupData := range groups {
		wg.Add(1)
		go func(prefix string, data []PDFOccupationCode) {
			defer wg.Done()

			// 获取并发许可
			if err := c.concurrency.AcquirePermit(ctx, taskType); err != nil {
				resultCh <- groupResult{prefix: prefix, err: err}
				return
			}
			defer c.concurrency.ReleasePermit(taskType)

			// 处理单个分组
			items, err := c.processSingleGroup(ctx, prefix, data, taskType)
			resultCh <- groupResult{
				prefix: prefix,
				items:  items,
				err:    err,
			}

			// 更新指标
			c.concurrency.UpdateMetrics(taskType, TaskMetrics{
				Duration: time.Since(time.Now()),
				Success:  err == nil,
				ErrorType: func() string {
					if err != nil {
						return err.Error()
					}
					return ""
				}(),
			})
		}(prefix, groupData)
	}

	// 等待所有goroutine完成
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 收集结果
	var allResults []CleanedDataItem
	var errors []error

	for result := range resultCh {
		if result.err != nil {
			errors = append(errors, fmt.Errorf("group %s failed: %w", result.prefix, result.err))
		} else {
			allResults = append(allResults, result.items...)
		}
	}

	// 检查错误
	if len(errors) > 0 {
		return allResults, fmt.Errorf("partial failures: %v", errors)
	}

	return allResults, nil
}

// processSingleGroup 处理单个分组
func (c *LLMServiceClient) processSingleGroup(ctx context.Context, prefix string, data []PDFOccupationCode, taskType string) ([]CleanedDataItem, error) {
	// 构建分组数据的JSON
	groupData := map[string]interface{}{
		"category": prefix,
		"items":    data,
	}
	
	jsonData, err := json.MarshalIndent(groupData, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal group data failed: %w", err)
	}

	// 构建清洗提示词
	prompt := c.buildCleaningPrompt(prefix, string(jsonData))

	// 调用LLM服务
	result, err := c.callLLMServiceWithRetry(ctx, taskType, prompt, c.config.MaxRetries)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// 解析结果
	return c.parseCleaningResult(result, prefix)
}

// buildCleaningPrompt 构建数据清洗提示词
func (c *LLMServiceClient) buildCleaningPrompt(prefix string, data string) string {
	return fmt.Sprintf(`你是一名数据清洗专家。请分析以下从PDF提取的第%s大类职业分类数据。

核心数据：
%s

清洗规则：
1. 识别所有有效的职业编码（格式如：%s-01-01-01）
2. 修正OCR识别错误
3. 标准化职业名称
4. 保持同一大类内的一致性
5. 去除描述性文字和无关内容

输出格式要求：
返回JSON数组，每个元素包含：
{
  "code": "职业编码",
  "name": "职业名称",
  "confidence": "置信度(0-1)",
  "source": "pdf",
  "level": "细类"
}

只返回JSON数组，不要有其他内容。`, prefix, data, prefix)
}

// parseCleaningResult 解析清洗结果
func (c *LLMServiceClient) parseCleaningResult(result string, prefix string) ([]CleanedDataItem, error) {
	// 清理响应，提取JSON部分
	cleanResult := c.extractJSON(result)

	var items []CleanedDataItem
	if err := json.Unmarshal([]byte(cleanResult), &items); err != nil {
		return nil, fmt.Errorf("parse cleaning result failed: %w", err)
	}

	// 后处理：设置处理时间和来源
	now := time.Now()
	for i := range items {
		items[i].ProcessedBy = "llm_cleaning"
		items[i].CleanedAt = now
	}

	return items, nil
}

// processSemanticChoicesWithTaskRotation 使用任务类型轮询处理语义选择
func (c *LLMServiceClient) processSemanticChoicesWithTaskRotation(ctx context.Context, choices []SemanticChoice, baseTaskType string) ([]FinalResultItem, error) {
	// 定义任务类型轮询池
	taskTypes := []string{
		"semantic_analysis",
		"data_cleaning", // 复用数据清洗队列
	}

	type itemResult struct {
		index  int
		result FinalResultItem
		err    error
	}

	resultCh := make(chan itemResult, len(choices))
	var wg sync.WaitGroup

	// 为每个选择项启动goroutine，轮询使用不同任务类型
	for i, choice := range choices {
		wg.Add(1)
		taskType := taskTypes[i%len(taskTypes)] // 轮询分配任务类型

		go func(idx int, item SemanticChoice, tType string) {
			defer wg.Done()

			// 获取并发许可
			if err := c.concurrency.AcquirePermit(ctx, tType); err != nil {
				resultCh <- itemResult{index: idx, err: err}
				return
			}
			defer c.concurrency.ReleasePermit(tType)

			// 处理单个语义选择
			result, err := c.processSingleSemanticChoice(ctx, item, tType)
			resultCh <- itemResult{
				index:  idx,
				result: result,
				err:    err,
			}

			// 更新指标
			c.concurrency.UpdateMetrics(tType, TaskMetrics{
				Duration: time.Since(time.Now()),
				Success:  err == nil,
				ErrorType: func() string {
					if err != nil {
						return err.Error()
					}
					return ""
				}(),
			})
		}(i, choice, taskType)
	}

	// 等待所有goroutine完成
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 收集结果并保持顺序
	results := make([]FinalResultItem, len(choices))
	errorCount := 0

	for res := range resultCh {
		if res.err != nil {
			errorCount++
			// 使用默认值
			results[res.index] = c.createDefaultResult(choices[res.index])
		} else {
			results[res.index] = res.result
		}
	}

	// 检查错误率
	if errorCount > len(choices)/2 {
		return results, fmt.Errorf("too many failures: %d/%d", errorCount, len(choices))
	}

	return results, nil
}

// processSingleSemanticChoice 处理单个语义选择
func (c *LLMServiceClient) processSingleSemanticChoice(ctx context.Context, choice SemanticChoice, taskType string) (FinalResultItem, error) {
	// 构建语义分析提示词
	prompt := c.buildSemanticPrompt(choice)

	// 调用LLM服务
	result, err := c.callLLMServiceWithRetry(ctx, taskType, prompt, c.config.MaxRetries)
	if err != nil {
		return FinalResultItem{}, fmt.Errorf("LLM call failed: %w", err)
	}

	// 解析结果
	return c.parseSemanticResult(result, choice)
}

// buildSemanticPrompt 构建语义分析提示词
func (c *LLMServiceClient) buildSemanticPrompt(choice SemanticChoice) string {
	return fmt.Sprintf(`你是职业分类专家。请为以下职业编码选择最合适的名称：

编码：%s
选项1：%s
选项2：%s
父级类别：%s

选择规则：
- 只能选择选项1或选项2，不能创造新名称
- 选择与父级层次语义更连贯的名称
- 优先选择完整的、名词性的职业名称
- 如果两个名称相似，选择更完整、更规范的版本
- 排除包含"本小类包括"、"进行..."、"担任..."等描述性短语

返回JSON格式：
{
  "code": "编码",
  "name": "选择后的名称",
  "parent_name": "父级类别名称",
  "selected_from": "rule"或"pdf"
}`, choice.Code, choice.RuleName, choice.PDFName, choice.ParentHierarchy)
}

// parseSemanticResult 解析语义分析结果
func (c *LLMServiceClient) parseSemanticResult(result string, choice SemanticChoice) (FinalResultItem, error) {
	// 清理响应，提取JSON部分
	cleanResult := c.extractJSON(result)

	var semanticResult map[string]interface{}
	if err := json.Unmarshal([]byte(cleanResult), &semanticResult); err != nil {
		return FinalResultItem{}, fmt.Errorf("parse semantic result failed: %w", err)
	}

	// 构建最终结果
	finalResult := FinalResultItem{
		Code:        choice.Code,
		Level:       "细类",
		Source:      "llm_semantic",
		ProcessedAt: time.Now(),
	}

	// 解析名称
	if name, ok := semanticResult["name"].(string); ok {
		finalResult.Name = name
	} else {
		finalResult.Name = choice.RuleName // 默认值
	}

	// 解析父级编码
	if parentCode, ok := semanticResult["parent_code"].(string); ok {
		finalResult.ParentCode = parentCode
	} else {
		finalResult.ParentCode = c.inferParentCode(choice.Code)
	}

	// 解析元数据
	finalResult.Metadata.ProcessingStage = "semantic_analysis"
	if selectedFrom, ok := semanticResult["selected_from"].(string); ok {
		finalResult.Metadata.SelectedFrom = selectedFrom
		// 设置备选名称
		if selectedFrom == "rule" {
			finalResult.Metadata.AlternativeName = choice.PDFName
		} else {
			finalResult.Metadata.AlternativeName = choice.RuleName
		}
	}

	// 计算置信度
	finalResult.Confidence = c.calculateConfidence(choice, semanticResult)
	finalResult.Metadata.QualityScore = finalResult.Confidence

	return finalResult, nil
}

// createDefaultResult 创建默认结果
func (c *LLMServiceClient) createDefaultResult(choice SemanticChoice) FinalResultItem {
	return FinalResultItem{
		Code:        choice.Code,
		Name:        choice.RuleName, // 默认使用规则名称
		Level:       "细类",
		ParentCode:  c.inferParentCode(choice.Code),
		Source:      "default_fallback",
		Confidence:  0.5, // 中等置信度
		ProcessedAt: time.Now(),
		Metadata: struct {
			SelectedFrom    string  `json:"selected_from"`
			AlternativeName string  `json:"alternative_name,omitempty"`
			ProcessingStage string  `json:"processing_stage"`
			QualityScore    float64 `json:"quality_score"`
		}{
			SelectedFrom:    "rule",
			AlternativeName: choice.PDFName,
			ProcessingStage: "fallback",
			QualityScore:    0.5,
		},
	}
}

// calculateConfidence 计算置信度
func (c *LLMServiceClient) calculateConfidence(choice SemanticChoice, result map[string]interface{}) float64 {
	baseConfidence := 0.8

	// 根据规则和PDF的置信度调整
	ruleConf := choice.Confidence.RuleConfidence
	pdfConf := choice.Confidence.PDFConfidence

	if ruleConf > 0 && pdfConf > 0 {
		// 两个来源都有数据，取平均并加权
		avgConf := (ruleConf + pdfConf) / 2
		return baseConfidence * avgConf
	} else if ruleConf > 0 {
		// 只有规则数据
		return baseConfidence * ruleConf
	} else if pdfConf > 0 {
		// 只有PDF数据
		return baseConfidence * pdfConf * 0.8 // PDF数据权重稍低
	}

	return baseConfidence
}

// inferParentCode 推断父级编码
func (c *LLMServiceClient) inferParentCode(code string) string {
	parts := strings.Split(code, "-")
	if len(parts) > 1 {
		return strings.Join(parts[:len(parts)-1], "-")
	}
	return ""
}

// extractJSON 从响应中提取JSON部分
func (c *LLMServiceClient) extractJSON(response string) string {
	response = strings.TrimSpace(response)
	
	// 移除markdown标记
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
		response = strings.TrimSuffix(response, "```")
		response = strings.TrimSpace(response)
	} else if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
		response = strings.TrimSuffix(response, "```")
		response = strings.TrimSpace(response)
	}

	// 查找JSON数组
	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")
	if start != -1 && end != -1 && end > start {
		return response[start : end+1]
	}

	// 查找JSON对象
	start = strings.Index(response, "{")
	end = strings.LastIndex(response, "}")
	if start != -1 && end != -1 && end > start {
		return response[start : end+1]
	}

	return response
}

// callLLMServiceWithRetry 带重试的LLM服务调用
func (c *LLMServiceClient) callLLMServiceWithRetry(ctx context.Context, taskType string, prompt string, maxRetries int) (string, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			// 指数退避
			backoff := time.Duration(i*i) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}

		result, err := c.callLLMServiceAsync(ctx, taskType, prompt)
		if err == nil {
			return result, nil
		}

		lastErr = err
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}

	return "", fmt.Errorf("LLM service call failed after %d retries: %w", maxRetries, lastErr)
}

// callLLMServiceAsync 异步调用LLM服务
func (c *LLMServiceClient) callLLMServiceAsync(ctx context.Context, taskType string, prompt string) (string, error) {
	fmt.Printf("🌐 [LLM调用] 任务类型=%s, Prompt长度=%d\n", taskType, len(prompt))
	
	// 构建请求
	request := map[string]interface{}{
		"type":    taskType,
		"prompt":  prompt,
		"model":   "moonshot-v1-128k",
		"priority": "normal",
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("marshal request failed: %w", err)
	}

	// 调用LLM服务
	url := fmt.Sprintf("http://%s/api/v1/tasks", c.config.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		fmt.Printf("❌ [LLM调用失败] HTTP请求错误: %v\n", err)
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
		fmt.Printf("❌ [LLM调用失败] 响应状态码: %d\n", resp.StatusCode)
		return "", fmt.Errorf("LLM service returned error %d", resp.StatusCode)
	}

	// 获取任务ID
	var taskResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&taskResp); err != nil {
		return "", fmt.Errorf("decode task response failed: %w", err)
	}

	taskID, ok := taskResp["task_id"].(string)
	if !ok {
		fmt.Printf("❌ [LLM调用失败] 响应中无效的task_id: %+v\n", taskResp)
		return "", fmt.Errorf("invalid task_id in response")
	}

	fmt.Printf("🕒 [LLM等待] 任务ID=%s, 等待结果...\n", taskID)
	// 等待结果
	return c.waitForLLMResult(ctx, taskID)
}

// waitForLLMResult 等待LLM任务完成
func (c *LLMServiceClient) waitForLLMResult(ctx context.Context, taskID string) (string, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeout := time.After(5 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timeout:
			return "", fmt.Errorf("LLM task timeout")
		case <-ticker.C:
			status, err := c.checkLLMTaskStatus(ctx, taskID)
			if err != nil {
				continue // 重试
			}

			statusStr := status["status"].(string)
			switch statusStr {
			case "completed", "success":
				if result, ok := status["result"].(string); ok {
					fmt.Printf("✅ [LLM完成] 任务ID=%s, 结果长度=%d\n", taskID, len(result))
					// 验证结果是否为有效的JSON
					result = strings.TrimSpace(result)
					if result == "" {
						fmt.Printf("⚠️ [LLM结果为空] 任务ID=%s\n", taskID)
						return "", fmt.Errorf("LLM返回空结果")
					}
					
					// 打印结果的前200字符作为示例
					if len(result) > 200 {
						fmt.Printf("  📝 [LLM结果示例] %s...\n", result[:200])
					} else {
						fmt.Printf("  📝 [LLM结果] %s\n", result)
					}
					
					// 检查是否是截断的结果
					if !strings.HasSuffix(result, "}") && !strings.HasSuffix(result, "]") {
						fmt.Printf("⚠️ [LLM结果可能被截断] 结果不以}或]结尾\n")
					}
					
					return result, nil
				}
				fmt.Printf("⚠️ [LLM空结果] 任务ID=%s\n", taskID)
				return "", fmt.Errorf("empty result")
			case "failed", "error":
				errorMsg := "unknown error"
				if errStr, ok := status["error"].(string); ok {
					errorMsg = errStr
				}
				fmt.Printf("❌ [LLM失败] 任务ID=%s, 错误=%s\n", taskID, errorMsg)
				return "", fmt.Errorf("LLM task failed: %s", errorMsg)
			case "cancelled":
				fmt.Printf("❌ [LLM取消] 任务ID=%s\n", taskID)
				return "", fmt.Errorf("LLM task cancelled")
			case "processing", "queued":
				// 正在处理，继续等待
			default:
				fmt.Printf("🔄 [LLM状态] 任务ID=%s, 状态=%s\n", taskID, statusStr)
			}
		}
	}
}

// checkLLMTaskStatus 检查LLM任务状态
func (c *LLMServiceClient) checkLLMTaskStatus(ctx context.Context, taskID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("http://%s/api/v1/tasks/%s", c.config.BaseURL, taskID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status check failed: %d", resp.StatusCode)
	}

	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}

	return status, nil
}