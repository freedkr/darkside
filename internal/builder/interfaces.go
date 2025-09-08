// Package builder 定义层级构建器相关接口
package builder

import (
	"context"

	"github.com/freedkr/moonshot/internal/model"
)

// HierarchyBuilder 层级构建器接口
// 负责将扁平的ParsedInfo记录构建成层级结构
type HierarchyBuilder interface {
	// Build 构建层级结构
	Build(ctx context.Context, records []*model.ParsedInfo) ([]*model.Category, error)
	
	// BuildWithOptions 使用选项构建层级结构
	BuildWithOptions(ctx context.Context, records []*model.ParsedInfo, options *BuildOptions) ([]*model.Category, error)
	
	// Validate 验证层级结构
	Validate(categories []*model.Category) *model.ErrorList
	
	// GetName 获取构建器名称
	GetName() string
	
	// GetVersion 获取构建器版本
	GetVersion() string
}

// TreeBuilder 树形构建器接口
// 专门用于构建树形结构
type TreeBuilder interface {
	HierarchyBuilder
	
	// BuildTree 构建树形结构
	BuildTree(ctx context.Context, records []*model.ParsedInfo) (*Tree, error)
	
	// MergeTree 合并树形结构
	MergeTree(tree1, tree2 *Tree) (*Tree, error)
	
	// PruneTree 修剪树形结构（移除空节点等）
	PruneTree(tree *Tree, options *PruneOptions) (*Tree, error)
}

// GraphBuilder 图形构建器接口
// 支持构建具有复杂关系的图形结构
type GraphBuilder interface {
	// BuildGraph 构建图形结构
	BuildGraph(ctx context.Context, records []*model.ParsedInfo) (*Graph, error)
	
	// AddEdge 添加边
	AddEdge(from, to string, weight float64, properties map[string]any) error
	
	// RemoveEdge 移除边
	RemoveEdge(from, to string) error
	
	// FindPath 查找路径
	FindPath(from, to string) ([]*Node, error)
	
	// GetConnectedComponents 获取连通分量
	GetConnectedComponents() ([][]*Node, error)
}

// BuildOptions 构建选项
type BuildOptions struct {
	// Strategy 构建策略
	Strategy BuildStrategy `json:"strategy"`
	
	// IgnoreOrphanNodes 忽略孤儿节点
	IgnoreOrphanNodes bool `json:"ignore_orphan_nodes"`
	
	// CreateMissingParents 创建缺失的父节点
	CreateMissingParents bool `json:"create_missing_parents"`
	
	// ValidateHierarchy 验证层级结构
	ValidateHierarchy bool `json:"validate_hierarchy"`
	
	// SortChildren 对子节点排序
	SortChildren bool `json:"sort_children"`
	
	// SortBy 排序字段
	SortBy SortField `json:"sort_by"`
	
	// SortOrder 排序顺序
	SortOrder SortOrder `json:"sort_order"`
	
	// MaxDepth 最大深度（0表示无限制）
	MaxDepth int `json:"max_depth"`
	
	// MaxChildren 每个节点最大子节点数（0表示无限制）
	MaxChildren int `json:"max_children"`
	
	// EnableMetadata 启用元数据
	EnableMetadata bool `json:"enable_metadata"`
	
	// MetadataFields 需要包含的元数据字段
	MetadataFields []string `json:"metadata_fields"`
	
	// CustomProperties 自定义属性
	CustomProperties map[string]any `json:"custom_properties"`
}

// BuildStrategy 构建策略
type BuildStrategy string

const (
	// StrategyCodeBased 基于编码的构建策略
	StrategyCodeBased BuildStrategy = "code_based"
	
	// StrategyNameBased 基于名称的构建策略  
	StrategyNameBased BuildStrategy = "name_based"
	
	// StrategyHybrid 混合策略
	StrategyHybrid BuildStrategy = "hybrid"
	
	// StrategyCustom 自定义策略
	StrategyCustom BuildStrategy = "custom"
)

// SortField 排序字段
type SortField string

const (
	// SortByCode 按编码排序
	SortByCode SortField = "code"
	
	// SortByName 按名称排序
	SortByName SortField = "name"
	
	// SortByLevel 按层级排序
	SortByLevel SortField = "level"
	
	// SortByCustom 自定义排序
	SortByCustom SortField = "custom"
)

// SortOrder 排序顺序
type SortOrder string

const (
	// OrderAsc 升序
	OrderAsc SortOrder = "asc"
	
	// OrderDesc 降序
	OrderDesc SortOrder = "desc"
)

// Tree 树形结构
type Tree struct {
	// Root 根节点
	Root *Node `json:"root"`
	
	// Nodes 所有节点的映射
	Nodes map[string]*Node `json:"nodes"`
	
	// Stats 统计信息
	Stats *TreeStats `json:"stats"`
	
	// Metadata 元数据
	Metadata map[string]any `json:"metadata"`
}

// Node 树节点
type Node struct {
	// ID 节点唯一标识
	ID string `json:"id"`
	
	// Category 分类信息
	Category *model.Category `json:"category"`
	
	// Parent 父节点
	Parent *Node `json:"parent,omitempty"`
	
	// Children 子节点列表
	Children []*Node `json:"children,omitempty"`
	
	// Level 层级（0-based）
	Level int `json:"level"`
	
	// Index 在父节点中的索引
	Index int `json:"index"`
	
	// Properties 节点属性
	Properties map[string]any `json:"properties,omitempty"`
}

// TreeStats 树统计信息
type TreeStats struct {
	// TotalNodes 总节点数
	TotalNodes int `json:"total_nodes"`
	
	// LeafNodes 叶子节点数
	LeafNodes int `json:"leaf_nodes"`
	
	// MaxDepth 最大深度
	MaxDepth int `json:"max_depth"`
	
	// AvgDepth 平均深度
	AvgDepth float64 `json:"avg_depth"`
	
	// MaxChildren 最大子节点数
	MaxChildren int `json:"max_children"`
	
	// AvgChildren 平均子节点数
	AvgChildren float64 `json:"avg_children"`
	
	// OrphanNodes 孤儿节点数
	OrphanNodes int `json:"orphan_nodes"`
}

// Graph 图形结构
type Graph struct {
	// Nodes 节点映射
	Nodes map[string]*Node `json:"nodes"`
	
	// Edges 边列表
	Edges []*Edge `json:"edges"`
	
	// AdjacencyList 邻接表
	AdjacencyList map[string][]*Edge `json:"adjacency_list"`
	
	// Stats 统计信息
	Stats *GraphStats `json:"stats"`
	
	// Properties 图属性
	Properties map[string]any `json:"properties"`
}

// Edge 图边
type Edge struct {
	// From 起始节点ID
	From string `json:"from"`
	
	// To 目标节点ID
	To string `json:"to"`
	
	// Weight 权重
	Weight float64 `json:"weight"`
	
	// Type 边类型
	Type EdgeType `json:"type"`
	
	// Properties 边属性
	Properties map[string]any `json:"properties"`
}

// EdgeType 边类型
type EdgeType string

const (
	// EdgeParentChild 父子关系
	EdgeParentChild EdgeType = "parent_child"
	
	// EdgeSibling 兄弟关系
	EdgeSibling EdgeType = "sibling"
	
	// EdgeReference 引用关系
	EdgeReference EdgeType = "reference"
	
	// EdgeCustom 自定义关系
	EdgeCustom EdgeType = "custom"
)

// GraphStats 图统计信息
type GraphStats struct {
	// NodeCount 节点数
	NodeCount int `json:"node_count"`
	
	// EdgeCount 边数
	EdgeCount int `json:"edge_count"`
	
	// Density 密度
	Density float64 `json:"density"`
	
	// ConnectedComponents 连通分量数
	ConnectedComponents int `json:"connected_components"`
	
	// MaxDegree 最大度数
	MaxDegree int `json:"max_degree"`
	
	// AvgDegree 平均度数
	AvgDegree float64 `json:"avg_degree"`
}

// PruneOptions 修剪选项
type PruneOptions struct {
	// RemoveEmptyNodes 移除空节点
	RemoveEmptyNodes bool `json:"remove_empty_nodes"`
	
	// RemoveOrphanNodes 移除孤儿节点
	RemoveOrphanNodes bool `json:"remove_orphan_nodes"`
	
	// MinChildrenCount 最小子节点数（少于此数的节点将被移除）
	MinChildrenCount int `json:"min_children_count"`
	
	// MaxDepth 最大深度（超过此深度的节点将被移除）
	MaxDepth int `json:"max_depth"`
	
	// KeepLevels 保留的层级列表
	KeepLevels []int `json:"keep_levels"`
	
	// FilterFunc 自定义过滤函数
	FilterFunc func(*Node) bool `json:"-"`
}

// BuildResult 构建结果
type BuildResult struct {
	// Categories 构建的分类列表
	Categories []*model.Category `json:"categories"`
	
	// Tree 树形结构（如果构建器支持）
	Tree *Tree `json:"tree,omitempty"`
	
	// Graph 图形结构（如果构建器支持）
	Graph *Graph `json:"graph,omitempty"`
	
	// Errors 构建过程中的错误
	Errors *model.ErrorList `json:"errors"`
	
	// Warnings 警告信息
	Warnings []*BuildWarning `json:"warnings"`
	
	// Stats 构建统计
	Stats *BuildStats `json:"stats"`
	
	// Metadata 元数据
	Metadata map[string]any `json:"metadata"`
}

// BuildWarning 构建警告
type BuildWarning struct {
	Code    string `json:"code"`    // 警告代码
	Message string `json:"message"` // 警告消息
	NodeID  string `json:"node_id"` // 相关节点ID
	Level   string `json:"level"`   // 警告级别
}

// BuildStats 构建统计
type BuildStats struct {
	InputRecords    int   `json:"input_records"`    // 输入记录数
	OutputNodes     int   `json:"output_nodes"`     // 输出节点数
	OrphanNodes     int   `json:"orphan_nodes"`     // 孤儿节点数
	MissingParents  int   `json:"missing_parents"`  // 缺失父节点数
	DuplicateNodes  int   `json:"duplicate_nodes"`  // 重复节点数
	ProcessingTime  int64 `json:"processing_time"`  // 处理时间(毫秒)
	MemoryUsage     int64 `json:"memory_usage"`     // 内存使用(字节)
}

// Traverser 遍历器接口
type Traverser interface {
	// TraverseBFS 广度优先遍历
	TraverseBFS(tree *Tree, callback func(*Node) error) error
	
	// TraverseDFS 深度优先遍历
	TraverseDFS(tree *Tree, callback func(*Node) error) error
	
	// TraverseLevel 按层级遍历
	TraverseLevel(tree *Tree, level int, callback func(*Node) error) error
	
	// Find 查找节点
	Find(tree *Tree, predicate func(*Node) bool) []*Node
	
	// FindFirst 查找第一个匹配的节点
	FindFirst(tree *Tree, predicate func(*Node) bool) *Node
}

// Transformer 转换器接口
type Transformer interface {
	// Transform 转换节点
	Transform(node *Node, options *TransformOptions) (*Node, error)
	
	// TransformTree 转换整棵树
	TransformTree(tree *Tree, options *TransformOptions) (*Tree, error)
	
	// Merge 合并节点
	Merge(nodes []*Node, options *MergeOptions) (*Node, error)
	
	// Split 拆分节点
	Split(node *Node, options *SplitOptions) ([]*Node, error)
}

// TransformOptions 转换选项
type TransformOptions struct {
	// Rules 转换规则
	Rules []TransformRule `json:"rules"`
	
	// SkipIfExists 如果目标已存在则跳过
	SkipIfExists bool `json:"skip_if_exists"`
	
	// UpdateMetadata 更新元数据
	UpdateMetadata bool `json:"update_metadata"`
	
	// PreserveOriginal 保留原始数据
	PreserveOriginal bool `json:"preserve_original"`
}

// TransformRule 转换规则
type TransformRule interface {
	// Name 规则名称
	Name() string
	
	// Apply 应用规则
	Apply(node *Node) (*Node, error)
	
	// Validate 验证规则
	Validate() error
}

// MergeOptions 合并选项
type MergeOptions struct {
	// Strategy 合并策略
	Strategy MergeStrategy `json:"strategy"`
	
	// ConflictResolution 冲突解决方式
	ConflictResolution ConflictResolution `json:"conflict_resolution"`
	
	// MergeChildren 合并子节点
	MergeChildren bool `json:"merge_children"`
	
	// MergeMetadata 合并元数据
	MergeMetadata bool `json:"merge_metadata"`
}

// MergeStrategy 合并策略
type MergeStrategy string

const (
	// MergeStrategyUnion 并集合并
	MergeStrategyUnion MergeStrategy = "union"
	
	// MergeStrategyIntersection 交集合并
	MergeStrategyIntersection MergeStrategy = "intersection"
	
	// MergeStrategyFirst 使用第一个
	MergeStrategyFirst MergeStrategy = "first"
	
	// MergeStrategyLast 使用最后一个
	MergeStrategyLast MergeStrategy = "last"
)

// ConflictResolution 冲突解决方式
type ConflictResolution string

const (
	// ConflictResolveOverwrite 覆盖
	ConflictResolveOverwrite ConflictResolution = "overwrite"
	
	// ConflictResolveSkip 跳过
	ConflictResolveSkip ConflictResolution = "skip"
	
	// ConflictResolveError 报错
	ConflictResolveError ConflictResolution = "error"
	
	// ConflictResolveCallback 回调处理
	ConflictResolveCallback ConflictResolution = "callback"
)

// SplitOptions 拆分选项
type SplitOptions struct {
	// SplitBy 拆分依据
	SplitBy SplitCriteria `json:"split_by"`
	
	// MaxParts 最大拆分部分数
	MaxParts int `json:"max_parts"`
	
	// PreserveHierarchy 保留层级关系
	PreserveHierarchy bool `json:"preserve_hierarchy"`
	
	// SplitChildren 拆分子节点
	SplitChildren bool `json:"split_children"`
}

// SplitCriteria 拆分标准
type SplitCriteria string

const (
	// SplitBySize 按大小拆分
	SplitBySize SplitCriteria = "size"
	
	// SplitByLevel 按层级拆分
	SplitByLevel SplitCriteria = "level"
	
	// SplitByProperty 按属性拆分
	SplitByProperty SplitCriteria = "property"
	
	// SplitByCustom 自定义拆分
	SplitByCustom SplitCriteria = "custom"
)

// BuilderFactory 构建器工厂接口
type BuilderFactory interface {
	// CreateBuilder 创建构建器
	CreateBuilder(builderType string, options *BuildOptions) (HierarchyBuilder, error)
	
	// RegisterBuilder 注册构建器
	RegisterBuilder(builderType string, creator BuilderCreator) error
	
	// GetSupportedTypes 获取支持的构建器类型
	GetSupportedTypes() []string
	
	// GetBuilderInfo 获取构建器信息
	GetBuilderInfo(builderType string) (*BuilderInfo, error)
}

// BuilderCreator 构建器创建函数
type BuilderCreator func(options *BuildOptions) (HierarchyBuilder, error)

// BuilderInfo 构建器信息
type BuilderInfo struct {
	Type        string   `json:"type"`        // 构建器类型
	Name        string   `json:"name"`        // 构建器名称
	Version     string   `json:"version"`     // 版本
	Description string   `json:"description"` // 描述
	Features    []string `json:"features"`    // 支持的特性
	Strategies  []string `json:"strategies"`  // 支持的策略
}