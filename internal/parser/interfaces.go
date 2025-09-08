// Package parser 定义解析器相关接口
package parser

import (
	"context"
	"io"

	"github.com/freedkr/moonshot/internal/model"
)

// Parser 通用解析器接口
// 所有解析器都必须实现此接口
type Parser interface {
	// Parse 从输入流解析数据
	Parse(ctx context.Context, input io.Reader) ([]*model.ParsedInfo, error)
	
	// Validate 验证解析器配置
	Validate() error
	
	// GetName 获取解析器名称
	GetName() string
	
	// GetVersion 获取解析器版本
	GetVersion() string
	
	// GetSupportedFormats 获取支持的文件格式
	GetSupportedFormats() []string
}

// ExcelParser Excel专用解析器接口
// 继承Parser接口，添加Excel特定功能
type ExcelParser interface {
	Parser
	
	// ParseFile 解析Excel文件
	ParseFile(ctx context.Context, filepath string) ([]*model.ParsedInfo, error)
	
	// ParseSheet 解析指定工作表
	ParseSheet(ctx context.Context, filepath, sheetName string) ([]*model.ParsedInfo, error)
	
	// GetSheetNames 获取所有工作表名称
	GetSheetNames(filepath string) ([]string, error)
	
	// GetSheetInfo 获取工作表信息
	GetSheetInfo(filepath, sheetName string) (*SheetInfo, error)
}

// CellParser 单元格解析器接口
// 负责解析单个单元格内容
type CellParser interface {
	// ParseCell 解析单个单元格
	ParseCell(content string, row, col int) (*model.ParsedInfo, error)
	
	// ParseCells 批量解析单元格
	ParseCells(contents []string, startRow, startCol int) ([]*model.ParsedInfo, error)
	
	// SetPattern 设置解析模式
	SetPattern(pattern string) error
	
	// GetPattern 获取当前解析模式
	GetPattern() string
}

// Validator 数据验证器接口
type Validator interface {
	// ValidateRecord 验证单个记录
	ValidateRecord(record *model.ParsedInfo) error
	
	// ValidateRecords 批量验证记录
	ValidateRecords(records []*model.ParsedInfo) *model.ErrorList
	
	// ValidateHierarchy 验证层级结构
	ValidateHierarchy(categories []*model.Category) *model.ErrorList
	
	// SetRules 设置验证规则
	SetRules(rules []ValidationRule) error
	
	// GetRules 获取验证规则
	GetRules() []ValidationRule
}

// ValidationRule 验证规则接口
type ValidationRule interface {
	// Name 规则名称
	Name() string
	
	// Validate 执行验证
	Validate(record *model.ParsedInfo) error
	
	// Description 规则描述
	Description() string
	
	// IsEnabled 是否启用
	IsEnabled() bool
	
	// SetEnabled 设置启用状态
	SetEnabled(enabled bool)
}

// SheetInfo 工作表信息
type SheetInfo struct {
	Name       string `json:"name"`       // 工作表名称
	RowCount   int    `json:"row_count"`  // 行数
	ColCount   int    `json:"col_count"`  // 列数
	HasHeader  bool   `json:"has_header"` // 是否有标题行
	DataRange  string `json:"data_range"` // 数据范围
	LastCell   string `json:"last_cell"`  // 最后一个单元格位置
}

// ParseOptions 解析选项
type ParseOptions struct {
	// SkipRows 跳过的行号列表
	SkipRows []int `json:"skip_rows"`
	
	// SkipCols 跳过的列号列表
	SkipCols []int `json:"skip_cols"`
	
	// HeaderRow 标题行号（0-based）
	HeaderRow int `json:"header_row"`
	
	// DataStartRow 数据开始行号（0-based）
	DataStartRow int `json:"data_start_row"`
	
	// MaxRows 最大处理行数（0表示无限制）
	MaxRows int `json:"max_rows"`
	
	// MaxCols 最大处理列数（0表示无限制）
	MaxCols int `json:"max_cols"`
	
	// StrictMode 严格模式，遇到错误立即停止
	StrictMode bool `json:"strict_mode"`
	
	// ContinueOnError 遇到错误继续处理
	ContinueOnError bool `json:"continue_on_error"`
	
	// Encoding 文件编码
	Encoding string `json:"encoding"`
	
	// TrimSpace 是否去除空格
	TrimSpace bool `json:"trim_space"`
	
	// ReplaceNbsp 是否替换非断空格
	ReplaceNbsp bool `json:"replace_nbsp"`
}

// ParseResult 解析结果
type ParseResult struct {
	// Records 解析成功的记录
	Records []*model.ParsedInfo `json:"records"`
	
	// Errors 解析过程中的错误
	Errors *model.ErrorList `json:"errors"`
	
	// Warnings 警告信息
	Warnings []*ParseWarning `json:"warnings"`
	
	// Stats 统计信息
	Stats *ParseStats `json:"stats"`
	
	// Metadata 元数据
	Metadata map[string]interface{} `json:"metadata"`
}

// ParseWarning 解析警告
type ParseWarning struct {
	Row      int    `json:"row"`      // 行号
	Col      int    `json:"col"`      // 列号
	Message  string `json:"message"`  // 警告消息
	Code     string `json:"code"`     // 警告代码
	Severity string `json:"severity"` // 严重程度: info, warning, error
}

// ParseStats 解析统计
type ParseStats struct {
	TotalRows      int   `json:"total_rows"`      // 总行数
	ProcessedRows  int   `json:"processed_rows"`  // 处理的行数
	SkippedRows    int   `json:"skipped_rows"`    // 跳过的行数
	SuccessRecords int   `json:"success_records"` // 成功解析的记录数
	ErrorRecords   int   `json:"error_records"`   // 错误记录数
	WarningCount   int   `json:"warning_count"`   // 警告数量
	ProcessingTime int64 `json:"processing_time"` // 处理时间(毫秒)
	MemoryUsage    int64 `json:"memory_usage"`    // 内存使用(字节)
}

// ProgressCallback 进度回调函数
type ProgressCallback func(current, total int, message string)

// ParserFactory 解析器工厂接口
type ParserFactory interface {
	// CreateParser 创建解析器
	CreateParser(parserType string, options *ParseOptions) (Parser, error)
	
	// RegisterParser 注册解析器
	RegisterParser(parserType string, creator ParserCreator) error
	
	// GetSupportedTypes 获取支持的解析器类型
	GetSupportedTypes() []string
	
	// GetParserInfo 获取解析器信息
	GetParserInfo(parserType string) (*ParserInfo, error)
}

// ParserCreator 解析器创建函数
type ParserCreator func(options *ParseOptions) (Parser, error)

// ParserInfo 解析器信息
type ParserInfo struct {
	Type        string   `json:"type"`         // 解析器类型
	Name        string   `json:"name"`         // 解析器名称
	Version     string   `json:"version"`      // 版本
	Description string   `json:"description"`  // 描述
	Formats     []string `json:"formats"`      // 支持的格式
	Features    []string `json:"features"`     // 支持的特性
}

// ConfigurableParser 可配置解析器接口
type ConfigurableParser interface {
	Parser
	
	// SetOptions 设置解析选项
	SetOptions(options *ParseOptions) error
	
	// GetOptions 获取当前选项
	GetOptions() *ParseOptions
	
	// SetProgressCallback 设置进度回调
	SetProgressCallback(callback ProgressCallback)
	
	// SetValidator 设置验证器
	SetValidator(validator Validator)
	
	// GetValidator 获取验证器
	GetValidator() Validator
}

// StreamParser 流式解析器接口
// 支持大文件的流式解析
type StreamParser interface {
	Parser
	
	// ParseStream 流式解析
	ParseStream(ctx context.Context, input io.Reader, callback func(*model.ParsedInfo) error) error
	
	// SetBatchSize 设置批处理大小
	SetBatchSize(size int)
	
	// GetBatchSize 获取批处理大小
	GetBatchSize() int
}

// ConcurrentParser 并发解析器接口
// 支持多线程并发解析
type ConcurrentParser interface {
	Parser
	
	// ParseConcurrently 并发解析
	ParseConcurrently(ctx context.Context, input io.Reader, workers int) ([]*model.ParsedInfo, error)
	
	// SetMaxWorkers 设置最大工作线程数
	SetMaxWorkers(workers int)
	
	// GetMaxWorkers 获取最大工作线程数
	GetMaxWorkers() int
}

// CacheableParser 可缓存解析器接口
type CacheableParser interface {
	Parser
	
	// EnableCache 启用缓存
	EnableCache(enabled bool)
	
	// IsCacheEnabled 是否启用了缓存
	IsCacheEnabled() bool
	
	// ClearCache 清除缓存
	ClearCache() error
	
	// GetCacheStats 获取缓存统计
	GetCacheStats() *CacheStats
}

// CacheStats 缓存统计
type CacheStats struct {
	HitCount  int64 `json:"hit_count"`  // 命中次数
	MissCount int64 `json:"miss_count"` // 未命中次数
	HitRate   float64 `json:"hit_rate"` // 命中率
	Size      int64 `json:"size"`      // 缓存大小
	MaxSize   int64 `json:"max_size"`  // 最大大小
}