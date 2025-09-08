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

	"github.com/freedkr/moonshot/internal/database"
	"github.com/freedkr/moonshot/internal/model"
	"gorm.io/datatypes"
)

// ===== PDF服务客户端 =====

// PDFServiceClient PDF服务客户端实现
type PDFServiceClient struct {
	config     PDFServiceConfig
	httpClient *http.Client
}

// NewPDFServiceClient 创建PDF服务客户端
func NewPDFServiceClient(config PDFServiceConfig) PDFService {
	return &PDFServiceClient{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// ValidateAndExtract 验证并提取PDF内容
func (c *PDFServiceClient) ValidateAndExtract(ctx context.Context, request PDFValidationRequest) (PDFResult, error) {
	// 读取PDF文件
	pdfFile, err := os.Open(request.FilePath)
	if err != nil {
		return PDFResult{}, fmt.Errorf("failed to open PDF file %s: %w", request.FilePath, err)
	}
	defer pdfFile.Close()

	// 创建multipart请求
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加文件字段
	part, err := writer.CreateFormFile("file", filepath.Base(request.FilePath))
	if err != nil {
		return PDFResult{}, fmt.Errorf("create form file failed: %w", err)
	}

	if _, err := io.Copy(part, pdfFile); err != nil {
		return PDFResult{}, fmt.Errorf("copy file content failed: %w", err)
	}

	// 添加验证类型字段
	if err := writer.WriteField("validation_type", request.ValidationType); err != nil {
		return PDFResult{}, fmt.Errorf("write validation_type failed: %w", err)
	}

	if err := writer.Close(); err != nil {
		return PDFResult{}, fmt.Errorf("close multipart writer failed: %w", err)
	}

	// 调用PDF服务
	url := fmt.Sprintf("http://%s/api/v1/upload-and-validate", c.config.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return PDFResult{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return PDFResult{}, fmt.Errorf("PDF service call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return PDFResult{}, fmt.Errorf("PDF service returned error %d: %s", resp.StatusCode, string(body))
	}

	// 获取任务ID
	var taskResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&taskResp); err != nil {
		return PDFResult{}, err
	}

	pdfTaskID := taskResp["task_id"].(string)

	// 等待处理完成
	if err := c.waitForCompletion(ctx, pdfTaskID); err != nil {
		return PDFResult{}, err
	}

	// 获取结果
	return c.GetOccupationCodes(ctx, pdfTaskID)
}

// GetTaskStatus 获取任务状态 
func (c *PDFServiceClient) GetTaskStatus(ctx context.Context, taskID string) (TaskStatus, error) {
	url := fmt.Sprintf("http://%s/api/v1/status/%s", c.config.BaseURL, taskID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return TaskStatus{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TaskStatus{}, err
	}
	defer resp.Body.Close()

	var status TaskStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return TaskStatus{}, err
	}

	return status, nil
}

// GetOccupationCodes 获取职业编码结果
func (c *PDFServiceClient) GetOccupationCodes(ctx context.Context, taskID string) (PDFResult, error) {
	url := fmt.Sprintf("http://%s/api/v1/blocks/%s/occupation-codes", c.config.BaseURL, taskID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return PDFResult{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return PDFResult{}, fmt.Errorf("get occupation codes failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return PDFResult{}, fmt.Errorf("get codes failed %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return PDFResult{}, fmt.Errorf("parse result failed: %w", err)
	}

	// 转换为标准格式
	pdfResult := PDFResult{
		TaskID:      taskID,
		Status:      "completed",
		ProcessedAt: time.Now(),
	}

	if codes, ok := result["occupation_codes"].([]interface{}); ok {
		pdfResult.TotalFound = len(codes)
		for _, code := range codes {
			if codeMap, ok := code.(map[string]interface{}); ok {
				occupationCode := PDFOccupationCode{
					ExtractedAt: time.Now(),
				}
				
				if c, ok := codeMap["code"].(string); ok {
					occupationCode.Code = c
				}
				if n, ok := codeMap["name"].(string); ok {
					occupationCode.Name = n
				}
				if conf, ok := codeMap["confidence"].(float64); ok {
					occupationCode.Confidence = conf
				} else {
					occupationCode.Confidence = 0.8 // 默认置信度
				}
				occupationCode.Source = "pdf"
				
				pdfResult.OccupationCodes = append(pdfResult.OccupationCodes, occupationCode)
			}
		}
	}

	return pdfResult, nil
}

// waitForCompletion 等待处理完成
func (c *PDFServiceClient) waitForCompletion(ctx context.Context, taskID string) error {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	timeout := time.After(180 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return nil // 超时后尝试直接获取结果
		case <-ticker.C:
			if completed, err := c.checkStatus(ctx, taskID); err != nil {
				continue // 忽略状态检查错误
			} else if completed {
				return nil
			}
		}
	}
}

// checkStatus 检查状态
func (c *PDFServiceClient) checkStatus(ctx context.Context, taskID string) (bool, error) {
	url := fmt.Sprintf("http://%s/api/v1/status/%s", c.config.BaseURL, taskID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusInternalServerError {
		return false, nil // 继续等待
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("status code: %d", resp.StatusCode)
	}

	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return false, err
	}

	statusStr, _ := status["status"].(string)
	return statusStr == "completed", nil
}

// ===== 数据映射器 =====

// DataMapperImpl 数据映射器实现
type DataMapperImpl struct{}

// NewDataMapper 创建数据映射器
func NewDataMapper() DataMapper {
	return &DataMapperImpl{}
}

// FuseRuleAndPDFData 融合规则和PDF数据
func (m *DataMapperImpl) FuseRuleAndPDFData(categories []*model.Category, cleanedData []CleanedDataItem) []SemanticChoice {
	// 构建父子关系映射
	parentNameMap := make(map[string]string)
	var buildParentMap func([]*model.Category, string)
	buildParentMap = func(cats []*model.Category, parentName string) {
		for _, cat := range cats {
			if cat == nil {
				continue
			}

			if parentName != "" {
				parentNameMap[cat.Code] = parentName
			}

			if len(cat.Children) > 0 {
				buildParentMap(cat.Children, cat.Name)
			}
		}
	}
	buildParentMap(categories, "")

	// 收集规则数据（只收集叶子节点）
	ruleData := make(map[string]string)
	var collectLeafNodes func([]*model.Category)
	collectLeafNodes = func(cats []*model.Category) {
		for _, cat := range cats {
			if cat == nil {
				continue
			}

			if len(cat.Children) == 0 {
				ruleData[cat.Code] = cat.Name
			} else {
				collectLeafNodes(cat.Children)
			}
		}
	}
	collectLeafNodes(categories)

	// 收集PDF数据
	pdfDataMap := make(map[string]string)
	for _, item := range cleanedData {
		pdfDataMap[item.Code] = item.Name
	}

	// 创建语义选择项
	var choices []SemanticChoice
	allCodes := make(map[string]bool)
	
	for code := range ruleData {
		allCodes[code] = true
	}
	for code := range pdfDataMap {
		allCodes[code] = true
	}

	for code := range allCodes {
		parentName := parentNameMap[code]

		choice := SemanticChoice{
			Code:            code,
			RuleName:        ruleData[code],
			PDFName:         pdfDataMap[code],
			ParentHierarchy: parentName,
		}

		// 设置置信度
		if ruleData[code] != "" {
			choice.Confidence.RuleConfidence = 0.9
		}
		if pdfDataMap[code] != "" {
			choice.Confidence.PDFConfidence = 0.8
		}

		// 设置上下文
		choice.Context.ParentCode = m.inferParentCode(code)
		choice.Context.Level = m.inferLevel(code)

		if choice.RuleName != "" || choice.PDFName != "" {
			choices = append(choices, choice)
		}
	}

	return choices
}

// TransformPDFResult 转换PDF结果
func (m *DataMapperImpl) TransformPDFResult(pdfResult PDFResult) []CleanedDataItem {
	var items []CleanedDataItem
	
	for _, code := range pdfResult.OccupationCodes {
		item := CleanedDataItem{
			Code:        code.Code,
			Name:        code.Name,
			Level:       code.Level,
			Confidence:  code.Confidence,
			Source:      "pdf_direct",
			ProcessedBy: "data_mapper",
			CleanedAt:   time.Now(),
		}
		items = append(items, item)
	}

	return items
}

// TransformSemanticResult 转换语义结果
func (m *DataMapperImpl) TransformSemanticResult(choices []SemanticChoice, llmResults []string) []FinalResultItem {
	var items []FinalResultItem
	
	for i, choice := range choices {
		item := FinalResultItem{
			Code:        choice.Code,
			Name:        choice.RuleName, // 默认值
			Level:       choice.Context.Level,
			ParentCode:  choice.Context.ParentCode,
			Source:      "semantic_transform",
			Confidence:  choice.Confidence.RuleConfidence,
			ProcessedAt: time.Now(),
		}

		if i < len(llmResults) && llmResults[i] != "" {
			// 解析LLM结果
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(llmResults[i]), &result); err == nil {
				if name, ok := result["name"].(string); ok {
					item.Name = name
				}
				if selectedFrom, ok := result["selected_from"].(string); ok {
					item.Metadata.SelectedFrom = selectedFrom
				}
			}
		}

		items = append(items, item)
	}

	return items
}

// inferParentCode 推断父编码
func (m *DataMapperImpl) inferParentCode(code string) string {
	parts := strings.Split(code, "-")
	if len(parts) > 1 {
		return strings.Join(parts[:len(parts)-1], "-")
	}
	return ""
}

// inferLevel 推断层级
func (m *DataMapperImpl) inferLevel(code string) string {
	parts := strings.Split(code, "-")
	switch len(parts) {
	case 1:
		return "大类"
	case 2:
		return "中类"
	case 3:
		return "小类"
	case 4:
		return "细类"
	default:
		return "细类"
	}
}

// ===== 处理结果存储 =====

// ProcessingRepositoryImpl 处理结果存储实现
type ProcessingRepositoryImpl struct {
	db database.DatabaseInterface
}

// NewProcessingRepository 创建处理结果存储
func NewProcessingRepository(db database.DatabaseInterface) ProcessingRepository {
	return &ProcessingRepositoryImpl{
		db: db,
	}
}

// SaveProcessingResults 保存处理结果
func (r *ProcessingRepositoryImpl) SaveProcessingResults(ctx context.Context, request PersistenceRequest) error {
	// 获取GORM实例
	pgDB, ok := r.db.(*database.PostgreSQLDB)
	if !ok {
		return fmt.Errorf("database type error")
	}

	// 删除旧数据
	if request.Options.ReplaceExisting {
		if err := pgDB.GetDB().WithContext(ctx).Where("task_id = ?", request.TaskID).Delete(&database.Category{}).Error; err != nil {
			return fmt.Errorf("delete old categories failed: %w", err)
		}
	}

	// 转换并保存新数据
	var categories []*database.Category
	batchID := fmt.Sprintf("components-%s-%d", request.TaskID[:8], time.Now().Unix())
	currentTime := time.Now()
	
	for _, item := range request.Results {
		cat := &database.Category{
			TaskID:          request.TaskID,
			Code:            item.Code,
			Name:            item.Name,
			Level:           item.Level,
			ParentCode:      item.ParentCode,
			// 设置版本化字段
			UploadBatchID:   batchID,
			UploadTimestamp: currentTime,
			IsCurrent:       true,
		}
		categories = append(categories, cat)
	}

	// 批量插入
	batchSize := request.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	if err := pgDB.GetDB().WithContext(ctx).CreateInBatches(categories, batchSize).Error; err != nil {
		return fmt.Errorf("batch insert failed: %w", err)
	}

	return nil
}

// GetProcessingHistory 获取处理历史
func (r *ProcessingRepositoryImpl) GetProcessingHistory(ctx context.Context, taskID string) ([]ProcessingRecord, error) {
	// 简化实现，返回空历史
	return []ProcessingRecord{}, nil
}

// UpdateTaskStatus 更新任务状态
func (r *ProcessingRepositoryImpl) UpdateTaskStatus(ctx context.Context, taskID string, status string, result interface{}) error {
	task, err := r.db.GetTask(ctx, taskID)
	if err != nil {
		return err
	}

	task.Status = status
	resultJSON, _ := json.Marshal(result)
	task.Result = datatypes.JSON(resultJSON)
	task.UpdatedAt = time.Now()

	return r.db.UpdateTask(ctx, task)
}

// ===== 指标收集器 =====

// MetricsCollectorImpl 指标收集器实现
type MetricsCollectorImpl struct {
	metrics ProcessingMetrics
	mutex   sync.RWMutex
}

// NewMetricsCollector 创建指标收集器
func NewMetricsCollector() MetricsCollector {
	return &MetricsCollectorImpl{
		metrics: ProcessingMetrics{
			StageMetrics:      make(map[string]StageMetrics),
			ErrorDistribution: make(map[string]int64),
			RecentActivity:    make([]ActivityRecord, 0, 100),
			Timestamp:         time.Now(),
		},
	}
}

// RecordProcessingDuration 记录处理时长
func (c *MetricsCollectorImpl) RecordProcessingDuration(stage string, duration time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	stageMetrics := c.metrics.StageMetrics[stage]
	stageMetrics.Count++
	
	if stageMetrics.MinDuration == 0 || duration < stageMetrics.MinDuration {
		stageMetrics.MinDuration = duration
	}
	if duration > stageMetrics.MaxDuration {
		stageMetrics.MaxDuration = duration
	}
	
	// 计算平均时长
	totalDuration := stageMetrics.AvgDuration * time.Duration(stageMetrics.Count-1)
	stageMetrics.AvgDuration = (totalDuration + duration) / time.Duration(stageMetrics.Count)
	
	c.metrics.StageMetrics[stage] = stageMetrics

	// 记录活动
	c.addActivity(stage, "duration_recorded", duration, "")
}

// RecordSuccess 记录成功
func (c *MetricsCollectorImpl) RecordSuccess(stage string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.metrics.TotalProcessed++
	c.metrics.SuccessCount++
	c.metrics.SuccessRate = float64(c.metrics.SuccessCount) / float64(c.metrics.TotalProcessed)

	stageMetrics := c.metrics.StageMetrics[stage]
	stageMetrics.Count++
	// 成功率计算需要追踪成功次数
	c.metrics.StageMetrics[stage] = stageMetrics

	c.addActivity(stage, "success", 0, "")
}

// RecordError 记录错误
func (c *MetricsCollectorImpl) RecordError(stage string, err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.metrics.TotalProcessed++
	c.metrics.ErrorCount++
	c.metrics.SuccessRate = float64(c.metrics.SuccessCount) / float64(c.metrics.TotalProcessed)

	errorType := err.Error()
	c.metrics.ErrorDistribution[errorType]++

	stageMetrics := c.metrics.StageMetrics[stage]
	stageMetrics.Count++
	stageMetrics.Errors = append(stageMetrics.Errors, errorType)
	c.metrics.StageMetrics[stage] = stageMetrics

	c.addActivity(stage, "error", 0, errorType)
}

// GetMetrics 获取指标
func (c *MetricsCollectorImpl) GetMetrics() ProcessingMetrics {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	// 获取当前时间戳，不修改原始数据
	currentTime := time.Now()
	
	// 深拷贝以避免并发修改
	metricsCopy := ProcessingMetrics{
		TotalProcessed:    c.metrics.TotalProcessed,
		SuccessCount:      c.metrics.SuccessCount,
		ErrorCount:        c.metrics.ErrorCount,
		SuccessRate:       c.metrics.SuccessRate,
		Timestamp:         currentTime,
		StageMetrics:      make(map[string]StageMetrics),
		ErrorDistribution: make(map[string]int64),
		RecentActivity:    make([]ActivityRecord, len(c.metrics.RecentActivity)),
	}
	
	// 深拷贝StageMetrics
	for k, v := range c.metrics.StageMetrics {
		metricsCopy.StageMetrics[k] = v
	}
	
	// 深拷贝ErrorDistribution
	for k, v := range c.metrics.ErrorDistribution {
		metricsCopy.ErrorDistribution[k] = v
	}
	
	// 深拷贝RecentActivity
	copy(metricsCopy.RecentActivity, c.metrics.RecentActivity)

	return metricsCopy
}

// Reset 重置指标
func (c *MetricsCollectorImpl) Reset() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.metrics = ProcessingMetrics{
		StageMetrics:      make(map[string]StageMetrics),
		ErrorDistribution: make(map[string]int64),
		RecentActivity:    make([]ActivityRecord, 0, 100),
		Timestamp:         time.Now(),
	}
}

// addActivity 添加活动记录
func (c *MetricsCollectorImpl) addActivity(stage, status string, duration time.Duration, errorMsg string) {
	activity := ActivityRecord{
		Timestamp: time.Now(),
		Stage:     stage,
		Status:    status,
		Duration:  duration,
		Error:     errorMsg,
	}

	c.metrics.RecentActivity = append(c.metrics.RecentActivity, activity)
	
	// 保持最近100条记录
	if len(c.metrics.RecentActivity) > 100 {
		c.metrics.RecentActivity = c.metrics.RecentActivity[1:]
	}
}