package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/freedkr/moonshot/internal/builder"
	"github.com/freedkr/moonshot/internal/config"
	"github.com/freedkr/moonshot/internal/database"
	"github.com/freedkr/moonshot/internal/integration"
	"github.com/freedkr/moonshot/internal/model"
	"github.com/freedkr/moonshot/internal/parser"
	"github.com/freedkr/moonshot/internal/queue"
	"github.com/freedkr/moonshot/internal/storage"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// RuleWorker 规则处理Worker
type RuleWorker struct {
	config               *config.Config
	db                   database.DatabaseInterface
	queue                queue.Client
	storage              storage.StorageInterface
	parser               *parser.ExcelParserImpl
	builder              *builder.HierarchyBuilderImpl
	pdfProcessor         *integration.PDFLLMProcessor
	incrementalProcessor *integration.IncrementalProcessor
}

func main() {
	// 解析命令行参数
	var configPath string
	if len(os.Args) > 1 && os.Args[1] == "-config" && len(os.Args) > 2 {
		configPath = os.Args[2]
	} else {
		configPath = "configs/config.yaml"
	}

	// 加载Rule Worker配置
	cfg, err := config.LoadConfigForService(config.ServiceTypeRuleWorker, configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 创建Worker
	worker, err := NewRuleWorker(cfg)
	if err != nil {
		log.Fatalf("创建Worker失败: %v", err)
	}

	// 启动Worker
	if err := worker.Start(); err != nil {
		log.Fatalf("启动Worker失败: %v", err)
	}
}

func NewRuleWorker(cfg *config.Config) (*RuleWorker, error) {
	// 初始化数据库
	dbConfig := &database.PostgreSQLConfig{
		Host:      cfg.Database.Host,
		Port:      cfg.Database.Port,
		Database:  cfg.Database.Database,
		Username:  cfg.Database.Username,
		Password:  cfg.Database.Password,
		SSLMode:   cfg.Database.SSLMode,
		BatchSize: cfg.Database.BatchSize,
	}
	db, err := database.NewPostgreSQLDB(dbConfig)
	if err != nil {
		return nil, fmt.Errorf("初始化数据库失败: %w", err)
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

	// 初始化解析器
	parserConfig := &parser.ParserConfig{
		SheetName:     cfg.Parser.SheetName,
		StrictMode:    cfg.Parser.StrictMode,
		SkipEmptyRows: cfg.Parser.SkipEmptyRows,
		MaxRows:       cfg.Parser.MaxRows,
	}
	excelParser := parser.NewExcelParser(parserConfig)

	// 初始化构建器
	builderConfig := &builder.BuilderConfig{
		EnableOrphanHandling: cfg.Builder.EnableOrphanHandling,
		StrictMode:           cfg.Builder.StrictMode,
	}
	hierarchyBuilder := builder.NewHierarchyBuilder(builderConfig)

	// 初始化PDF和LLM处理器
	pdfProcessor := integration.NewPDFLLMProcessor(cfg, db)

	// 初始化增量处理器
	incrementalProcessor := integration.NewIncrementalProcessor(cfg, db)

	return &RuleWorker{
		config:               cfg,
		db:                   db,
		queue:                redisQueue,
		storage:              minioStorage,
		parser:               excelParser,
		builder:              hierarchyBuilder,
		pdfProcessor:         pdfProcessor,
		incrementalProcessor: incrementalProcessor,
	}, nil
}

func (w *RuleWorker) Start() error {
	log.Println("规则处理Worker启动中...")

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 设置信号处理
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 启动工作循环
	go w.workLoop(ctx)

	log.Println("规则处理Worker已启动，等待任务...")

	// 等待退出信号
	<-quit
	log.Println("正在关闭规则处理Worker...")

	// 关闭连接
	w.cleanup()

	log.Println("规则处理Worker已关闭")
	return nil
}

func (w *RuleWorker) workLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second) // 每2秒检查一次队列
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processTask(ctx)
		}
	}
}

func (w *RuleWorker) processTask(ctx context.Context) {
	// 从队列获取任务
	task, err := w.queue.DequeueTask("queue:rule")
	if err != nil {
		log.Printf("获取任务失败: %v", err)
		return
	}

	if task == nil {
		// 队列为空，继续等待
		return
	}

	log.Printf("开始处理规则任务: %s", task.ID)

	// 处理任务
	if err := w.handleRuleTask(ctx, task); err != nil {
		log.Printf("处理任务失败: %s, 错误: %v", task.ID, err)

		// 更新任务状态为失败
		w.queue.UpdateTaskStatus(task.ID, "failed", err.Error())

		// 更新数据库记录
		w.updateTaskInDB(ctx, task.ID, "failed", "", err.Error())
	} else {
		log.Printf("任务处理完成: %s", task.ID)
		// 调用llm 状态为llm语义话清洗
		// 		prompt := `你是一名数据清洗专家。我将给你一份列表，其中每个对象包含编码（code）、名称（name）及其他元数据。你的任务是根据以下规则，为每个唯一的编码（code）从其关联的名称列表中，选出最准确、最精炼的职业名称。

		// 请严格遵守以下规则进行判断：

		// 1.  **分组处理**：将列表中的数据按 code 字段进行分组。
		// 2.  **字体与语义组合判断**：
		//     * **优先选择**：如果一个 code 对应的多个 name 中，只有一个是完整的、名词性的职业或实体名称，且其字体为 **E-HZ**，那么这一个就是正确的名称。
		//     * **次要排除**：如果一个 code 下的名称包含"本小类包括下列职业"、"进行..."或"担任..."等描述性或动词性短语，且其字体是 **E-BZ**，则这些名称应被排除。它们是辅助性说明，不是最终的职业名称。
		//     * **完整性优先**：对于像"航天动力装置制造工"和"航天动力装置制造工程技术人员"这样的情况，如果"航天动力装置制造工程技术人员"是完整的，而另一个是截断的（根据文本内容判断），则优先选择完整的名称。
		// 3.  **最终输出**：以 code: name 的JSON格式输出最终确认的词表列表。

		// 请使用此方法处理以下JSON数据，并仅返回最终结果。`

		// 更新任务状态为完成
		w.queue.UpdateTaskStatus(task.ID, "completed", "")
	}
}

func (w *RuleWorker) handleRuleTask(ctx context.Context, task *queue.Task) error {
	startTime := time.Now()

	// 从数据库获取任务详情
	taskRecord, err := w.db.GetTask(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("获取任务记录失败: %w", err)
	}

	// 从存储下载输入文件
	inputReader, err := w.storage.DownloadFile(ctx, taskRecord.InputPath)
	if err != nil {
		return fmt.Errorf("下载输入文件失败: %w", err)
	}
	defer inputReader.Close()

	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "input_*.xlsx")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// 复制文件内容
	if _, err := tmpFile.ReadFrom(inputReader); err != nil {
		return fmt.Errorf("复制文件失败: %w", err)
	}
	tmpFile.Close()

	// 1. 解析Excel文件
	log.Printf("解析Excel文件: %s", taskRecord.InputPath)
	records, err := w.parser.ParseFile(ctx, tmpFile.Name())
	if err != nil {
		return fmt.Errorf("解析Excel失败: %w", err)
	}
	log.Printf("成功解析 %d 条记录", len(records))

	// 2. 构建层级结构
	log.Printf("构建层级结构...")
	categories, err := w.builder.Build(ctx, records)
	if err != nil {
		return fmt.Errorf("构建层级结构失败: %w", err)
	}
	log.Printf("成功构建 %d 个根分类", len(categories))

	// Debug: 打印构建的层级结构信息
	// for i, rootCat := range categories {
	// 	if i < 3 { // 只显示前3个根节点的详细信息
	// 		log.Printf("DEBUG: 根节点[%d] - Code=%s, Name=%s, Level=%s, 子节点数=%d",
	// 			i, rootCat.Code, rootCat.Name, rootCat.Level, len(rootCat.Children))
	// 		if len(rootCat.Children) > 0 {
	// 			for j, child := range rootCat.Children {
	// 				if j < 2 { // 每个根节点只显示前2个子节点
	// 					log.Printf("  子节点[%d] - Code=%s, Name=%s, Level=%s, 子节点数=%d",
	// 						j, child.Code, child.Name, child.Level, len(child.Children))
	// 				}
	// 			}
	// 		}
	// 	}
	// }

	// 3. 将层级结构保存到数据库
	log.Printf("正在将层级结构保存到数据库...")
	err = w.saveHierarchyToDB(ctx, task.ID, categories)
	if err != nil {
		return fmt.Errorf("保存层级结构到数据库失败: %w", err)
	}
	log.Printf("层级结构已成功保存")

	// 4. 更新数据库任务记录
	processingTime := time.Since(startTime)
	taskRecord.Status = "completed"
	resultMap := map[string]string{"status": "completed", "message": "Hierarchy saved to database"}
	resultJSON, _ := json.Marshal(resultMap)
	taskRecord.Result = datatypes.JSON(resultJSON)

	taskRecord.UpdatedAt = time.Now()
	now := time.Now()
	taskRecord.ProcessedAt = &now
	taskRecord.ProcessingLog = fmt.Sprintf("处理时间: %v, 结果已存入数据库", processingTime)

	if err := w.db.UpdateTask(ctx, taskRecord); err != nil {
		return fmt.Errorf("更新任务记录失败: %w", err)
	}

	// 5. 创建处理统计
	stats := &database.ProcessingStats{
		TaskID:           task.ID,
		TotalRecords:     len(records),
		ProcessedRecords: len(records), // 规则处理通常处理所有记录
		SkippedRecords:   0,
		ErrorRecords:     0,
		ProcessingTimeMs: processingTime.Milliseconds(),
		MemoryUsageMB:    0, // TODO: 实现内存使用监控
		CreatedAt:        time.Now(),
	}

	if err := w.db.CreateProcessingStats(ctx, stats); err != nil {
		log.Printf("警告：创建处理统计失败: %v", err) // 非致命错误
	}

	log.Printf("规则处理完成，耗时: %v", processingTime)

	// 6. 调用增量处理器进行5步流程处理（异步执行，不阻塞主流程）
	log.Printf("开始增量处理流程（PDF验证和LLM语义分析）...")
	go func() {
		// 使用独立的context，避免主任务context取消影响LLM处理
		llmCtx := context.Background()
		if err := w.incrementalProcessor.ProcessIncrementalFlow(llmCtx, task.ID, taskRecord.InputPath, categories); err != nil {
			log.Printf("警告：增量处理失败: %v", err)
		} else {
			log.Printf("增量处理流程完成")
		}
	}()
	log.Printf("增量处理已在后台启动")

	return nil
}

func (w *RuleWorker) saveHierarchyToDB(ctx context.Context, taskID string, categories []*model.Category) error {
	var allCategories []*database.Category
	processedCodes := make(map[string]bool) // 用于跟踪已处理的Code，防止重复
	var flatten func([]*model.Category, string, int)

	flatten = func(nodes []*model.Category, parentCode string, depth int) {
		log.Printf("DEBUG: flatten调用 - depth=%d, 节点数=%d, parentCode=%s", depth, len(nodes), parentCode)
		for i, node := range nodes {
			if node == nil {
				continue
			}

			log.Printf("DEBUG: 处理节点[%d] - Code=%s, Name=%s, Level=%s, 子节点数=%d",
				i, node.Code, node.Name, node.Level, len(node.Children))

			// 如果Code已经处理过，则跳过，防止违反数据库唯一约束
			if _, ok := processedCodes[node.Code]; ok {
				log.Printf("DEBUG: 跳过重复节点 - Code=%s", node.Code)
				continue
			}
			dbCategory := &database.Category{
				TaskID:     taskID,
				Code:       node.Code,
				Name:       node.Name,
				Level:      node.Level,
				ParentCode: parentCode,
				Status:     "excel_parsed",
				DataSource: "excel",
			}
			allCategories = append(allCategories, dbCategory)
			processedCodes[node.Code] = true // 标记为已处理

			log.Printf("DEBUG: 已添加节点到allCategories - Code=%s, allCategories长度=%d", node.Code, len(allCategories))

			if len(node.Children) > 0 {
				log.Printf("DEBUG: 递归处理子节点 - Code=%s, 子节点数=%d", node.Code, len(node.Children))
				flatten(node.Children, node.Code, depth+1)
			}
		}
	}

	log.Printf("DEBUG: 开始flatten处理 - 根节点数=%d", len(categories))
	flatten(categories, "", 0) // 根节点的父编码为空字符串

	if len(allCategories) == 0 {
		return nil // 没有需要插入的数据
	}

	// 使用版本化的批量插入方法，正确处理版本管理
	log.Printf("DEBUG: saveHierarchyToDB - 准备插入 %d 条记录", len(allCategories))

	// 检查第一条记录的字段设置情况
	if len(allCategories) > 0 {
		first := allCategories[0]
		log.Printf("DEBUG: 第一条记录字段 - Status: %s, DataSource: %s", first.Status, first.DataSource)
	}

	// 生成新的批次ID用于版本管理
	batchID := uuid.New().String()
	log.Printf("DEBUG: 生成新的批次ID: %s", batchID)

	// 使用版本化插入方法，这会正确处理is_current字段
	err := w.db.BatchInsertCategoriesWithVersion(ctx, taskID, batchID, allCategories)
	if err != nil {
		log.Printf("ERROR: BatchInsertCategoriesWithVersion 调用失败: %v", err)
	} else {
		log.Printf("DEBUG: BatchInsertCategoriesWithVersion 调用成功")
	}
	return err
}

func (w *RuleWorker) updateTaskInDB(ctx context.Context, taskID string, status string, result, errorMsg string) {
	task, err := w.db.GetTask(ctx, taskID)
	if err != nil {
		log.Printf("获取任务记录失败: %v", err)
		return
	}

	task.Status = status
	task.UpdatedAt = time.Now()
	if status == "completed" || status == "failed" {
		now := time.Now()
		task.ProcessedAt = &now
	}
	if result != "" {
		task.Result = datatypes.JSON(result)
	}
	if errorMsg != "" {
		task.ErrorMsg = errorMsg
	}

	if err := w.db.UpdateTask(ctx, task); err != nil {
		log.Printf("更新任务记录失败: %v", err)
	}
}

func (w *RuleWorker) cleanup() {
	if err := w.db.Close(); err != nil {
		log.Printf("关闭数据库失败: %v", err)
	}
	w.queue.Close()
}
