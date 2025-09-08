# Builder 包

职业分类层级结构构建器，将扁平化的ParsedInfo数据构建为树形层级结构。

## 概述

builder包提供了将解析器产生的扁平化职业分类数据转换为树形层级结构的核心功能。通过智能的编码分析和严格的层级验证，确保构建出的层级结构符合职业分类体系的规范要求。支持孤儿节点处理、自动排序和错误恢复等高级特性。

## 核心组件

### 核心结构和接口

#### HierarchyBuilderImpl 实现
层级构建器的主要实现：

```go
type HierarchyBuilderImpl struct {
    config *BuilderConfig  // 构建器配置
}

type BuilderConfig struct {
    EnableOrphanHandling bool  // 开启孤儿节点处理
    StrictMode           bool  // 严格模式
}
```

**核心方法：**
- `Build(ctx context.Context, records []*model.ParsedInfo) ([]*model.Category, error)` - 主构建方法
- `determineLevel(code string) string` - 基于编码判断层级
- `getParentCode(code string) (string, bool)` - 获取父级编码
- `sortChildren(category *model.Category)` - 递归排序子节点

#### 层级识别规则
基于编码格式自动判断层级：

```go
const (
    LevelMajor  = "大类"  // 0个短横线："1", "2"
    LevelMiddle = "中类"  // 1个短横线："1-01", "2-05"
    LevelSmall  = "小类"  // 2个短横线："1-01-01", "2-05-03"
    LevelDetail = "细类"  // 3个短横线："1-01-01-01"
)
```

### 构建算法和策略

#### 三步构建算法

HierarchyBuilderImpl 使用三步算法构建层级结构：

```go
func (b *HierarchyBuilderImpl) Build(ctx context.Context, records []*model.ParsedInfo) ([]*model.Category, error) {
    // 第一步：创建所有节点
    nodeMap := make(map[string]*model.Category)
    for _, record := range records {
        level := b.determineLevel(record.Code)
        category := &model.Category{
            Code:    record.Code,
            GbmCode: record.GbmCode,
            Name:    record.Name,
            Level:   level,
        }
        nodeMap[record.Code] = category
    }
    
    // 第二步：建立父子关系
    for _, node := range nodeMap {
        parentCode, hasParent := b.getParentCode(node.Code)
        if hasParent && nodeMap[parentCode] != nil {
            nodeMap[parentCode].AddChild(node)
        }
    }
    
    // 第三步：收集根节点并排序
    var rootCategories []*model.Category
    // ...排序逻辑
}
```

#### 父子关系识别
基于编码的层级规律识别父子关系：

```go
func (b *HierarchyBuilderImpl) getParentCode(code string) (string, bool) {
    lastDash := strings.LastIndex(code, "-")
    if lastDash == -1 {
        return "", false  // 没有父级（大类）
    }
    return code[:lastDash], true
}
```

**示例：**
- `"1-01-01-01"` 的父级是 `"1-01-01"`
- `"1-01"` 的父级是 `"1"`
- `"1"` 没有父级（为根节点）

### 排序和优化机制

#### 自动排序功能

构建完成后自动按编码顺序排序：

```go
// 根节点排序
sort.Slice(rootCategories, func(i, j int) bool {
    return rootCategories[i].Code < rootCategories[j].Code
})

// 递归排序子节点
func (b *HierarchyBuilderImpl) sortChildren(category *model.Category) {
    if category.Children == nil {
        return
    }
    
    sort.Slice(category.Children, func(i, j int) bool {
        return category.Children[i].Code < category.Children[j].Code
    })
    
    for _, child := range category.Children {
        b.sortChildren(child)  // 递归排序
    }
}
```

**排序结果示例：**
原本乱序：5, 6, 7, 8, 1, 2, 3, 4  
排序后：1, 2, 3, 4, 5, 6, 7, 8

#### 孤儿节点处理

孤儿节点（父节点不存在的节点）的处理策略：

```go
func (b *HierarchyBuilderImpl) handleOrphans(nodeMap map[string]*model.Category, rootCategories []*model.Category) []*model.Category {
    if !b.config.EnableOrphanHandling {
        return rootCategories
    }
    
    for _, node := range nodeMap {
        parentCode, hasParent := b.getParentCode(node.Code)
        if hasParent {
            // 检查父节点是否存在
            if _, parentExists := nodeMap[parentCode]; !parentExists {
                log.Printf("孤儿节点: %s, 父节点 %s 不存在", node.Code, parentCode)
                if b.config.EnableOrphanHandling {
                    rootCategories = append(rootCategories, node)  // 作为根节点处理
                }
            }
        }
    }
    
    return rootCategories
}
```

#### 上下文取消支持

构建过程支持context取消，用于大数据集处理：

```go
select {
case <-ctx.Done():
    return nil, ctx.Err()
default:
    // 继续处理
}
```

### 数据结构

#### BuildStats 结构体
构建过程的统计信息：

```go
type BuildStats struct {
    TotalNodes      int           `json:"total_nodes"`      // 总节点数
    RootNodes       int           `json:"root_nodes"`       // 根节点数
    OrphanNodes     int           `json:"orphan_nodes"`     // 孤儿节点数
    MaxDepth        int           `json:"max_depth"`        // 最大深度
    ProcessTime     time.Duration `json:"process_time"`     // 处理时间
    ErrorCount      int           `json:"error_count"`      // 错误数量
    ValidationIssues []string     `json:"validation_issues"` // 验证问题
}
```

#### RelationshipType 枚举
定义节点间的关系类型：

```go
type RelationshipType int

const (
    RelationshipParent RelationshipType = iota // 父子关系
    RelationshipSibling                        // 兄弟关系  
    RelationshipOrphan                         // 孤儿节点
    RelationshipUnknown                        // 未知关系
)
```

## 使用示例

### 基本构建示例

```go
// 创建构建器配置
config := &builder.BuilderConfig{
    EnableOrphanHandling: true,   // 开启孤儿节点处理
    StrictMode:           false,  // 非严格模式，允许错误恢复
}

// 创建构建器实例
builder := builder.NewHierarchyBuilder(config)

// 准备解析数据
records := []*model.ParsedInfo{
    {Code: "1", Name: "国家机关、党群组织、企业、事业单位负责人", Level: 0},
    {Code: "1-01", Name: "国家机关负责人", Level: 1},
    {Code: "1-01-01", Name: "国家权力机关负责人", Level: 2},
    {Code: "1-01-01-01", Name: "党的机关负责人", Level: 3},
}

// 执行层级构建
ctx := context.Background()
categories, err := builder.Build(ctx, records)
if err != nil {
    log.Fatalf("构建失败: %v", err)
}

// 输出结果
fmt.Printf("构建完成，根节点数量: %d\n", len(categories))
for _, root := range categories {
    fmt.Printf("根节点: [%s] %s - 子节点: %d\n", 
        root.Code, root.Name, root.GetChildrenCount())
}
```

### 递归遍历结果

```go
// 递归打印层级结构
func printHierarchy(category *model.Category, depth int) {
    indent := strings.Repeat("  ", depth)
    fmt.Printf("%s[%s] %s (%s)\n", indent, category.Code, category.Name, category.Level)
    
    for _, child := range category.Children {
        printHierarchy(child, depth+1)
    }
}

// 打印所有根节点
for _, root := range categories {
    printHierarchy(root, 0)
}
```

### 配置选项说明

```go
// 严格模式配置
strictConfig := &builder.BuilderConfig{
    EnableOrphanHandling: false,  // 不容忍孤儿节点
    StrictMode:           true,   // 严格模式，发现问题立即停止
}

// 宽松模式配置
relaxedConfig := &builder.BuilderConfig{
    EnableOrphanHandling: true,   // 允许孤儿节点，作为根节点处理
    StrictMode:           false,  // 非严格模式，容错能力强
}
```

### 错误处理示例

```go
// 处理构建错误
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

categories, err := builder.Build(ctx, records)
if err != nil {
    if err == context.DeadlineExceeded {
        log.Printf("构建超时")
    } else if err == context.Canceled {
        log.Printf("构建被取消")
    } else {
        log.Printf("构建失败: %v", err)
    }
    return
}

// 检查结果质量
for _, root := range categories {
    descendants := root.GetTotalDescendantsCount()
    log.Printf("根节点 %s: %d 个后代节点", root.Code, descendants)
}
```

### 层级验证

```go
builder := builder.NewHierarchyBuilder(config)

// 构建完成后验证
categories, err := builder.Build(ctx, parsedData)
if err != nil {
    return err
}

// 执行层级验证
if err := builder.ValidateHierarchy(categories); err != nil {
    log.Printf("层级验证失败: %v", err)
    
    // 获取详细的验证问题
    stats := builder.GetBuildStats()
    for _, issue := range stats.ValidationIssues {
        log.Printf("验证问题: %s", issue)
    }
}
```

### 自定义构建策略

```go
// 实现自定义构建策略
type CustomBuilder struct {
    *builder.HierarchyBuilder
}

func (cb *CustomBuilder) Build(ctx context.Context, data []*model.ParsedInfo) ([]*model.Category, error) {
    // 预处理：应用自定义逻辑
    processedData := cb.customPreprocess(data)
    
    // 调用基础构建器
    result, err := cb.HierarchyBuilder.Build(ctx, processedData)
    if err != nil {
        return nil, err
    }
    
    // 后处理：应用自定义优化
    return cb.customPostprocess(result), nil
}

func (cb *CustomBuilder) customPreprocess(data []*model.ParsedInfo) []*model.ParsedInfo {
    // 实现自定义预处理逻辑
    // 例如：编码标准化、数据清洗等
    return data
}
```

## 技术特点和优化

### 核心算法优势

**高效构建：**
- 单遍历创建节点：O(n)时间复杂度
- 哈希表查找父节点：O(1)查找效率
- 就地排序：避免额外内存分配

**内存优化：**
- 节点共享：相同编码的节点只创建一次
- 及时释放：NodeMap在构建完成后自动回收
- 浅拷贝：避免不必要的数据复制

### 数据一致性保证

**编码验证：**
```go
func (b *HierarchyBuilderImpl) determineLevel(code string) string {
    dashCount := strings.Count(code, "-")
    switch dashCount {
    case 0: return LevelMajor   // "1"
    case 1: return LevelMiddle  // "1-01" 
    case 2: return LevelSmall   // "1-01-01"
    case 3: return LevelDetail  // "1-01-01-01"
    default: return "未知层级"
    }
}
```

**层级一致性检查：**
确保父子节点的层级关系正确

### 错误处理和恢复

**常见错误处理：**
- 重复编码：只保留第一个出现的节点
- 无效层级：自动设置为"未知层级"
- 孤儿节点：根据配置作为根节点或忽略

**上下文支持：**
- 支持context取消，可用于超时控制
- 在大数据集处理过程中定期检查取消信号

**计数器和统计：**
构建过程中统计：
- 处理的记录数
- 创建的节点数  
- 发现的孤儿节点数
- 的排序耗时

## 错误处理

### 常见错误类型

- `HierarchyError`: 层级结构错误
- `OrphanNodeError`: 孤儿节点错误
- `ValidationError`: 验证失败错误
- `DepthExceededError`: 超过最大深度错误

### 错误恢复策略

```go
func (hb *HierarchyBuilder) handleBuildError(err error, data []*model.ParsedInfo) ([]*model.Category, error) {
    switch e := err.(type) {
    case *OrphanNodeError:
        if hb.config.AllowOrphans {
            return hb.buildWithOrphans(data)
        }
        return nil, e
        
    case *ValidationError:
        if hb.config.AutoCorrect {
            correctedData := hb.autoCorrectData(data)
            return hb.Build(context.Background(), correctedData)
        }
        return nil, e
        
    default:
        return nil, err
    }
}
```

## 性能优化

### 索引优化
- 使用哈希表进行O(1)节点查找
- 维护父子关系索引加速构建过程
- 支持延迟加载和按需构建

### 并发处理
```go
// 并发构建支持
func (hb *HierarchyBuilder) BuildConcurrent(ctx context.Context, data []*model.ParsedInfo, workers int) ([]*model.Category, error) {
    // 数据分片
    chunks := hb.partitionData(data, workers)
    
    // 并发处理
    results := make(chan buildResult, workers)
    for i, chunk := range chunks {
        go hb.buildChunk(ctx, chunk, i, results)
    }
    
    // 合并结果
    return hb.mergeResults(results, len(chunks))
}
```

### 内存管理
- 使用对象池减少内存分配
- 支持流式构建处理大数据集
- 及时释放不需要的中间数据

## 测试

### 运行测试
```bash
# 运行所有测试
go test ./internal/builder -v

# 运行性能测试
go test ./internal/builder -bench=. -benchmem

# 查看测试覆盖率
go test ./internal/builder -cover
```

### 基准测试
```go
func BenchmarkHierarchyBuilder_Build(b *testing.B) {
    builder := NewHierarchyBuilder(defaultConfig)
    data := generateTestData(1000) // 1000个节点
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := builder.Build(context.Background(), data)
        if err != nil {
            b.Fatalf("构建失败: %v", err)
        }
    }
}
```

## 测试和验证

### 运行测试

```bash
# 运行所有Builder测试
go test ./internal/builder -v

# 运行层级构建测试
go test ./internal/builder -run TestHierarchyBuilder_Build -v

# 查看测试覆盖率
go test ./internal/builder -cover

# 性能测试
go test ./internal/builder -bench=. -benchmem
```

### 测试用例

```go
func TestHierarchyBuilder_Build(t *testing.T) {
    builder := NewHierarchyBuilder(nil)
    
    records := []*model.ParsedInfo{
        {Code: "1", Name: "大类1", Level: 0},
        {Code: "1-01", Name: "中类1", Level: 1},
        {Code: "1-01-01", Name: "小类1", Level: 2},
    }
    
    categories, err := builder.Build(context.Background(), records)
    assert.NoError(t, err)
    assert.Len(t, categories, 1)  // 一个根节点
    
    root := categories[0]
    assert.Equal(t, "1", root.Code)
    assert.Len(t, root.Children, 1)
    
    middle := root.Children[0]
    assert.Equal(t, "1-01", middle.Code) 
    assert.Len(t, middle.Children, 1)
}
```

## 最佳实践建议

### 配置选择
1. **生产环境**：使用StrictMode=true，确保数据质量
2. **开发测试**：使用EnableOrphanHandling=true，允许容错
3. **大数据集**：设置合理的context超时时间

### 性能优化
1. **预检查**：构建前验证输入数据格式
2. **内存监控**：大量节点处理时监控内存使用
3. **结果验证**：构建完成后检查根节点数量和层级结构

### 错误排查
1. **位置定位**：利用ParsedInfo中的RowIndex和ColumnIndex
2. **日志记录**：记录孤儿节点和层级异常情况
3. **统计信息**：通过GetTotalDescendantsCount()检查数据完整性

## 相关包依赖

- `internal/model` - Category和ParsedInfo数据结构定义
- `context` - 上下文取消和超时控制
- `sort` - 层级结构排序功能
- `strings` - 编码分析和层级判断