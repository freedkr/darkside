// Package scheduler 优先级队列实现
package scheduler

import (
	"container/heap"
	"fmt"
	"sync"

	"github.com/freedkr/moonshot/services/llm-service/internal/models"
)

// PriorityQueue 优先级队列
type PriorityQueue struct {
	items    TaskHeap
	maxSize  int
	mutex    sync.RWMutex
}

// NewPriorityQueue 创建新的优先级队列
func NewPriorityQueue(maxSize int) *PriorityQueue {
	pq := &PriorityQueue{
		items:   make(TaskHeap, 0),
		maxSize: maxSize,
	}
	heap.Init(&pq.items)
	return pq
}

// Push 添加任务到队列
func (pq *PriorityQueue) Push(task *models.LLMTask) error {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()
	
	if pq.maxSize > 0 && len(pq.items) >= pq.maxSize {
		return fmt.Errorf("队列已满，最大容量: %d", pq.maxSize)
	}
	
	heap.Push(&pq.items, task)
	return nil
}

// Pop 从队列中取出优先级最高的任务
func (pq *PriorityQueue) Pop() *models.LLMTask {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()
	
	if len(pq.items) == 0 {
		return nil
	}
	
	item := heap.Pop(&pq.items)
	return item.(*models.LLMTask)
}

// Peek 查看队列头部任务但不移除
func (pq *PriorityQueue) Peek() *models.LLMTask {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	if len(pq.items) == 0 {
		return nil
	}
	
	return pq.items[0]
}

// Len 获取队列长度
func (pq *PriorityQueue) Len() int {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	return len(pq.items)
}

// IsEmpty 检查队列是否为空
func (pq *PriorityQueue) IsEmpty() bool {
	return pq.Len() == 0
}

// IsFull 检查队列是否已满
func (pq *PriorityQueue) IsFull() bool {
	if pq.maxSize <= 0 {
		return false
	}
	return pq.Len() >= pq.maxSize
}

// Clear 清空队列
func (pq *PriorityQueue) Clear() {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()
	
	pq.items = make(TaskHeap, 0)
	heap.Init(&pq.items)
}

// TaskHeap 任务堆，实现heap.Interface
type TaskHeap []*models.LLMTask

// Len 返回堆的长度
func (h TaskHeap) Len() int {
	return len(h)
}

// Less 比较两个任务的优先级
func (h TaskHeap) Less(i, j int) bool {
	// 首先按优先级排序（高优先级在前）
	if h[i].Priority != h[j].Priority {
		return h[i].Priority > h[j].Priority
	}
	
	// 如果优先级相同，按创建时间排序（早创建的在前）
	return h[i].CreatedAt.Before(h[j].CreatedAt)
}

// Swap 交换两个元素
func (h TaskHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

// Push 向堆中添加元素
func (h *TaskHeap) Push(x interface{}) {
	*h = append(*h, x.(*models.LLMTask))
}

// Pop 从堆中移除并返回最后一个元素
func (h *TaskHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

