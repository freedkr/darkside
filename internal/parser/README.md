# Parser 包

职业分类数据解析器，实现传统Excel解析和混合智能解析两种模式。

## 概述

parser包提供了职业分类数据的解析功能，包括传统的ExcelParser和新的HybridParser。HybridParser实现了V2混合解析方案：本地构建骨架结构（大、中、小类），AI处理细类关联，显著提升了处理效率和准确性。

## 核心组件

### 接口定义

#### Parser 基础接口
定义了所有解析器的通用功能：

```go
type Parser interface {
    Parse(ctx context.Context, input io.Reader) ([]*model.ParsedInfo, error)
    Validate() error
    GetName() string
    GetVersion() string
    GetSupportedFormats() []string
}
```

#### ExcelParser 接口
专用于Excel文件解析：

```go
type ExcelParser interface {
    Parser
    ParseFile(ctx context.Context, filepath string) ([]*model.ParsedInfo, error)
    ParseSheet(ctx context.Context, filepath, sheetName string) ([]*model.ParsedInfo, error)
    GetSheetNames(filepath string) ([]string, error)
    GetSheetInfo(filepath, sheetName string) (*SheetInfo, error)
}
```

### 核心实现

#### HybridParser 混合智能解析器
实现V2混合解析方案的核心解析器：

```go
type HybridParser struct {
    config       *ParserConfig
    reWhitespace *regexp.Regexp  // 空白字符匹配
    reUnified    *regexp.Regexp  // 统一格式解析
    reCodeFinder *regexp.Regexp  // 编码查找
    reMajorClass *regexp.Regexp  // 大类识别
}
```

**主要功能：**
- `ParseHybrid(filepath, sheetName string) (*HybridParseResult, error)` - 混合解析主方法
- `ParseSkeletonStructure()` - 解析骨架结构（大、中、小类）
- `CreateAITasks()` - 为细类创建AI处理任务

#### ExcelParserImpl 传统Excel解析器

```go
type ExcelParserImpl struct {
    config       *ParserConfig
    reWhitespace *regexp.Regexp  // 空白字符处理
    reUnified    *regexp.Regexp  // 统一解析正则
    reCodeFinder *regexp.Regexp  // 编码查找正则
}
```

#### ParserConfig 解析器配置

```go
type ParserConfig struct {
    SheetName     string `yaml:"sheet_name"`     // 工作表名称（默认："Table1"）
    StrictMode    bool   `yaml:"strict_mode"`    // 严格模式
    SkipEmptyRows bool   `yaml:"skip_empty"`     // 跳过空行
    MaxRows       int    `yaml:"max_rows"`       // 最大行数限制
}
```

**主要方法:**
- `NewExcelParser(config *ParserConfig) Parser` - 创建解析器实例
- `ParseFile(ctx context.Context, filePath string) ([]*model.ParsedInfo, error)` - 解析整个文件
- `ParseSheet(ctx context.Context, filePath, sheetName string) ([]*model.ParsedInfo, error)` - 解析指定工作表
- `ParseCell(cellValue string) (*model.ParsedInfo, error)` - 解析单个单元格

### 解析策略和规则

#### 混合解析（V2方案）

**骨架结构解析：**
- 识别大类：`第X大类 N (GBM XXXXX) 职业名称`
- 识别中类：基于编码格式 `N-XX` 和内容模式
- 识别小类：基于编码格式 `N-XX-XX` 和内容模式
- 自动跳过标题行和说明行

**AI任务打包：**
- 以小类为单位收集细类信息
- E列细类编码 + F列细类名称配对
- 处理数量不匹配问题（已修复）
- 为AI提供小类上下文信息

#### 正则表达式系统

```go
// 核心解析正则（统一格式）
reUnified := regexp.MustCompile(`^(.*?)([\d-]+)\s*(?:\(\s*GBM\s*(\d+)\s*\))?\s*(.*)$`)

// 大类特殊识别
reMajorClass := regexp.MustCompile(`第[一二三四五六七八]大类\s+([1-8])\s*(?:\(\s*GBM\s*(\d+)\s*\))?\s*(.*)$`)

// 编码查找定位
reCodeFinder := regexp.MustCompile(`[\d-]+(?:\s*\(\s*GBM\s*\d+\s*\))?`)
```

#### 传统解析模式

支持的格式类型：
1. **完整格式**: `专业技术 1-02-03 (GBM 10203) 高级工程师`
2. **标准格式**: `1-01 (GBM 10100) 党群组织负责人`
3. **简化格式**: `1-01-01 委员会负责人`
4. **纯编码**: `1-01-01-01`

### 错误处理

#### 错误类型
- `ValidationError` - 文件验证失败
- `ParsingError` - 单元格解析失败  
- `FileAccessError` - 文件访问错误
- `SheetNotFoundError` - 工作表不存在

#### 错误恢复策略
- **跳过模式**: 跳过解析失败的行，记录错误日志
- **严格模式**: 遇到错误立即停止解析
- **容错模式**: 允许一定数量的错误，超过阈值后停止

## 使用示例

### 混合智能解析（推荐）

```go
// 创建混合解析器
config := &parser.ParserConfig{
    SheetName:     "Table1",
    StrictMode:    true,
    SkipEmptyRows: true,
    MaxRows:       0, // 不限制行数
}

hybridParser := parser.NewHybridParser(config)

// 执行混合解析
result, err := hybridParser.ParseHybrid("occupation_data.xlsx", "Table1")
if err != nil {
    log.Fatalf("混合解析失败: %v", err)
}

// 处理骨架结构
fmt.Printf("解析到 %d 条骨架记录\n", len(result.SkeletonRecords))
for _, skeleton := range result.SkeletonRecords {
    fmt.Printf("%s[%s] %s - %s\n", skeleton.Level, skeleton.Code, skeleton.Name)
}

// 处理AI任务
fmt.Printf("生成 %d 个AI任务\n", len(result.AITasks))
for i, task := range result.AITasks {
    fmt.Printf("任务%d: %s(%s) - %d个细类编码, %d个细类名称\n", 
        i+1, task.ParentName, task.ParentCode, 
        len(task.DetailCodesRaw), len(task.DetailNamesRaw))
}
```

### 传统Excel解析

```go
// 创建传统Excel解析器
excelParser := parser.NewExcelParser(config)

// 解析指定工作表
ctx := context.Background()
records, err := excelParser.ParseSheet(ctx, "input.xlsx", "Table1")
if err != nil {
    log.Fatalf("解析失败: %v", err)
}

// 处理解析结果
for _, record := range records {
    fmt.Printf("[%s] %s - %s (行%d列%d)\n", 
        record.GetLevelName(), record.Code, record.Name, 
        record.RowIndex, record.ColumnIndex)
}
```

### 解析结果处理

```go
// 处理混合解析统计信息
stats := result.Stats
fmt.Printf("解析统计:\n")
fmt.Printf("  总行数: %d\n", stats.TotalRows)
fmt.Printf("  骨架记录: %d\n", stats.SkeletonCount)
fmt.Printf("  AI任务: %d\n", stats.AITaskCount)
fmt.Printf("  处理时间: %dms\n", stats.ProcessingTime)

// 验证AI任务数据完整性
for i, task := range result.AITasks {
    if len(task.DetailCodesRaw) != len(task.DetailNamesRaw) {
        log.Printf("⚠️ AI任务 %d 数据不匹配：编码%d个，名称%d个\n", 
            i+1, len(task.DetailCodesRaw), len(task.DetailNamesRaw))
    } else {
        log.Printf("✅ AI任务 %d 数据匹配：%d对编码-名称\n", 
            i+1, len(task.DetailCodesRaw))
    }
}
```

### 文件验证

```go
parser := parser.NewExcelParser(nil)

// 验证文件格式
if err := parser.ValidateFile("input.xlsx"); err != nil {
    log.Fatalf("文件验证失败: %v", err)
}

// 获取工作表列表
sheets, err := parser.GetSheetNames("input.xlsx")
if err != nil {
    log.Fatalf("获取工作表失败: %v", err)
}

fmt.Printf("可用工作表: %v\n", sheets)
```

### 上下文取消

```go
// 创建可取消的上下文
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// 解析操作会在超时后自动取消
results, err := parser.ParseFile(ctx, "large_file.xlsx")
if err == context.DeadlineExceeded {
    log.Println("解析超时")
} else if err != nil {
    log.Printf("解析错误: %v", err)
}
```

## 配置和选项

### ParserConfig 配置说明

- `SheetName`: 要解析的工作表名称（默认："Table1"）
- `StrictMode`: 严格模式，遇到关键错误立即停止（默认：true）
- `SkipEmptyRows`: 跳过空行和无效行（默认：true）
- `MaxRows`: 最大处理行数，0表示不限制（默认：0）

### 混合解析配置特点

**智能识别模式：**
- 自动识别大类：通过"第X大类"模式识别
- 层级推断：基于编码格式自动判断层级
- 内容分析：智能区分标题、说明和数据行

**数据收集策略：**
- 骨架优先：先构建大、中、小类骨架
- 细类打包：按小类分组收集细类信息
- 配对保证：确保编码和名称一一对应

### 环境变量支持

```bash
export PARSER_SHEET_NAME="CustomSheet"
export PARSER_START_ROW="2"
export PARSER_STRICT_MODE="true"
export PARSER_MAX_ROWS="1000"
```

## 性能特点和优化

### 混合解析性能优势

**处理效率提升：**
- 本地骨架构建：无需AI处理大、中、小类，处理速度快
- AI任务批量化：细类按小类分组，减少API调用次数
- 智能跳过：自动跳过无关行，提高解析效率

**内存使用优化：**
- 流式处理：逐行读取，避免整个文件加载到内存
- 即时释放：处理完的Excel对象及时释放
- 正则缓存：编译好的正则表达式重复使用

### 错误处理和恢复

```go
// 数量不匹配修复策略
if len(matchedCodes) != len(matchedNames) {
    minLen := len(matchedCodes)
    if len(matchedNames) < minLen {
        minLen = len(matchedNames)
    }
    // 配对截取，确保数量一致
    for i := 0; i < minLen; i++ {
        // 安全配对添加
    }
}
```

## 测试

### 运行测试
```bash
# 运行所有解析器测试
go test ./internal/parser -v

# 运行混合解析测试
go test ./internal/parser -run TestHybridParser -v

# 运行Excel解析测试
go test ./internal/parser -run TestExcelParser -v

# 查看测试覆盖率
go test ./internal/parser -cover

# 运行基准测试
go test ./internal/parser -bench=. -benchmem
```

### 测试策略

```go
// 混合解析测试
func TestHybridParser_ParseHybrid(t *testing.T) {
    parser := NewHybridParser(nil)
    
    result, err := parser.ParseHybrid("testdata/sample.xlsx", "Table1")
    if err != nil {
        t.Fatalf("混合解析失败: %v", err)
    }
    
    // 验证骨架结构
    assert.True(t, len(result.SkeletonRecords) > 0)
    
    // 验证AI任务
    for _, task := range result.AITasks {
        assert.Equal(t, len(task.DetailCodesRaw), len(task.DetailNamesRaw))
    }
}

// 传统解析测试
func TestExcelParser_ParseSheet(t *testing.T) {
    parser := NewExcelParser(nil)
    
    records, err := parser.ParseSheet(context.Background(), "testdata/sample.xlsx", "Table1")
    if err != nil {
        t.Fatalf("解析失败: %v", err)
    }
    
    // 验证解析结果
    for _, record := range records {
        assert.True(t, record.IsValid())
    }
}
```

## 最佳实践建议

### 解析器选择
1. **新项目推荐**：使用HybridParser，支持AI辅助处理
2. **简单场景**：数据规整且无需AI处理时使用ExcelParserImpl
3. **测试调试**：先用传统解析验证数据格式，再切换到混合解析

### 配置优化
1. **严格模式**：生产环境建议开启StrictMode，确保数据质量
2. **行数限制**：大文件处理时设置MaxRows，避免内存溢出
3. **工作表选择**：确认SheetName正确，避免解析错误的工作表

### 错误处理
1. **位置追踪**：利用RowIndex和ColumnIndex快速定位问题
2. **批量验证**：解析完成后检查AI任务的数据配对情况
3. **日志记录**：记录解析统计信息，监控处理效果

### 性能调优
1. **预检查**：解析前验证文件存在性和格式正确性
2. **内存监控**：大文件处理时监控内存使用情况
3. **超时设置**：为解析操作设置合理的超时时间

## 设计理念和技术特色

### V2混合解析方案

**设计原则：**
1. **人机结合**：人工智能处理复杂语义，程序处理结构化数据
2. **效率优先**：减少AI调用，提升整体处理速度
3. **准确性保证**：多重验证机制，确保数据质量
4. **可扩展性**：支持不同的AI服务接入

**技术亮点：**
- 正则表达式优化：统一解析模式，支持多种格式
- 层级智能识别：基于编码规律自动判断层级关系
- 错误恢复机制：自动修复常见的数据不匹配问题
- 上下文保持：为AI提供小类名称作为语义上下文

## 相关包依赖

- `internal/model` - 核心数据结构和错误类型定义
- `internal/llm` - AI服务接口和Kimi API集成
- `github.com/xuri/excelize/v2` - Excel文件读取库
- `regexp` - 正则表达式解析引擎