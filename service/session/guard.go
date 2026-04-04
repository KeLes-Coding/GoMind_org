package session

import (
	"GopherAI/common/code"
	"GopherAI/common/observability"
	myredis "GopherAI/common/redis"
	rt "GopherAI/common/runtime"
	"context"
	"log"
	"sync"
	"time"
)

const (
	// 这里先给聊天主链路一个较保守的本地固定窗口限流。
	// 目标不是做最终形态的网关限流，而是先在应用层拦住明显异常流量。
	defaultChatRateLimit       = 12
	defaultChatRateLimitWindow = 30 * time.Second
)

type sessionOwnerGuardContextKey string

const ownerGuardContextKey sessionOwnerGuardContextKey = "session-owner-guard"

type sessionOwnerGuard struct {
	SessionID   string
	OwnerID     string
	FenceToken  int64
}

func withSessionOwnerGuard(ctx context.Context, guard *sessionOwnerGuard) context.Context {
	if guard == nil {
		return ctx
	}
	return context.WithValue(ctx, ownerGuardContextKey, guard)
}

func sessionOwnerGuardFromContext(ctx context.Context) *sessionOwnerGuard {
	if ctx == nil {
		return nil
	}
	guard, _ := ctx.Value(ownerGuardContextKey).(*sessionOwnerGuard)
	return guard
}

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
func withSessionExecutionGuard(ctx context.Context, sessionID string, fn func(context.Context) codeExecutorResult) codeExecutorResult {
	localLock := globalSessionLocalLockManager.getLock(sessionID)
	waitStart := time.Now()
	localLock.Lock()
	defer localLock.Unlock()
	observability.RecordSessionWait(time.Since(waitStart))

	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if myredis.IsAvailable() && rt.IsOwnerEligible() {
		preferredOwner, activeInstances, err := myredis.ResolvePreferredSessionOwner(execCtx, sessionID)
		if err != nil {
			observability.RecordSessionRedisLockDegrade()
			logSessionTrace(execCtx, "owner_route_degrade", "err=%v", err)
			log.Println("withSessionExecutionGuard resolve preferred session owner degraded:", err)
		} else {
			currentInstanceID := rt.CurrentInstanceID()
			currentLease, ownerErr := myredis.GetSessionOwnerLeaseDetail(execCtx, sessionID)
			if ownerErr != nil {
				observability.RecordSessionRedisLockDegrade()
				logSessionTrace(execCtx, "owner_lease_read_degrade", "err=%v", ownerErr)
				log.Println("withSessionExecutionGuard get session owner lease degraded:", ownerErr)
			}
			currentOwner := ""
			if currentLease != nil {
				currentOwner = currentLease.OwnerID
			}

			// 如果当前实例既不是 hash 首选 owner，也不是现存 lease owner，就直接返回 busy。
			// 这样可以减少同一 session 在多实例之间漂移，而不用先上请求转发。
			if preferredOwner != "" && preferredOwner != currentInstanceID && currentOwner != currentInstanceID {
				observability.RecordSessionOwnerRouteMismatch()
				logSessionTrace(execCtx, "owner_route_mismatch", "preferred=%s current=%s active=%d current_owner=%s", preferredOwner, currentInstanceID, len(activeInstances), currentOwner)
				return codeExecutorResult{busy: true}
			}

			ownerLease, ownerState, err := myredis.AcquireOrRefreshSessionOwnerLease(execCtx, sessionID, currentInstanceID)
			if err != nil {
				observability.RecordSessionRedisLockDegrade()
				logSessionTrace(execCtx, "owner_lease_degrade", "err=%v", err)
				log.Println("withSessionExecutionGuard acquire session owner lease degraded:", err)
			} else if ownerLease == nil {
				observability.RecordSessionOwnerRouteMismatch()
				logSessionTrace(execCtx, "owner_lease_busy", "current=%s owner=%s state=%s", currentInstanceID, ownerState, myredis.CurrentMode())
				return codeExecutorResult{busy: true}
			} else {
				execCtx = withSessionOwnerGuard(execCtx, &sessionOwnerGuard{
					SessionID:  sessionID,
					OwnerID:    ownerLease.OwnerID,
					FenceToken: ownerLease.FenceToken,
				})
				logSessionTrace(execCtx, "owner_lease_ready", "owner=%s preferred=%s fence=%d state=%s", currentInstanceID, preferredOwner, ownerLease.FenceToken, ownerState)
				ownerLease.StartSessionOwnerWatchdog(execCtx, func() {
					logSessionTrace(execCtx, "owner_lease_lost", "detail=watchdog_detected_owner_takeover")
					cancel()
				})
			}
		}
	}

	distributedLock, err := myredis.AcquireSessionDistributedLock(execCtx, sessionID)
	if err != nil {
		// Redis 锁是多实例增强能力，不是单机正确性的唯一前提。
		// 所以这里明确降级为“仅本地锁保护”，同时把事实打到日志和观测里。
		observability.RecordSessionRedisLockDegrade()
		logSessionTrace(execCtx, "redis_lock_degrade", "mode=%s err=%v", myredis.CurrentMode(), err)
		log.Println("withSessionExecutionGuard acquire redis session lock degraded:", err)
		return fn(execCtx)
	}
	if distributedLock == nil && myredis.IsAvailable() {
		observability.RecordSessionLockBusy()
		logSessionTrace(execCtx, "redis_lock_busy", "detail=distributed_lock_conflict")
		return codeExecutorResult{busy: true}
	}
	if distributedLock == nil && !myredis.IsAvailable() {
		observability.RecordSessionRedisLockDegrade()
		logSessionTrace(execCtx, "redis_mode_degraded", "detail=local_lock_only")
		return fn(execCtx)
	}
	if distributedLock != nil {
		distributedLock.StartWatchdog(execCtx, func() {
			logSessionTrace(execCtx, "redis_lock_lost", "detail=watchdog_detected_token_lost")
			cancel()
		})
		defer func() {
			_ = myredis.ReleaseSessionDistributedLock(execCtx, distributedLock)
		}()
	}

	logSessionTrace(execCtx, "guard_enter", "mode=%s", myredis.CurrentMode())
	return fn(execCtx)
}

type codeExecutorResult struct {
	code       code.Code
	aiResponse string
	err        error
	busy       bool
}
