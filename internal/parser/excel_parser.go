// Package parser 实现数据解析功能
package parser

import (
	"context"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"

	"github.com/freedkr/moonshot/internal/model"
	"github.com/xuri/excelize/v2"
)

// ExcelParserImpl Excel解析器实现
type ExcelParserImpl struct {
	config *ParserConfig
	// reWhitespace 用于匹配一个或多个连续的空白字符（包括空格、制表符、换行符等）。
	reWhitespace *regexp.Regexp
	// reUnified 是一个核心的正则表达式，用于从一个完整的记录字符串中解析出各个部分。
	// 示例: "专业技术 1-02-03 (GBM 10203) 高级工程师"
	// 捕获组:
	// 1. (.*?): 前缀名称 (非贪婪) -> "专业技术 "
	// 2. ([\d-]+): 职业代码 -> "1-02-03"
	// 3. (\d+): GBM代码 (在可选的GBM部分内) -> "10203"
	// 4. (.*): 后缀名称 (贪婪) -> " 高级工程师"
	reUnified *regexp.Regexp
	// reCodeFinder 用于在可能包含多个记录的文本行中定位每个记录的起始位置。
	// 它通过查找 "代码" 或 "代码 (GBM xxxx)" 模式来分割字符串。
	// 示例: "1 (GBM 10000) A 2-01 B" -> 会找到 "1 (GBM 10000)" 和 "2-01"
	reCodeFinder *regexp.Regexp
}

// ParserConfig 解析器配置
type ParserConfig struct {
	SheetName     string `yaml:"sheet_name" json:"sheet_name"`
	StrictMode    bool   `yaml:"strict_mode" json:"strict_mode"`
	SkipEmptyRows bool   `yaml:"skip_empty_rows" json:"skip_empty_rows"`
	MaxRows       int    `yaml:"max_rows" json:"max_rows"`
}

// NewExcelParser 创建新的Excel解析器
func NewExcelParser(config *ParserConfig) *ExcelParserImpl {
	if config == nil {
		config = &ParserConfig{
			SheetName:     "Table1",
			StrictMode:    true,
			SkipEmptyRows: true,
			MaxRows:       0, // 0表示不限制
		}
	}

	return &ExcelParserImpl{
		config:       config,
		reWhitespace: regexp.MustCompile(`\s+`),
		reUnified:    regexp.MustCompile(`^(.*?)([\d-]+)\s*(?:\(\s*GBM\s*(\d+)\s*\))?\s*(.*)$`), // 详见结构体注释
		reCodeFinder: regexp.MustCompile(`[\d-]+(?:\s*\(\s*GBM\s*\d+\s*\))?`),
	}
}

// Parse 解析输入数据
func (p *ExcelParserImpl) Parse(ctx context.Context, input io.Reader) ([]*model.ParsedInfo, error) {
	// 由于excelize需要文件路径，这里需要特殊处理
	// 在实际实现中，我们可能需要将io.Reader的内容写入临时文件
	// 或者修改接口设计来直接接受文件路径
	return nil, model.NewSystemError("excel_parser", "parse", "Excel解析器需要文件路径，不能直接从io.Reader读取", fmt.Errorf("unsupported operation"))
}

// ParseFile 解析Excel文件
func (p *ExcelParserImpl) ParseFile(ctx context.Context, filePath string) ([]*model.ParsedInfo, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, model.NewFileError(model.ErrCodeFileReadError, filePath, "open", "打开Excel文件失败", err)
	}
	defer f.Close()

	rows, err := f.GetRows(p.config.SheetName)
	if err != nil {
		return nil, model.NewFileError(model.ErrCodeFileReadError, p.config.SheetName, "read_sheet", "读取工作表数据失败", err)
	}

	// 第一步：从E/F列（索引4和5）直接提取所有细类记录。
	detailRecords, err := p.extractDetailRecords(ctx, rows)
	if err != nil {
		return nil, fmt.Errorf("提取细类记录失败: %w", err)
	}
	// fmt.Println(rows)
	// 第二步：从前4列（A-D）提取骨架结构（大类、中类、小类）。
	skeletonRecords, err := p.extractSkeletonRecords(ctx, rows)
	if err != nil {
		return nil, fmt.Errorf("提取骨架记录失败: %w", err)
	}

	// 合并所有记录
	var allRecords []*model.ParsedInfo
	allRecords = append(allRecords, skeletonRecords...)
	allRecords = append(allRecords, detailRecords...)

	log.Printf("提取到 %d 条骨架记录，%d 条细类记录，合计 %d 条",
		len(skeletonRecords), len(detailRecords), len(allRecords))

	return allRecords, nil
}

// extractDetailRecords 从E/F列（索引4和5）提取细类记录。
func (p *ExcelParserImpl) extractDetailRecords(ctx context.Context, rows [][]string) ([]*model.ParsedInfo, error) {
	var detailRecords []*model.ParsedInfo

	for _, row := range rows {
		if len(row) <= 5 {
			continue
		}

		codeData := strings.TrimSpace(row[4]) // E列
		nameData := strings.TrimSpace(row[5]) // F列

		// 跳过无效行
		if codeData == "" || nameData == "" || codeData == "续表" || nameData == "续表" {
			continue
		}

		// 按换行符分割E列和F列的数据
		codes := strings.Split(codeData, "\n")
		names := strings.Split(nameData, "\n")

		// 清理和过滤空项
		var cleanCodes []string
		var cleanNames []string

		for _, code := range codes {
			code = strings.TrimSpace(code)
			if code != "" && strings.Count(code, "-") == 3 {
				cleanCodes = append(cleanCodes, code)
			}
		}

		for _, name := range names {
			name = strings.TrimSpace(name)
			if name != "" {
				cleanNames = append(cleanNames, name)
			}
		}

		// 建立细类记录
		minLen := len(cleanCodes)
		if len(cleanNames) < minLen {
			minLen = len(cleanNames)
		}

		for i := 0; i < minLen; i++ {
			code := cleanCodes[i]
			name := cleanNames[i]

			// 直接创建细类记录
			detailRecords = append(detailRecords, &model.ParsedInfo{
				Code: code,
				Name: p.normalizeName(name),
			})
		}

		// 检查上下文取消
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	return detailRecords, nil
}

// extractSkeletonRecords 从前4列（A-D）提取骨架结构（大类、中类、小类）。
func (p *ExcelParserImpl) extractSkeletonRecords(ctx context.Context, rows [][]string) ([]*model.ParsedInfo, error) {
	var skeletonRecords []*model.ParsedInfo

	for i, row := range rows {
		if p.isJunkRow(row) {
			continue
		}

		// 只处理前4列的内容提取骨架结构
		firstFourCols := make([]string, 0, 4)
		for j := 0; j < len(row) && j < 4; j++ {
			firstFourCols = append(firstFourCols, row[j])
		}

		fullText := strings.Join(firstFourCols, " ")
		records, err := p.extractRecords(fullText)
		if err != nil {
			if p.config.StrictMode {
				return nil, model.NewParseError(i+1, 0, fullText, "", fmt.Sprintf("处理Excel第 %d 行时提取记录失败: %v", i+1, err))
			}
			log.Printf("警告：处理Excel第 %d 行时提取记录失败: %v", i+1, err)
			continue
		}
		skeletonRecords = append(skeletonRecords, records...)

		// 检查上下文取消
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	return skeletonRecords, nil
}

// extractRecords 从给定的文本字符串中提取一个或多个记录。
// 适用于一行中包含多个职业分类的情况。
func (p *ExcelParserImpl) extractRecords(text string) ([]*model.ParsedInfo, error) {
	locs := p.reCodeFinder.FindAllStringIndex(text, -1)
	if locs == nil {
		return nil, nil
	}

	var records []*model.ParsedInfo
	for i, loc := range locs {
		var contentPart string
		if i+1 < len(locs) {
			contentPart = text[loc[0]:locs[i+1][0]]
		} else {
			contentPart = text[loc[0]:]
		}

		info, err := p.parseCellContent(contentPart)
		if err != nil {
			log.Printf("警告：在提取记录时跳过一个片段，原因: %v", err)
			continue
		}
		if info != nil {
			records = append(records, info)
		}
	}
	return records, nil
}

// parseCellContent 解析单个记录的字符串，并将其转换为ParsedInfo结构体。
// 它处理非标准空格，并使用reUnified正则表达式来提取代码、GBM代码和名称。
func (p *ExcelParserImpl) parseCellContent(raw string) (*model.ParsedInfo, error) {
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

	info := &model.ParsedInfo{
		Code:    code,
		GbmCode: gbmCode,
		Name:    p.normalizeName(prefixName + " " + suffixName),
	}
	return info, nil
}

// normalizeName 统一规范化名称字段，清理各种制表符和多余空格
// 从 HybridParser 引入以增强传统模式的清理能力
func (p *ExcelParserImpl) normalizeName(name string) string {
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
func (p *ExcelParserImpl) removeChineseSpaces(name string) string {
	if name == "" {
		return ""
	}
	chineseSpacePattern := regexp.MustCompile("([\\x{4e00}-\\x{9fff}])\\s+([\\x{4e00}-\\x{9fff}])")
	// 循环替换，直到没有更多匹配项，以处理 "字 符 串" 这样的情况
	for {
		newName := chineseSpacePattern.ReplaceAllString(name, "$1$2")
		if newName == name {
			// 如果字符串不再变化，说明已经清理干净
			break
		}
		name = newName
	}
	return name
}

// isJunkRow 检查给定的Excel行是否为应被忽略的“垃圾行”。
// 这包括空行、表头行（如“大类”、“中类”）、续表标记或文档标题行。
func (p *ExcelParserImpl) isJunkRow(row []string) bool {
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

// Validate 验证解析器配置
func (p *ExcelParserImpl) Validate() error {
	if p.config.SheetName == "" {
		return model.NewValidationError("工作表名称不能为空", "sheet_name", "", "required")
	}
	return nil
}

// GetSupportedFormats 获取支持的格式
func (p *ExcelParserImpl) GetSupportedFormats() []string {
	return []string{"xlsx", "xls"}
}

// GetName 获取解析器名称
func (p *ExcelParserImpl) GetName() string {
	return "ExcelParser"
}

// GetVersion 获取解析器版本
func (p *ExcelParserImpl) GetVersion() string {
	return "1.0.0"
}
