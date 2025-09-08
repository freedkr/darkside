# Model 包

定义职业分类数据处理系统的核心数据模型和错误类型。

## 概述

model包定义了职业分类处理系统中使用的所有核心数据结构，包括职业分类层级结构、解析结果、AI任务以及混合解析相关的数据模型。该包还实现了完整的错误处理体系，支持多种类型的错误和错误链。

## 核心组件

### 数据模型

#### Category 结构体
职业分类的核心数据结构，支持四级层级结构：

```go
type Category struct {
    Code     string      `json:"code" yaml:"code" validate:"required"`               // 职业编码，如 "1-01-00-01"
    GbmCode  string      `json:"gbm_code,omitempty" yaml:"gbm_code,omitempty"`        // GBM编码
    Name     string      `json:"name" yaml:"name" validate:"required"`              // 职业名称
    Level    string      `json:"level" yaml:"level" validate:"required,oneof=大类 中类 小类 细类"` // 分类层级：大类/中类/小类/细类
    Children []*Category `json:"children,omitempty" yaml:"children,omitempty"`      // 子分类列表
}
```

**层级规则:**
- 大类: 单个数字 (如: "1", "2")
- 中类: "父码-子码" (如: "1-01", "2-05") 
- 小类: "父码-子码-孙码" (如: "1-01-01", "2-05-03")
- 细类: "父码-子码-孙码-曾孙码" (如: "1-01-01-01")

**主要方法:**
- `GetParentCode() string` - 获取父级编码
- `GetLevel() int` - 获取数字层级
- `AddChild(*Category)` - 添加子分类
- `FindChild(code string) *Category` - 查找子分类
- `IsRoot() bool` - 判断是否为根节点
- `GetDepth() int` - 获取深度
- `Validate() error` - 验证数据完整性

#### ParsedInfo 结构体
解析后的数据记录，包含位置信息用于错误定位：

```go
type ParsedInfo struct {
    Code          string `json:"code" validate:"required"`          // 职业编码
    GbmCode       string `json:"gbm_code,omitempty"`               // GBM编码
    Name          string `json:"name" validate:"required"`          // 职业名称
    Level         int    `json:"level"`                            // 计算得出的层级 (0-3)
    RowIndex      int    `json:"row_index"`                        // 原始数据行号
    ColumnIndex   int    `json:"column_index"`                     // 原始数据列号
    RawContent    string `json:"raw_content,omitempty"`            // 原始单元格内容
}
```

**辅助方法:**
- `GetLevelName() string` - 根据层级数字返回层级名称
- `IsValid() bool` - 检查数据有效性

#### CategoryMetadata 结构体
分类元数据信息：

```go
type CategoryMetadata struct {
    Source    string    `json:"source"`     // 数据来源
    Version   string    `json:"version"`    // 版本信息
    Tags      []string  `json:"tags"`       // 标签
    CreatedAt time.Time `json:"created_at"` // 创建时间
    UpdatedAt time.Time `json:"updated_at"` // 更新时间
}
```

## 混合解析相关数据结构

### AITask 结构体
AI任务结构，以小类为单位打包细类信息：

```go
type AITask struct {
    ParentCode      string   `json:"parent_code" validate:"required"`  // 小类编码 (如: "3-02-01")
    ParentName      string   `json:"parent_name" validate:"required"`  // 小类名称，提供语义上下文
    DetailCodesRaw  []string `json:"detail_codes_raw"`                // E列原始文本数组
    DetailNamesRaw  []string `json:"detail_names_raw"`                // F列原始文本数组
}
```

### HybridParseResult 结构体
混合解析的完整结果：

```go
type HybridParseResult struct {
    SkeletonRecords []*SkeletonRecord  `json:"skeleton_records"` // 骨架记录（大、中、小类）
    AITasks         []*AITask          `json:"ai_tasks"`         // AI任务列表
    Stats           *HybridParseStats  `json:"stats"`           // 统计信息
}
```

## 错误处理系统

### ErrorCode 错误代码
支持详细的错误分类：

```go
const (
    // 通用错误
    ErrCodeInternal      ErrorCode = "INTERNAL_ERROR"
    ErrCodeInvalidInput  ErrorCode = "INVALID_INPUT"
    
    // 文件操作错误
    ErrCodeFileNotFound  ErrorCode = "FILE_NOT_FOUND"
    ErrCodeFileReadError ErrorCode = "FILE_READ_ERROR"
    
    // 解析错误
    ErrCodeParseError    ErrorCode = "PARSE_ERROR"
    ErrCodeCellError     ErrorCode = "CELL_ERROR"
    
    // 业务逻辑错误
    ErrCodeHierarchy     ErrorCode = "HIERARCHY_ERROR"
    ErrCodeValidation    ErrorCode = "VALIDATION_ERROR"
)
```

### 专用错误类型

#### ParseError - 解析错误
包含详细的位置信息：
```go
type ParseError struct {
    BaseError
    Row        int    `json:"row"`        // 错误行号
    Column     int    `json:"column"`     // 错误列号
    Content    string `json:"content"`    // 单元格内容
    Field      string `json:"field"`      // 字段名称
}
```

#### HierarchyError - 层级结构错误
用于层级关系验证：
```go
type HierarchyError struct {
    BaseError
    Code1     string `json:"code1"`     // 主要编码
    Code2     string `json:"code2"`     // 关联编码
    Level     int    `json:"level"`     // 层级
    Operation string `json:"operation"` // 操作类型
}
```

#### ErrorList - 错误集合
批量错误处理：
```go
type ErrorList struct {
    Errors []error `json:"errors"`
}
```

**便捷函数:**
- `NewParseError(row, column int, content, field, message string)` - 创建解析错误
- `NewHierarchyError(code1, code2, operation, message string, level int)` - 创建层级错误
- `IsErrorType(err error, code ErrorCode) bool` - 检查错误类型

## 使用示例

### 创建和操作职业分类

```go
// 创建大类
major := &model.Category{
    Code:  "1",
    Name:  "国家机关、党群组织、企业、事业单位负责人",
    Level: "大类",
}

// 创建中类
middle := &model.Category{
    Code:  "1-01",
    Name:  "国家机关负责人", 
    Level: "中类",
}
major.AddChild(middle)

// 创建小类
small := &model.Category{
    Code:  "1-01-01",
    Name:  "国家权力机关负责人",
    Level: "小类",
}
middle.AddChild(small)

// 层级操作
parentCode := small.GetParentCode() // 返回 "1-01"
level := small.GetLevel()           // 返回 2 (小类)
totalDesc := major.GetTotalDescendantsCount() // 递归统计后代数量
```

### AI任务创建示例

```go
// 创建AI任务
aiTask := &model.AITask{
    ParentCode:     "3-02-01",
    ParentName:     "农作物生产人员",
    DetailCodesRaw: []string{"3-02-01-01", "3-02-01-02", "3-02-01-03"},
    DetailNamesRaw: []string{"谷物种植人员", "豆类作物种植人员", "薯类作物种植人员"},
}

// 验证任务数据完整性
if len(aiTask.DetailCodesRaw) != len(aiTask.DetailNamesRaw) {
    log.Printf("AI任务数据不匹配: 编码%d个，名称%d个", 
        len(aiTask.DetailCodesRaw), len(aiTask.DetailNamesRaw))
}
```

### 错误处理示例

```go
// 创建解析错误
parseErr := model.NewParseError(15, 2, "无效编码", "Code", "编码格式不正确")

// 创建层级错误
hierErr := model.NewHierarchyError("1-01-01", "1-02", "build", "父级关系错误", 2)

// 批量错误处理
errorList := model.NewErrorList()
errorList.Add(parseErr)
errorList.Add(hierErr)

if errorList.HasError() {
    log.Printf("发现%d个错误: %v", errorList.Count(), errorList.Error())
}
```

### 元数据使用

```go
// 创建带元数据的分类
category := &model.Category{
    Code:  "1-01-01",
    Name:  "委员会负责人",
    Level: "小类",
    Metadata: &model.CategoryMetadata{
        Source:  "excel_parser",
        Version: "1.0.0", 
        Tags:    []string{"重要", "核心"},
        CreatedAt: time.Now(),
    },
}
```

## 数据验证和约束

### Category 验证规则
- **Code**: 必填，符合层级编码格式
- **Name**: 必填，职业名称
- **Level**: 必填，且必须是：大类、中类、小类、细类之一
- **层级一致性**: 编码层级与Level字段必须匹配

### ParsedInfo 验证
- **IsValid()**: 检查Code、Name非空，Level在0-3范围内
- **GetLevelName()**: 将数字层级转换为中文层级名称

### 编码层级规则
通过编码格式自动识别层级：
```go
func (c *Category) GetLevel() int {
    count := 0
    for _, ch := range c.Code {
        if ch == '-' {
            count++
        }
    }
    return count // 0=大类, 1=中类, 2=小类, 3=细类
}
```

## 设计特点和最佳实践

### 核心特点
1. **层级自动识别**: 通过编码格式自动计算层级，无需手动指定
2. **位置跟踪**: ParsedInfo包含原始位置信息，便于错误定位
3. **混合解析支持**: 专为AI辅助解析设计的数据结构
4. **完整错误链**: 支持错误堆栈跟踪和错误分类

### 最佳实践
1. **数据完整性**: 使用IsValid()验证ParsedInfo，确保必要字段非空
2. **错误处理**: 优先使用专用错误类型，提供详细的上下文信息
3. **AI任务**: 确保DetailCodesRaw和DetailNamesRaw数量匹配
4. **层级构建**: 利用GetParentCode()和GetLevel()方法构建正确的层级关系
5. **内存效率**: 大量数据处理时，及时清理不需要的Children引用

## 测试

运行模型测试:

```bash
go test ./internal/model -v
```

查看测试覆盖率:

```bash
go test ./internal/model -cover
```

## 相关包

- `internal/parser` - 数据解析，使用model定义的结构
- `internal/builder` - 层级构建，操作Category结构
- `internal/exporter` - 数据导出，序列化model数据