// Package providers 速率限制实现
package providers

import (
	"context"
	"sync"
	"time"
)

// RateLimiter 速率限制器
type RateLimiter struct {
	config        RateLimit
	requestsCount int
	tokensCount   int64
	windowStart   time.Time
	concurrentReq int
	mutex         sync.Mutex
}

// NewRateLimiter 创建新的速率限制器
func NewRateLimiter(config RateLimit) *RateLimiter {
	return &RateLimiter{
		config:      config,
		windowStart: time.Now(),
	}
}

// Wait 等待直到可以发送请求
func (r *RateLimiter) Wait(ctx context.Context) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	now := time.Now()
	
	// 检查时间窗口是否需要重置
	if now.Sub(r.windowStart) >= r.config.ResetInterval {
		r.requestsCount = 0
		r.tokensCount = 0
		r.windowStart = now
	}
	
	// 检查并发限制
	if r.config.ConcurrentRequests > 0 && r.concurrentReq >= r.config.ConcurrentRequests {
		// 等待直到有并发槽位可用
		for r.concurrentReq >= r.config.ConcurrentRequests {
			r.mutex.Unlock()
			select {
			case <-ctx.Done():
				r.mutex.Lock()
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				r.mutex.Lock()
			}
		}
	}
	
	// 检查请求频率限制
	if r.config.RequestsPerMinute > 0 && r.requestsCount >= r.config.RequestsPerMinute {
		// 需要等待到下一个时间窗口
		waitTime := r.config.ResetInterval - now.Sub(r.windowStart)
		if waitTime > 0 {
			r.mutex.Unlock()
			select {
			case <-ctx.Done():
				r.mutex.Lock()
				return ctx.Err()
			case <-time.After(waitTime):
				r.mutex.Lock()
				// 重置计数器
				r.requestsCount = 0
				r.tokensCount = 0
				r.windowStart = time.Now()
			}
		}
	}
	
	// 允许请求
	r.requestsCount++
	r.concurrentReq++
	
	return nil
}

// Release 释放并发槽位
func (r *RateLimiter) Release() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	if r.concurrentReq > 0 {
		r.concurrentReq--
	}
}

// AddTokenUsage 添加Token使用量
func (r *RateLimiter) AddTokenUsage(tokens int64) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	r.tokensCount += tokens
}

// CanRequest 检查是否可以发送请求
func (r *RateLimiter) CanRequest() bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	now := time.Now()
	
	// 检查时间窗口
	if now.Sub(r.windowStart) >= r.config.ResetInterval {
		return true
	}
	
	// 检查请求数限制
	if r.config.RequestsPerMinute > 0 && r.requestsCount >= r.config.RequestsPerMinute {
		return false
	}
	
	// 检查并发限制
	if r.config.ConcurrentRequests > 0 && r.concurrentReq >= r.config.ConcurrentRequests {
		return false
	}
	
	return true
}

// GetStats 获取限流统计
func (r *RateLimiter) GetStats() RateLimiterStats {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	return RateLimiterStats{
		RequestsInWindow:  r.requestsCount,
		TokensInWindow:    r.tokensCount,
		ConcurrentRequest: r.concurrentReq,
		WindowStart:       r.windowStart,
		WindowDuration:    r.config.ResetInterval,
	}
}

// RateLimiterStats 限流统计
type RateLimiterStats struct {
	RequestsInWindow  int           `json:"requests_in_window"`
	TokensInWindow    int64         `json:"tokens_in_window"`
	ConcurrentRequest int           `json:"concurrent_requests"`
	WindowStart       time.Time     `json:"window_start"`
	WindowDuration    time.Duration `json:"window_duration"`
}