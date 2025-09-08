package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/freedkr/moonshot/internal/model"
)

// BatchProcessor PDF和LLM批量并发处理器
type BatchProcessor struct {
	processor     *PDFLLMProcessor
	batchSize     int
	maxConcurrent int
}

// NewBatchProcessor 创建批量处理器
func NewBatchProcessor(processor *PDFLLMProcessor) *BatchProcessor {
	return &BatchProcessor{
		processor:     processor,
		batchSize:     100, // 每批100条数据
		maxConcurrent: 8,   // 最多8个并发
	}
}

// ProcessPDFDataConcurrently 并发处理PDF数据
func (b *BatchProcessor) ProcessPDFDataConcurrently(ctx context.Context, pdfData map[string]interface{}) ([]map[string]interface{}, error) {
	fmt.Printf("DEBUG: ProcessPDFDataConcurrently 开始执行\n")

	// 1. 按照编码前缀分组（如 1-xx, 2-xx, 3-xx）
	groups := b.groupByCodePrefix(pdfData)
	fmt.Printf("DEBUG: 分组完成，共 %d 个分组\n", len(groups))

	// 2. 创建结果收集通道
	resultCh := make(chan []map[string]interface{}, len(groups))
	errorCh := make(chan error, len(groups))
	fmt.Printf("DEBUG: 通道创建完成\n")

	// 3. 使用信号量控制并发数
	sem := make(chan struct{}, b.maxConcurrent)
	var wg sync.WaitGroup
	fmt.Printf("DEBUG: 开始启动 %d 个并发goroutine\n", len(groups))

	// 4. 并发处理每个组
	for prefix, groupData := range groups {
		wg.Add(1)
		fmt.Printf("DEBUG: 启动goroutine处理分组 %s\n", prefix)
		go func(prefix string, data map[string]interface{}) {
			defer wg.Done()
			defer fmt.Printf("DEBUG: 分组 %s goroutine 结束\n", prefix)

			fmt.Printf("DEBUG: 分组 %s 获取信号量\n", prefix)
			// 获取信号量
			sem <- struct{}{}
			defer func() {
				<-sem
				fmt.Printf("DEBUG: 分组 %s 释放信号量\n", prefix)
			}()

			fmt.Printf("DEBUG: 分组 %s 开始调用processSingleGroup\n", prefix)
			// 处理这一组数据
			result, err := b.processSingleGroup(ctx, prefix, data)
			if err != nil {
				fmt.Printf("DEBUG: 分组 %s 处理失败: %v\n", prefix, err)
				errorCh <- fmt.Errorf("处理组 %s 失败: %w", prefix, err)
				return
			}

			fmt.Printf("DEBUG: 分组 %s 处理成功，发送结果\n", prefix)
			resultCh <- result
		}(prefix, groupData)
	}

	fmt.Printf("DEBUG: 所有goroutine已启动，开始等待完成\n")

	// 等待所有goroutine完成
	go func() {
		fmt.Printf("DEBUG: 开始等待所有goroutine完成\n")
		wg.Wait()
		fmt.Printf("DEBUG: 所有goroutine完成，关闭通道\n")
		close(resultCh)
		close(errorCh)
	}()

	// 5. 收集结果
	fmt.Printf("DEBUG: 开始收集结果\n")
	var allResults []map[string]interface{}
	var errors []error

	for {
		select {
		case result, ok := <-resultCh:
			if !ok {
				fmt.Printf("DEBUG: resultCh 已关闭\n")
				resultCh = nil
			} else {
				fmt.Printf("DEBUG: 收到结果，长度: %d\n", len(result))
				allResults = append(allResults, result...)
			}
		case err, ok := <-errorCh:
			if !ok {
				fmt.Printf("DEBUG: errorCh 已关闭\n")
				errorCh = nil
			} else {
				fmt.Printf("DEBUG: 收到错误: %v\n", err)
				errors = append(errors, err)
			}
		}

		if resultCh == nil && errorCh == nil {
			fmt.Printf("DEBUG: 所有通道已关闭，退出收集循环\n")
			break
		}
	}

	fmt.Printf("DEBUG: 结果收集完成，allResults长度: %d, errors长度: %d\n", len(allResults), len(errors))

	// 检查错误
	if len(errors) > 0 {
		fmt.Printf("DEBUG: 发现错误，返回失败: %v\n", errors)
		return allResults, fmt.Errorf("部分组处理失败: %v", errors)
	}

	fmt.Printf("DEBUG: ProcessPDFDataConcurrently 成功完成\n")
	return allResults, nil
}

// groupByCodePrefix 按编码前缀分组
func (b *BatchProcessor) groupByCodePrefix(pdfData map[string]interface{}) map[string]map[string]interface{} {
	groups := make(map[string]map[string]interface{})

	// 首先尝试PDF服务格式 {"occupation_codes": [...]}
	var items []interface{}
	if occupationCodes, ok := pdfData["occupation_codes"].([]interface{}); ok {
		items = occupationCodes
		fmt.Printf("DEBUG: groupByCodePrefix 找到occupation_codes数组，长度: %d\n", len(items))
	} else if itemsArray, ok := pdfData["items"].([]interface{}); ok {
		// 备用：尝试items格式
		items = itemsArray
		fmt.Printf("DEBUG: groupByCodePrefix 找到items数组，长度: %d\n", len(items))
	} else {
		// 如果不是预期格式，作为单个组处理
		fmt.Printf("DEBUG: groupByCodePrefix 未找到occupation_codes或items字段，使用all组\n")
		groups["all"] = pdfData
		return groups
	}

	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		code, ok := itemMap["code"].(string)
		if !ok {
			continue
		}

		// 获取主分类前缀（如 "1", "2", "3" 等）
		prefix := getMainCategory(code)

		if groups[prefix] == nil {
			groups[prefix] = map[string]interface{}{
				"occupation_codes": []interface{}{},
			}
		}

		groupItems := groups[prefix]["occupation_codes"].([]interface{})
		groups[prefix]["occupation_codes"] = append(groupItems, item)
	}

	return groups
}

// getMainCategory 获取主分类
func getMainCategory(code string) string {
	// 处理不同格式的编码
	// "1-01-01-01" -> "1"
	// "2-03" -> "2"
	parts := strings.Split(code, "-")
	if len(parts) > 0 {
		return parts[0]
	}

	// 如果没有连字符，取第一个字符
	if len(code) > 0 {
		return string(code[0])
	}

	return "unknown"
}

// processSingleGroup 处理单个分组
func (b *BatchProcessor) processSingleGroup(ctx context.Context, prefix string, data map[string]interface{}) ([]map[string]interface{}, error) {
	fmt.Printf("DEBUG: processSingleGroup 开始处理分组 %s\n", prefix)

	// 第一步：提取核心字段(code和name)，减少token使用
	coreData := extractCoreFields(data)
	fmt.Printf("DEBUG: 分组 %s 提取核心字段完成\n", prefix)

	// 调试：记录提取的核心字段数量
	if items, ok := coreData["items"].([]interface{}); ok {
		fmt.Printf("DEBUG: 分组 %s 提取了 %d 个核心条目（只包含code和name）\n", prefix, len(items))
		if len(items) > 0 {
			// 显示第一个条目的结构
			if firstItem, ok := items[0].(map[string]interface{}); ok {
				fields := make([]string, 0, len(firstItem))
				for key := range firstItem {
					fields = append(fields, key)
				}
				fmt.Printf("DEBUG: 核心字段包含: %v\n", fields)
			}
		}
	}

	fmt.Printf("DEBUG: 分组 %s 即将构建prompt - 检查点A\n", prefix)

	// 检查context状态
	select {
	case <-ctx.Done():
		fmt.Printf("DEBUG: 分组 %s context已取消: %v\n", prefix, ctx.Err())
		return nil, ctx.Err()
	default:
		fmt.Printf("DEBUG: 分组 %s context正常\n", prefix)
	}

	fmt.Printf("DEBUG: 分组 %s 开始构建prompt\n", prefix)
	// 构建针对这个分组的prompt，只包含核心字段
	prompt := fmt.Sprintf(`你是一名数据清洗专家。以下是一份列表，其中每个对象包含编码（code）、名称（name）及其他元数据。你的任务是根据以下规则，为每个唯一的编码（code）从其关联的名称列表中，选出最准确、最精炼的职业名称。

请严格遵守以下规则进行判断：

1.  **分组处理**：将列表中的数据按 code 字段进行分组。
2.  **语义组合判断**：
    * **优先选择**：如果一个 code 对应的多个 name 中，只有一个是完整的、名词性的职业或实体名称，那么这一个就是正确的名称。
    * **次要排除**：如果一个 code 下的名称包含"本小类包括下列职业"、"进行..."或"担任..."等描述性或动词性短语，则这些名称应被排除。它们是辅助性说明，不是最终的职业名称。
    * **完整性优先**：对于像"航天动力装置制造工"和"航天动力装置制造工程技术人员"这样的情况，如果"航天动力装置制造工程技术人员"是完整的，而另一个是截断的（根据文本内容判断），则优先选择完整的名称。
3.  **最终输出**：以 code: name 的JSON格式输出最终确认的词表列表。

请使用此方法处理以下JSON数据，并仅返回最终结果。

%s

输出JSON数组格式，不要有其他内容：
[
  {
    "code": "职业编码",
    "name": "职业名称",
    "confidence": "置信度(0-1)"
  }
]
`, jsonString(coreData))

	fmt.Printf("DEBUG: 分组 %s 开始调用LLM服务\n", prefix)
	// 调用LLM服务
	result, err := b.processor.callLLMService(ctx, "data_cleaning", prompt)
	if err != nil {
		fmt.Printf("DEBUG: 分组 %s LLM调用失败: %v\n", prefix, err)
		return nil, err
	}
	fmt.Printf("DEBUG: 分组 %s LLM调用成功，结果长度: %d\n", prefix, len(result))
	
	// 打印LLM原始响应以便调试
	if len(result) > 0 {
		fmt.Printf("🔍 [分组%s-LLM原始响应] 长度=%d\n", prefix, len(result))
		if len(result) <= 500 {
			fmt.Printf("📝 [分组%s-完整响应]:\n%s\n", prefix, result)
		} else {
			// 打印开头和结尾
			fmt.Printf("📝 [分组%s-响应开头200字符]:\n%s\n", prefix, result[:200])
			fmt.Printf("📝 [分组%s-响应结尾200字符]:\n%s\n", prefix, result[len(result)-200:])
		}
		
		// 检查是否可能被截断
		if !strings.HasSuffix(strings.TrimSpace(result), "]") && !strings.HasSuffix(strings.TrimSpace(result), "}") {
			fmt.Printf("⚠️ [分组%s-可能截断] 结果不以}]结尾\n", prefix)
		}
	}

	fmt.Printf("DEBUG: 分组 %s 开始解析结果\n", prefix)
	// 解析结果 - 处理两种格式：{"items": [...]} 或 直接的JSON数组
	var cleanedData []map[string]interface{}
	
	// 不要清理，直接使用原始结果（因为extractJSON可能破坏wrapper格式）
	cleanResult := strings.TrimSpace(result)
	
	// 移除可能的markdown标记
	if strings.HasPrefix(cleanResult, "```json") {
		cleanResult = strings.TrimPrefix(cleanResult, "```json")
		cleanResult = strings.TrimSuffix(cleanResult, "```")
		cleanResult = strings.TrimSpace(cleanResult)
	} else if strings.HasPrefix(cleanResult, "```") {
		cleanResult = strings.TrimPrefix(cleanResult, "```")
		cleanResult = strings.TrimSuffix(cleanResult, "```")
		cleanResult = strings.TrimSpace(cleanResult)
	}
	
	if cleanResult != result {
		fmt.Printf("🔄 [分组%s-清理Markdown] 原始长度=%d, 清理后=%d\n", prefix, len(result), len(cleanResult))
	}
	
	// 检测JSON格式 - 根据第一个字符判断
	isArray := strings.HasPrefix(cleanResult, "[")
	isObject := strings.HasPrefix(cleanResult, "{")
	
	fmt.Printf("🔍 [分组%s-格式检测] isObject=%v, isArray=%v\n", prefix, isObject, isArray)
	
	if isObject {
		// 对象格式，尝试 wrapper 格式
		var resultWrapper struct {
			Items []map[string]interface{} `json:"items"`
		}
		if err := json.Unmarshal([]byte(cleanResult), &resultWrapper); err != nil {
			fmt.Printf("⚠️ [分组%s-wrapper格式失败] 错误: %v\n", prefix, err)
			// wrapper格式失败，可能是其他对象格式
			return nil, fmt.Errorf("wrapper格式解析失败: %v", err)
		}
		cleanedData = resultWrapper.Items
		fmt.Printf("✅ [分组%s-解析成功] wrapper格式，获得 %d 条数据\n", prefix, len(cleanedData))
		
	} else if isArray {
		// 数组格式
		if err := json.Unmarshal([]byte(cleanResult), &cleanedData); err != nil {
			fmt.Printf("⚠️ [分组%s-数组格式失败] 错误: %v\n", prefix, err)
			
			// 尝试部分解析
			fmt.Printf("🔄 [分组%s-尝试部分解析]...\n", prefix)
			if partialData := tryParsePartialJSON(cleanResult); partialData != nil && len(partialData) > 0 {
				fmt.Printf("✅ [分组%s-部分解析成功] 解析出 %d 条数据\n", prefix, len(partialData))
				return partialData, nil
			}
			return nil, fmt.Errorf("数组格式解析失败: %v", err)
		}
		fmt.Printf("✅ [分组%s-解析成功] 直接数组格式，获得 %d 条数据\n", prefix, len(cleanedData))
		
	} else {
		// 可能是双重编码的字符串
		var jsonString string
		if err := json.Unmarshal([]byte(cleanResult), &jsonString); err != nil {
			fmt.Printf("❌ [分组%s-无法识别格式] 既不是对象也不是数组\n", prefix)
			return nil, fmt.Errorf("无法识别的JSON格式")
		}
		
		fmt.Printf("🔄 [分组%s-双重编码] 检测到JSON字符串，二次解析...\n", prefix)
		// 递归调用自己来解析
		return b.processSingleGroup(ctx, prefix, map[string]interface{}{"result": jsonString})
	}
	fmt.Printf("DEBUG: 分组 %s 解析成功，清洗后数据条数: %d\n", prefix, len(cleanedData))

	return cleanedData, nil
}

// ProcessInBatches 分批处理数据
func (b *BatchProcessor) ProcessInBatches(ctx context.Context, categories []*model.Category) ([]map[string]interface{}, error) {
	// 将categories分批
	batches := b.splitIntoBatches(categories)

	// 创建worker池
	workerCount := b.maxConcurrent
	if len(batches) < workerCount {
		workerCount = len(batches)
	}

	// 创建任务通道和结果通道
	taskCh := make(chan []*model.Category, len(batches))
	resultCh := make(chan batchResult, len(batches))

	// 启动workers
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go b.batchWorker(ctx, i, taskCh, resultCh, &wg)
	}

	// 分发任务
	for _, batch := range batches {
		taskCh <- batch
	}
	close(taskCh)

	// 等待所有worker完成
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 收集结果
	var allResults []map[string]interface{}
	errors := []error{}

	for result := range resultCh {
		if result.err != nil {
			errors = append(errors, result.err)
		} else {
			allResults = append(allResults, result.data...)
		}
	}

	if len(errors) > 0 {
		return allResults, fmt.Errorf("批处理中有错误: %v", errors)
	}

	return allResults, nil
}

// splitIntoBatches 将数据分批
func (b *BatchProcessor) splitIntoBatches(categories []*model.Category) [][]*model.Category {
	var batches [][]*model.Category

	// 先按大类分组
	groupedByMain := make(map[string][]*model.Category)

	for _, cat := range categories {
		if cat == nil {
			continue
		}
		mainCat := getMainCategory(cat.Code)
		groupedByMain[mainCat] = append(groupedByMain[mainCat], cat)
	}

	// 每个大类再按batchSize分批
	for _, group := range groupedByMain {
		for i := 0; i < len(group); i += b.batchSize {
			end := i + b.batchSize
			if end > len(group) {
				end = len(group)
			}
			batches = append(batches, group[i:end])
		}
	}

	return batches
}

// batchResult 批处理结果
type batchResult struct {
	data []map[string]interface{}
	err  error
}

// batchWorker 批处理工作协程
func (b *BatchProcessor) batchWorker(ctx context.Context, id int, taskCh <-chan []*model.Category, resultCh chan<- batchResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for batch := range taskCh {
		// 处理一批数据
		result, err := b.processBatch(ctx, id, batch)

		select {
		case resultCh <- batchResult{data: result, err: err}:
		case <-ctx.Done():
			return
		}
	}
}

// processBatch 处理一批数据
func (b *BatchProcessor) processBatch(ctx context.Context, workerID int, batch []*model.Category) ([]map[string]interface{}, error) {
	// 转换为map格式
	var items []map[string]interface{}
	for _, cat := range batch {
		items = append(items, map[string]interface{}{
			"code":  cat.Code,
			"name":  cat.Name,
			"level": cat.Level,
		})
	}

	// 构建prompt
	prompt := fmt.Sprintf(`分析并优化以下职业分类数据（批次%d，共%d条）：

%s

要求：
1. 验证编码格式正确性
2. 优化职业名称表述
3. 确保层级关系合理

输出JSON数组。`, workerID, len(items), jsonString(items))

	// 调用LLM（带重试）
	result, err := b.processor.callLLMServiceWithRetry(ctx, "batch_processing", prompt, 3)
	if err != nil {
		return nil, fmt.Errorf("worker %d 处理失败: %w", workerID, err)
	}

	// 解析结果
	var processedData []map[string]interface{}
	if err := json.Unmarshal([]byte(result), &processedData); err != nil {
		return nil, fmt.Errorf("worker %d 解析结果失败: %w", workerID, err)
	}

	return processedData, nil
}

// OptimizeWithPipeline 使用pipeline模式优化处理
func (b *BatchProcessor) OptimizeWithPipeline(ctx context.Context, taskID string, categories []*model.Category) error {
	// 创建pipeline阶段
	stages := []pipelineStage{
		{name: "分组", fn: b.groupingStage},
		{name: "清洗", fn: b.cleaningStage},
		{name: "验证", fn: b.validationStage},
		{name: "合并", fn: b.mergeStage},
	}

	// 执行pipeline
	data := &pipelineData{
		taskID:     taskID,
		categories: categories,
		results:    make(map[string]interface{}),
	}

	for _, stage := range stages {
		start := time.Now()
		if err := stage.fn(ctx, data); err != nil {
			return fmt.Errorf("pipeline阶段 %s 失败: %w", stage.name, err)
		}
		fmt.Printf("Pipeline阶段 %s 完成，耗时: %v\n", stage.name, time.Since(start))
	}

	return nil
}

// pipelineStage pipeline阶段
type pipelineStage struct {
	name string
	fn   func(context.Context, *pipelineData) error
}

// pipelineData pipeline数据
type pipelineData struct {
	taskID     string
	categories []*model.Category
	results    map[string]interface{}
	mu         sync.RWMutex
}

// groupingStage 分组阶段
func (b *BatchProcessor) groupingStage(ctx context.Context, data *pipelineData) error {
	// 按大类分组
	grouped := make(map[string][]*model.Category)
	for _, cat := range data.categories {
		mainCat := getMainCategory(cat.Code)
		grouped[mainCat] = append(grouped[mainCat], cat)
	}

	data.mu.Lock()
	data.results["grouped"] = grouped
	data.mu.Unlock()

	return nil
}

// cleaningStage 清洗阶段
func (b *BatchProcessor) cleaningStage(ctx context.Context, data *pipelineData) error {
	data.mu.RLock()
	grouped := data.results["grouped"].(map[string][]*model.Category)
	data.mu.RUnlock()

	// 并发清洗每个组
	var wg sync.WaitGroup
	cleanedResults := make(map[string][]map[string]interface{})
	var mu sync.Mutex

	for mainCat, cats := range grouped {
		wg.Add(1)
		go func(mc string, categories []*model.Category) {
			defer wg.Done()

			// 清洗这一组
			cleaned, err := b.ProcessInBatches(ctx, categories)
			if err != nil {
				fmt.Printf("清洗组 %s 失败: %v\n", mc, err)
				return
			}

			mu.Lock()
			cleanedResults[mc] = cleaned
			mu.Unlock()
		}(mainCat, cats)
	}

	wg.Wait()

	data.mu.Lock()
	data.results["cleaned"] = cleanedResults
	data.mu.Unlock()

	return nil
}

// validationStage 验证阶段
func (b *BatchProcessor) validationStage(ctx context.Context, data *pipelineData) error {
	// 验证清洗后的数据
	data.mu.RLock()
	cleaned := data.results["cleaned"].(map[string][]map[string]interface{})
	data.mu.RUnlock()

	validated := make(map[string][]map[string]interface{})

	for mainCat, items := range cleaned {
		validItems := []map[string]interface{}{}
		for _, item := range items {
			if b.validateItem(item) {
				validItems = append(validItems, item)
			}
		}
		validated[mainCat] = validItems
	}

	data.mu.Lock()
	data.results["validated"] = validated
	data.mu.Unlock()

	return nil
}

// validateItem 验证单个项目
func (b *BatchProcessor) validateItem(item map[string]interface{}) bool {
	// 验证必要字段
	_, hasCode := item["code"].(string)
	_, hasName := item["name"].(string)

	return hasCode && hasName
}

// mergeStage 合并阶段
func (b *BatchProcessor) mergeStage(ctx context.Context, data *pipelineData) error {
	data.mu.RLock()
	validated := data.results["validated"].(map[string][]map[string]interface{})
	data.mu.RUnlock()

	// 合并所有结果
	var allResults []map[string]interface{}

	// 按编码排序的键
	var keys []string
	for k := range validated {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 按顺序合并
	for _, key := range keys {
		allResults = append(allResults, validated[key]...)
	}

	data.mu.Lock()
	data.results["final"] = allResults
	data.mu.Unlock()

	return nil
}


// extractCoreFields 提取核心字段(code和name)，减少token使用量
func extractCoreFields(data map[string]interface{}) map[string]interface{} {
	// 调试信息：打印输入数据结构的键
	var keys []string
	for k := range data {
		keys = append(keys, k)
	}
	fmt.Printf("DEBUG: extractCoreFields 输入数据键: %v\n", keys)

	coreData := map[string]interface{}{
		"items": []interface{}{},
	}

	// 首先尝试获取PDF服务格式的数据 {"occupation_codes": [...]}
	var items []interface{}
	if occupationCodes, ok := data["occupation_codes"].([]interface{}); ok {
		items = occupationCodes
		fmt.Printf("DEBUG: 找到occupation_codes数组，长度: %d\n", len(items))
	} else if itemsArray, ok := data["items"].([]interface{}); ok {
		// 备用：尝试items格式
		items = itemsArray
		fmt.Printf("DEBUG: 找到items数组，长度: %d\n", len(items))
	} else {
		fmt.Printf("DEBUG: 无法找到occupation_codes或items字段\n")
		return coreData
	}

	var coreItems []interface{}
	// 限制最多处理前5个条目进行验证测试
	maxItems := 500
	processedCount := 0

	for _, item := range items {
		if processedCount >= maxItems {
			break
		}

		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// 只提取code和name字段
		coreItem := map[string]interface{}{}

		if code, exists := itemMap["code"]; exists {
			coreItem["code"] = code
		}

		if name, exists := itemMap["name"]; exists {
			coreItem["name"] = name
		}

		// 只有当code或name存在时才添加
		if len(coreItem) > 0 {
			coreItems = append(coreItems, coreItem)
			processedCount++
		}
	}

	fmt.Printf("DEBUG: 限制处理条目数量，原始: %d, 处理: %d, 提取: %d\n", len(items), processedCount, len(coreItems))

	coreData["items"] = coreItems
	return coreData
}
