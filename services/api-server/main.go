package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/freedkr/moonshot/internal/config"
	"github.com/freedkr/moonshot/internal/database"
	"github.com/freedkr/moonshot/internal/queue"
	"github.com/freedkr/moonshot/internal/storage"
	"github.com/freedkr/moonshot/services/api-server/handlers"
	"github.com/freedkr/moonshot/services/api-server/middleware"
	"github.com/gin-gonic/gin"
)

type Server struct {
	config   *config.Config
	db       database.DatabaseInterface
	queue    queue.Client
	storage  storage.StorageInterface
	router   *gin.Engine
	handlers *handlers.Handlers
}

func main() {
	// 解析命令行参数
	var configPath string
	if len(os.Args) > 1 && os.Args[1] == "-config" && len(os.Args) > 2 {
		configPath = os.Args[2]
	} else {
		configPath = "configs/config.yaml"
	}

	// 加载API服务器配置
	cfg, err := config.LoadConfigForService(config.ServiceTypeAPIServer, configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	// 创建服务器
	server, err := NewServer(cfg)
	if err != nil {
		log.Fatalf("创建服务器失败: %v", err)
	}

	// 启动服务器
	if err := server.Start(); err != nil {
		log.Fatalf("启动服务器失败: %v", err)
	}
}

func NewServer(cfg *config.Config) (*Server, error) {
	// 设置Gin模式
	gin.SetMode(cfg.APIServer.Mode)
	if cfg.App.Debug {
		gin.SetMode(gin.DebugMode)
	}
	log.Printf("正在初始化数据库连接: db=%s", cfg.Database.Database)
	// 初始化数据库
	dbConfig := &database.PostgreSQLConfig{ // This can be simplified if NewPostgreSQLDB takes config.DatabaseConfig directly
		Host:            cfg.Database.Host,
		Port:            cfg.Database.Port,
		Database:        cfg.Database.Database,
		Username:        cfg.Database.Username,
		Password:        cfg.Database.Password,
		SSLMode:         cfg.Database.SSLMode,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.Database.ConnMaxIdleTime,
	}
	db, err := database.NewPostgreSQLDB(dbConfig)
	if err != nil {
		return nil, fmt.Errorf("初始化数据库失败: %w", err)
	}

	// 创建表结构
	ctx := context.Background()
	if err := db.CreateTables(ctx); err != nil {
		return nil, fmt.Errorf("创建数据库表失败: %w", err)
	}

	// 初始化队列
	redisQueue, err := queue.NewRedisQueue(cfg.Queue)
	if err != nil {
		return nil, fmt.Errorf("初始化队列失败: %w", err)
	}

	// 初始化存储
	storageConfig := &storage.MinIOConfig{
		Endpoint:        cfg.Storage.Endpoint,
		AccessKeyID:     cfg.Storage.AccessKeyID,
		SecretAccessKey: cfg.Storage.SecretAccessKey,
		UseSSL:          cfg.Storage.UseSSL,
		BucketName:      cfg.Storage.BucketName,
	}
	minioStorage, err := storage.NewMinIOStorage(storageConfig)
	if err != nil {
		return nil, fmt.Errorf("初始化存储失败: %w", err)
	}

	// 确保存储桶存在
	if err := minioStorage.EnsureBucket(ctx); err != nil {
		return nil, fmt.Errorf("确保存储桶失败: %w", err)
	}

	// 创建处理器
	handlers := handlers.NewHandlers(db, redisQueue, minioStorage)

	// 创建路由
	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(middleware.CORS())
	router.Use(middleware.RequestID())

	server := &Server{
		config:   cfg,
		db:       db,
		queue:    redisQueue,
		storage:  minioStorage,
		router:   router,
		handlers: handlers,
	}

	// 设置路由
	server.setupRoutes()

	return server, nil
}

func (s *Server) setupRoutes() {
	// 静态文件服务 - 提供前端页面
	s.router.Static("/static", "./web")
	s.router.StaticFile("/", "./web/index.html")

	api := s.router.Group("/api/v1")

	// 健康检查
	api.GET("/health", s.handlers.Health)
	api.GET("/ready", s.handlers.Ready)

	// 任务管理
	tasks := api.Group("/tasks")
	{
		tasks.POST("", s.handlers.CreateTask)
		tasks.GET("/:id", s.handlers.GetTask)
		tasks.GET("", s.handlers.ListTasks)
		tasks.DELETE("/:id", s.handlers.DeleteTask)
	}

	// 文件管理
	files := api.Group("/files")
	{
		files.POST("/upload", s.handlers.UploadFile)
		files.GET("/:id", s.handlers.DownloadFile)
		files.GET("/download", s.handlers.DownloadResultByTaskID)
		files.DELETE("/:id", s.handlers.DeleteFile)
	}

	// 数据管理
	data := api.Group("/data")
	{
		data.GET("/structured", s.handlers.GetAllStructuredData)           // 获取指定版本的所有结构化数据
		data.GET("/versions/:task_id", s.handlers.GetTaskVersionHistory)   // 获取任务版本历史
		data.GET("/categories", s.handlers.GetVersionCategories)           // 获取指定版本的分类数据
		data.GET("/recent-tasks", s.handlers.GetRecentTasks)               // 获取最近的任务列表
	}

	// 监控和统计
	monitor := api.Group("/monitor")
	{
		monitor.GET("/stats", s.handlers.GetStats)
		monitor.GET("/queues", s.handlers.GetQueueStats)
	}
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.APIServer.Host, s.config.APIServer.Port)

	server := &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  s.config.APIServer.Timeout,
		WriteTimeout: s.config.APIServer.Timeout,
	}

	// 在goroutine中启动服务器
	go func() {
		log.Printf("API服务器启动在 %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("启动服务器失败: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("正在关闭服务器...")

	// 创建关闭上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 关闭HTTP服务器
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("服务器关闭失败: %v", err)
		return err
	}

	// 关闭数据库连接
	if err := s.db.Close(); err != nil {
		log.Printf("关闭数据库失败: %v", err)
	}

	// 关闭队列连接
	s.queue.Close()

	log.Println("服务器已关闭")
	return nil
}
