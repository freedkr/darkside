// Package parser 实现混合智能解析功能
package parser

import (
	"context"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/freedkr/moonshot/internal/model"
	"github.com/xuri/excelize/v2"
)

// HybridParser 混合智能解析器
// 实现V2方案：本地构建骨架，AI处理细类关联
type HybridParser struct {
	config       *ParserConfig
	reWhitespace *regexp.Regexp
	reUnified    *regexp.Regexp
	reCodeFinder *regexp.Regexp
	reMajorClass *regexp.Regexp  // 专门用于识别大类的正则
}

// NewHybridParser 创建新的混合解析器
func NewHybridParser(config *ParserConfig) *HybridParser {
	if config == nil {
		config = &ParserConfig{
			SheetName:     "Table1",
			StrictMode:    true,
			SkipEmptyRows: true,
			MaxRows:       0,
		}
	}

	return &HybridParser{
		config:       config,
		reWhitespace: regexp.MustCompile(`\s+`),
		reUnified:    regexp.MustCompile(`^(.*?)([\d-]+)\s*(?:\(\s*GBM\s*(\d+)\s*\))?\s*(.*)$`),
		reCodeFinder: regexp.MustCompile(`[\d-]+(?:\s*\(\s*GBM\s*\d+\s*\))?`),
		reMajorClass: regexp.MustCompile(`第[一二三四五六七八]大类\s+([1-8])\s*(?:\(\s*GBM\s*(\d+)\s*\))?\s*(.*)$`),
	}
}

// Parse 解析输入数据（混合智能解析）
func (p *HybridParser) Parse(ctx context.Context, input io.Reader) ([]*model.ParsedInfo, error) {
	return nil, model.NewSystemError("hybrid_parser", "parse", "混合解析器需要文件路径，不能直接从io.Reader读取", fmt.Errorf("unsupported operation"))
}

// ParseFile 解析Excel文件（混合智能解析入口）
func (p *HybridParser) ParseFile(ctx context.Context, filePath string) (*model.HybridParseResult, error) {
	startTime := time.Now()
	
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, model.NewFileError(model.ErrCodeFileReadError, filePath, "open", "打开Excel文件失败", err)
	}
	defer f.Close()

	rows, err := f.GetRows(p.config.SheetName)
	if err != nil {
		return nil, model.NewFileError(model.ErrCodeFileReadError, p.config.SheetName, "read_sheet", "读取工作表数据失败", err)
	}

	// 第一步：本地预处理 — 以"小类"为单位打包AI任务
	result, err := p.hybridParse(ctx, rows)
	if err != nil {
		return nil, fmt.Errorf("混合解析失败: %w", err)
	}

	// 统计信息
	result.Stats = &model.HybridParseStats{
		TotalRows:      len(rows),
		SkeletonCount:  len(result.SkeletonRecords),
		AITaskCount:    len(result.AITasks),
		ProcessingTime: time.Since(startTime).Milliseconds(),
	}

	log.Printf("混合解析完成: 总行数=%d, 骨架记录=%d, AI任务=%d, 处理时间=%dms",
		result.Stats.TotalRows, result.Stats.SkeletonCount, result.Stats.AITaskCount, result.Stats.ProcessingTime)

	return result, nil
}

// hybridParse 核心混合解析逻辑
// 实现V2方案的四步工作流
func (p *HybridParser) hybridParse(ctx context.Context, rows [][]string) (*model.HybridParseResult, error) {
	var skeletonRecords []*model.SkeletonRecord
	var aiTasks []*model.AITask
	
	// 第一遍：收集所有骨架记录
	for rowIndex, row := range rows {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if p.isJunkRow(row) {
			continue
		}

		// 识别骨架节点（大类、中类、小类）
		skeletonRecords_row := p.identifySkeletonNode(row, rowIndex)
		if len(skeletonRecords_row) > 0 {
			skeletonRecords = append(skeletonRecords, skeletonRecords_row...)
		}
	}
	
	// 第二遍：为每个小类收集对应的EF列数据
	for _, skeletonRecord := range skeletonRecords {
		if skeletonRecord.Level == model.LevelSmall {
			// 创建AI任务
			task := &model.AITask{
				ParentCode: skeletonRecord.Code,
				ParentName: skeletonRecord.Name,
				DetailCodesRaw: make([]string, 0),
				DetailNamesRaw: make([]string, 0),
			}
			
			// 收集该小类对应的所有EF数据
			p.collectDetailDataByPrefix(rows, task, skeletonRecord.Code)
			
			// 只有有数据的任务才添加
			if p.hasTaskContent(task) {
				aiTasks = append(aiTasks, task)
			}
		}
	}

	return &model.HybridParseResult{
		SkeletonRecords: skeletonRecords,
		AITasks:         aiTasks,
	}, nil
}

// identifySkeletonNode 识别骨架节点（大类、中类、小类）
// 新策略：逐列检查每个单元格，精确定位和提取完整信息
func (p *HybridParser) identifySkeletonNode(row []string, rowIndex int) []*model.SkeletonRecord {
	var records []*model.SkeletonRecord
	
	// 注释掉这个检查，因为大类行可能只有1列
	// if len(row) < 4 {
	//	return records
	// }

	// 检查每个单元格（A-D列）是否包含骨架信息
	for colIndex := 0; colIndex < 4 && colIndex < len(row); colIndex++ {
		cellContent := strings.TrimSpace(row[colIndex])
		if cellContent == "" {
			continue
		}
		

		// 尝试从单元格提取骨架信息（可能有多个条目）
		cellRecords := p.extractSkeletonFromCell(cellContent, rowIndex, colIndex)
		if len(cellRecords) > 0 {
			records = append(records, cellRecords...)
		}
	}

	return records
}

// extractSkeletonFromCell 从单个单元格提取骨架信息
// 修改为支持单元格内多个条目的拆分，优化大类识别
func (p *HybridParser) extractSkeletonFromCell(cellContent string, rowIndex, colIndex int) []*model.SkeletonRecord {
	var records []*model.SkeletonRecord
	
	// 第一步：尝试专门的大类识别（八大类：1-8）
	majorRecords := p.extractMajorCategories(cellContent)
	records = append(records, majorRecords...)
	
	// 第二步：使用通用方法识别中类和小类
	generalRecords := p.extractGeneralSkeletonRecords(cellContent)
	records = append(records, generalRecords...)

	return records
}

// extractMajorCategories 专门提取大类（八大类：1-8）
func (p *HybridParser) extractMajorCategories(cellContent string) []*model.SkeletonRecord {
	var records []*model.SkeletonRecord
	
	// 使用专门的大类正则匹配 "第X大类"格式
	matches := p.reMajorClass.FindAllStringSubmatch(cellContent, -1)
	
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		
		code := strings.TrimSpace(match[1])      // 数字编码(1-8)
		gbmCode := strings.TrimSpace(match[2])   // GBM编码
		name := strings.TrimSpace(match[3])      // 名称部分
		
		// 确保这确实是大类（单个数字1-8）
		if len(code) != 1 || code < "1" || code > "8" {
			continue
		}
		
		// 使用统一的名称规范化函数
		name = p.normalizeName(name)
		if name == "" {
			continue
		}
		
		record := &model.SkeletonRecord{
			Code:  code,
			GBM:   p.parseGBM(gbmCode),
			Name:  name,
			Level: model.LevelMajor,
		}
		records = append(records, record)
	}
	
	return records
}

// extractGeneralSkeletonRecords 提取中类和小类
func (p *HybridParser) extractGeneralSkeletonRecords(cellContent string) []*model.SkeletonRecord {
	var records []*model.SkeletonRecord
	
	// 使用reCodeFinder找到所有编码位置
	locs := p.reCodeFinder.FindAllStringIndex(cellContent, -1)
	if locs == nil {
		return records
	}

	// 为每个找到的编码位置提取独立的记录
	for i, loc := range locs {
		var contentPart string
		if i+1 < len(locs) {
			// 取到下一个编码的开始位置
			contentPart = cellContent[loc[0]:locs[i+1][0]]
		} else {
			// 最后一个编码，取到字符串末尾
			contentPart = cellContent[loc[0]:]
		}

		// 解析这个部分
		info, err := p.parseCellContent(contentPart)
		if err != nil || info == nil {
			continue
		}

		// 根据编码确定层级
		level := p.determineLevel(info.Code)
		if level == "" {
			continue // 跳过无效的骨架节点
		}

		// 只处理中类和小类（大类由专门方法处理）
		if level != model.LevelMiddle && level != model.LevelSmall {
			continue
		}

		record := &model.SkeletonRecord{
			Code:  info.Code,
			GBM:   p.parseGBM(info.GbmCode),
			Name:  p.normalizeName(info.Name),
			Level: level,
		}
		records = append(records, record)
	}

	return records
}


// parseGBM 解析GBM编码为整数
func (p *HybridParser) parseGBM(gbmCode string) int {
	if gbmCode == "" {
		return 0
	}
	
	// 简单的字符串转整数，忽略错误
	gbmInt := 0
	for _, char := range gbmCode {
		if char >= '0' && char <= '9' {
			gbmInt = gbmInt*10 + int(char-'0')
		}
	}
	return gbmInt
}

// normalizeName 统一规范化名称字段，清理各种制表符和多余空格
func (p *HybridParser) normalizeName(name string) string {
	if name == "" {
		return ""
	}
	
	// 第一步：替换各种不可见字符为空格
	normalized := strings.ReplaceAll(name, "\r\n", " ")
	normalized = strings.ReplaceAll(normalized, "\r", " ")
	normalized = strings.ReplaceAll(normalized, "\n", " ")
	normalized = strings.ReplaceAll(normalized, "\t", " ")
	normalized = strings.ReplaceAll(normalized, "\u00A0", " ") // 不间断空格
	normalized = strings.ReplaceAll(normalized, "\u2000", " ") // EN四分空格
	normalized = strings.ReplaceAll(normalized, "\u2001", " ") // EM四分空格
	normalized = strings.ReplaceAll(normalized, "\u2002", " ") // EN二分空格
	normalized = strings.ReplaceAll(normalized, "\u2003", " ") // EM二分空格
	normalized = strings.ReplaceAll(normalized, "\u2004", " ") // 三分EM空格
	normalized = strings.ReplaceAll(normalized, "\u2005", " ") // 四分EM空格
	normalized = strings.ReplaceAll(normalized, "\u2006", " ") // 六分EM空格
	normalized = strings.ReplaceAll(normalized, "\u2007", " ") // 数字空格
	normalized = strings.ReplaceAll(normalized, "\u2008", " ") // 标点空格
	normalized = strings.ReplaceAll(normalized, "\u2009", " ") // 薄空格
	normalized = strings.ReplaceAll(normalized, "\u200A", " ") // 发丝空格
	normalized = strings.ReplaceAll(normalized, "\u3000", " ") // 中文全角空格
	
	// 第二步：使用正则表达式将多个连续空格替换为单个空格
	normalized = p.reWhitespace.ReplaceAllString(normalized, " ")
	
	// 第三步：去除首尾空格
	normalized = strings.TrimSpace(normalized)
	
	// 第四步：智能去除中文职业名称中的人为空格
	normalized = p.removeChineseSpaces(normalized)
	
	return normalized
}

// removeChineseSpaces 智能去除中文职业名称中的人为空格
func (p *HybridParser) removeChineseSpaces(name string) string {
	if name == "" {
		return ""
	}
	
	// 用正则表达式去除中文字符之间的单个空格
	// 匹配：中文字符 + 空格 + 中文字符，替换为：中文字符 + 中文字符
	chineseSpacePattern := regexp.MustCompile("([\\x{4e00}-\\x{9fff}])\\s+([\\x{4e00}-\\x{9fff}])")
	
	// 循环替换，直到没有更多匹配（处理连续多个空格的情况）
	for {
		oldName := name
		name = chineseSpacePattern.ReplaceAllString(name, "$1$2")
		if name == oldName {
			break // 没有更多替换了
		}
	}
	
	return name
}

// collectDetailDataByPrefix 精准版：E列精确前缀匹配，F列对应匹配减少LLM输入长度
func (p *HybridParser) collectDetailDataByPrefix(rows [][]string, task *model.AITask, smallClassCode string) {
	// 用于存储该小类对应的细类编码和名称的精确对应关系
	var allDetailCodes []string
	var allDetailNames []string
	
	for _, row := range rows {
		if len(row) <= 5 {
			continue
		}

		eCol := strings.TrimSpace(row[4]) // E列
		fCol := strings.TrimSpace(row[5]) // F列

		// 跳过明显无效的行
		if eCol == "续表" || fCol == "续表" {
			continue
		}

		// 检查E列是否包含该小类前缀的细类编码
		if eCol != "" {
			// 按换行符分割E列内容，处理跨格子的情况
			eCodes := strings.Split(eCol, "\n")
			fNames := strings.Split(fCol, "\n")
			
			var matchedCodes []string
			var matchedNames []string
			var hasTargetCode bool // 标记该行是否有目标小类的细类编码
			
			for _, code := range eCodes {
				cleanCode := strings.TrimSpace(code)
				if cleanCode == "" {
					continue
				}
				
				// 精确匹配：必须是 smallClassCode-数字 格式
				if p.isExactDetailCode(cleanCode, smallClassCode) {
					matchedCodes = append(matchedCodes, cleanCode)
					hasTargetCode = true
				}
			}
			
			// 只有当该行E列包含目标小类的细类编码时，才收集对应的F列数据
			if hasTargetCode && len(matchedCodes) > 0 {
				// 清理F列名称数据
				for _, name := range fNames {
					cleanName := strings.TrimSpace(name)
					if cleanName != "" {
						// 使用统一的名称规范化函数
						cleanName = p.normalizeName(cleanName)
						if cleanName != "" {
							matchedNames = append(matchedNames, cleanName)
						}
					}
				}
				
				// 确保编码和名称数量匹配 - 取最小长度
				minLen := len(matchedCodes)
				if len(matchedNames) < minLen {
					minLen = len(matchedNames)
				}
				
				// 只添加配对的编码和名称
				for i := 0; i < minLen; i++ {
					allDetailCodes = append(allDetailCodes, matchedCodes[i])
					allDetailNames = append(allDetailNames, matchedNames[i])
				}
			}
		}
	}
	
	// 设置任务数据
	if len(allDetailCodes) > 0 {
		task.DetailCodesRaw = allDetailCodes
	}
	
	if len(allDetailNames) > 0 {
		task.DetailNamesRaw = allDetailNames
	}
}

// isExactDetailCode 检查编码是否精确匹配小类前缀格式
func (p *HybridParser) isExactDetailCode(code, smallClassCode string) bool {
	// 编码必须以 smallClassCode- 开头
	if !strings.HasPrefix(code, smallClassCode+"-") {
		return false
	}
	
	// 检查后缀是否为数字格式
	suffix := strings.TrimPrefix(code, smallClassCode+"-")
	if suffix == "" {
		return false
	}
	
	// 验证后缀是否为数字
	for _, char := range suffix {
		if char < '0' || char > '9' {
			return false
		}
	}
	
	return true
}

// collectDetailData 收集细类数据（E列和F列）- 修复数量匹配问题
func (p *HybridParser) collectDetailData(row []string, task *model.AITask) {
	if len(row) <= 5 {
		return
	}

	codeData := strings.TrimSpace(row[4]) // E列
	nameData := strings.TrimSpace(row[5]) // F列

	// 跳过无效数据
	if codeData == "" && nameData == "" {
		return
	}
	if codeData == "续表" || nameData == "续表" {
		return
	}

	// 按换行符分割E列和F列的数据
	codes := strings.Split(codeData, "\n")
	names := strings.Split(nameData, "\n")

	// 清理和过滤空项
	var cleanCodes []string
	var cleanNames []string

	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code != "" {
			cleanCodes = append(cleanCodes, code)
		}
	}

	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			cleanNames = append(cleanNames, name)
		}
	}

	// 确保编码和名称数量匹配 - 取最小长度
	minLen := len(cleanCodes)
	if len(cleanNames) < minLen {
		minLen = len(cleanNames)
	}

	// 只添加配对的编码和名称
	for i := 0; i < minLen; i++ {
		task.DetailCodesRaw = append(task.DetailCodesRaw, cleanCodes[i])
		task.DetailNamesRaw = append(task.DetailNamesRaw, cleanNames[i])
	}
}

// hasTaskContent 检查任务是否有内容
func (p *HybridParser) hasTaskContent(task *model.AITask) bool {
	return len(task.DetailCodesRaw) > 0 || len(task.DetailNamesRaw) > 0
}

// determineLevel 根据编码确定层级
func (p *HybridParser) determineLevel(code string) string {
	if code == "" {
		return ""
	}
	
	dashCount := strings.Count(code, "-")
	switch dashCount {
	case 0:
		return model.LevelMajor  // 大类：如 "1"
	case 1:
		return model.LevelMiddle // 中类：如 "1-01"
	case 2:
		return model.LevelSmall  // 小类：如 "1-01-00"
	default:
		return "" // 细类由AI处理，这里不识别
	}
}

// parseCellContent 解析单个记录的字符串
func (p *HybridParser) parseCellContent(raw string) (*model.ParsedInfo, error) {
	clean := strings.ReplaceAll(raw, "\u00A0", " ")
	preprocessed := p.reWhitespace.ReplaceAllString(clean, " ")
	preprocessed = strings.TrimSpace(preprocessed)
	if preprocessed == "" {
		return nil, nil
	}

	matches := p.reUnified.FindStringSubmatch(preprocessed)
	if len(matches) != 5 {
		return nil, fmt.Errorf("无法解析单元格内容: '%s'", raw)
	}

	prefixName := strings.TrimSpace(matches[1])
	code := strings.TrimSpace(matches[2])
	gbmCode := strings.TrimSpace(matches[3])
	suffixName := strings.TrimSpace(matches[4])

	// 智能拼接名称，避免强制添加空格
	var fullName string
	if prefixName != "" && suffixName != "" {
		fullName = prefixName + suffixName
	} else if prefixName != "" {
		fullName = prefixName
	} else {
		fullName = suffixName
	}
	
	info := &model.ParsedInfo{
		Code:    code,
		GbmCode: gbmCode,
		Name:    p.normalizeName(fullName),
	}
	return info, nil
}

// isJunkRow 检查是否为垃圾行
func (p *HybridParser) isJunkRow(row []string) bool {
	if len(row) == 0 {
		return true
	}

	firstCell := strings.TrimSpace(row[0])
	if firstCell == "大类" || firstCell == "中类" {
		return true
	}

	for _, cell := range row {
		cleanCell := p.reWhitespace.ReplaceAllString(cell, "")
		if cleanCell == "续表" || strings.Contains(cleanCell, "职业分类大典") || cleanCell == "分类体系表" {
			return true
		}
	}
	return false
}

// 实现Parser接口
func (p *HybridParser) Validate() error {
	if p.config.SheetName == "" {
		return model.NewValidationError("工作表名称不能为空", "sheet_name", "", "required")
	}
	return nil
}

func (p *HybridParser) GetName() string {
	return "HybridParser"
}

func (p *HybridParser) GetVersion() string {
	return "2.0.0"
}

func (p *HybridParser) GetSupportedFormats() []string {
	return []string{"xlsx", "xls"}
}