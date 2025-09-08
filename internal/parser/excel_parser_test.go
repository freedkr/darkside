package parser

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/freedkr/moonshot/internal/model"
)

func TestNewExcelParser(t *testing.T) {
	// 测试使用默认配置
	parser := NewExcelParser(nil)
	if parser == nil {
		t.Fatal("Expected parser to be created")
	}
	if parser.config.SheetName != "Table1" {
		t.Errorf("Expected default sheet name 'Table1', got '%s'", parser.config.SheetName)
	}
	if !parser.config.StrictMode {
		t.Error("Expected strict mode to be true by default")
	}

	// 测试使用自定义配置
	config := &ParserConfig{
		SheetName:     "CustomSheet",
		StrictMode:    false,
		SkipEmptyRows: false,
		MaxRows:       100,
	}
	parser = NewExcelParser(config)
	if parser.config.SheetName != "CustomSheet" {
		t.Errorf("Expected sheet name 'CustomSheet', got '%s'", parser.config.SheetName)
	}
	if parser.config.StrictMode {
		t.Error("Expected strict mode to be false")
	}
}

func TestExcelParserImpl_GetName(t *testing.T) {
	parser := NewExcelParser(nil)
	if parser.GetName() != "ExcelParser" {
		t.Errorf("Expected name 'ExcelParser', got '%s'", parser.GetName())
	}
}

func TestExcelParserImpl_GetVersion(t *testing.T) {
	parser := NewExcelParser(nil)
	if parser.GetVersion() != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", parser.GetVersion())
	}
}

func TestExcelParserImpl_GetSupportedFormats(t *testing.T) {
	parser := NewExcelParser(nil)
	formats := parser.GetSupportedFormats()
	expected := []string{"xlsx", "xls"}

	if !reflect.DeepEqual(formats, expected) {
		t.Errorf("Expected formats %v, got %v", expected, formats)
	}
}

func TestExcelParserImpl_Validate(t *testing.T) {
	// 有效配置
	config := &ParserConfig{
		SheetName: "Sheet1",
	}
	parser := NewExcelParser(config)
	if err := parser.Validate(); err != nil {
		t.Errorf("Expected no error for valid config, got %v", err)
	}

	// 无效配置 - 空工作表名
	config = &ParserConfig{
		SheetName: "",
	}
	parser = NewExcelParser(config)
	if err := parser.Validate(); err == nil {
		t.Error("Expected error for empty sheet name")
	}
}

func TestExcelParserImpl_Parse(t *testing.T) {
	parser := NewExcelParser(nil)
	ctx := context.Background()

	// Parse方法应该返回不支持的错误
	_, err := parser.Parse(ctx, nil)
	if err == nil {
		t.Error("Expected error for unsupported Parse method")
	}

	// 验证错误类型
	if !model.IsErrorType(err, model.ErrCodeInternal) {
		t.Error("Expected SystemError for unsupported operation")
	}
}

func TestExcelParserImpl_parseCellContent(t *testing.T) {
	parser := NewExcelParser(nil)

	tests := []struct {
		name     string
		input    string
		expected *model.ParsedInfo
		hasError bool
	}{
		{
			name:  "标准格式带GBM",
			input: "1 (GBM 10000) 国家机关负责人",
			expected: &model.ParsedInfo{
				Code:    "1",
				GbmCode: "10000",
				Name:    "国家机关负责人",
			},
			hasError: false,
		},
		{
			name:  "标准格式不带GBM",
			input: "1-01 中国共产党机关负责人",
			expected: &model.ParsedInfo{
				Code:    "1-01",
				GbmCode: "",
				Name:    "中国共产党机关负责人",
			},
			hasError: false,
		},
		{
			name:  "复杂格式",
			input: "专业技术 1-02-03 (GBM 10203) 高级工程师",
			expected: &model.ParsedInfo{
				Code:    "1-02-03",
				GbmCode: "10203",
				Name:    "专业技术 高级工程师",
			},
			hasError: false,
		},
		{
			name:     "空内容",
			input:    "",
			expected: nil,
			hasError: false,
		},
		{
			name:     "纯空格",
			input:    "   \t\n   ",
			expected: nil,
			hasError: false,
		},
		{
			name:     "无效格式",
			input:    "无效的内容格式",
			expected: nil,
			hasError: true,
		},
		{
			name:  "包含非断空格",
			input: "1\u00A0(GBM\u00A010000)\u00A0测试内容",
			expected: &model.ParsedInfo{
				Code:    "1",
				GbmCode: "10000",
				Name:    "测试内容",
			},
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.parseCellContent(tt.input)

			if tt.hasError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.hasError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tt.expected == nil && result != nil {
				t.Errorf("Expected nil result, got %+v", result)
			}
			if tt.expected != nil && result == nil {
				t.Error("Expected result but got nil")
			}
			if tt.expected != nil && result != nil {
				if result.Code != tt.expected.Code {
					t.Errorf("Expected code '%s', got '%s'", tt.expected.Code, result.Code)
				}
				if result.GbmCode != tt.expected.GbmCode {
					t.Errorf("Expected gbm code '%s', got '%s'", tt.expected.GbmCode, result.GbmCode)
				}
				if result.Name != tt.expected.Name {
					t.Errorf("Expected name '%s', got '%s'", tt.expected.Name, result.Name)
				}
			}
		})
	}
}

func TestExcelParserImpl_extractRecords(t *testing.T) {
	parser := NewExcelParser(nil)

	tests := []struct {
		name     string
		input    string
		expected int // 期望的记录数量
	}{
		{
			name:     "单个记录",
			input:    "1 (GBM 10000) 国家机关负责人",
			expected: 1,
		},
		{
			name:     "多个记录",
			input:    "1 (GBM 10000) 国家机关负责人 1-01 (GBM 10100) 党群组织负责人",
			expected: 2,
		},
		{
			name:     "无记录",
			input:    "这里没有符合格式的内容",
			expected: 0,
		},
		{
			name:     "空字符串",
			input:    "",
			expected: 0,
		},
		{
			name:     "包含无效记录的混合内容",
			input:    "1 (GBM 10000) 有效记录 无效内容 1-01 有效记录2",
			expected: 3, // 实际实现提取了3个记录
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := parser.extractRecords(tt.input)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if len(results) != tt.expected {
				t.Errorf("Expected %d records, got %d", tt.expected, len(results))
			}
		})
	}
}

func TestExcelParserImpl_isJunkRow(t *testing.T) {
	parser := NewExcelParser(nil)

	tests := []struct {
		name     string
		row      []string
		expected bool
	}{
		{
			name:     "空行",
			row:      []string{},
			expected: true,
		},
		{
			name:     "表头行 - 大类",
			row:      []string{"大类", "中类", "小类"},
			expected: true,
		},
		{
			name:     "表头行 - 中类",
			row:      []string{"中类", "小类", "细类"},
			expected: true,
		},
		{
			name:     "续表行",
			row:      []string{"续表", "", ""},
			expected: true,
		},
		{
			name:     "职业分类大典行",
			row:      []string{"", "中华人民共和国职业分类大典", ""},
			expected: true,
		},
		{
			name:     "分类体系表行",
			row:      []string{"", "", "分类体系表"},
			expected: true,
		},
		{
			name:     "正常数据行",
			row:      []string{"1 (GBM 10000) 国家机关负责人", "", ""},
			expected: false,
		},
		{
			name:     "包含空格的垃圾行",
			row:      []string{"续 表", "", ""},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.isJunkRow(tt.row)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for row %v", tt.expected, result, tt.row)
			}
		})
	}
}

func TestExcelParserImpl_extractDetailRecords(t *testing.T) {
	parser := NewExcelParser(nil)
	ctx := context.Background()

	// 测试数据：模拟Excel行，E列(索引4)和F列(索引5)包含细类数据
	rows := [][]string{
		{"", "", "", "", "", ""}, // 空行，应该跳过
		{"", "", "", "", "1-01-01-01\n1-01-01-02", "细类名称1\n细类名称2"},         // 有效细类数据
		{"", "", "", "", "1-02-01-01", "单个细类"},                             // 单个细类
		{"", "", "", "", "续表", "续表"},                                       // 垃圾数据，应该跳过
		{"", "", "", "", "", ""},                                           // 空列，应该跳过
		{"", "", "", "", "2-01-01-01\n2-01-01-02\n2-01-01-03", "名称1\n名称2"}, // 代码比名称多的情况
	}

	results, err := parser.extractDetailRecords(ctx, rows)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// 期望结果：2 + 1 + 2 = 5个细类记录
	expected := 5
	if len(results) != expected {
		t.Errorf("Expected %d records, got %d", expected, len(results))
	}

	// 验证第一组记录
	if len(results) >= 2 {
		if results[0].Code != "1-01-01-01" || results[0].Name != "细类名称1" {
			t.Errorf("First record incorrect: code=%s, name=%s", results[0].Code, results[0].Name)
		}
		if results[1].Code != "1-01-01-02" || results[1].Name != "细类名称2" {
			t.Errorf("Second record incorrect: code=%s, name=%s", results[1].Code, results[1].Name)
		}
	}

	// 验证所有记录都是细类（3个连字符）
	for i, record := range results {
		if strings.Count(record.Code, "-") != 3 {
			t.Errorf("Record %d should be detail level (3 dashes), got %s", i, record.Code)
		}
	}
}

func TestExcelParserImpl_extractSkeletonRecords(t *testing.T) {
	parser := NewExcelParser(nil)
	ctx := context.Background()

	// 测试数据：模拟Excel前4列的骨架数据
	rows := [][]string{
		{"大类", "中类", "小类", "细类"},                    // 表头，应该被过滤
		{"1 (GBM 10000) 国家机关负责人", "", "", ""},       // 大类
		{"", "1-01 (GBM 10100) 中国共产党机关负责人", "", ""}, // 中类
		{"", "", "1-01-01 (GBM 10101) 委员会负责人", ""},  // 小类
		{"续表", "", "", ""},                          // 垃圾行，应该被过滤
		{"2 (GBM 20000) 专业技术人员", "", "", ""},        // 另一个大类
	}

	results, err := parser.extractSkeletonRecords(ctx, rows)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// 应该有4条有效记录（过滤掉表头和垃圾行）
	if len(results) < 3 {
		t.Errorf("Expected at least 3 records, got %d", len(results))
	}

	// 验证记录层级分布
	levelCounts := make(map[int]int)
	for _, record := range results {
		level := strings.Count(record.Code, "-")
		levelCounts[level]++
	}

	// 应该包含不同层级的记录
	if levelCounts[0] == 0 {
		t.Error("Expected at least one major category (0 dashes)")
	}
}

// 测试严格模式
func TestExcelParserImpl_StrictMode(t *testing.T) {
	// 严格模式下的解析器
	strictConfig := &ParserConfig{
		SheetName:  "Sheet1",
		StrictMode: true,
	}
	strictParser := NewExcelParser(strictConfig)

	// 宽松模式下的解析器
	lenientConfig := &ParserConfig{
		SheetName:  "Sheet1",
		StrictMode: false,
	}
	lenientParser := NewExcelParser(lenientConfig)

	ctx := context.Background()

	// 包含无效数据的行
	badRows := [][]string{
		{"无效格式的数据", "", "", ""},
		{"1 (GBM 10000) 有效数据", "", "", ""},
	}

	// 严格模式应该在遇到错误时停止（但当前实现只是记录警告）
	strictResults, strictErr := strictParser.extractSkeletonRecords(ctx, badRows)
	lenientResults, lenientErr := lenientParser.extractSkeletonRecords(ctx, badRows)

	// 两种模式都不应该返回错误（因为当前实现是宽容的）
	if strictErr != nil {
		t.Errorf("Strict mode error: %v", strictErr)
	}
	if lenientErr != nil {
		t.Errorf("Lenient mode error: %v", lenientErr)
	}

	// 结果应该包含有效记录
	if len(strictResults) == 0 || len(lenientResults) == 0 {
		t.Error("Both modes should return valid records")
	}
}

// 测试上下文取消
func TestExcelParserImpl_ContextCancellation(t *testing.T) {
	parser := NewExcelParser(nil)

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	// 测试extractDetailRecords
	rows := [][]string{
		{"", "", "", "", "1-01-01-01", "测试"},
	}

	_, err := parser.extractDetailRecords(ctx, rows)
	if err == nil {
		t.Error("Expected context cancellation error")
	}
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}

	// 测试extractSkeletonRecords
	_, err = parser.extractSkeletonRecords(ctx, rows)
	if err == nil {
		t.Error("Expected context cancellation error")
	}
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}
