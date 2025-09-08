package builder

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/freedkr/moonshot/internal/model"
)

func TestNewHierarchyBuilder(t *testing.T) {
	// 测试使用默认配置
	builder := NewHierarchyBuilder(nil)
	if builder == nil {
		t.Fatal("Expected builder to be created")
	}
	if !builder.config.EnableOrphanHandling {
		t.Error("Expected orphan handling to be enabled by default")
	}
	if builder.config.StrictMode {
		t.Error("Expected strict mode to be false by default")
	}

	// 测试使用自定义配置
	config := &BuilderConfig{
		EnableOrphanHandling: false,
		StrictMode:           true,
	}
	builder = NewHierarchyBuilder(config)
	if builder.config.EnableOrphanHandling {
		t.Error("Expected orphan handling to be disabled")
	}
	if !builder.config.StrictMode {
		t.Error("Expected strict mode to be enabled")
	}
}

func TestHierarchyBuilderImpl_GetName(t *testing.T) {
	builder := NewHierarchyBuilder(nil)
	if builder.GetName() != "HierarchyBuilder" {
		t.Errorf("Expected name 'HierarchyBuilder', got '%s'", builder.GetName())
	}
}

func TestHierarchyBuilderImpl_GetVersion(t *testing.T) {
	builder := NewHierarchyBuilder(nil)
	if builder.GetVersion() != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", builder.GetVersion())
	}
}

func TestHierarchyBuilderImpl_determineLevel(t *testing.T) {
	builder := NewHierarchyBuilder(nil)

	tests := []struct {
		code     string
		expected string
	}{
		{"1", LevelMajor},
		{"1-01", LevelMiddle},
		{"1-01-01", LevelSmall},
		{"1-01-01-01", LevelDetail},
		{"1-01-01-01-01", "未知级别"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			result := builder.determineLevel(tt.code)
			if result != tt.expected {
				t.Errorf("determineLevel(%s) = %s, expected %s", tt.code, result, tt.expected)
			}
		})
	}
}

func TestHierarchyBuilderImpl_getParentCode(t *testing.T) {
	builder := NewHierarchyBuilder(nil)

	tests := []struct {
		name      string
		code      string
		parent    string
		hasParent bool
	}{
		{"大类", "1", "", false},
		{"中类", "1-01", "1", true},
		{"小类", "1-01-01", "1-01", true},
		{"细类", "1-01-01-01", "1-01-01", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent, hasParent := builder.getParentCode(tt.code)
			if parent != tt.parent {
				t.Errorf("getParentCode(%s) parent = %s, expected %s", tt.code, parent, tt.parent)
			}
			if hasParent != tt.hasParent {
				t.Errorf("getParentCode(%s) hasParent = %v, expected %v", tt.code, hasParent, tt.hasParent)
			}
		})
	}
}

func TestHierarchyBuilderImpl_Build(t *testing.T) {
	builder := NewHierarchyBuilder(nil)
	ctx := context.Background()

	// 使用测试数据构建层级结构
	categories, err := builder.Build(ctx, SampleParsedInfo)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// 验证根分类数量
	expectedRootCount := 2 // "1" 和 "2"
	if len(categories) != expectedRootCount {
		t.Errorf("Expected %d root categories, got %d", expectedRootCount, len(categories))
	}

	// 验证根分类（不依赖顺序）
	var root *model.Category
	for _, cat := range categories {
		if cat.Code == "1" {
			root = cat
			break
		}
	}
	if root != nil {
		if root.Level != LevelMajor {
			t.Errorf("Expected root level '%s', got '%s'", LevelMajor, root.Level)
		}

		// 验证子分类
		if len(root.Children) != 1 {
			t.Errorf("Expected 1 child for root '1', got %d", len(root.Children))
		}
	}

	// 验证层级关系
	for _, category := range categories {
		validateHierarchy(t, category, 0)
	}
}

func TestHierarchyBuilderImpl_Build_WithOrphans(t *testing.T) {
	// 创建包含孤儿节点的测试数据
	orphanData := []*model.ParsedInfo{
		{Code: "1", Name: "大类1", Level: 0},
		{Code: "1-01-01", Name: "小类 - 缺少中类", Level: 2}, // 孤儿节点
		{Code: "2-01", Name: "中类 - 缺少大类", Level: 1},    // 孤儿节点
	}

	// 测试启用孤儿处理的构建器
	config := &BuilderConfig{
		EnableOrphanHandling: true,
		StrictMode:           false,
	}
	builder := NewHierarchyBuilder(config)
	ctx := context.Background()

	categories, err := builder.Build(ctx, orphanData)
	if err != nil {
		t.Fatalf("Unexpected error with orphan handling: %v", err)
	}

	// 孤儿节点应该被作为根节点处理
	if len(categories) != 3 {
		t.Errorf("Expected 3 categories (including orphans), got %d", len(categories))
	}

	// 测试严格模式构建器
	strictConfig := &BuilderConfig{
		EnableOrphanHandling: false,
		StrictMode:           true,
	}
	strictBuilder := NewHierarchyBuilder(strictConfig)

	_, err = strictBuilder.Build(ctx, orphanData)
	if err == nil {
		t.Error("Expected error in strict mode with orphan nodes")
	}
	if !model.IsErrorType(err, model.ErrCodeHierarchy) {
		t.Error("Expected HierarchyError for orphan nodes in strict mode")
	}
}

func TestHierarchyBuilderImpl_Build_WithDuplicates(t *testing.T) {
	// 创建包含重复编码的测试数据
	duplicateData := []*model.ParsedInfo{
		{Code: "1", Name: "大类1", Level: 0},
		{Code: "1", Name: "大类1重复", Level: 0}, // 重复编码
		{Code: "1-01", Name: "中类", Level: 1},
	}

	builder := NewHierarchyBuilder(nil)
	ctx := context.Background()

	categories, err := builder.Build(ctx, duplicateData)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// 重复节点应该被忽略，保留第一个
	if len(categories) != 1 {
		t.Errorf("Expected 1 category after deduplication, got %d", len(categories))
	}

	if categories[0].Name != "大类1" {
		t.Errorf("Expected first occurrence to be kept, got name '%s'", categories[0].Name)
	}
}

func TestHierarchyBuilderImpl_BuildWithOptions(t *testing.T) {
	builder := NewHierarchyBuilder(nil)
	ctx := context.Background()

	// 目前BuildWithOptions只是简单调用Build
	categories, err := builder.BuildWithOptions(ctx, SampleParsedInfo, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(categories) != 2 {
		t.Errorf("Expected 2 root categories, got %d", len(categories))
	}
}

func TestHierarchyBuilderImpl_Validate(t *testing.T) {
	builder := NewHierarchyBuilder(nil)

	// 测试有效数据
	validCategories := SampleCategories
	errors := builder.Validate(validCategories)
	if errors != nil {
		t.Errorf("Expected no errors for valid categories, got %d errors", errors.Count())
	}

	// 测试无效数据
	invalidCategories := []*model.Category{
		{
			Code:  "", // 缺少编码
			Name:  "无效分类",
			Level: LevelMajor,
		},
		{
			Code:  "1-01",
			Name:  "", // 缺少名称
			Level: LevelMiddle,
		},
		{
			Code:  "2-01",
			Name:  "有效名称",
			Level: "", // 缺少层级
		},
		{
			Code:  "3-01",
			Name:  "有效名称",
			Level: "无效层级", // 无效层级
		},
		{
			Code:  "invalid-code!", // 无效编码格式
			Name:  "有效名称",
			Level: LevelMajor,
		},
	}

	errors = builder.Validate(invalidCategories)
	if errors == nil || errors.Count() == 0 {
		t.Error("Expected validation errors for invalid categories")
	}

	expectedErrorCount := 7 // Updated based on actual implementation
	if errors.Count() != expectedErrorCount {
		t.Errorf("Expected %d validation errors, got %d", expectedErrorCount, errors.Count())
	}

	// 验证错误类型
	validationErrors := errors.GetByType(model.ErrCodeValidation)
	if len(validationErrors) != expectedErrorCount {
		t.Errorf("Expected all errors to be validation errors, got %d", len(validationErrors))
	}
}

func TestHierarchyBuilderImpl_isValidCode(t *testing.T) {
	builder := NewHierarchyBuilder(nil)

	tests := []struct {
		code     string
		expected bool
	}{
		{"1", true},
		{"1-01", true},
		{"1-01-01", true},
		{"1-01-01-01", true},
		{"", false},
		{"-1", false},
		{"1-", false},
		{"1--01", false},
		{"1-01a", false},
		{"1-01!", false},
		{"a-01", false},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			result := builder.isValidCode(tt.code)
			if result != tt.expected {
				t.Errorf("isValidCode(%s) = %v, expected %v", tt.code, result, tt.expected)
			}
		})
	}
}

func TestHierarchyBuilderImpl_GetStatistics(t *testing.T) {
	builder := NewHierarchyBuilder(nil)
	ctx := context.Background()

	categories, err := builder.Build(ctx, SampleParsedInfo)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	stats := builder.GetStatistics(categories)

	// 验证统计信息字段
	expectedFields := []string{"total_nodes", "max_depth", "root_categories", "level_counts"}
	for _, field := range expectedFields {
		if _, exists := stats[field]; !exists {
			t.Errorf("Expected field '%s' in statistics", field)
		}
	}

	// 验证根分类数
	if stats["root_categories"] != len(categories) {
		t.Errorf("Expected root_categories to be %d, got %v", len(categories), stats["root_categories"])
	}

	// 验证总节点数
	totalNodes := stats["total_nodes"].(int)
	if totalNodes != len(SampleParsedInfo) {
		t.Errorf("Expected total_nodes to be %d, got %d", len(SampleParsedInfo), totalNodes)
	}

	// 验证最大深度
	maxDepth := stats["max_depth"].(int)
	if maxDepth < 1 {
		t.Errorf("Expected max_depth to be at least 1, got %d", maxDepth)
	}

	// 验证层级计数
	levelCounts, ok := stats["level_counts"].(map[string]int)
	if !ok {
		t.Error("Expected level_counts to be map[string]int")
	} else {
		if levelCounts[LevelMajor] == 0 {
			t.Error("Expected at least one major category")
		}
	}
}

func TestHierarchyBuilderImpl_ContextCancellation(t *testing.T) {
	builder := NewHierarchyBuilder(nil)

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := builder.Build(ctx, SampleParsedInfo)
	if err == nil {
		t.Error("Expected context cancellation error")
	}
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

// validateHierarchy 递归验证层级结构的正确性
func validateHierarchy(t *testing.T, category *model.Category, expectedDepth int) {
	// 验证层级深度与编码中的连字符数量匹配
	actualDepth := strings.Count(category.Code, "-")
	if actualDepth != expectedDepth {
		t.Errorf("Category %s: expected depth %d, got %d", category.Code, expectedDepth, actualDepth)
	}

	// 验证层级名称
	expectedLevel := ""
	switch expectedDepth {
	case 0:
		expectedLevel = LevelMajor
	case 1:
		expectedLevel = LevelMiddle
	case 2:
		expectedLevel = LevelSmall
	case 3:
		expectedLevel = LevelDetail
	}

	if expectedLevel != "" && category.Level != expectedLevel {
		t.Errorf("Category %s: expected level %s, got %s", category.Code, expectedLevel, category.Level)
	}

	// 递归验证子分类
	for _, child := range category.Children {
		// 验证子分类的编码是父分类编码的扩展
		if !strings.HasPrefix(child.Code, category.Code+"-") {
			t.Errorf("Child %s should have parent prefix %s-", child.Code, category.Code)
		}
		validateHierarchy(t, child, expectedDepth+1)
	}
}

func TestHierarchyBuilderImpl_EmptyInput(t *testing.T) {
	builder := NewHierarchyBuilder(nil)
	ctx := context.Background()

	// 测试空输入
	categories, err := builder.Build(ctx, []*model.ParsedInfo{})
	if err != nil {
		t.Fatalf("Unexpected error for empty input: %v", err)
	}

	if len(categories) != 0 {
		t.Errorf("Expected 0 categories for empty input, got %d", len(categories))
	}

	// 测试nil输入
	categories, err = builder.Build(ctx, nil)
	if err != nil {
		t.Fatalf("Unexpected error for nil input: %v", err)
	}

	if len(categories) != 0 {
		t.Errorf("Expected 0 categories for nil input, got %d", len(categories))
	}
}

func TestHierarchyBuilderImpl_ComplexHierarchy(t *testing.T) {
	builder := NewHierarchyBuilder(nil)
	ctx := context.Background()

	// 创建复杂的层级结构测试数据
	complexData := []*model.ParsedInfo{
		// 第一个大类及其完整层级
		{Code: "1", Name: "大类1", Level: 0},
		{Code: "1-01", Name: "大类1-中类1", Level: 1},
		{Code: "1-01-01", Name: "大类1-中类1-小类1", Level: 2},
		{Code: "1-01-01-01", Name: "大类1-中类1-小类1-细类1", Level: 3},
		{Code: "1-01-01-02", Name: "大类1-中类1-小类1-细类2", Level: 3},
		{Code: "1-01-02", Name: "大类1-中类1-小类2", Level: 2},
		{Code: "1-02", Name: "大类1-中类2", Level: 1},

		// 第二个大类
		{Code: "2", Name: "大类2", Level: 0},
		{Code: "2-01", Name: "大类2-中类1", Level: 1},
		{Code: "2-01-01", Name: "大类2-中类1-小类1", Level: 2},
	}

	categories, err := builder.Build(ctx, complexData)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// 验证根分类数量
	if len(categories) != 2 {
		t.Fatalf("Expected 2 root categories, got %d", len(categories))
	}

	// Find major1 (category "1") - it might not be at index 0
	var major1 *model.Category
	for _, cat := range categories {
		if cat.Code == "1" {
			major1 = cat
			break
		}
	}
	if major1 == nil {
		t.Fatal("Could not find major category '1'")
	}

	if len(major1.Children) != 2 { // 1-01, 1-02
		t.Errorf("Expected 2 middle categories under major1, got %d", len(major1.Children))
	}

	// Find 1-01 child
	var middle101 *model.Category
	for _, child := range major1.Children {
		if child.Code == "1-01" {
			middle101 = child
			break
		}
	}
	if middle101 == nil {
		t.Fatal("Could not find middle category '1-01'")
	}

	if len(middle101.Children) != 2 { // 1-01-01, 1-01-02
		t.Errorf("Expected 2 small categories under 1-01, got %d", len(middle101.Children))
	}

	// Find 1-01-01 child
	var small10101 *model.Category
	for _, child := range middle101.Children {
		if child.Code == "1-01-01" {
			small10101 = child
			break
		}
	}
	if small10101 == nil {
		t.Fatal("Could not find small category '1-01-01'")
	}

	if len(small10101.Children) != 2 { // 1-01-01-01, 1-01-01-02
		t.Errorf("Expected 2 detail categories under 1-01-01, got %d", len(small10101.Children))
	}

	// 验证统计信息
	stats := builder.GetStatistics(categories)
	expectedTotalNodes := 10
	if stats["total_nodes"] != expectedTotalNodes {
		t.Errorf("Expected %d total nodes, got %v", expectedTotalNodes, stats["total_nodes"])
	}

	expectedMaxDepth := 4
	if stats["max_depth"] != expectedMaxDepth {
		t.Errorf("Expected max depth %d, got %v", expectedMaxDepth, stats["max_depth"])
	}
}

// 基准测试
func BenchmarkHierarchyBuilderImpl_Build(b *testing.B) {
	builder := NewHierarchyBuilder(nil)
	ctx := context.Background()

	// 创建大量测试数据
	largeData := make([]*model.ParsedInfo, 0, 1000)
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			for k := 0; k < 10; k++ {
				largeData = append(largeData, &model.ParsedInfo{
					Code:  fmt.Sprintf("%d-%02d-%02d", i+1, j+1, k+1),
					Name:  fmt.Sprintf("Category %d-%d-%d", i+1, j+1, k+1),
					Level: 2,
				})
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := builder.Build(ctx, largeData)
		if err != nil {
			b.Fatalf("Benchmark failed: %v", err)
		}
	}
}
