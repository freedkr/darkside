package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/freedkr/moonshot/internal/config"
)

type Client interface {
	EnqueueTask(task *Task) error
	EnqueueTaskWithContext(ctx context.Context, task *Task) error
	DequeueTask(queueName string) (*Task, error)
	GetTaskStatus(taskID string) (*Task, error)
	UpdateTaskStatus(taskID string, status string, error string) error
	UpdateTaskResult(taskID string, resultObjectName string) error
	Close()
}

type Task struct {
	ID               string                 `json:"id"`
	Type             string                 `json:"type"`
	FileID           string                 `json:"file_id"`
	FileName         string                 `json:"file_name"`
	ObjectName       string                 `json:"object_name"`
	Status           string                 `json:"status"` // pending, processing, completed, failed
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	Error            string                 `json:"error,omitempty"`
	ResultObjectName string                 `json:"result_object_name,omitempty"`
	ProcessorID      string                 `json:"processor_id,omitempty"`
	Data             map[string]interface{} `json:"data,omitempty"`
}

type redisClient struct {
	client *redis.Client
	ctx    context.Context
}

func NewRedisQueue(qcfg config.QueueConfig) (Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     qcfg.Addr,
		Password: qcfg.Password,
		DB:       qcfg.DB,
	})
	ctx := context.Background()

	// 测试连接
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %v", err)
	}

	return &redisClient{
		client: rdb,
		ctx:    ctx,
	}, nil
}

func (c *redisClient) EnqueueTask(task *Task) error {
	// 序列化任务
	taskJSON, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %v", err)
	}

	// 保存任务状态
	taskKey := fmt.Sprintf("task:%s", task.ID)
	err = c.client.Set(c.ctx, taskKey, taskJSON, 24*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to save task: %v", err)
	}

	// 根据任务类型选择队列
	queueName := c.getQueueName(task.Type)

	// 添加到队列
	err = c.client.LPush(c.ctx, queueName, task.ID).Err()
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %v", err)
	}

	return nil
}

func (c *redisClient) EnqueueTaskWithContext(ctx context.Context, task *Task) error {
	// 序列化任务
	taskJSON, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %v", err)
	}

	// 保存任务状态
	taskKey := fmt.Sprintf("task:%s", task.ID)
	err = c.client.Set(ctx, taskKey, taskJSON, 24*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to save task: %v", err)
	}

	// 根据任务类型选择队列
	queueName := c.getQueueName(task.Type)

	// 添加到队列
	err = c.client.LPush(ctx, queueName, task.ID).Err()
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %v", err)
	}

	return nil
}

func (c *redisClient) DequeueTask(queueName string) (*Task, error) {
	// 阻塞式从队列获取任务ID
	result, err := c.client.BRPop(c.ctx, 5*time.Second, queueName).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // 没有任务
		}
		return nil, fmt.Errorf("failed to dequeue task: %v", err)
	}

	if len(result) != 2 {
		return nil, fmt.Errorf("unexpected redis result format")
	}

	taskID := result[1]

	// 获取任务详情
	return c.GetTaskStatus(taskID)
}

func (c *redisClient) GetTaskStatus(taskID string) (*Task, error) {
	taskKey := fmt.Sprintf("task:%s", taskID)

	taskJSON, err := c.client.Get(c.ctx, taskKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("task not found")
		}
		return nil, fmt.Errorf("failed to get task: %v", err)
	}

	var task Task
	err = json.Unmarshal([]byte(taskJSON), &task)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal task: %v", err)
	}

	return &task, nil
}

func (c *redisClient) UpdateTaskStatus(taskID string, status string, errorMsg string) error {
	task, err := c.GetTaskStatus(taskID)
	if err != nil {
		return err
	}

	task.Status = status
	task.UpdatedAt = time.Now()
	if errorMsg != "" {
		task.Error = errorMsg
	}

	return c.saveTask(task)
}

func (c *redisClient) UpdateTaskResult(taskID string, resultObjectName string) error {
	task, err := c.GetTaskStatus(taskID)
	if err != nil {
		return err
	}

	task.Status = "completed"
	task.UpdatedAt = time.Now()
	task.ResultObjectName = resultObjectName

	return c.saveTask(task)
}

func (c *redisClient) saveTask(task *Task) error {
	taskJSON, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %v", err)
	}

	taskKey := fmt.Sprintf("task:%s", task.ID)
	err = c.client.Set(c.ctx, taskKey, taskJSON, 24*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to save task: %v", err)
	}

	return nil
}

func (c *redisClient) getQueueName(taskType string) string {
	switch taskType {
	case "excel_processing":
		return "queue:excel"
	case "ai_processing":
		return "queue:ai"
	case "rule":
		return "queue:rule"
	case "pdf":
		return "queue:pdf"
	case "llm-batch":
		return "queue:llm-batch"
	case "merger":
		return "queue:merger"
	case "semantic":
		return "queue:semantic"
	default:
		return "queue:default"
	}
}

func (c *redisClient) Close() {
	c.client.Close()
}
