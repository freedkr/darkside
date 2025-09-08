// Package builder 实现层级结构构建功能
package builder

import (
	"context"
	"log"
	"sort"
	"strings"

	"github.com/freedkr/moonshot/internal/model"
)

// HierarchyBuilderImpl 层级构建器实现
type HierarchyBuilderImpl struct {
	config *BuilderConfig
}

// BuilderConfig 构建器配置
type BuilderConfig struct {
	EnableOrphanHandling bool `yaml:"enable_orphan_handling" json:"enable_orphan_handling"`
	StrictMode           bool `yaml:"strict_mode" json:"strict_mode"`
}

// 层级级别常量
const (
	LevelMajor  = "大类"
	LevelMiddle = "中类"
	LevelSmall  = "小类"
	LevelDetail = "细类"
)

// NewHierarchyBuilder 创建新的层级构建器
func NewHierarchyBuilder(config *BuilderConfig) *HierarchyBuilderImpl {
	if config == nil {
		config = &BuilderConfig{
			EnableOrphanHandling: true,
			StrictMode:           false,
		}
	}

	return &HierarchyBuilderImpl{
		config: config,
	}
}

// Build 构建层级结构
func (b *HierarchyBuilderImpl) Build(ctx context.Context, records []*model.ParsedInfo) ([]*model.Category, error) {
	nodeMap := make(map[string]*model.Category)
	var rootCategories []*model.Category

	// 第一步：创建所有节点
	for _, record := range records {
		if _, exists := nodeMap[record.Code]; !exists {
			level := b.determineLevel(record.Code)

			category := &model.Category{
				Code:    record.Code,
				GbmCode: record.GbmCode,
				Name:    record.Name,
				Level:   level,
			}

			nodeMap[record.Code] = category
		}

		// 检查上下文取消
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	// 第二步：建立父子关系（严格遵循原始数据）
	for _, node := range nodeMap {
		parentCode, hasParent := b.getParentCode(node.Code)
		if hasParent {
			if parent, ok := nodeMap[parentCode]; ok {
				parent.Children = append(parent.Children, node)
			} else {
				// 父节点不存在，根据配置处理孤儿节点
				if b.config.EnableOrphanHandling {
					rootCategories = append(rootCategories, node)
					log.Printf("⚠️ 警告：发现孤儿节点，编码 '%s' 的父节点 '%s' 不存在，已将其作为根节点处理", node.Code, parentCode)
				} else if b.config.StrictMode {
					return nil, model.NewHierarchyError(
						node.Code, parentCode, "missing_parent", "发现孤儿节点", 1)
				}
			}
		} else {
			rootCategories = append(rootCategories, node)
		}

		// 检查上下文取消
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	// 对根节点进行排序（按编码升序）
	sort.Slice(rootCategories, func(i, j int) bool {
		return rootCategories[i].Code < rootCategories[j].Code
	})

	// 递归对所有子节点进行排序
	for _, root := range rootCategories {
		b.sortChildren(root)
	}

	return rootCategories, nil
}

// determineLevel 确定节点级别
func (b *HierarchyBuilderImpl) determineLevel(code string) string {
	level := strings.Count(code, "-")
	switch level {
	case 0:
		return LevelMajor
	case 1:
		return LevelMiddle
	case 2:
		return LevelSmall
	case 3:
		return LevelDetail
	default:
		return "未知级别"
	}
}

// getParentCode 获取父节点编码
func (b *HierarchyBuilderImpl) getParentCode(code string) (string, bool) {
	lastDash := strings.LastIndex(code, "-")
	if lastDash == -1 {
		return "", false
	}
	return code[:lastDash], true
}

// Validate 验证层级结构
func (b *HierarchyBuilderImpl) Validate(categories []*model.Category) *model.ErrorList {
	errors := &model.ErrorList{}

	for _, category := range categories {
		b.validateCategory(category, errors)
	}

	if len(errors.Errors) == 0 {
		return nil
	}
	return errors
}

// validateCategory 验证单个分类
func (b *HierarchyBuilderImpl) validateCategory(category *model.Category, errors *model.ErrorList) {
	// 验证必填字段
	if category.Code == "" {
		errors.Add(model.NewValidationError("code", "", "required", "分类编码不能为空"))
	}

	if category.Name == "" {
		errors.Add(model.NewValidationError("name", "", "required", "分类名称不能为空"))
	}

	if category.Level == "" {
		errors.Add(model.NewValidationError("level", "", "required", "分类级别不能为空"))
	}

	// 验证级别是否有效
	validLevels := []string{LevelMajor, LevelMiddle, LevelSmall, LevelDetail}
	isValidLevel := false
	for _, validLevel := range validLevels {
		if category.Level == validLevel {
			isValidLevel = true
			break
		}
	}
	if !isValidLevel {
		errors.Add(model.NewValidationError("level", category.Level, "oneof=大类 中类 小类 细类", "无效的分类级别"))
	}

	// 验证编码格式
	if !b.isValidCode(category.Code) {
		errors.Add(model.NewValidationError("code", category.Code, "code_format", "无效的分类编码格式"))
	}

	// 递归验证子分类
	for _, child := range category.Children {
		b.validateCategory(child, errors)
	}
}

// isValidCode 验证编码格式
func (b *HierarchyBuilderImpl) isValidCode(code string) bool {
	if code == "" {
		return false
	}

	// 简单的格式验证：应该只包含数字和连字符
	for _, char := range code {
		if !((char >= '0' && char <= '9') || char == '-') {
			return false
		}
	}

	// 不能以连字符开始或结束
	if strings.HasPrefix(code, "-") || strings.HasSuffix(code, "-") {
		return false
	}

	// 不能有连续的连字符
	if strings.Contains(code, "--") {
		return false
	}

	return true
}

// GetStatistics 获取构建统计信息
func (b *HierarchyBuilderImpl) GetStatistics(categories []*model.Category) map[string]interface{} {
	stats := make(map[string]interface{})

	// 统计各级别数量
	levelCounts := make(map[string]int)
	totalNodes := 0
	maxDepth := 0

	for _, category := range categories {
		b.collectStatistics(category, levelCounts, &totalNodes, 1, &maxDepth)
	}

	stats["total_nodes"] = totalNodes
	stats["max_depth"] = maxDepth
	stats["root_categories"] = len(categories)
	stats["level_counts"] = levelCounts

	return stats
}

// collectStatistics 收集统计信息
func (b *HierarchyBuilderImpl) collectStatistics(category *model.Category, levelCounts map[string]int, totalNodes *int, currentDepth int, maxDepth *int) {
	*totalNodes++
	levelCounts[category.Level]++

	if currentDepth > *maxDepth {
		*maxDepth = currentDepth
	}

	for _, child := range category.Children {
		b.collectStatistics(child, levelCounts, totalNodes, currentDepth+1, maxDepth)
	}
}

// BuildWithOptions 使用选项构建层级结构
func (b *HierarchyBuilderImpl) BuildWithOptions(ctx context.Context, records []*model.ParsedInfo, options *BuildOptions) ([]*model.Category, error) {
	// 简单实现，暂时忽略options
	return b.Build(ctx, records)
}

// GetName 获取构建器名称
func (b *HierarchyBuilderImpl) GetName() string {
	return "HierarchyBuilder"
}

// GetVersion 获取构建器版本
func (b *HierarchyBuilderImpl) GetVersion() string {
	return "1.0.0"
}

// sortChildren 递归排序子节点
func (b *HierarchyBuilderImpl) sortChildren(category *model.Category) {
	if len(category.Children) == 0 {
		return
	}

	// 对直接子节点按编码排序
	sort.Slice(category.Children, func(i, j int) bool {
		return category.Children[i].Code < category.Children[j].Code
	})

	// 递归排序每个子节点的子节点
	for _, child := range category.Children {
		b.sortChildren(child)
	}
}
