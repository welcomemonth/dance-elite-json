package middleware

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
)

// RateLimit 返回按客户端 IP 的滑动窗口限流中间件：window 内最多 limit 次。
//
// 进程内实现，单机部署足够；多实例部署需换成共享存储（如 Redis）。
// 超限返回 42901，不执行业务 handler。
func RateLimit(limit int, window time.Duration) gin.HandlerFunc {
	l := newIPLimiter(limit, window)
	return func(c *gin.Context) {
		if !l.allow(c.ClientIP(), time.Now()) {
			httpx.AbortError(c, httpx.ErrTooManyRequests("操作过于频繁，请稍后再试"))
			return
		}
		c.Next()
	}
}

type ipLimiter struct {
	mu        sync.Mutex
	limit     int
	window    time.Duration
	hits      map[string][]time.Time
	lastSweep time.Time
}

func newIPLimiter(limit int, window time.Duration) *ipLimiter {
	return &ipLimiter{
		limit:     limit,
		window:    window,
		hits:      make(map[string][]time.Time),
		lastSweep: time.Now(),
	}
}

// allow 记录一次请求并返回是否放行。每次调用顺带清理该 key 的过期记录；
// 每过一个窗口周期整体清扫一次空 key，避免 IP 数量增长导致内存泄漏。
func (l *ipLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastSweep) > l.window {
		for k, times := range l.hits {
			if len(times) == 0 || !times[len(times)-1].After(now.Add(-l.window)) {
				delete(l.hits, k)
			}
		}
		l.lastSweep = now
	}

	cutoff := now.Add(-l.window)
	times := l.hits[key]
	i := 0
	for i < len(times) && !times[i].After(cutoff) {
		i++
	}
	times = times[i:]

	if len(times) >= l.limit {
		l.hits[key] = times
		return false
	}
	l.hits[key] = append(times, now)
	return true
}
