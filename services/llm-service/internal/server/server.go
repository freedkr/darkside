// Package server HTTP服务器实现
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/freedkr/moonshot/services/llm-service/internal/models"
	"github.com/freedkr/moonshot/services/llm-service/internal/providers"
	"github.com/freedkr/moonshot/services/llm-service/internal/scheduler"
)

// LLMServer LLM服务HTTP服务器
type LLMServer struct {
	// 核心组件
	scheduler       scheduler.TaskScheduler
	providerManager providers.ProviderManager

	// HTTP服务器
	engine *gin.Engine
	server *http.Server

	// WebSocket升级器
	upgrader websocket.Upgrader

	// WebSocket连接管理
	wsListener *scheduler.WebSocketCallbackListener

	// 配置
	config ServerConfig
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port            int           `json:"port"`
	ReadTimeout     time.Duration `json:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout"`
	IdleTimeout     time.Duration `json:"idle_timeout"`
	MaxRequestSize  int64         `json:"max_request_size"`
	EnableCORS      bool          `json:"enable_cors"`
	EnableMetrics   bool          `json:"enable_metrics"`
	EnableWebSocket bool          `json:"enable_websocket"`
	AuthToken       string        `json:"auth_token,omitempty"`
}

// NewLLMServer 创建LLM服务器
func NewLLMServer(
	taskScheduler scheduler.TaskScheduler,
	providerManager providers.ProviderManager,
	config ServerConfig,
) *LLMServer {
	// 设置默认值
	if config.Port == 0 {
		config.Port = 8080
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 30 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 30 * time.Second
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = 60 * time.Second
	}
	if config.MaxRequestSize == 0 {
		config.MaxRequestSize = 32 << 20 // 32MB
	}

	// 创建Gin引擎
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery())

	// WebSocket升级器
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // 允许所有来源，生产环境应该更严格
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	// WebSocket监听器
	wsListener := scheduler.NewWebSocketCallbackListener()

	server := &LLMServer{
		scheduler:       taskScheduler,
		providerManager: providerManager,
		engine:          engine,
		upgrader:        upgrader,
		wsListener:      wsListener,
		config:          config,
	}

	// 注册WebSocket监听器到调度器
	if defaultScheduler, ok := taskScheduler.(*scheduler.DefaultTaskScheduler); ok {
		defaultScheduler.RegisterListener(wsListener)
	}

	// 设置路由
	server.setupRoutes()

	return server
}

// Start 启动服务器
func (s *LLMServer) Start(ctx context.Context) error {
	s.server = &http.Server{
		Addr:           fmt.Sprintf(":%d", s.config.Port),
		Handler:        s.engine,
		ReadTimeout:    s.config.ReadTimeout,
		WriteTimeout:   s.config.WriteTimeout,
		IdleTimeout:    s.config.IdleTimeout,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// 启动HTTP服务器
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("HTTP服务器启动失败: %v\n", err)
		}
	}()

	fmt.Printf("LLM服务器启动，监听端口: %d\n", s.config.Port)
	return nil
}

// Stop 停止服务器
func (s *LLMServer) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// setupRoutes 设置路由
func (s *LLMServer) setupRoutes() {
	// 健康检查
	s.engine.GET("/health", s.handleHealth)
	s.engine.GET("/ready", s.handleReady)

	// API路由组
	api := s.engine.Group("/api/v1")

	// 可选的认证中间件
	if s.config.AuthToken != "" {
		api.Use(s.authMiddleware())
	}

	// 任务管理
	api.POST("/tasks", s.handleSubmitTask)
	api.GET("/tasks/:id", s.handleGetTask)
	api.DELETE("/tasks/:id", s.handleCancelTask)
	api.GET("/tasks", s.handleListTasks)

	// 批量处理
	api.POST("/tasks/batch", s.handleBatchSubmit)

	// 同步处理
	api.POST("/process/sync", s.handleSyncProcess)

	// 流式处理
	api.POST("/process/stream", s.handleStreamProcess)

	// 提供商管理
	api.GET("/providers", s.handleListProviders)
	api.GET("/providers/:name/status", s.handleGetProviderStatus)
	api.GET("/providers/status", s.handleGetAllProvidersStatus)

	// 统计和监控
	api.GET("/stats", s.handleGetStats)
	api.GET("/metrics", s.handleGetMetrics)

	// WebSocket端点
	if s.config.EnableWebSocket {
		s.engine.GET("/ws", s.handleWebSocket)
	}

	// CORS支持
	if s.config.EnableCORS {
		s.engine.Use(s.corsMiddleware())
	}
}

// handleHealth 健康检查处理器
func (s *LLMServer) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now(),
		"service":   "llm-service",
		"version":   "1.0.0",
	})
}

// handleReady 就绪检查处理器
func (s *LLMServer) handleReady(c *gin.Context) {
	// 检查调度器状态
	stats := s.scheduler.GetStats()

	// 检查提供商状态
	providerStatus := s.providerManager.GetAllProvidersStatus()
	availableProviders := 0
	for _, status := range providerStatus {
		if status.Available {
			availableProviders++
		}
	}

	if availableProviders == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"reason": "no available providers",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":              "ready",
		"timestamp":           time.Now(),
		"available_providers": availableProviders,
		"total_tasks":         stats.TotalTasks,
		"running_tasks":       stats.RunningTasks,
	})
}

// handleSubmitTask 提交任务处理器
func (s *LLMServer) handleSubmitTask(c *gin.Context) {
	var req SubmitTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求格式: " + err.Error(),
		})
		return
	}

	// 创建任务
	task := &models.LLMTask{
		ID:           generateTaskID(),
		Type:         req.Type,
		Provider:     req.Provider,
		Model:        req.Model,
		Temperature:  req.Temperature,
		Prompt:       req.Prompt,
		SystemPrompt: req.SystemPrompt,
		Priority:     req.Priority,
		Config:       req.Config,
		CallbackURL:  req.CallbackURL,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Metadata:     req.Metadata,
	}

	// 设置数据
	if req.Data != nil {
		if err := task.SetData(req.Data); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "无效的数据格式: " + err.Error(),
			})
			return
		}
	}

	// 提交任务
	if err := s.scheduler.SubmitTask(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "提交任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, SubmitTaskResponse{
		TaskID: task.ID,
		Status: string(task.Status),
	})
}

// handleGetTask 获取任务处理器
func (s *LLMServer) handleGetTask(c *gin.Context) {
	taskID := c.Param("id")

	task, err := s.scheduler.GetTaskStatus(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, task)
}

// handleCancelTask 取消任务处理器
func (s *LLMServer) handleCancelTask(c *gin.Context) {
	taskID := c.Param("id")

	if err := s.scheduler.CancelTask(taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "任务已取消",
		"task_id": taskID,
	})
}

// handleListTasks 列出任务处理器
func (s *LLMServer) handleListTasks(c *gin.Context) {
	// 获取查询参数
	var limit = 10
	var offset = 0
	
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit := parseIntWithDefault(limitParam, 10); parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		}
	}
	
	if offsetParam := c.Query("offset"); offsetParam != "" {
		if parsedOffset := parseIntWithDefault(offsetParam, 0); parsedOffset >= 0 {
			offset = parsedOffset
		}
	}
	
	// 获取任务列表
	tasks, total, err := s.scheduler.ListTasks(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponseFromError(err))
		return
	}
	
	// 构造响应
	response := gin.H{
		"tasks":  tasks,
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"count":  len(tasks),
	}
	
	c.JSON(http.StatusOK, response)
}

// handleBatchSubmit 批量提交处理器
func (s *LLMServer) handleBatchSubmit(c *gin.Context) {
	var req BatchSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求格式: " + err.Error(),
		})
		return
	}

	responses := make([]SubmitTaskResponse, 0, len(req.Tasks))

	// 提交每个任务
	for _, taskReq := range req.Tasks {
		task := &models.LLMTask{
			ID:           generateTaskID(),
			Type:         taskReq.Type,
			Provider:     taskReq.Provider,
			Model:        taskReq.Model,
			Temperature:  taskReq.Temperature,
			Prompt:       taskReq.Prompt,
			SystemPrompt: taskReq.SystemPrompt,
			Priority:     taskReq.Priority,
			Config:       taskReq.Config,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			Metadata:     taskReq.Metadata,
		}

		if taskReq.Data != nil {
			task.SetData(taskReq.Data)
		}

		if err := s.scheduler.SubmitTask(c.Request.Context(), task); err != nil {
			responses = append(responses, SubmitTaskResponse{
				TaskID: task.ID,
				Status: "failed",
				Error:  err.Error(),
			})
		} else {
			responses = append(responses, SubmitTaskResponse{
				TaskID: task.ID,
				Status: string(task.Status),
			})
		}
	}

	c.JSON(http.StatusOK, BatchSubmitResponse{
		Results: responses,
	})
}

// handleSyncProcess 同步处理处理器
func (s *LLMServer) handleSyncProcess(c *gin.Context) {
	var req SubmitTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求格式: " + err.Error(),
		})
		return
	}

	// 创建任务
	task := &models.LLMTask{
		ID:           generateTaskID(),
		Type:         req.Type,
		Provider:     req.Provider,
		Model:        req.Model,
		Temperature:  req.Temperature,
		Prompt:       req.Prompt,
		SystemPrompt: req.SystemPrompt,
		Priority:     req.Priority,
		Config:       req.Config,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Metadata:     req.Metadata,
	}

	if req.Data != nil {
		task.SetData(req.Data)
	}

	// 同步处理：提交任务并等待完成
	if err := s.scheduler.SubmitTask(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "提交任务失败: " + err.Error(),
		})
		return
	}

	// 轮询等待任务完成（简化实现）
	timeout := 5 * time.Minute
	if req.Config.Timeout != "" {
		if parsedTimeout, err := time.ParseDuration(req.Config.Timeout); err == nil {
			timeout = parsedTimeout
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.JSON(http.StatusRequestTimeout, gin.H{
				"error":   "任务处理超时",
				"task_id": task.ID,
			})
			return
		case <-ticker.C:
			currentTask, err := s.scheduler.GetTaskStatus(task.ID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "获取任务状态失败: " + err.Error(),
				})
				return
			}

			if currentTask.IsTerminal() {
				if currentTask.Status == models.StatusCompleted {
					// 获取结果
					var result interface{}
					if len(currentTask.Result) > 0 {
						json.Unmarshal(currentTask.Result, &result)
					}

					c.JSON(http.StatusOK, SyncProcessResponse{
						TaskID:      currentTask.ID,
						Status:      string(currentTask.Status),
						Result:      result,
						TokenUsage:  currentTask.TokenUsage,
						ProcessTime: currentTask.GetDuration().String(),
					})
				} else {
					c.JSON(http.StatusInternalServerError, gin.H{
						"error":   "任务失败: " + currentTask.Error,
						"task_id": currentTask.ID,
					})
				}
				return
			}
		}
	}
}

// handleStreamProcess 流式处理处理器
func (s *LLMServer) handleStreamProcess(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "流式处理功能待实现",
	})
}

// handleListProviders 列出提供商处理器
func (s *LLMServer) handleListProviders(c *gin.Context) {
	providers := s.providerManager.ListProviders()
	c.JSON(http.StatusOK, gin.H{
		"providers": providers,
	})
}

// handleGetProviderStatus 获取提供商状态处理器
func (s *LLMServer) handleGetProviderStatus(c *gin.Context) {
	providerName := c.Param("name")

	status, err := s.providerManager.GetProviderStatus(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, status)
}

// handleGetAllProvidersStatus 获取所有提供商状态处理器
func (s *LLMServer) handleGetAllProvidersStatus(c *gin.Context) {
	status := s.providerManager.GetAllProvidersStatus()
	c.JSON(http.StatusOK, status)
}

// handleGetStats 获取统计处理器
func (s *LLMServer) handleGetStats(c *gin.Context) {
	stats := s.scheduler.GetStats()
	c.JSON(http.StatusOK, stats)
}

// handleGetMetrics 获取指标处理器
func (s *LLMServer) handleGetMetrics(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "指标功能待实现",
	})
}

// handleWebSocket WebSocket处理器
func (s *LLMServer) handleWebSocket(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "WebSocket升级失败: " + err.Error(),
		})
		return
	}

	connID := uuid.New().String()
	s.wsListener.AddConnection(connID, &WebSocketConn{conn: conn})

	// 保持连接直到客户端断开
	defer s.wsListener.RemoveConnection(connID)

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// 中间件

// authMiddleware 认证中间件
func (s *LLMServer) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			token = c.Query("token")
		}

		if token != "Bearer "+s.config.AuthToken && token != s.config.AuthToken {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "认证失败",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// corsMiddleware CORS中间件
func (s *LLMServer) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// 辅助函数

// generateTaskID 生成任务ID
func generateTaskID() string {
	return "task_" + uuid.New().String()
}

// parseIntWithDefault 解析整数参数，失败时返回默认值
func parseIntWithDefault(str string, defaultValue int) int {
	if value, err := strconv.Atoi(str); err == nil {
		return value
	}
	return defaultValue
}

// WebSocketConn WebSocket连接包装器
type WebSocketConn struct {
	conn *websocket.Conn
}

func (w *WebSocketConn) WriteJSON(v interface{}) error {
	return w.conn.WriteJSON(v)
}

func (w *WebSocketConn) Close() error {
	return w.conn.Close()
}
