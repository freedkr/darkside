// Package model 定义核心数据模型
package model

import (
	"time"
)

// Level 层级常量
const (
	LevelMajor  = "大类"
	LevelMiddle = "中类"
	LevelSmall  = "小类"
	LevelDetail = "细类"
)

// Category 职业分类结构体
// 表示职业分类体系中的一个节点，支持层级结构
type Category struct {
	// Code 职业编码，如 "1-01-00-01"
	Code string `json:"code" yaml:"code" validate:"required"`

	// GbmCode GBM编码
	GbmCode string `json:"gbm_code,omitempty" yaml:"gbm_code,omitempty"`

	// Name 职业名称
	Name string `json:"name" yaml:"name" validate:"required"`

	// Level 层级名称：大类/中类/小类/细类
	Level string `json:"level" yaml:"level" validate:"required,oneof=大类 中类 小类 细类"`

	// Children 子分类列表
	Children []*Category `json:"children,omitempty" yaml:"children,omitempty"`

	// Metadata 元数据信息
	// Metadata *CategoryMetadata `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// CategoryMetadata 分类元数据
type CategoryMetadata struct {
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`

	// Source 数据源信息
	Source string `json:"source,omitempty" yaml:"source,omitempty"`

	// Version 数据版本
	Version string `json:"version,omitempty" yaml:"version,omitempty"`

	// Tags 标签列表
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`
}

// ParsedInfo 解析后的信息记录
// 表示从Excel中解析出的原始数据记录
type ParsedInfo struct {
	// Code 职业编码
	Code string `json:"code" validate:"required"`

	// GbmCode GBM编码
	GbmCode string `json:"gbm_code,omitempty"`

	// Name 职业名称
	Name string `json:"name" validate:"required"`

	// Level 计算得出的层级
	Level int `json:"level"`

	// RowIndex 原始数据行号（用于错误定位）
	RowIndex int `json:"row_index"`

	// ColumnIndex 原始数据列号（用于错误定位）
	ColumnIndex int `json:"column_index"`

	// RawContent 原始单元格内容（用于调试）
	RawContent string `json:"raw_content,omitempty"`
}

// GetLevelName 根据层级数字返回层级名称
func (p *ParsedInfo) GetLevelName() string {
	switch p.Level {
	case 0:
		return LevelMajor
	case 1:
		return LevelMiddle
	case 2:
		return LevelSmall
	case 3:
		return LevelDetail
	default:
		return "未知层级"
	}
}

// IsValid 检查ParsedInfo是否有效
func (p *ParsedInfo) IsValid() bool {
	return p.Code != "" && p.Name != "" && p.Level >= 0 && p.Level <= 3
}

// GetParentCode 获取父级编码
// 例如："1-01-00-01" -> "1-01-00"
func (c *Category) GetParentCode() string {
	if c.Code == "" {
		return ""
	}

	// 找到最后一个短横线的位置
	lastDash := -1
	for i := len(c.Code) - 1; i >= 0; i-- {
		if c.Code[i] == '-' {
			lastDash = i
			break
		}
	}

	if lastDash == -1 {
		return "" // 没有父级
	}

	return c.Code[:lastDash]
}

// GetLevel 根据编码计算层级
func (c *Category) GetLevel() int {
	if c.Code == "" {
		return -1
	}

	count := 0
	for _, ch := range c.Code {
		if ch == '-' {
			count++
		}
	}
	return count
}

// AddChild 添加子分类
func (c *Category) AddChild(child *Category) {
	if c.Children == nil {
		c.Children = make([]*Category, 0)
	}
	c.Children = append(c.Children, child)
}

// GetChildrenCount 获取子分类数量
func (c *Category) GetChildrenCount() int {
	if c.Children == nil {
		return 0
	}
	return len(c.Children)
}

// GetTotalDescendantsCount 获取所有后代数量（递归）
func (c *Category) GetTotalDescendantsCount() int {
	if c.Children == nil {
		return 0
	}

	total := len(c.Children)
	for _, child := range c.Children {
		total += child.GetTotalDescendantsCount()
	}
	return total
}

// FindChild 根据编码查找直接子分类
func (c *Category) FindChild(code string) *Category {
	if c.Children == nil {
		return nil
	}

	for _, child := range c.Children {
		if child.Code == code {
			return child
		}
	}
	return nil
}

// FindDescendant 根据编码查找后代分类（递归）
func (c *Category) FindDescendant(code string) *Category {
	if c.Code == code {
		return c
	}

	if c.Children == nil {
		return nil
	}

	for _, child := range c.Children {
		if found := child.FindDescendant(code); found != nil {
			return found
		}
	}
	return nil
}

// ToFlat 将层级结构转换为扁平列表
func (c *Category) ToFlat() []*Category {
	var result []*Category
	result = append(result, c)

	if c.Children != nil {
		for _, child := range c.Children {
			result = append(result, child.ToFlat()...)
		}
	}

	return result
}

// AITask AI任务结构体
// 表示一个以"小类"为单位的AI处理任务
type AITask struct {
	// ParentCode 小类的编码 (如: "3-02-01")
	ParentCode string `json:"parent_code" validate:"required"`

	// ParentName 小类的名称 (如: "农作物生产人员")，为AI提供关键的语义上下文
	ParentName string `json:"parent_name" validate:"required"`

	// DetailCodesRaw E列所有相关单元格的原始文本数组
	DetailCodesRaw []string `json:"detail_codes_raw"`

	// DetailNamesRaw F列所有相关单元格的原始文本数组
	DetailNamesRaw []string `json:"detail_names_raw"`
}

// AITaskResult AI任务处理结果
type AITaskResult struct {
	// TaskID 任务ID
	TaskID string `json:"task_id"`

	// ParentCode 对应的小类编码
	ParentCode string `json:"parent_code"`

	// DetailCategories AI返回的细类列表
	DetailCategories []*Category `json:"detail_categories"`

	// Success 是否处理成功
	Success bool `json:"success"`

	// Error 错误信息（如果有）
	Error string `json:"error,omitempty"`

	// ProcessedAt 处理时间
	ProcessedAt time.Time `json:"processed_at"`
}

// SkeletonRecord 骨架记录
// 表示大类、中类、小类的骨架信息
type SkeletonRecord struct {
	// Code 编码
	Code string `json:"code" validate:"required"`
	// GBM GBM编码
	GBM int `json:"gbm"`
	// Name 名称
	Name string `json:"name" validate:"required"`
	// Level 层级：大类/中类/小类
	Level string `json:"level" validate:"required,oneof=大类 中类 小类"`
}

// HybridParseResult 混合解析结果
type HybridParseResult struct {
	// SkeletonRecords 骨架记录（大、中、小类）
	SkeletonRecords []*SkeletonRecord `json:"skeleton_records"`

	// AITasks AI任务列表（以小类为单位打包的细类信息）
	AITasks []*AITask `json:"ai_tasks"`

	// Stats 统计信息
	Stats *HybridParseStats `json:"stats"`
}

// HybridParseStats 混合解析统计
type HybridParseStats struct {
	// TotalRows 总行数
	TotalRows int `json:"total_rows"`

	// SkeletonCount 骨架记录数量
	SkeletonCount int `json:"skeleton_count"`

	// AITaskCount AI任务数量
	AITaskCount int `json:"ai_task_count"`

	// ProcessingTime 处理时间(毫秒)
	ProcessingTime int64 `json:"processing_time"`
}
