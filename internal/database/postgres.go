package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// PostgreSQLConfig PostgreSQL配置
type PostgreSQLConfig struct {
	Host            string        `yaml:"host" env:"POSTGRES_HOST" default:"localhost"`
	Port            int           `yaml:"port" env:"POSTGRES_PORT" default:"5432"`
	Database        string        `yaml:"database" env:"POSTGRES_DB" default:"moonshot"`
	Username        string        `yaml:"username" env:"POSTGRES_USER" default:"postgres"`
	Password        string        `yaml:"password" env:"POSTGRES_PASSWORD" default:""`
	SSLMode         string        `yaml:"ssl_mode" env:"POSTGRES_SSLMODE" default:"disable"`
	Schema          string        `yaml:"schema" env:"POSTGRES_SCHEMA" default:"moonshot"`
	MaxOpenConns    int           `yaml:"max_open_conns" env:"POSTGRES_MAX_OPEN_CONNS" default:"25"`
	MaxIdleConns    int           `yaml:"max_idle_conns" env:"POSTGRES_MAX_IDLE_CONNS" default:"5"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" env:"POSTGRES_CONN_MAX_LIFETIME" default:"5m"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" env:"POSTGRES_CONN_MAX_IDLE_TIME" default:"5m"`
	BatchSize       int           `yaml:"batch_size" env:"POSTGRES_BATCH_SIZE" default:"100"`
}

// PostgreSQLDB PostgreSQL数据库
type PostgreSQLDB struct {
	db     *gorm.DB
	config *PostgreSQLConfig
}

// NewPostgreSQLDB 创建PostgreSQL数据库连接
func NewPostgreSQLDB(config *PostgreSQLConfig) (*PostgreSQLDB, error) {
	// 如果schema为空，使用默认值
	if config.Schema == "" {
		config.Schema = "moonshot"
		log.Printf("WARNING: Schema was empty, using default: moonshot")
	}

	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s search_path=%s",
		config.Host, config.Port, config.Username, config.Password, config.Database, config.SSLMode, config.Schema)

	gormConfig := &gorm.Config{}
	if log.Default().Writer() == os.Stdout {
		gormConfig.Logger = logger.Default.LogMode(logger.Info)
	}

	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	// 确保设置正确的schema search_path
	if err := db.Exec(fmt.Sprintf("SET search_path TO %s", config.Schema)).Error; err != nil {
		return nil, fmt.Errorf("设置schema失败: %w", err)
	}

	// 验证search_path是否设置成功
	var currentSearchPath string
	if err := db.Raw("SHOW search_path").Scan(&currentSearchPath).Error; err != nil {
		log.Printf("WARNING: 无法验证search_path: %v", err)
	} else {
		log.Printf("DEBUG: Current search_path: %s", currentSearchPath)
	}
	// 设置连接池参数
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接池失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(config.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	// 测试连接
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("数据库ping失败: %w", err)
	}

	return &PostgreSQLDB{
		db:     db,
		config: config,
	}, nil
}

// CreateTables 创建表结构
func (p *PostgreSQLDB) CreateTables(ctx context.Context) error {
	// 使用 GORM 的 AutoMigrate 功能
	err := p.db.WithContext(ctx).AutoMigrate(
		&TaskRecord{},
		&FileRecord{},
		&ProcessingStats{},
		&Category{},
		&PDFResult{},
	)
	if err != nil {
		return fmt.Errorf("自动迁移失败: %w", err)
	}
	return nil
}

// CreateTask 创建任务
func (p *PostgreSQLDB) CreateTask(ctx context.Context, task *TaskRecord) error {
	result := p.db.WithContext(ctx).Create(task)
	err := result.Error
	if err != nil {
		log.Printf("[SQL ERROR] CreateTask failed: %v", err)
		return fmt.Errorf("创建任务失败: %w", err)
	}
	return nil
}

// GetTask 获取任务
func (p *PostgreSQLDB) GetTask(ctx context.Context, taskID string) (*TaskRecord, error) {
	var task TaskRecord
	result := p.db.WithContext(ctx).First(&task, "id = ?", taskID)
	err := result.Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("任务不存在: %s", taskID)
		}
		return nil, fmt.Errorf("获取任务失败: %w", err)
	}

	return &task, nil
}

// UpdateTask 更新任务
func (p *PostgreSQLDB) UpdateTask(ctx context.Context, task *TaskRecord) error {
	result := p.db.WithContext(ctx).Save(task)
	err := result.Error
	if err != nil {
		log.Printf("[SQL ERROR] UpdateTask failed: %v", err)
		return fmt.Errorf("更新任务失败: %w", err)
	}
	return nil
}

// DeleteTask 删除任务
func (p *PostgreSQLDB) DeleteTask(ctx context.Context, taskID string) error {
	result := p.db.WithContext(ctx).Delete(&TaskRecord{}, "id = ?", taskID)
	err := result.Error
	if err != nil {
		return fmt.Errorf("删除任务失败: %w", err)
	}
	return nil
}

// ListTasks 列出任务
func (p *PostgreSQLDB) ListTasks(ctx context.Context, limit, offset int) ([]*TaskRecord, error) {
	var tasks []*TaskRecord
	result := p.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Offset(offset).Find(&tasks)
	err := result.Error
	if err != nil {
		return nil, fmt.Errorf("列出任务失败: %w", err)
	}

	return tasks, nil
}

// CreateFile 创建文件记录
func (p *PostgreSQLDB) CreateFile(ctx context.Context, file *FileRecord) error {
	result := p.db.WithContext(ctx).Create(file)
	err := result.Error
	if err != nil {
		return fmt.Errorf("创建文件记录失败: %w", err)
	}

	return nil
}

// CreateProcessingStats 创建处理统计
func (p *PostgreSQLDB) CreateProcessingStats(ctx context.Context, stats *ProcessingStats) error {
	result := p.db.WithContext(ctx).Create(stats)
	err := result.Error
	if err != nil {
		return fmt.Errorf("创建处理统计失败: %w", err)
	}

	return nil
}

// GetCategoriesByTaskID 根据任务ID获取所有分类
// 这个方法会返回一个扁平化的列表，包含前端渲染所需的 code, name, level, 和 parent_code 字段。
func (db *PostgreSQLDB) GetCategoriesByTaskID(ctx context.Context, taskID string) ([]*Category, error) {
	var categories []*Category

	// 查询与 taskID 关联的所有分类记录，并选择必要的字段
	// 为了性能，我们只选择需要的字段，并按 code 排序以保证前端处理时的一致性。
	err := db.db.WithContext(ctx).
		Model(&Category{}).
		Select("code", "name", "level", "parent_code").
		Where("task_id = ?", taskID).
		Order("code ASC").
		Find(&categories).Error

	return categories, err
}

// Close 关闭数据库连接
func (p *PostgreSQLDB) Close() error {
	sqlDB, err := p.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Ping 测试连接
func (p *PostgreSQLDB) Ping(ctx context.Context) error {
	sqlDB, err := p.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// GetDB 获取原始数据库连接
func (p *PostgreSQLDB) GetDB() *gorm.DB {
	return p.db
}

// GetChildrenByParentCode 根据父节点ID获取其直接子节点
func (p *PostgreSQLDB) GetChildrenByParentCode(ctx context.Context, taskID string, version string, parentCode string) ([]*Category, error) {
	var categories []*Category
	query := p.db.WithContext(ctx).Where("task_id = ? AND parent_code = ?", taskID, parentCode)

	if version != "" {
		// 如果指定了版本 (upload_batch_id)，则查询该版本下的子节点
		query = query.Where("upload_batch_id = ?", version)
	} else {
		// 否则，查询当前激活版本的子节点
		query = query.Where("is_current = ?", true)
	}

	// 按 code 排序以保证前端展示顺序一致
	err := query.Order("code asc").Find(&categories).Error
	if err != nil {
		return nil, fmt.Errorf("获取父节点 %s 的子节点失败: %w", parentCode, err)
	}

	// HasChildren 字段的计算将在API层处理，这里不再设置

	return categories, nil
}

// ======================= 兼容性方法（为旧代码提供版本化支持）=======================

// BatchInsertCategories 批量插入分类数据（兼容性方法，自动设置版本化字段）
func (p *PostgreSQLDB) BatchInsertCategories(ctx context.Context, categories []*Category) error {
	log.Printf("DEBUG: BatchInsertCategories 开始处理 %d 条记录", len(categories))

	if len(categories) == 0 {
		log.Printf("DEBUG: 没有数据需要插入")
		return nil
	}

	// 打印前几条记录的详细信息
	log.Printf("DEBUG: 前3条待插入记录:")
	for i, cat := range categories {
		if i >= 3 {
			break
		}
		log.Printf("  %d: Code=%s, Name=%s, Level=%s, TaskID=%s", i, cat.Code, cat.Name, cat.Level, cat.TaskID)
	}

	// 为每个Category设置版本化字段的默认值 - 使用UUID格式
	batchID := uuid.New().String()
	currentTime := time.Now()
	log.Printf("DEBUG: 生成批次ID (UUID): %s", batchID)

	processedCount := 0
	for _, cat := range categories {
		if cat.UploadBatchID == "" {
			cat.UploadBatchID = batchID
		}
		if cat.UploadTimestamp.IsZero() {
			cat.UploadTimestamp = currentTime
		}
		if !cat.IsCurrent {
			cat.IsCurrent = true // 默认为当前版本
		}
		processedCount++
	}
	log.Printf("DEBUG: 已设置 %d 条记录的版本化字段", processedCount)

	// 使用标准批量插入
	log.Printf("DEBUG: 开始数据库插入，批次大小: %d", p.config.BatchSize)
	err := p.db.WithContext(ctx).CreateInBatches(categories, p.config.BatchSize).Error
	if err != nil {
		log.Printf("ERROR: 批量插入失败: %v", err)
		return fmt.Errorf("批量插入分类失败: %w", err)
	}

	log.Printf("DEBUG: 批量插入成功完成")

	// 验证插入结果
	var count int64
	err = p.db.WithContext(ctx).Model(&Category{}).Where("upload_batch_id = ?", batchID).Count(&count).Error
	if err != nil {
		log.Printf("WARNING: 无法验证插入结果: %v", err)
	} else {
		log.Printf("DEBUG: 验证插入结果 - 数据库中实际插入了 %d 条记录", count)
	}

	return nil
}

// ======================= 版本管理相关方法 =======================

// GetCurrentCategoriesByTaskID 获取任务的当前版本分类数据
func (p *PostgreSQLDB) GetCurrentCategoriesByTaskID(ctx context.Context, taskID string) ([]*Category, error) {
	var categories []*Category
	err := p.db.WithContext(ctx).
		Model(&Category{}).
		Where("task_id = ? AND is_current = true", taskID).
		Order("code ASC").
		Find(&categories).Error
	if err != nil {
		return nil, fmt.Errorf("获取当前版本分类失败: %w", err)
	}
	return categories, nil
}

// GetCategoriesByBatchID 根据批次ID获取分类数据
func (p *PostgreSQLDB) GetCategoriesByBatchID(ctx context.Context, batchID string) ([]*Category, error) {
	var categories []*Category
	err := p.db.WithContext(ctx).
		Model(&Category{}).
		Where("upload_batch_id = ?", batchID).
		Order("code ASC").
		Find(&categories).Error
	if err != nil {
		return nil, fmt.Errorf("根据批次ID获取分类失败: %w", err)
	}
	return categories, nil
}

// BatchInsertCategoriesWithVersion 批量插入分类数据（支持版本管理）
func (p *PostgreSQLDB) BatchInsertCategoriesWithVersion(ctx context.Context, taskID, batchID string, categories []*Category) error {
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 将现有的当前版本标记为历史版本
		err := tx.Model(&Category{}).
			Where("task_id = ? AND is_current = true", taskID).
			Update("is_current", false).Error
		if err != nil {
			return fmt.Errorf("标记历史版本失败: %w", err)
		}

		// 2. 设置新记录的版本信息
		currentTime := time.Now()
		for _, cat := range categories {
			cat.UploadBatchID = batchID
			cat.UploadTimestamp = currentTime
			cat.IsCurrent = true
		}

		// 3. 批量插入新的当前版本数据
		if err := p.db.WithContext(ctx).Omit("id").CreateInBatches(categories, p.config.BatchSize).Error; err != nil {
			return fmt.Errorf("批量插入版本化分类失败: %w", err)
		}

		return nil
	})
}

// MarkPreviousVersionsAsOld 将之前的版本标记为非当前版本
func (p *PostgreSQLDB) MarkPreviousVersionsAsOld(ctx context.Context, taskID string) error {
	err := p.db.WithContext(ctx).
		Model(&Category{}).
		Where("task_id = ? AND is_current = true", taskID).
		Update("is_current", false).Error
	if err != nil {
		return fmt.Errorf("标记历史版本失败: %w", err)
	}
	return nil
}

// GetCategoryVersionHistory 获取分类的版本历史
func (p *PostgreSQLDB) GetCategoryVersionHistory(ctx context.Context, taskID string) ([]*CategoryVersion, error) {
	var versions []*CategoryVersion

	// 使用原生SQL查询来获取版本统计信息
	rows, err := p.db.WithContext(ctx).Raw(`
		SELECT 
			upload_batch_id, 
			upload_timestamp, 
			COUNT(*) as record_count,
			is_current
		FROM categories 
		WHERE task_id = ? 
		GROUP BY upload_batch_id, upload_timestamp, is_current 
		ORDER BY upload_timestamp DESC
	`, taskID).Rows()

	if err != nil {
		return nil, fmt.Errorf("获取版本历史失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var version CategoryVersion
		err := rows.Scan(
			&version.UploadBatchID,
			&version.UploadTimestamp,
			&version.RecordCount,
			&version.IsCurrent,
		)
		if err != nil {
			return nil, fmt.Errorf("扫描版本历史记录失败: %w", err)
		}
		versions = append(versions, &version)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历版本历史记录失败: %w", err)
	}

	return versions, nil
}

// DatabaseInterface 数据库接口
type DatabaseInterface interface {
	CreateTables(ctx context.Context) error
	CreateTask(ctx context.Context, task *TaskRecord) error
	GetTask(ctx context.Context, taskID string) (*TaskRecord, error)
	UpdateTask(ctx context.Context, task *TaskRecord) error
	ListTasks(ctx context.Context, limit, offset int) ([]*TaskRecord, error)
	DeleteTask(ctx context.Context, taskID string) error
	CreateFile(ctx context.Context, file *FileRecord) error
	CreateProcessingStats(ctx context.Context, stats *ProcessingStats) error
	GetCategoriesByTaskID(ctx context.Context, taskID string) ([]*Category, error)
	BatchInsertCategories(ctx context.Context, categories []*Category) error
	GetChildrenByParentCode(ctx context.Context, taskID string, version string, parentCode string) ([]*Category, error)

	// 版本管理相关方法
	GetCurrentCategoriesByTaskID(ctx context.Context, taskID string) ([]*Category, error)
	GetCategoriesByBatchID(ctx context.Context, batchID string) ([]*Category, error)
	BatchInsertCategoriesWithVersion(ctx context.Context, taskID, batchID string, categories []*Category) error
	MarkPreviousVersionsAsOld(ctx context.Context, taskID string) error
	GetCategoryVersionHistory(ctx context.Context, taskID string) ([]*CategoryVersion, error)

	Close() error
	Ping(ctx context.Context) error
}
