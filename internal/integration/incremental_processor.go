package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/freedkr/moonshot/internal/config"
	"github.com/freedkr/moonshot/internal/database"
	"github.com/freedkr/moonshot/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// IncrementalProcessor 增量更新处理器 - 实现理想的5步流程
type IncrementalProcessor struct {
	config        *config.Config
	httpClient    *http.Client
	db            database.DatabaseInterface
	llmServiceURL string
	pdfServiceURL string
	metrics       MetricsCollector
}

// NewIncrementalProcessor 创建增量处理器
func NewIncrementalProcessor(cfg *config.Config, db database.DatabaseInterface) *IncrementalProcessor {
	return &IncrementalProcessor{
		config: cfg,
		db:     db,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		llmServiceURL: getServiceURL(cfg, "llm-service", "8090"),
		pdfServiceURL: getServiceURL(cfg, "pdf-validator", "8000"),
		metrics:       NewMetricsCollector(),
	}
}

// ProcessIncrementalFlow 执行增量更新的5步流程
func (p *IncrementalProcessor) ProcessIncrementalFlow(ctx context.Context, taskID string, excelPath string, categories []*model.Category) error {
	fmt.Printf("🚀 DEBUG: IncrementalProcessor.ProcessIncrementalFlow 开始执行 - taskID: %s\n", taskID)
	// 步骤1：先解析excel保存到表中，此时外部接口可以调用得到数据渲染
	err := p.step1SaveExcelData(ctx, taskID, categories)
	if err != nil {
		return fmt.Errorf("步骤1失败: %w", err)
	}

	// 步骤2：pdf处理得到的结果调用llm进行第一步的清洗，对应的数据是name，code
	fmt.Printf("🚀 DEBUG: 开始执行步骤2 - PDF处理和LLM清洗 - taskID: %s\n", taskID)
	pdfData, err := p.step2ProcessPDFWithLLM(ctx, taskID)
	if err != nil {
		fmt.Printf("❌ ERROR: 步骤2失败 - taskID: %s, 错误: %v\n", taskID, err)
		return fmt.Errorf("步骤2失败: %w", err)
	}
	fmt.Printf("✅ DEBUG: 步骤2完成 - taskID: %s, PDF数据条数: %d\n", taskID, len(pdfData))

	// 步骤3：将excel与pdf的数据通过code或者name进行两部分的合并，区分excel和pdf
	fmt.Printf("🚀 DEBUG: 开始执行步骤3 - 合并Excel和PDF数据 - taskID: %s\n", taskID)
	err = p.step3MergeExcelAndPDFData(ctx, taskID, pdfData)
	if err != nil {
		fmt.Printf("❌ ERROR: 步骤3失败 - taskID: %s, 错误: %v\n", taskID, err)
		return fmt.Errorf("步骤3失败: %w", err)
	}
	fmt.Printf("✅ DEBUG: 步骤3完成 - taskID: %s\n", taskID)

	// 步骤4：第二次调用llm，通过3步骤得到更丰富的数据投喂给llm进行筛选
	fmt.Printf("🚀 DEBUG: 开始执行步骤4 - 第二次LLM增强 - taskID: %s\n", taskID)
	enhancedData, err := p.step4EnhanceWithSecondLLM(ctx, taskID)
	if err != nil {
		fmt.Printf("❌ ERROR: 步骤4失败 - taskID: %s, 错误: %v\n", taskID, err)
		return fmt.Errorf("步骤4失败: %w", err)
	}
	fmt.Printf("✅ DEBUG: 步骤4完成 - taskID: %s, 增强数据条数: %d\n", taskID, len(enhancedData))

	// 步骤5：最终筛选后的结果更新会分类表中
	fmt.Printf("🚀 DEBUG: 开始执行步骤5 - 更新最终结果 - taskID: %s\n", taskID)
	err = p.step5UpdateFinalResults(ctx, taskID, enhancedData)
	if err != nil {
		fmt.Printf("❌ ERROR: 步骤5失败 - taskID: %s, 错误: %v\n", taskID, err)
		return fmt.Errorf("步骤5失败: %w", err)
	}
	fmt.Printf("✅ DEBUG: 步骤5完成 - taskID: %s\n", taskID)
	fmt.Printf("🎉 DEBUG: 增量处理流程全部完成 - taskID: %s\n", taskID)

	return nil
}

// step1SaveExcelData 步骤1：保存Excel解析数据
func (p *IncrementalProcessor) step1SaveExcelData(ctx context.Context, taskID string, categories []*model.Category) error {
	p.metrics.RecordProcessingDuration("excel_parsing", time.Since(time.Now()))

	// 生成新的批次ID
	batchID := uuid.New().String()
	currentTime := time.Now()

	// 转换为数据库格式，包含版本化字段
	var dbCategories []*database.Category
	for _, cat := range categories {
		dbCat := &database.Category{
			TaskID:          taskID,
			Code:            cat.Code,
			Name:            cat.Name,
			Level:           cat.Level,
			ParentCode:      cat.GetParentCode(),
			Status:          database.StatusExcelParsed,
			DataSource:      database.DataSourceExcel,
			UploadBatchID:   batchID,
			UploadTimestamp: currentTime,
			IsCurrent:       true, // 新插入的记录设为当前版本
		}
		dbCategories = append(dbCategories, dbCat)
	}

	// 使用版本化逻辑：先标记旧版本，再插入新版本
	pgDB, ok := p.db.(*database.PostgreSQLDB)
	if !ok {
		return fmt.Errorf("数据库类型错误")
	}

	// 添加调试日志
	fmt.Printf("DEBUG: 准备处理taskID=%s的数据，共%d条记录，batchID=%s\n", taskID, len(dbCategories), batchID)

	err := pgDB.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 检查是否存在当前版本记录
		var existingCount int64
		if err := tx.Model(&database.Category{}).Where("task_id = ? AND is_current = true", taskID).Count(&existingCount).Error; err != nil {
			return fmt.Errorf("检查已存在数据失败: %w", err)
		}
		fmt.Printf("DEBUG: taskID=%s 已存在当前版本记录%d条\n", taskID, existingCount)

		// 将现有的当前版本标记为历史版本
		if existingCount > 0 {
			result := tx.Model(&database.Category{}).
				Where("task_id = ? AND is_current = true", taskID).
				Update("is_current", false)
			if result.Error != nil {
				return fmt.Errorf("标记历史版本失败: %w", result.Error)
			}
			fmt.Printf("DEBUG: 标记了%d条记录为历史版本\n", result.RowsAffected)
		}

		// 批量插入新的当前版本数据
		if err := tx.CreateInBatches(dbCategories, 100).Error; err != nil {
			return fmt.Errorf("批量插入数据失败: %w", err)
		}
		fmt.Printf("DEBUG: 成功插入%d条新的当前版本记录，batchID=%s\n", len(dbCategories), batchID)

		return nil
	})

	if err != nil {
		p.metrics.RecordError("excel_parsing", err)
		return fmt.Errorf("保存Excel数据失败: %w", err)
	}

	p.metrics.RecordSuccess("excel_parsing")
	fmt.Printf("DEBUG: Excel数据版本化保存完成 - taskID=%s, batchID=%s\n", taskID, batchID)
	return nil
}

// step2ProcessPDFWithLLM 步骤2：PDF处理并调用LLM清洗
func (p *IncrementalProcessor) step2ProcessPDFWithLLM(ctx context.Context, taskID string) ([]map[string]interface{}, error) {
	startTime := time.Now()
	defer func() {
		p.metrics.RecordProcessingDuration("pdf_llm_cleaning", time.Since(startTime))
	}()

	// 调用PDF验证服务 (复用现有逻辑)
	pdfResult, err := p.callPDFValidator(ctx, taskID)
	if err != nil {
		p.metrics.RecordError("pdf_llm_cleaning", err)
		return nil, fmt.Errorf("PDF验证失败: %w", err)
	}

	// 📊 DEBUG: PDF验证完成，记录原始数据大小
	fmt.Printf("📊 DEBUG: PDF验证完成，原始数据大小: %v\n", len(fmt.Sprintf("%+v", pdfResult)))

	// 第一轮LLM分析 - 清洗PDF结果
	cleanedPDFData, err := p.firstLLMAnalysis(ctx, pdfResult)
	if err != nil {
		p.metrics.RecordError("pdf_llm_cleaning", err)
		return nil, fmt.Errorf("第一轮LLM分析失败: %w", err)
	}

	fmt.Printf("🎯 DEBUG: 第一轮LLM分析完成，清洗后数据条数: %d\n", len(cleanedPDFData))

	p.metrics.RecordSuccess("pdf_llm_cleaning")
	return cleanedPDFData, nil
}

// step3MergeExcelAndPDFData 步骤3：融合Excel和PDF数据
func (p *IncrementalProcessor) step3MergeExcelAndPDFData(ctx context.Context, taskID string, pdfData []map[string]interface{}) error {
	startTime := time.Now()
	defer func() {
		p.metrics.RecordProcessingDuration("data_merging", time.Since(startTime))
	}()

	fmt.Printf("📊 [Step3-开始] taskID=%s, PDF数据条数=%d\n", taskID, len(pdfData))

	pgDB, ok := p.db.(*database.PostgreSQLDB)
	if !ok {
		return fmt.Errorf("数据库类型错误")
	}

	// 创建PDF数据的Code映射
	pdfCodeMap := make(map[string]map[string]interface{})
	pdfNameMap := make(map[string]map[string]interface{})

	for _, item := range pdfData {
		code, hasCode := item["code"].(string)
		name, hasName := item["name"].(string)

		if hasCode && code != "" {
			pdfCodeMap[code] = item
			fmt.Printf("  📝 [Step3-PDF映射] Code映射: %s -> %v\n", code, item["name"])
		}
		if hasName && name != "" {
			pdfNameMap[name] = item
		}
	}
	fmt.Printf("📊 [Step3-映射完成] Code映射数=%d, Name映射数=%d\n", len(pdfCodeMap), len(pdfNameMap))

	// 批量更新数据库中的记录
	var updates []database.CategoryUpdate

	// 获取现有的Excel数据
	var excelCategories []database.Category
	fmt.Printf("🔍 [Step3-查询] 正在查询 task_id=%s AND status=%s 的记录...\n", taskID, database.StatusExcelParsed)
	err := pgDB.GetDB().WithContext(ctx).Where("task_id = ? AND status = ?",
		taskID, database.StatusExcelParsed).Find(&excelCategories).Error
	if err != nil {
		p.metrics.RecordError("data_merging", err)
		return fmt.Errorf("获取Excel数据失败: %w", err)
	}
	fmt.Printf("✅ [Step3-查询结果] 找到 %d 条Excel数据记录\n", len(excelCategories))

	for i, cat := range excelCategories {
		var pdfInfo map[string]interface{}
		var found bool
		var matchType string

		// 优先按Code匹配
		if pdfInfo, found = pdfCodeMap[cat.Code]; found {
			matchType = "Code匹配"
		} else if pdfInfo, found = pdfNameMap[cat.Name]; found {
			// 备选按Name匹配
			matchType = "Name匹配"
		}

		if found {
			fmt.Printf("  ✅ [Step3-匹配成功] [%d/%d] Code=%s, Name=%s, 匹配方式=%s\n",
				i+1, len(excelCategories), cat.Code, cat.Name, matchType)
			// 序列化PDF信息
			pdfInfoJSON, _ := json.Marshal(pdfInfo)

			updates = append(updates, database.CategoryUpdate{
				Code: cat.Code,
				Updates: map[string]interface{}{
					"status":      database.StatusPDFMerged,
					"data_source": database.DataSourceMerged,
					"pdf_info":    string(pdfInfoJSON),
				},
			})
		} else {
			if i < 5 { // 只打印前5个未匹配的记录
				fmt.Printf("  ❌ [Step3-未匹配] [%d/%d] Code=%s, Name=%s\n",
					i+1, len(excelCategories), cat.Code, cat.Name)
			}
		}
	}
	fmt.Printf("📊 [Step3-匹配统计] 总记录=%d, 成功匹配=%d, 未匹配=%d\n",
		len(excelCategories), len(updates), len(excelCategories)-len(updates))

	// 执行批量更新
	if len(updates) > 0 {
		fmt.Printf("🔄 [Step3-更新] 准备批量更新 %d 条记录...\n", len(updates))
		err = p.batchUpdateCategoriesByCode(ctx, taskID, updates)
		if err != nil {
			fmt.Printf("❌ [Step3-更新失败] 错误: %v\n", err)
			p.metrics.RecordError("data_merging", err)
			return fmt.Errorf("批量更新失败: %w", err)
		}
		fmt.Printf("✅ [Step3-更新成功] 已更新 %d 条记录状态为 %s\n", len(updates), database.StatusPDFMerged)
	} else {
		fmt.Printf("⚠️ [Step3-无更新] 没有找到匹配的记录需要更新\n")
	}

	p.metrics.RecordSuccess("data_merging")
	fmt.Printf("✅ [Step3-完成] 数据融合步骤完成\n")
	return nil
}

// step4EnhanceWithSecondLLM 步骤4：第二轮LLM增强
func (p *IncrementalProcessor) step4EnhanceWithSecondLLM(ctx context.Context, taskID string) ([]map[string]interface{}, error) {
	startTime := time.Now()
	defer func() {
		p.metrics.RecordProcessingDuration("llm_enhancement", time.Since(startTime))
	}()

	fmt.Printf("\n🚀 [Step4-开始] 第二轮LLM增强 - taskID=%s\n", taskID)

	// 获取已融合的数据
	pgDB, ok := p.db.(*database.PostgreSQLDB)
	if !ok {
		return nil, fmt.Errorf("数据库类型错误")
	}

	// 先查询所有状态，了解数据分布
	var statusCount []struct {
		Status string
		Count  int64
	}
	pgDB.GetDB().Model(&database.Category{}).
		Select("status, count(*) as count").
		Where("task_id = ?", taskID).
		Group("status").
		Scan(&statusCount)

	fmt.Printf("📊 [Step4-状态分布] taskID=%s 的数据状态分布:\n", taskID)
	for _, sc := range statusCount {
		fmt.Printf("  - %s: %d 条\n", sc.Status, sc.Count)
	}

	var mergedCategories []database.Category
	fmt.Printf("🔍 [Step4-查询] 正在查询 task_id=%s AND status=%s 的记录...\n", taskID, database.StatusPDFMerged)
	err := pgDB.GetDB().WithContext(ctx).Where("task_id = ? AND status = ?",
		taskID, database.StatusPDFMerged).Find(&mergedCategories).Error
	if err != nil {
		fmt.Printf("❌ [Step4-查询失败] 错误: %v\n", err)
		p.metrics.RecordError("llm_enhancement", err)
		return nil, fmt.Errorf("获取融合数据失败: %w", err)
	}

	fmt.Printf("✅ [Step4-查询结果] 获取到融合数据 %d 条\n", len(mergedCategories))

	// 如果没有融合数据，尝试使用所有Excel数据
	if len(mergedCategories) == 0 {
		fmt.Printf("⚠️ [Step4-降级处理] 没有找到pdf_merged状态的数据，尝试使用excel_parsed状态的数据...\n")
		err = pgDB.GetDB().WithContext(ctx).Where("task_id = ? AND status = ?",
			taskID, database.StatusExcelParsed).Find(&mergedCategories).Error
		if err != nil {
			fmt.Printf("❌ [Step4-降级失败] 获取Excel数据失败: %v\n", err)
			return nil, fmt.Errorf("获取Excel数据失败: %w", err)
		}
		fmt.Printf("✅ [Step4-降级成功] 获取到Excel数据 %d 条\n", len(mergedCategories))
	}

	// 准备丰富数据供LLM分析
	enrichedChoices := p.prepareEnrichedData(mergedCategories)
	fmt.Printf("🔄 [Step4-准备数据] 准备第二轮LLM分析，候选数据: %d 条\n", len(enrichedChoices))

	// 批量处理：每批10条，处理完立即更新数据库
	batchSize := 10
	totalProcessed := 0
	var allResults []map[string]interface{}

	for i := 0; i < len(enrichedChoices); i += batchSize {
		end := i + batchSize
		if end > len(enrichedChoices) {
			end = len(enrichedChoices)
		}

		batch := enrichedChoices[i:end]
		batchNum := (i / batchSize) + 1
		fmt.Printf("\n📦 [Step4-批次%d] 处理第 %d-%d 条数据（共%d条）\n", batchNum, i+1, end, len(enrichedChoices))

		// 打印当前批次的前3个候选数据
		for j, choice := range batch {
			if j >= 3 {
				break
			}
			fmt.Printf("  📝 [批次%d-数据%d] Code=%s, RuleName=%s, PdfName=%s\n",
				batchNum, j+1, choice.Code, choice.RuleName, choice.PdfName)
		}

		// 第二轮LLM分析 - 处理当前批次
		fmt.Printf("🤖 [Step4-批次%d-LLM] 开始LLM分析...\n", batchNum)
		batchResult, err := p.secondLLMAnalysis(ctx, batch)
		if err != nil {
			fmt.Printf("❌ [Step4-批次%d-失败] LLM分析失败: %v，跳过本批次\n", batchNum, err)
			p.metrics.RecordError("llm_enhancement_batch", err)
			continue // 跳过失败的批次，继续处理下一批
		}

		fmt.Printf("✅ [Step4-批次%d-成功] LLM分析完成，返回 %d 条结果\n", batchNum, len(batchResult))

		// 立即更新这批数据到数据库
		if len(batchResult) > 0 {
			fmt.Printf("💾 [Step4-批次%d-更新] 立即更新数据库...\n", batchNum)
			if err := p.updateBatchLLMResults(ctx, taskID, batchResult); err != nil {
				fmt.Printf("❌ [Step4-批次%d-更新失败] 数据库更新失败: %v\n", batchNum, err)
			} else {
				fmt.Printf("✅ [Step4-批次%d-更新成功] 已更新 %d 条记录\n", batchNum, len(batchResult))
				totalProcessed += len(batchResult)
			}
		}

		// 收集所有结果（用于返回）
		allResults = append(allResults, batchResult...)

		// 添加短暂延迟，避免过度压力
		if i+batchSize < len(enrichedChoices) {
			fmt.Printf("⏱️ [Step4-批次%d] 等待1秒后处理下一批...\n", batchNum)
			time.Sleep(1 * time.Second)
		}
	}

	fmt.Printf("\n✅ [Step4-完成] 批量LLM分析完成，总计处理并更新: %d 条\n", totalProcessed)
	p.metrics.RecordSuccess("llm_enhancement")
	return allResults, nil
}

// step5UpdateFinalResults 步骤5：最终状态检查（数据已在step4批量更新）
func (p *IncrementalProcessor) step5UpdateFinalResults(ctx context.Context, taskID string, enhancedData []map[string]interface{}) error {
	startTime := time.Now()
	defer func() {
		p.metrics.RecordProcessingDuration("final_update", time.Since(startTime))
	}()

	fmt.Printf("\n🚀 [Step5-开始] 最终状态检查 - taskID=%s\n", taskID)

	// 由于数据已在step4中批量更新，这里只做状态检查
	pgDB, ok := p.db.(*database.PostgreSQLDB)
	if !ok {
		return fmt.Errorf("数据库类型错误")
	}

	// 统计更新结果
	var statusStats []struct {
		Status string
		Count  int64
	}
	err := pgDB.GetDB().Model(&database.Category{}).
		Select("status, count(*) as count").
		Where("task_id = ?", taskID).
		Group("status").
		Scan(&statusStats).Error

	if err != nil {
		fmt.Printf("❌ [Step5-统计失败] 错误: %v\n", err)
		return fmt.Errorf("统计状态失败: %w", err)
	}

	fmt.Printf("📊 [Step5-最终统计] 任务 %s 的最终数据状态:\n", taskID)
	for _, stat := range statusStats {
		fmt.Printf("  - %s: %d 条\n", stat.Status, stat.Count)
	}

	// 检查llm_enhancements字段是否已填充
	var enhancedCount int64
	pgDB.GetDB().Model(&database.Category{}).
		Where("task_id = ? AND llm_enhancements IS NOT NULL AND llm_enhancements != ''", taskID).
		Count(&enhancedCount)

	fmt.Printf("📊 [Step5-LLM增强统计] LLM增强字段已填充: %d 条\n", enhancedCount)

	// 如果有未处理的数据，尝试补充处理（容错机制）
	if len(enhancedData) > int(enhancedCount) {
		fmt.Printf("⚠️ [Step5-补充处理] 检测到 %d 条数据可能未更新，尝试补充更新...\n",
			len(enhancedData)-int(enhancedCount))

		// 只更新那些llm_enhancements为空的记录
		var updates []database.CategoryUpdate
		for _, item := range enhancedData {
			code, ok := item["code"].(string)
			if !ok || code == "" {
				continue
			}

			// 检查该记录是否已有llm_enhancements
			var count int64
			pgDB.GetDB().Model(&database.Category{}).
				Where("task_id = ? AND code = ? AND (llm_enhancements IS NULL OR llm_enhancements = '')",
					taskID, code).
				Count(&count)

			if count > 0 {
				llmInfoJSON, _ := json.Marshal(item)
				updates = append(updates, database.CategoryUpdate{
					Code: code,
					Updates: map[string]interface{}{
						"status":           database.StatusCompleted,
						"llm_enhancements": string(llmInfoJSON),
						"name":             item["name"],
					},
				})
			}
		}

		if len(updates) > 0 {
			fmt.Printf("🔄 [Step5-补充更新] 更新 %d 条遗漏的记录...\n", len(updates))
			if err := p.batchUpdateCategoriesByCode(ctx, taskID, updates); err != nil {
				fmt.Printf("❌ [Step5-补充失败] 错误: %v\n", err)
			} else {
				fmt.Printf("✅ [Step5-补充成功] 已补充更新 %d 条记录\n", len(updates))
			}
		}
	}

	p.metrics.RecordSuccess("final_update")
	fmt.Printf("✅ [Step5-完成] 最终检查完成，共 %d 条记录已完成LLM增强\n\n", enhancedCount)
	return nil
}

// 辅助方法 - 复用现有逻辑
func (p *IncrementalProcessor) callPDFValidator(ctx context.Context, taskID string) (map[string]interface{}, error) {
	// 复用现有的PDFLLMProcessor的callPDFValidator方法
	processor := NewPDFLLMProcessor(p.config, p.db)
	return processor.callPDFValidator(ctx, taskID, "")
}

func (p *IncrementalProcessor) firstLLMAnalysis(ctx context.Context, pdfResult map[string]interface{}) ([]map[string]interface{}, error) {
	// 复用现有的PDFLLMProcessor的firstLLMAnalysis方法
	processor := NewPDFLLMProcessor(p.config, p.db)
	return processor.firstLLMAnalysis(ctx, pdfResult)
}

func (p *IncrementalProcessor) secondLLMAnalysis(ctx context.Context, choices []SemanticChoiceItem) ([]map[string]interface{}, error) {
	// 复用现有的PDFLLMProcessor的SecondLLMAnalysis方法
	processor := NewPDFLLMProcessor(p.config, p.db)
	return processor.SecondLLMAnalysis(ctx, choices)
}

func (p *IncrementalProcessor) prepareEnrichedData(categories []database.Category) []SemanticChoiceItem {
	var choices []SemanticChoiceItem

	for _, cat := range categories {
		choice := SemanticChoiceItem{
			Code:     cat.Code,
			RuleName: cat.Name, // Excel数据作为规则名称
		}

		// 从PDF信息中提取名称
		if cat.PDFInfo != "" {
			var pdfInfo map[string]interface{}
			if err := json.Unmarshal([]byte(cat.PDFInfo), &pdfInfo); err == nil {
				if pdfName, ok := pdfInfo["name"].(string); ok {
					choice.PdfName = pdfName
				}
			}
		}

		// 设置父层级信息
		choice.ParentHierarchy = cat.ParentCode

		choices = append(choices, choice)
	}

	return choices
}

func (p *IncrementalProcessor) batchUpdateCategoriesByCode(ctx context.Context, taskID string, updates []database.CategoryUpdate) error {
	pgDB, ok := p.db.(*database.PostgreSQLDB)
	if !ok {
		return fmt.Errorf("数据库类型错误")
	}

	fmt.Printf("  🔄 [批量更新-开始] 准备更新 %d 条记录\n", len(updates))

	// 使用事务批量更新
	tx := pgDB.GetDB().Begin()
	defer tx.Rollback()

	successCount := 0
	for i, update := range updates {
		result := tx.WithContext(ctx).Model(&database.Category{}).
			Where("task_id = ? AND code = ?", taskID, update.Code).
			Updates(update.Updates)

		if result.Error != nil {
			fmt.Printf("    ❌ [更新失败] Code=%s, 错误=%v\n", update.Code, result.Error)
			return fmt.Errorf("更新Code %s 失败: %w", update.Code, result.Error)
		}

		if result.RowsAffected > 0 {
			successCount++
			if i < 3 { // 打印前3条成功的更新
				fmt.Printf("    ✅ [更新成功%d] Code=%s, 影响行数=%d\n", i+1, update.Code, result.RowsAffected)
			}
		} else {
			fmt.Printf("    ⚠️ [未找到记录] Code=%s\n", update.Code)
		}
	}

	fmt.Printf("  📊 [批量更新-统计] 成功更新=%d/%d\n", successCount, len(updates))

	if err := tx.Commit().Error; err != nil {
		fmt.Printf("  ❌ [事务提交失败] 错误=%v\n", err)
		return err
	}
	fmt.Printf("  ✅ [批量更新-完成] 事务提交成功\n")
	return nil
}

// updateBatchLLMResults 批量更新LLM分析结果到数据库
func (p *IncrementalProcessor) updateBatchLLMResults(ctx context.Context, taskID string, results []map[string]interface{}) error {
	var updates []database.CategoryUpdate

	for _, item := range results {
		code, ok := item["code"].(string)
		if !ok || code == "" {
			continue
		}

		// 序列化LLM增强信息
		llmInfoJSON, _ := json.Marshal(item)

		updates = append(updates, database.CategoryUpdate{
			Code: code,
			Updates: map[string]interface{}{
				"status":           database.StatusCompleted,
				"llm_enhancements": string(llmInfoJSON),
				"name":             item["name"], // 如果LLM优化了name，也更新
			},
		})
	}

	if len(updates) == 0 {
		return nil
	}

	// 使用现有的批量更新方法
	return p.batchUpdateCategoriesByCode(ctx, taskID, updates)
}

// GetMetrics 获取处理指标
func (p *IncrementalProcessor) GetMetrics() ProcessingMetrics {
	return p.metrics.GetMetrics()
}
