package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/freedkr/moonshot/internal/config"
	"github.com/freedkr/moonshot/internal/database"
	"github.com/freedkr/moonshot/internal/model"
	"gorm.io/datatypes"
)

// PDFLLMProcessor 处理PDF验证和LLM语义分析的集成
type PDFLLMProcessor struct {
	config        *config.Config
	httpClient    *http.Client
	db            database.DatabaseInterface
	llmServiceURL string
	pdfServiceURL string
}

// NewPDFLLMProcessor 创建新的处理器
func NewPDFLLMProcessor(cfg *config.Config, db database.DatabaseInterface) *PDFLLMProcessor {
	return &PDFLLMProcessor{
		config: cfg,
		db:     db,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		llmServiceURL: getServiceURL(cfg, "llm-service", "8090"),
		pdfServiceURL: getServiceURL(cfg, "pdf-validator", "8000"),
	}
}

// ProcessWithPDFAndLLM 使用新的增量更新流程处理职业分类数据
func (p *PDFLLMProcessor) ProcessWithPDFAndLLM(ctx context.Context, taskID string, excelPath string, categories []*model.Category) error {
	// 使用新的增量处理器执行5步流程
	incrementalProcessor := NewIncrementalProcessor(p.config, p.db)

	return incrementalProcessor.ProcessIncrementalFlow(ctx, taskID, excelPath, categories)
}

// ProcessWithPDFAndLLMLegacy 保留原始的删除重建逻辑（用于兼容性）
func (p *PDFLLMProcessor) ProcessWithPDFAndLLMLegacy(ctx context.Context, taskID string, excelPath string, categories []*model.Category) error {
	// 第一步：调用PDF验证服务
	pdfResult, err := p.callPDFValidator(ctx, taskID, excelPath)
	if err != nil {
		return fmt.Errorf("PDF验证失败: %w", err)
	}

	// 第二步：第一轮LLM语义分析 - 清洗PDF结果
	cleanedPDFData, err := p.firstLLMAnalysis(ctx, pdfResult)
	if err != nil {
		return fmt.Errorf("第一轮LLM分析失败: %w", err)
	}

	// 第三步：融合初始解析结果和清洗后的PDF数据
	choices := p.MergeResults(categories, cleanedPDFData)

	// 第四步：第二轮LLM语义分析 - 选择最优结果
	finalResult, err := p.SecondLLMAnalysis(ctx, choices)
	if err != nil {
		return fmt.Errorf("第二轮LLM分析失败: %w", err)
	}

	// 第五步：保存最终结果到数据库 (删除重建方式)
	err = p.saveFinalResult(ctx, taskID, finalResult)
	if err != nil {
		return fmt.Errorf("保存最终结果失败: %w", err)
	}

	return nil
}

// callPDFValidator 调用PDF验证服务
func (p *PDFLLMProcessor) callPDFValidator(ctx context.Context, taskID string, _ string) (map[string]interface{}, error) {
	// 使用固定的PDF文件路径，支持环境变量配置
	pdfFilePath := os.Getenv("PDF_TEST_FILE_PATH")
	if pdfFilePath == "" {
		// 默认路径，容器内使用绝对路径
		pdfFilePath = "/root/testdata/2025042918334715812.pdf"
		// 如果容器内路径不存在，尝试相对路径（用于本地开发）
		if _, err := os.Stat(pdfFilePath); os.IsNotExist(err) {
			pdfFilePath = "testdata/2025042918334715812.pdf"
		}
	}

	// 读取PDF文件
	pdfFile, err := os.Open(pdfFilePath)
	if err != nil {
		return nil, fmt.Errorf("无法打开PDF文件 %s: %w", pdfFilePath, err)
	}
	defer pdfFile.Close()

	// 创建multipart请求
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加文件字段
	part, err := writer.CreateFormFile("file", filepath.Base(pdfFilePath))
	if err != nil {
		return nil, fmt.Errorf("创建form文件失败: %w", err)
	}

	if _, err := io.Copy(part, pdfFile); err != nil {
		return nil, fmt.Errorf("复制文件内容失败: %w", err)
	}

	// 添加validation_type字段
	if err := writer.WriteField("validation_type", "standard"); err != nil {
		return nil, fmt.Errorf("写入validation_type失败: %w", err)
	}

	// 关闭writer
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("关闭multipart writer失败: %w", err)
	}

	// 调用upload-and-validate接口
	url := fmt.Sprintf("http://%s/api/v1/upload-and-validate", p.pdfServiceURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 发送请求
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用PDF验证服务失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("PDF服务返回错误 %d: %s", resp.StatusCode, string(body))
	}

	// 获取验证任务ID
	var validationResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&validationResp); err != nil {
		return nil, err
	}

	pdfTaskID := validationResp["task_id"].(string)

	// 等待处理完成
	if err := p.waitForPDFCompletion(ctx, pdfTaskID); err != nil {
		return nil, err
	}

	// 获取职业编码结果
	return p.getOccupationCodes(ctx, pdfTaskID)
}

// waitForPDFCompletion 等待PDF处理完成
func (p *PDFLLMProcessor) waitForPDFCompletion(ctx context.Context, pdfTaskID string) error {
	ticker := time.NewTicker(3 * time.Second) // 增加轮询间隔到3秒
	defer ticker.Stop()

	timeout := time.After(180 * time.Second) // 增加到3分钟超时，给PDF处理更多时间

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			// 超时后，尝试直接获取结果，可能已经完成但status接口有问题
			return nil // 返回nil让调用方尝试获取结果
		case <-ticker.C:
			// 尝试检查状态，如果失败则继续等待
			if completed, err := p.checkPDFStatus(ctx, pdfTaskID); err != nil {
				// 忽略status接口的错误，继续等待
				continue
			} else if completed {
				return nil
			}
		}
	}
}

// checkPDFStatus 检查PDF处理状态
func (p *PDFLLMProcessor) checkPDFStatus(ctx context.Context, pdfTaskID string) (bool, error) {
	url := fmt.Sprintf("http://%s/api/v1/status/%s", p.pdfServiceURL, pdfTaskID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	// 如果状态接口返回500（可能是DateTime序列化问题），假设还在处理中
	if resp.StatusCode == http.StatusInternalServerError {
		return false, nil // 返回未完成，继续等待
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("状态码: %d", resp.StatusCode)
	}

	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return false, err
	}

	statusStr, _ := status["status"].(string)
	switch statusStr {
	case "completed":
		return true, nil
	case "failed", "error":
		errorMsg, _ := status["error"].(string)
		return false, fmt.Errorf("PDF验证失败: %s", errorMsg)
	default:
		return false, nil // 还在处理中
	}
}

// getOccupationCodes 获取职业编码结果
func (p *PDFLLMProcessor) getOccupationCodes(ctx context.Context, pdfTaskID string) (map[string]interface{}, error) {
	// 调用occupation-codes接口获取结果
	url := fmt.Sprintf("http://%s/api/v1/blocks/%s/occupation-codes", p.pdfServiceURL, pdfTaskID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取职业编码结果失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取结果失败 %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析结果失败: %w", err)
	}

	return result, nil
}

// firstLLMAnalysis 第一轮LLM分析 - 清洗PDF解析结果（使用并发）
func (p *PDFLLMProcessor) firstLLMAnalysis(ctx context.Context, pdfData map[string]interface{}) ([]map[string]interface{}, error) {
	fmt.Printf("🚀 [FirstLLMAnalysis-开始] pdfData keys数量: %d\n", len(pdfData))
	
	// 打印PDF数据的结构
	for key, value := range pdfData {
		if key == "occupation_codes" {
			if codes, ok := value.([]interface{}); ok {
				fmt.Printf("  📊 [PDF数据] occupation_codes 数量: %d\n", len(codes))
				if len(codes) > 0 && len(codes) <= 3 {
					// 打印前几个示例
					for i, code := range codes {
						if i >= 3 {
							break
						}
						fmt.Printf("    示例%d: %+v\n", i+1, code)
					}
				}
			}
		} else {
			fmt.Printf("  🔑 [PDF数据] %s: %v\n", key, value)
		}
	}
	
	// 使用批量处理器进行并发处理
	batchProcessor := NewBatchProcessor(p)
	fmt.Printf("🔄 [FirstLLMAnalysis] 创建BatchProcessor，准备并发处理\n")

	// 并发处理PDF数据，按编码前缀（1-xx, 2-xx等）分组
	fmt.Printf("🤖 [FirstLLMAnalysis] 开始调用LLM进行数据清洗...\n")
	cleanedData, err := batchProcessor.ProcessPDFDataConcurrently(ctx, pdfData)
	
	if err != nil {
		fmt.Printf("❌ [FirstLLMAnalysis-并发失败] 错误: %v, 回退到单次处理\n", err)
		// 如果并发处理失败，回退到单次处理
		return p.firstLLMAnalysisFallback(ctx, pdfData)
	}

	fmt.Printf("✅ [FirstLLMAnalysis-成功] 清洗后数据条数: %d\n", len(cleanedData))
	
	// 打印前3条清洗后的数据示例
	for i, data := range cleanedData {
		if i >= 3 {
			break
		}
		fmt.Printf("  📝 [清洗结果%d] %+v\n", i+1, data)
	}
	
	return cleanedData, nil
}

// firstLLMAnalysisFallback 第一轮LLM分析的回退方案（单次处理）
func (p *PDFLLMProcessor) firstLLMAnalysisFallback(ctx context.Context, pdfData map[string]interface{}) ([]map[string]interface{}, error) {
	// 先提取核心字段(只包含code和name)，避免token限制
	coreData := extractCoreFields(pdfData)

	// 调试信息：记录核心字段提取情况
	fmt.Printf("DEBUG: firstLLMAnalysisFallback 提取了核心字段（只包含code和name）\n")

	prompt := fmt.Sprintf(`你是一名数据清洗专家。请分析以下从PDF提取的职业分类数据，识别并提取准确的职业编码和名称。

PDF提取的核心数据（已过滤只包含code和name）：
%s

请遵循以下规则进行清洗：
1. 识别所有有效的职业编码（格式如：1-01-01-01）
2. 为每个编码匹配最准确的职业名称
3. 去除描述性文字和无关内容
4. 修正明显的OCR识别错误

输出格式要求：
返回JSON数组，每个元素包含：
{
  "code": "职业编码",
  "name": "职业名称",
  "confidence": "置信度(0-1)",
  "source": "pdf"
}

只返回JSON数组，不要有其他内容。`, jsonString(coreData))

	result, err := p.callLLMService(ctx, "data_cleaning", prompt)
	if err != nil {
		return nil, err
	}

	// 打印原始LLM返回结果以便调试
	fmt.Printf("🔍 [LLM原始响应] 长度=%d\n", len(result))
	if len(result) > 0 {
		// 打印前500个字符和后500个字符
		if len(result) <= 1000 {
			fmt.Printf("📝 [LLM完整响应]:\n%s\n", result)
		} else {
			fmt.Printf("📝 [LLM响应开头500字符]:\n%s\n", result[:500])
			fmt.Printf("📝 [LLM响应结尾500字符]:\n%s\n", result[len(result)-500:])
		}
	}

	// 解析结果 - 处理三种格式：{"items": [...]}、直接JSON数组、或JSON字符串
	var cleanedData []map[string]interface{}
	
	// 首先尝试解析为 {"items": [...]} 格式
	var resultWrapper struct {
		Items []map[string]interface{} `json:"items"`
	}
	// 先尝试清理可能的非JSON内容
	cleanResult := extractJSON(result)
	fmt.Printf("🔍 [清理后JSON] 长度=%d\n", len(cleanResult))
	if cleanResult != result {
		fmt.Printf("📝 [清理后JSON内容]:\n%s\n", cleanResult)
	}
	
	if err := json.Unmarshal([]byte(cleanResult), &resultWrapper); err != nil {
		fmt.Printf("⚠️ [wrapper格式解析失败] 错误: %v\n", err)
		// 如果包装格式失败，尝试解析为直接的JSON数组
		if err2 := json.Unmarshal([]byte(cleanResult), &cleanedData); err2 != nil {
			fmt.Printf("⚠️ [数组格式解析失败] 错误: %v\n", err2)
			// 如果直接数组也失败，尝试解析为JSON字符串（双重编码的情况）
			var jsonString string
			if err3 := json.Unmarshal([]byte(cleanResult), &jsonString); err3 != nil {
				fmt.Printf("❌ [字符串格式解析失败] 错误: %v\n", err3)
				fmt.Printf("❌ [JSON解析全部失败] 三种格式都无法解析\n")
				
				// 尝试使用更宽松的解析方式
				fmt.Printf("🔄 [尝试宽松解析] 尝试解析部分数据...\n")
				if partialData := tryParsePartialJSON(cleanResult); partialData != nil && len(partialData) > 0 {
					fmt.Printf("✅ [部分解析成功] 解析出 %d 条数据\n", len(partialData))
					return partialData, nil
				}
				
				return nil, fmt.Errorf("解析LLM返回结果失败: wrapper_err=%v, array_err=%v, string_err=%v", err, err2, err3)
			}
			fmt.Printf("🔄 [双重编码] 检测到JSON字符串，尝试二次解析...\n")
			// 解析JSON字符串中的实际JSON
			if err4 := json.Unmarshal([]byte(jsonString), &resultWrapper); err4 != nil {
				if err5 := json.Unmarshal([]byte(jsonString), &cleanedData); err5 != nil {
					fmt.Printf("❌ [二次解析失败] 错误: %v\n", err5)
					return nil, fmt.Errorf("解析JSON字符串失败: %v", err5)
				}
				fmt.Printf("✅ [二次解析成功] 解析为JSON数组\n")
			} else {
				cleanedData = resultWrapper.Items
				fmt.Printf("✅ [二次解析成功] 解析为wrapper格式\n")
			}
		} else {
			fmt.Printf("✅ [解析成功] 直接解析为JSON数组\n")
		}
	} else {
		cleanedData = resultWrapper.Items
		fmt.Printf("✅ [解析成功] 解析为wrapper格式\n")
	}
	
	fmt.Printf("📊 [解析结果] 成功解析 %d 条数据\n", len(cleanedData))

	return cleanedData, nil
}

// tryParsePartialJSON 尝试解析部分JSON数据（宽松模式）
func tryParsePartialJSON(input string) []map[string]interface{} {
	// 尝试找到JSON数组的开始和结束
	input = strings.TrimSpace(input)
	
	// 如果是不完整的JSON，尝试修复
	if strings.HasPrefix(input, "[") && !strings.HasSuffix(input, "]") {
		// 尝试补全结尾
		input = input + "]"
	}
	
	// 尝试解析
	var result []map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber() // 使用Number类型避免精度问题
	
	if err := decoder.Decode(&result); err != nil {
		// 如果还是失败，尝试逐行解析
		fmt.Printf("⚠️ [宽松解析] 完整解析失败，尝试逐行解析...\n")
		
		lines := strings.Split(input, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || line == "[" || line == "]" || line == "," {
				continue
			}
			
			// 移除可能的逗号结尾
			line = strings.TrimSuffix(line, ",")
			
			var item map[string]interface{}
			if err := json.Unmarshal([]byte(line), &item); err == nil {
				result = append(result, item)
			}
		}
	}
	
	return result
}

// SecondLLMAnalysis 第二轮LLM分析 - 使用任务类型轮询实现并发（导出供测试）
func (p *PDFLLMProcessor) SecondLLMAnalysis(ctx context.Context, choices []SemanticChoiceItem) ([]map[string]interface{}, error) {
	fmt.Printf("🤖 [SecondLLMAnalysis-开始] 开始第二轮LLM分析，待处理条目数: %d\n", len(choices))
	// 定义可用的任务类型池，只使用LLM服务已配置路由的类型
	taskTypes := []string{
		"semantic_analysis", // 主要用于语义分析
		"data_cleaning",     // 复用数据清洗队列
	}

	// 结果收集
	type itemResult struct {
		index  int
		result map[string]interface{}
		err    error
	}

	resultCh := make(chan itemResult, len(choices))

	// 使用goroutine池处理，每个goroutine使用不同的任务类型
	var wg sync.WaitGroup
	for i, choice := range choices {
		wg.Add(1)
		// 轮询分配任务类型
		taskType := taskTypes[i%len(taskTypes)]

		go func(idx int, item SemanticChoiceItem, tType string) {
			defer wg.Done()

			// 单条处理，使用分配的任务类型
			result, err := p.analyzeSingleChoice(ctx, item, tType)
			resultCh <- itemResult{
				index:  idx,
				result: result,
				err:    err,
			}
		}(i, choice, taskType)
	}

	// 等待所有goroutine完成
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 收集结果并保持顺序
	results := make([]map[string]interface{}, len(choices))
	errorCount := 0

	for res := range resultCh {
		if res.err != nil {
			errorCount++
			fmt.Printf("  ❌ [LLM处理失败] 条目 %d (Code=%s) 失败: %v\n", 
				res.index, choices[res.index].Code, res.err)
			// 使用默认值
			results[res.index] = map[string]interface{}{
				"code":        choices[res.index].Code,
				"name":        choices[res.index].RuleName, // 默认使用规则名称
				"level":       "细类",
				"parent_code": inferParentCode(choices[res.index].Code),
			}
		} else {
			if res.index < 3 { // 打印前3个成功的结果
				fmt.Printf("  ✅ [LLM处理成功] 条目 %d (Code=%s): %+v\n", 
					res.index, choices[res.index].Code, res.result)
			}
			results[res.index] = res.result
		}
	}

	fmt.Printf("📊 [SecondLLMAnalysis-统计] 总条目=%d, 成功=%d, 失败=%d\n", 
		len(choices), len(choices)-errorCount, errorCount)
	
	if errorCount > len(choices)/2 {
		fmt.Printf("❌ [SecondLLMAnalysis-错误] 超过50%%的条目处理失败\n")
		return results, fmt.Errorf("超过50%%的条目处理失败(%d/%d)", errorCount, len(choices))
	}

	// 过滤掉nil结果
	var finalResults []map[string]interface{}
	for _, r := range results {
		if r != nil {
			finalResults = append(finalResults, r)
		}
	}

	fmt.Printf("✅ [SecondLLMAnalysis-完成] 返回有效结果=%d条\n", len(finalResults))
	return finalResults, nil
}

// analyzeSingleChoice 分析单个选择项，使用指定的任务类型
func (p *PDFLLMProcessor) analyzeSingleChoice(ctx context.Context, choice SemanticChoiceItem, taskType string) (map[string]interface{}, error) {
	// 构建单条数据的精确提示
	prompt := fmt.Sprintf(`你是职业分类专家.请为以下职业编码选择最合适的名称:

编码:%s
选项1:%s
选项2:%s
父级类别:%s

选择规则:
- 只能选择选项1或选项2,不能创造新名称。
- 选择与父级层次语义更连贯的名称
- 优先选择完整的、名词性的职业名称
- 如果两个名称相似,选择更完整、更规范的版本
- 排除包含"本小类包括"、"进行..."、"担任..."等描述性短语

返回JSON格式:
{
  "code": "编码",
  "name": "选择后的名称",
  "parent_name": "父级类别名称"
}`,
		choice.Code,
		choice.RuleName,
		choice.PdfName,
		choice.ParentHierarchy)

	// 使用指定的任务类型调用LLM服务
	result, err := p.callLLMServiceWithRetry(ctx, taskType, prompt, 3)
	if err != nil {
		return nil, err
	}

	// 解析结果
	var singleResult map[string]interface{}
	if err := json.Unmarshal([]byte(result), &singleResult); err != nil {
		// 尝试提取JSON
		result = extractJSON(result)
		if err := json.Unmarshal([]byte(result), &singleResult); err != nil {
			return nil, fmt.Errorf("解析结果失败: %w", err)
		}
	}

	// 验证和补充必要字段
	if code, ok := singleResult["code"].(string); !ok || code != choice.Code {
		singleResult["code"] = choice.Code
	}

	if _, ok := singleResult["name"].(string); !ok {
		// 如果LLM没有正确返回，使用规则名称作为默认值
		singleResult["name"] = choice.RuleName
	}

	if _, ok := singleResult["level"].(string); !ok {
		singleResult["level"] = "细类"
	}

	if _, ok := singleResult["parent_code"].(string); !ok {
		singleResult["parent_code"] = inferParentCode(choice.Code)
	}

	return singleResult, nil
}

// inferParentCode 从编码推断父编码
func inferParentCode(code string) string {
	// 例如："1-01-01-01" -> "1-01-01"
	parts := strings.Split(code, "-")
	if len(parts) > 1 {
		return strings.Join(parts[:len(parts)-1], "-")
	}
	return ""
}

// extractJSON 从LLM响应中提取JSON部分
func extractJSON(response string) string {
	// 查找JSON数组的开始和结束
	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")

	if start != -1 && end != -1 && end > start {
		return response[start : end+1]
	}

	// 查找JSON对象的开始和结束
	start = strings.Index(response, "{")
	end = strings.LastIndex(response, "}")

	if start != -1 && end != -1 && end > start {
		return response[start : end+1]
	}

	return response
}

// SemanticChoiceItem 语义选择项结构
type SemanticChoiceItem struct {
	Code            string `json:"code"`
	RuleName        string `json:"rule_name"`
	PdfName         string `json:"pdf_name"`
	ParentHierarchy string `json:"parent_hierarchy"`
}

// MergeResults 融合规则解析结果和PDF清洗结果为语义选择结构（导出供测试）
func (p *PDFLLMProcessor) MergeResults(categories []*model.Category, pdfData []map[string]interface{}) []SemanticChoiceItem {
	// 构建父子关系映射 - 只记录直接父级名称
	parentNameMap := make(map[string]string)
	var buildParentMap func([]*model.Category, string)
	buildParentMap = func(cats []*model.Category, parentName string) {
		for _, cat := range cats {
			if cat == nil {
				continue
			}

			// 记录当前节点的父级名称
			if parentName != "" {
				parentNameMap[cat.Code] = parentName
			}

			// 递归处理子节点，当前节点名称作为子节点的父级名称
			if len(cat.Children) > 0 {
				buildParentMap(cat.Children, cat.Name)
			}
		}
	}
	buildParentMap(categories, "")

	// 收集骨架数据 - 只收集细类（最长编码）
	ruleData := make(map[string]string)
	var collectDetailedCodes func([]*model.Category)
	collectDetailedCodes = func(cats []*model.Category) {
		for _, cat := range cats {
			if cat == nil {
				continue
			}

			// 如果是叶子节点（细类），收集
			if len(cat.Children) == 0 {
				ruleData[cat.Code] = cat.Name
			} else {
				collectDetailedCodes(cat.Children)
			}
		}
	}
	collectDetailedCodes(categories)

	// 收集PDF数据
	pdfDataMap := make(map[string]string)
	for _, pdfItem := range pdfData {
		code := pdfItem["code"].(string)
		name := pdfItem["name"].(string)
		pdfDataMap[code] = name
	}

	// 创建语义选择项
	var choices []SemanticChoiceItem

	// 合并所有有数据的编码
	allCodes := make(map[string]bool)
	for code := range ruleData {
		allCodes[code] = true
	}
	for code := range pdfDataMap {
		allCodes[code] = true
	}

	for code := range allCodes {
		// 获取直接父级名称
		parentName := parentNameMap[code] // 如果没有父级则为空字符串

		choice := SemanticChoiceItem{
			Code:            code,
			RuleName:        ruleData[code],   // 如果没有则为空
			PdfName:         pdfDataMap[code], // 如果没有则为空
			ParentHierarchy: parentName,       // 只包含直接父级名称
		}

		// 只有至少有一个名称才加入
		if choice.RuleName != "" || choice.PdfName != "" {
			choices = append(choices, choice)
		}
	}

	return choices
}

// callLLMService 调用LLM服务（使用异步方式）
func (p *PDFLLMProcessor) callLLMService(ctx context.Context, taskType string, prompt string) (string, error) {
	// 使用带重试的异步调用
	return p.callLLMServiceWithRetry(ctx, taskType, prompt, 3)
}

// saveFinalResult 保存最终结果到数据库
func (p *PDFLLMProcessor) saveFinalResult(ctx context.Context, taskID string, finalData []map[string]interface{}) error {
	// 先删除旧的分类数据 - 通过直接使用GORM
	pgDB, ok := p.db.(*database.PostgreSQLDB)
	if !ok {
		return fmt.Errorf("数据库类型错误")
	}

	// 删除旧数据
	if err := pgDB.GetDB().WithContext(ctx).Where("task_id = ?", taskID).Delete(&database.Category{}).Error; err != nil {
		return fmt.Errorf("删除旧分类数据失败: %w", err)
	}

	// 转换并保存新的分类数据
	var categories []*database.Category
	for _, item := range finalData {
		// Level 字段应该是字符串类型（大类/中类/小类/细类）
		levelStr, ok := item["level"].(string)
		if !ok {
			// 如果不是字符串，尝试根据code推断
			code := item["code"].(string)
			levelStr = inferLevelFromCode(code)
		}

		cat := &database.Category{
			TaskID:     taskID,
			Code:       item["code"].(string),
			Name:       item["name"].(string),
			Level:      levelStr,
			ParentCode: "",
		}

		if parentCode, ok := item["parent_code"].(string); ok {
			cat.ParentCode = parentCode
		}

		categories = append(categories, cat)
	}

	// 批量插入 - 使用上面已经获取的pgDB
	if err := pgDB.GetDB().WithContext(ctx).CreateInBatches(categories, 100).Error; err != nil {
		return fmt.Errorf("批量插入失败: %w", err)
	}

	// 更新任务状态
	task, err := p.db.GetTask(ctx, taskID)
	if err != nil {
		return err
	}

	task.Status = "llm_processed"
	resultJSON, _ := json.Marshal(map[string]interface{}{
		"status":           "completed",
		"message":          "LLM语义分析完成",
		"total_categories": len(categories),
	})
	task.Result = datatypes.JSON(resultJSON)
	task.UpdatedAt = time.Now()

	return p.db.UpdateTask(ctx, task)
}

// getServiceURL 获取服务URL
func getServiceURL(cfg *config.Config, serviceName string, defaultPort string) string {
	// 根据服务名称返回对应的URL
	switch serviceName {
	case "llm-service":
		// 优先使用环境变量，然后是配置文件
		if llmURL := os.Getenv("LLM_SERVICE_URL"); llmURL != "" {
			return llmURL
		}
		if cfg.LLM.ServiceURL != "" {
			return cfg.LLM.ServiceURL
		}
		return fmt.Sprintf("llm-service:%s", defaultPort)
	case "pdf-validator":
		// PDF验证服务地址，支持环境变量配置
		if pdfURL := os.Getenv("PDF_VALIDATOR_URL"); pdfURL != "" {
			return pdfURL
		}
		return fmt.Sprintf("pdf-validator:%s", defaultPort)
	default:
		return fmt.Sprintf("localhost:%s", defaultPort)
	}
}

// inferLevelFromCode 根据编码推断层级
func inferLevelFromCode(code string) string {
	// 根据编码格式判断层级
	// 移除所有非数字和连字符的字符
	parts := strings.Split(code, "-")

	switch len(parts) {
	case 1:
		// 1位数字编码为大类
		return "大类"
	case 2:
		// 如 "1-01" 为中类
		return "中类"
	case 3:
		// 如 "1-01-01" 为小类
		return "小类"
	case 4:
		// 如 "1-01-01-01" 为细类
		return "细类"
	default:
		// 默认为细类
		return "细类"
	}
}

// truncatePDFData 截断PDF数据以避免token限制
func (p *PDFLLMProcessor) truncatePDFData(pdfData map[string]interface{}, maxChars int) map[string]interface{} {
	truncated := make(map[string]interface{})

	// 复制基本字段
	if taskID, ok := pdfData["task_id"]; ok {
		truncated["task_id"] = taskID
	}
	if totalFound, ok := pdfData["total_found"]; ok {
		truncated["total_found"] = totalFound
	}

	// 处理职业编码数组，只取前面部分
	if codes, ok := pdfData["occupation_codes"].([]interface{}); ok {
		var truncatedCodes []interface{}
		currentSize := 0

		for _, code := range codes {
			codeJSON, _ := json.Marshal(code)
			if currentSize+len(codeJSON) > maxChars {
				break
			}
			truncatedCodes = append(truncatedCodes, code)
			currentSize += len(codeJSON)
		}

		truncated["occupation_codes"] = truncatedCodes
		truncated["_truncated"] = len(truncatedCodes) < len(codes)
		truncated["_original_count"] = len(codes)
		truncated["_processed_count"] = len(truncatedCodes)
	}

	return truncated
}

// jsonString 将对象转换为JSON字符串
func jsonString(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
