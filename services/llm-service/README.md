# LLM Service - 通用大语言模型服务

LLM Service 是一个通用的大语言模型服务平台，提供统一的API接口来访问多种LLM提供商，支持任务调度、并发控制、智能路由和实时监控。

## ✨ 特性

### 🚀 核心功能
- **多提供商支持**: 统一接口访问 Kimi、OpenAI、Qwen 等多个LLM提供商
- **智能任务调度**: 基于优先级的任务队列和工作池管理
- **并发控制**: 全局、提供商和任务类型级别的并发限制
- **智能路由**: 根据成本、速度、质量自动选择最优提供商
- **实时监控**: WebSocket实时状态推送和详细指标统计

### 🎯 任务类型
- **语义分析** (semantic_analysis): 文本语义理解和分析
- **数据清洗** (data_cleaning): 数据标准化和清洗
- **分类匹配** (category_match): 文本分类和匹配
- **文本摘要** (text_summarization): 自动文本摘要
- **翻译** (translation): 多语言翻译
- **自定义任务** (custom): 用户自定义处理逻辑

### 🔧 高级特性
- **同步/异步处理**: 支持同步调用和异步任务处理
- **批量处理**: 高效的批量任务提交和处理
- **流式处理**: 实时流式响应（计划中）
- **缓存机制**: 智能结果缓存以提高性能
- **重试机制**: 自动重试和错误恢复
- **回调支持**: Webhook和WebSocket回调通知

## 🏗️ 架构设计

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   HTTP API      │    │  Task Scheduler │    │ Provider Manager │
│                 │    │                 │    │                 │
│ • REST API      │◄──►│ • Priority Queue│◄──►│ • Kimi Provider │
│ • WebSocket     │    │ • Worker Pool   │    │ • OpenAI Provider│
│ • Batch API     │    │ • Concurrency   │    │ • Smart Routing │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  Callback       │    │   Cache Layer   │    │   Monitoring    │
│                 │    │                 │    │                 │
│ • Webhook       │    │ • Result Cache  │    │ • Metrics       │
│ • WebSocket     │    │ • Rate Limiting │    │ • Health Check  │
│ • Event Bus     │    │ • Token Bucket  │    │ • Statistics    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## 🚀 快速开始

### 环境要求
- Go 1.23+
- Redis (可选，用于分布式缓存)
- PostgreSQL (可选，用于任务持久化)

### 安装运行

1. **设置环境变量**
```bash
export KIMI_API_KEY="your_kimi_api_key"
export LLM_PORT=8080
export LLM_MAX_WORKERS=10
```

2. **启动服务**
```bash
cd services/llm-service
go run main.go
```

3. **验证服务**
```bash
# 健康检查
curl http://localhost:8090/health

# 就绪检查
curl http://localhost:8090/ready
```

## 📚 API 文档

### 基础端点

#### 健康检查
```http
GET /health
```

#### 就绪检查
```http
GET /ready
```

### 任务管理

#### 提交任务
```http
POST /api/v1/tasks
Content-Type: application/json

{
  "type": "semantic_analysis",
  "prompt": "请分析以下文本的语义",
  "data": {
    "text": "这是需要分析的文本内容"
  },
  "priority": "normal",
  "config": {
    "timeout": "60s",
    "cache_enabled": true
  }
}
```

响应:
```json
{
  "task_id": "task_123e4567-e89b-12d3-a456-426614174000",
  "status": "queued"
}
```

#### 获取任务状态
```http
GET /api/v1/tasks/{task_id}
```

#### 同步处理
```http
POST /api/v1/process/sync
Content-Type: application/json

{
  "type": "data_cleaning",
  "prompt": "请清洗以下数据",
  "data": [...],
  "config": {
    "timeout": "120s"
  }
}
```

#### 批量提交
```http
POST /api/v1/tasks/batch
Content-Type: application/json

{
  "tasks": [
    {
      "type": "semantic_analysis",
      "prompt": "分析文本1",
      "data": {...}
    },
    {
      "type": "data_cleaning", 
      "prompt": "清洗数据2",
      "data": {...}
    }
  ]
}
```

### 提供商管理

#### 列出提供商
```http
GET /api/v1/providers
```

#### 获取提供商状态
```http
GET /api/v1/providers/kimi/status
```

#### 获取所有提供商状态
```http
GET /api/v1/providers/status
```

### 监控和统计

#### 获取统计信息
```http
GET /api/v1/stats
```

#### 获取详细指标
```http
GET /api/v1/metrics
```

### WebSocket 实时通知

连接到 WebSocket 端点以接收实时任务状态更新：

```javascript
const ws = new WebSocket('ws://localhost:8090/ws');

ws.onmessage = function(event) {
  const data = JSON.parse(event.data);
  console.log('任务状态更新:', data);
};
```

## 🔧 配置说明

### 环境变量

| 变量名 | 描述 | 默认值 |
|--------|------|--------|
| `KIMI_API_KEY` | Kimi API密钥 | - |
| `LLM_PORT` | 服务端口 | 8080 |
| `LLM_MAX_WORKERS` | 最大工作协程数 | 10 |
| `LLM_MAX_QUEUE_SIZE` | 最大队列大小 | 1000 |
| `LLM_TASK_TIMEOUT` | 任务超时时间 | 5m |
| `LLM_ENABLE_CORS` | 启用CORS | true |
| `LLM_ENABLE_WEBSOCKET` | 启用WebSocket | true |
| `LLM_AUTH_TOKEN` | API认证令牌 | - |

### 配置文件

参见 `configs/config.yaml` 了解详细的配置选项。

## 📊 监控指标

### 服务级指标
- `llm_requests_total`: 总请求数
- `llm_request_duration_seconds`: 请求处理时间
- `llm_active_tasks`: 活跃任务数
- `llm_queue_length`: 队列长度

### 提供商指标
- `llm_provider_requests_total`: 提供商请求数
- `llm_provider_errors_total`: 提供商错误数
- `llm_provider_latency_seconds`: 提供商响应时间
- `llm_provider_cost_total`: 提供商使用成本

## 🔗 与现有系统集成

### Rule Worker 集成示例

```go
// 在 rule-worker 中调用 LLM 服务
func (w *RuleWorker) processWithLLM(categories []*Category) error {
    client := &http.Client{}
    
    req := map[string]interface{}{
        "type": "semantic_analysis",
        "prompt": "请分析这些分类数据",
        "data": categories,
        "callback_url": "http://rule-worker:8080/llm-callback",
    }
    
    resp, err := client.Post("http://llm-service:8080/api/v1/tasks", 
        "application/json", bytes.NewBuffer(reqData))
    if err != nil {
        return err
    }
    
    // 处理响应...
    return nil
}
```

### 回调处理

```go
// 在调用方服务中处理LLM回调
func (w *RuleWorker) HandleLLMCallback(c *gin.Context) {
    var callback LLMCallbackEvent
    if err := c.ShouldBindJSON(&callback); err != nil {
        return
    }
    
    switch callback.EventType {
    case "completed":
        // 处理完成的任务
        w.processLLMResult(callback.TaskID, callback.Data)
    case "failed":
        // 处理失败的任务
        w.handleLLMError(callback.TaskID, callback.Error)
    }
}
```

## 🛠️ 开发和扩展

### 添加新的提供商

1. 实现 `Provider` 接口：
```go
type MyProvider struct {
    // 实现字段
}

func (p *MyProvider) Process(ctx context.Context, task *models.LLMTask) (*models.LLMResult, error) {
    // 实现处理逻辑
}
```

2. 注册提供商工厂：
```go
func init() {
    providers.RegisterProviderFactory("my_provider", func(config providers.ProviderConfig) (providers.Provider, error) {
        return NewMyProvider(config)
    })
}
```

### 添加新的任务类型

1. 在 `models/task.go` 中添加任务类型：
```go
const (
    TaskTypeMyCustomTask LLMTaskType = "my_custom_task"
)
```

2. 在提供商中实现处理逻辑。

## 📝 最佳实践

1. **任务设计**: 保持任务粒度适中，避免过大的数据负载
2. **错误处理**: 实现适当的重试和降级策略
3. **监控**: 定期检查服务健康状况和性能指标
4. **安全**: 在生产环境中启用认证和限流
5. **成本控制**: 监控Token使用量和API调用成本

## 🤝 贡献

欢迎贡献代码、报告问题或提出建议！

## 📄 许可证

本项目采用 MIT 许可证。