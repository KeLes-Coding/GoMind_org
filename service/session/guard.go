package session

import (
	"GopherAI/common/code"
	myredis "GopherAI/common/redis"
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// 这里先给聊天主链路一个较保守的本地固定窗口限流。
	// 目标不是做最终形态的网关限流，而是先在应用层拦住明显异常流量。
	defaultChatRateLimit      = 12
	defaultChatRateLimitWindow = 30 * time.Second
)

// sessionLocalLockManager 用于在单机内保证同一个 session 串行推进。
// 第二轮接入 Redis 分布式锁后，它仍然保留，作为“本地正确性兜底”。
type sessionLocalLockManager struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newSessionLocalLockManager() *sessionLocalLockManager {
	return &sessionLocalLockManager{
		locks: make(map[string]*sync.Mutex),
	}
}

func (m *sessionLocalLockManager) getLock(sessionID string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()

	if lock, exists := m.locks[sessionID]; exists {
		return lock
	}

	lock := &sync.Mutex{}
	m.locks[sessionID] = lock
	return lock
}

var globalSessionLocalLockManager = newSessionLocalLockManager()

type localRateLimiter struct {
	mu      sync.Mutex
	windows map[string][]time.Time
}

func newLocalRateLimiter() *localRateLimiter {
	return &localRateLimiter{
		windows: make(map[string][]time.Time),
	}
}

func (l *localRateLimiter) allow(key string, limit int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	history := l.windows[key]
	kept := make([]time.Time, 0, len(history))
	for _, ts := range history {
		if now.Sub(ts) < window {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= limit {
		l.windows[key] = kept
		return false
	}

	kept = append(kept, now)
	l.windows[key] = kept
	return true
}

var globalLocalRateLimiter = newLocalRateLimiter()

// allowChatRateLimit 先尝试使用 Redis 做跨实例限流；
// 如果 Redis 当前不可用，则退回到本地固定窗口限流，至少保证单机不会被无限打穿。
func allowChatRateLimit(ctx context.Context, userName string) bool {
	key := myredis.BuildRateLimitKey("ai_chat_user", userName)
	allowed, err := myredis.AllowRateLimit(ctx, key, defaultChatRateLimit, defaultChatRateLimitWindow)
	if err == nil {
		return allowed
	}

	return globalLocalRateLimiter.allow(key, defaultChatRateLimit, defaultChatRateLimitWindow)
}

// withSessionExecutionGuard 统一封装聊天主链路的执行保护：
// 1. 本地 session 锁，保证单实例下同一会话串行；
// 2. Redis 分布式锁，保证多实例下尽量同样串行；
// 3. 使用 defer 做统一释放，避免中途 return 时遗留锁。
func withSessionExecutionGuard(ctx context.Context, sessionID string, fn func() codeExecutorResult) codeExecutorResult {
	localLock := globalSessionLocalLockManager.getLock(sessionID)
	localLock.Lock()
	defer localLock.Unlock()

	distributedLock, err := myredis.AcquireSessionDistributedLock(ctx, sessionID)
	if err != nil {
		return codeExecutorResult{err: fmt.Errorf("acquire redis session lock failed: %w", err)}
	}
	if distributedLock == nil && myredis.IsAvailable() {
		return codeExecutorResult{busy: true}
	}
	if distributedLock != nil {
		defer func() {
			_ = myredis.ReleaseSessionDistributedLock(ctx, distributedLock)
		}()
	}

	return fn()
}

type codeExecutorResult struct {
	code       code.Code
	aiResponse string
	err        error
	busy       bool
}
