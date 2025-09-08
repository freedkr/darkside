package handlers

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/freedkr/moonshot/internal/database"
	"github.com/freedkr/moonshot/internal/queue"
	"github.com/freedkr/moonshot/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Handlers API处理器
type Handlers struct {
	db      database.DatabaseInterface
	queue   queue.Client
	storage storage.StorageInterface
}

// NewHandlers 创建处理器
func NewHandlers(db database.DatabaseInterface, queue queue.Client, storage storage.StorageInterface) *Handlers {
	return &Handlers{
		db:      db,
		queue:   queue,
		storage: storage,
	}
}

// CreateTaskRequest 创建任务请求
type CreateTaskRequest struct {
	Type     string                 `json:"type" binding:"required,oneof=rule ai"`
	Priority int                    `json:"priority"`
	Config   map[string]interface{} `json:"config"`
}

// CreateTaskResponse 创建任务响应
type CreateTaskResponse struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

// Health 健康检查
func (h *Handlers) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now(),
		"service":   "api-server",
	})
}

// Ready 就绪检查
func (h *Handlers) Ready(c *gin.Context) {
	ctx := c.Request.Context()

	// 检查数据库
	if err := h.db.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"reason": "database not available",
		})
		return
	}

	// TODO: 检查队列连接 - 当前Client接口不支持Ping方法
	// if err := h.queue.Ping(ctx); err != nil {
	//	c.JSON(http.StatusServiceUnavailable, gin.H{
	//		"status": "not ready",
	//		"reason": "queue not available",
	//	})
	//	return
	// }

	c.JSON(http.StatusOK, gin.H{
		"status":    "ready",
		"timestamp": time.Now(),
	})
}

// CreateTask 创建任务
func (h *Handlers) CreateTask(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	taskID := uuid.New().String()

	// 将 config map 序列化为 JSON
	var configJSON datatypes.JSON
	if req.Config != nil {
		configBytes, err := json.Marshal(req.Config)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的config格式"})
			return
		}
		configJSON = configBytes
	}

	// 创建任务记录
	task := &database.TaskRecord{
		ID:       taskID,
		Type:     req.Type,
		Status:   "pending",
		Priority: req.Priority,
		Config:   configJSON,
	}

	if err := h.db.CreateTask(ctx, task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建任务失败"})
		return
	}

	// 将任务加入队列
	queueTask := &queue.Task{
		ID:        taskID,
		Type:      req.Type,
		Data:      req.Config,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Status:    "pending",
	}

	if err := h.queue.EnqueueTaskWithContext(ctx, queueTask); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "任务入队失败"})
		return
	}

	c.JSON(http.StatusCreated, CreateTaskResponse{
		TaskID: taskID,
		Status: "pending",
	})
}

// GetTask 获取任务
func (h *Handlers) GetTask(c *gin.Context) {
	taskID := c.Param("id")
	ctx := c.Request.Context()

	task, err := h.db.GetTask(ctx, taskID)
	if err != nil {
		// 添加详细错误日志
		log.Printf("GetTask失败 - TaskID: %s, Error: %v", taskID, err)
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "任务不存在",
			"taskId":  taskID,
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, task)
}

// ListTasks 列出任务
func (h *Handlers) ListTasks(c *gin.Context) {
	ctx := c.Request.Context()

	// 解析分页参数
	limit := 20
	offset := 0

	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	tasks, err := h.db.ListTasks(ctx, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取任务列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks":  tasks,
		"limit":  limit,
		"offset": offset,
	})
}

// DeleteTask 删除任务
func (h *Handlers) DeleteTask(c *gin.Context) {
	taskID := c.Param("id")

	// TODO: 实现任务删除逻辑
	// 注意：需要考虑正在处理中的任务

	c.JSON(http.StatusOK, gin.H{
		"message": "任务删除功能待实现",
		"task_id": taskID,
	})
}

// UploadFile 上传文件并创建任务
func (h *Handlers) UploadFile(c *gin.Context) {
	ctx := c.Request.Context()

	// 解析文件
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid file upload: " + err.Error(),
		})
		return
	}
	defer file.Close()

	// 验证文件类型
	ext := filepath.Ext(header.Filename)
	if ext != ".xlsx" && ext != ".xls" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Only Excel files (.xlsx, .xls) are supported",
		})
		return
	}

	// 生成唯一ID
	fileID := uuid.New().String()
	taskID := uuid.New().String()

	// 计算MD5
	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "计算文件哈希失败"})
		return
	}
	md5Hash := fmt.Sprintf("%x", hash.Sum(nil))

	// 重置文件指针
	file.Seek(0, 0)

	// 生成存储路径
	objectName := fmt.Sprintf("uploads/%s/%s", fileID, header.Filename)

	// 上传到存储
	err = h.storage.UploadFile(ctx, objectName, file, header.Size, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to upload file to storage: " + err.Error(),
		})
		return
	}

	// 创建任务记录
	// 预先定义好输入和输出路径
	outputPath := fmt.Sprintf("results/%s/output.json", taskID)
	// 生成上传批次ID，用于版本管理
	uploadBatchID := uuid.New().String()

	task := &database.TaskRecord{
		ID:            taskID,
		Type:          "rule", // 默认使用规则处理
		Status:        "pending",
		Priority:      0,
		InputPath:     objectName,                   // 关联输入文件路径
		OutputPath:    outputPath,                   // 关联输出文件路径
		Config:        datatypes.JSON([]byte(`{}`)), // 为JSONB字段设置默认值
		UploadBatchID: uploadBatchID,                // 设置上传批次ID
	}

	if err := h.db.CreateTask(ctx, task); err != nil {
		// 清理已上传的文件
		h.storage.DeleteFile(ctx, objectName)
		log.Printf("CreateTask失败 - TaskID: %s, Error: %v", taskID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建任务失败"})
		return
	}
	log.Printf("CreateTask成功 - TaskID: %s", taskID)

	// 创建文件记录
	fileRecord := &database.FileRecord{
		ID:           fileID,
		OriginalName: header.Filename,
		StoragePath:  objectName,
		FileSize:     header.Size,
		ContentType:  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		MD5Hash:      md5Hash,
		TaskID:       taskID, // 确保文件记录关联到正确的任务ID
		CreatedAt:    time.Now(),
	}

	if err := h.db.CreateFile(ctx, fileRecord); err != nil {
		// 删除已上传的文件和任务
		h.storage.DeleteFile(ctx, objectName)
		h.db.DeleteTask(ctx, taskID) // 补偿：删除已创建的任务记录
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建文件记录失败"})
		return
	}

	// 创建多个任务：Excel处理 + PDF处理

	// 1. Excel处理任务
	excelTask := &queue.Task{
		ID:   taskID,
		Type: "rule",
		Data: map[string]interface{}{
			"file_id":         fileID,
			"file_name":       header.Filename,
			"object_name":     objectName,
			"operation":       "excel_processing",
			"upload_batch_id": uploadBatchID, // 传递上传批次ID给工作节点
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Status:    "pending",
	}

	if err := h.queue.EnqueueTaskWithContext(ctx, excelTask); err != nil {
		// 补偿：删除文件和任务
		h.storage.DeleteFile(ctx, objectName)
		h.db.DeleteTask(ctx, taskID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Excel任务入队失败"})
		return
	}

	// 2. PDF处理任务（触发事件，使用固定PDF文件）
	pdfTaskID := fmt.Sprintf("%s-pdf", taskID)
	pdfTask := &queue.Task{
		ID:   pdfTaskID,
		Type: "pdf",
		Data: map[string]interface{}{
			"parent_task_id":  taskID,
			"trigger_event":   "excel_uploaded",
			"operation":       "pdf_processing",
			"pdf_source":      "fixed_test_pdf", // 使用固定的PDF文件
			"upload_batch_id": uploadBatchID,    // 传递上传批次ID给PDF工作节点
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Status:    "pending",
	}

	if err := h.queue.EnqueueTaskWithContext(ctx, pdfTask); err != nil {
		// 补偿：删除文件和任务
		h.storage.DeleteFile(ctx, objectName)
		h.db.DeleteTask(ctx, taskID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "PDF任务入队失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"taskId":  taskID,
		"fileId":  fileID,
		"message": "File uploaded and task created successfully",
	})
}

// FlatCategory 定义了用于API响应的扁平化分类结构。
// 这种结构对前端更友好，便于快速渲染和处理大型数据集。
type FlatCategory struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Level       string `json:"level"`
	ParentCode  string `json:"parent_code"`
	HasChildren bool   `json:"has_children"` // 是否有子节点，用于前端展开/收起功能
	HasLLM      bool   `json:"has_llm"`      // 是否有LLM增强数据
	HasPDF      bool   `json:"has_pdf"`      // 是否有PDF信息数据
}

// DownloadFile 下载文件
func (h *Handlers) DownloadFile(c *gin.Context) {
	objectName := c.Query("path")
	if objectName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 path 参数"})
		return
	}

	reader, err := h.storage.DownloadFile(c.Request.Context(), objectName)
	if err != nil {
		log.Printf("下载文件失败: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "文件未找到或无法下载"})
		return
	}
	defer reader.Close()

	// 从路径中提取原始文件名
	fileName := filepath.Base(objectName)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	c.Header("Content-Type", "application/octet-stream")

	_, err = io.Copy(c.Writer, reader)
	if err != nil {
		log.Printf("写入响应流失败: %v", err)
		// 此时可能已经写入部分响应头，所以不再发送JSON错误
	}
}

// DownloadResultByTaskID 根据任务ID下载处理结果
func (h *Handlers) DownloadResultByTaskID(c *gin.Context) {
	taskID := c.Query("task_id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 task_id 参数"})
		return
	}

	// 1. 检查任务是否存在且已完成
	task, err := h.db.GetTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务未找到"})
		return
	}
	if task.Status != "completed" {
		c.JSON(http.StatusAccepted, gin.H{"error": "任务尚未完成", "status": task.Status})
		return
	}

	// 2. 从 'categories' 表获取与任务关联的当前版本分类数据
	dbCategories, err := h.db.GetCurrentCategoriesByTaskID(c.Request.Context(), taskID)
	if err != nil {
		log.Printf("获取任务 %s 的当前版本分类数据失败: %v", taskID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取结果数据失败"})
		return
	}

	// 3. 将数据库模型转换为API的DTO，确保JSON字段为小写
	flatCategories := make([]FlatCategory, len(dbCategories))
	for i, dbCat := range dbCategories {
		flatCategories[i] = FlatCategory{
			Code:       dbCat.Code,
			Name:       dbCat.Name,
			Level:      dbCat.Level,
			ParentCode: dbCat.ParentCode,
		}
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="task_%s_result.json"`, taskID))
	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusOK, flatCategories)
}

// DeleteFile 删除文件
func (h *Handlers) DeleteFile(c *gin.Context) {
	fileID := c.Param("id")

	// TODO: 实现文件删除逻辑

	c.JSON(http.StatusOK, gin.H{
		"message": "文件删除功能待实现",
		"file_id": fileID,
	})
}

// GetStats 获取统计信息
func (h *Handlers) GetStats(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"queues": gin.H{
			"rule_queue_length": 0, // TODO: implement queue length stats
			"ai_queue_length":   0, // TODO: implement queue length stats
		},
		"timestamp": time.Now(),
	})
}

// GetQueueStats 获取队列统计
func (h *Handlers) GetQueueStats(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"rule_queue": gin.H{
			"length": 0, // TODO: implement queue length stats
			"type":   "rule",
		},
		"ai_queue": gin.H{
			"length": 0, // TODO: implement queue length stats
			"type":   "ai",
		},
	})
}

// GetTaskVersionHistory 获取任务的版本历史
func (h *Handlers) GetTaskVersionHistory(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 task_id 参数"})
		return
	}

	// 获取版本历史
	versionHistory, err := h.db.GetCategoryVersionHistory(c.Request.Context(), taskID)
	if err != nil {
		log.Printf("获取任务 %s 的版本历史失败: %v", taskID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取版本历史失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task_id":  taskID,
		"versions": versionHistory,
	})
}

// GetVersionCategories 获取指定版本的分类数据
func (h *Handlers) GetVersionCategories(c *gin.Context) {
	batchID := c.Query("batch_id")
	if batchID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 batch_id 参数"})
		return
	}

	// 获取指定批次的分类数据
	dbCategories, err := h.db.GetCategoriesByBatchID(c.Request.Context(), batchID)
	if err != nil {
		log.Printf("获取批次 %s 的分类数据失败: %v", batchID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取分类数据失败"})
		return
	}

	// 转换为API DTO
	flatCategories := make([]FlatCategory, len(dbCategories))
	for i, dbCat := range dbCategories {
		// 对于版本分类查询，暂时不计算HasChildren以提高性能
		// 如果需要可以加上: hasChildren := h.hasChildren(ctx, "", batchID, dbCat.Code)
		flatCategories[i] = FlatCategory{
			Code:        dbCat.Code,
			Name:        dbCat.Name,
			Level:       dbCat.Level,
			ParentCode:  dbCat.ParentCode,
			HasChildren: false, // 暂时设为false，提高性能
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"batch_id":   batchID,
		"categories": flatCategories,
		"count":      len(flatCategories),
	})
}

// GetAllStructuredData 获取指定版本的所有结构化数据（包含完整骨架）
func (h *Handlers) GetAllStructuredData(c *gin.Context) {
	taskID := c.Query("task_id")
	version := c.Query("version")
	parentCode := c.Query("parent_code") // 新增：接收父节点ID

	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 task_id 参数"})
		return
	}

	ctx := c.Request.Context()

	var dbCategories []*database.Category
	var err error

	// 根据有无 parentCode 决定查询逻辑
	if parentCode != "" {
		// 按父节点查询子节点
		dbCategories, err = h.db.GetChildrenByParentCode(ctx, taskID, version, parentCode)
	} else {
		// 按版本获取数据（保持原有逻辑，但可以优化为只获取顶层）
		if version != "" {
			// 获取指定版本数据
			dbCategories, err = h.db.GetCategoriesByBatchID(ctx, version)
			log.Printf("指定版本 %s 返回 %d 条记录", version, len(dbCategories))
		} else {
			// 获取最新完整版本，而不是简单的 is_current=true
			dbCategories, err = h.getLatestCompleteVersion(ctx, taskID)
		}
	}

	if err != nil {
		log.Printf("获取任务 %s 的结构化数据失败: %v", taskID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取结构化数据失败"})
		return
	}

	// 如果是按父节点查询，直接返回扁平数据即可
	if parentCode != "" {
		flatCategories := make([]FlatCategory, len(dbCategories))
		for i, dbCat := range dbCategories {
			// 计算是否有子节点
			hasChildren := h.hasChildren(ctx, taskID, version, dbCat.Code)
			
			// 检查是否有LLM增强数据和PDF信息
			hasLLM := dbCat.LLMEnhancements != ""
			hasPDF := dbCat.PDFInfo != ""
			
			flatCategories[i] = FlatCategory{
				Code:        dbCat.Code,
				Name:        dbCat.Name,
				Level:       dbCat.Level,
				ParentCode:  dbCat.ParentCode,
				HasChildren: hasChildren,
				HasLLM:      hasLLM,
				HasPDF:      hasPDF,
			}
		}
		c.JSON(http.StatusOK, gin.H{"flat_data": flatCategories})
		return
	}

	// 转换为API DTO格式
	flatCategories := make([]FlatCategory, len(dbCategories))
	for i, dbCat := range dbCategories {
		// 计算是否有子节点
		hasChildren := h.hasChildren(ctx, taskID, version, dbCat.Code)
		
		// 检查是否有LLM增强数据和PDF信息
		hasLLM := dbCat.LLMEnhancements != ""
		hasPDF := dbCat.PDFInfo != ""
		
		flatCategories[i] = FlatCategory{
			Code:        dbCat.Code,
			Name:        dbCat.Name,
			Level:       dbCat.Level,
			ParentCode:  dbCat.ParentCode,
			HasChildren: hasChildren,
			HasLLM:      hasLLM,
			HasPDF:      hasPDF,
		}
	}

	// 构建层级结构
	hierarchicalData := h.buildHierarchicalStructure(flatCategories)

	c.JSON(http.StatusOK, gin.H{
		"task_id":           taskID,
		"version":           version,
		"flat_data":         flatCategories,
		"hierarchical_data": hierarchicalData,
		"total_count":       len(flatCategories),
		"skeleton_info": gin.H{
			"has_skeleton":       true,
			"complete_structure": true,
		},
	})
}

// buildHierarchicalStructure 构建层级结构
func (h *Handlers) buildHierarchicalStructure(categories []FlatCategory) interface{} {
	// 创建映射表
	categoryMap := make(map[string]*FlatCategory)
	for i := range categories {
		categoryMap[categories[i].Code] = &categories[i]
	}

	// 构建树形结构
	var rootNodes []FlatCategory

	for _, category := range categories {
		if category.ParentCode == "" || categoryMap[category.ParentCode] == nil {
			// 根节点
			rootNodes = append(rootNodes, category)
		}
	}

	return gin.H{
		"tree_structure": rootNodes,
		"statistics": gin.H{
			"total_nodes": len(categories),
			"root_nodes":  len(rootNodes),
		},
	}
}

// GetRecentTasks 获取最近的任务列表
func (h *Handlers) GetRecentTasks(c *gin.Context) {
	ctx := c.Request.Context()

	// 获取limit参数，默认为10
	limit := 10
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	// 获取最近的任务，按创建时间倒序排列
	tasks, err := h.db.ListTasks(ctx, limit, 0)
	if err != nil {
		log.Printf("获取最近任务列表失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取任务列表失败"})
		return
	}

	// 转换为简化的任务信息
	recentTasks := make([]gin.H, len(tasks))
	for i, task := range tasks {
		recentTasks[i] = gin.H{
			"task_id":         task.ID,
			"type":            task.Type,
			"status":          task.Status,
			"created_at":      task.CreatedAt,
			"updated_at":      task.UpdatedAt,
			"upload_batch_id": task.UploadBatchID,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"recent_tasks": recentTasks,
		"count":        len(recentTasks),
		"limit":        limit,
	})
}

// hasChildren 检查指定节点是否有子节点
func (h *Handlers) hasChildren(ctx context.Context, taskID string, version string, code string) bool {
	var count int64
	
	// 需要通过类型断言获取底层的*gorm.DB
	pgDB, ok := h.db.(*database.PostgreSQLDB)
	if !ok {
		return false
	}
	
	query := pgDB.GetDB().WithContext(ctx).Model(&database.Category{}).
		Where("task_id = ? AND parent_code = ?", taskID, code)
	
	if version != "" {
		// 如果指定了版本，则查询该版本下的子节点
		query = query.Where("upload_batch_id = ?", version)
	} else {
		// 否则，查询最新完整版本的子节点
		// 先获取最新完整版本的batch_id
		versionHistory, err := h.db.GetCategoryVersionHistory(ctx, taskID)
		if err == nil {
			var latestCompleteVersion *database.CategoryVersion
			for _, v := range versionHistory {
				if v.RecordCount > 1000 {
					if latestCompleteVersion == nil || v.UploadTimestamp.After(latestCompleteVersion.UploadTimestamp) {
						latestCompleteVersion = v
					}
				}
			}
			if latestCompleteVersion != nil {
				query = query.Where("upload_batch_id = ?", latestCompleteVersion.UploadBatchID)
			} else {
				query = query.Where("is_current = ?", true) // 降级
			}
		} else {
			query = query.Where("is_current = ?", true) // 降级
		}
	}
	
	query.Count(&count)
	return count > 0
}

// getLatestCompleteVersion 获取最新的完整版本（记录数量 > 1000）
func (h *Handlers) getLatestCompleteVersion(ctx context.Context, taskID string) ([]*database.Category, error) {
	// 1. 获取版本历史
	versionHistory, err := h.db.GetCategoryVersionHistory(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("获取版本历史失败: %w", err)
	}

	// 2. 找到最新的完整版本（记录数量 > 1000）
	var latestCompleteVersion *database.CategoryVersion
	for _, version := range versionHistory {
		if version.RecordCount > 1000 { // 只考虑完整版本
			if latestCompleteVersion == nil || version.UploadTimestamp.After(latestCompleteVersion.UploadTimestamp) {
				latestCompleteVersion = version
			}
		}
	}

	// 3. 如果没有找到完整版本，降级到 is_current=true 的版本
	if latestCompleteVersion == nil {
		log.Printf("WARNING: 没有找到完整版本，降级使用 is_current=true 版本")
		return h.db.GetCurrentCategoriesByTaskID(ctx, taskID)
	}

	// 4. 获取该版本的数据
	log.Printf("使用最新完整版本: %s (记录数: %d)", latestCompleteVersion.UploadBatchID, latestCompleteVersion.RecordCount)
	return h.db.GetCategoriesByBatchID(ctx, latestCompleteVersion.UploadBatchID)
}
