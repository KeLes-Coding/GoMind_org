package redis

import (
	"GopherAI/common/observability"
	"GopherAI/model"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	redisCli "github.com/redis/go-redis/v9"
)

const (
	sessionLockTTL      = 2 * time.Minute
	sessionHotStateTTL  = 30 * time.Minute
	rateLimitDefaultTTL = 30 * time.Second
)

var releaseLockScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`

var renewLockScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`

var saveHotStateCASScript = `
local current = redis.call("GET", KEYS[1])
if not current then
	redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
	return "applied"
end

local _, _, storedVersion = string.find(current, '"version"%s*:%s*(-?%d+)')
if not storedVersion then
	redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
	return "applied"
end

if tonumber(ARGV[3]) < tonumber(storedVersion) then
	return "ignored_stale"
end

redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
return "applied"
`

// SessionDistributedLock 表示一次成功获取到的 Redis 分布式锁。
// token 用于释放锁时校验“谁加的锁谁释放”，避免误删其他实例的锁。
type SessionDistributedLock struct {
	Key          string
	Token        string
	stopRenew    chan struct{}
	renewStopped chan struct{}
}

type SessionHotStateSaveResult string

const (
	SessionHotStateSaveApplied      SessionHotStateSaveResult = "applied"
	SessionHotStateSaveIgnoredStale SessionHotStateSaveResult = "ignored_stale"
	SessionHotStateSaveUnavailable  SessionHotStateSaveResult = "unavailable"
)

// AcquireSessionDistributedLock 尝试获取 session 维度的 Redis 锁。
// 如果 Redis 当前不可用，直接返回 nil，调用方会退回到本地锁方案；
// 这样可以保证“分布式能力不可用时，单机正确性仍然存在”。
func AcquireSessionDistributedLock(ctx context.Context, sessionID string) (*SessionDistributedLock, error) {
	if !IsAvailable() {
		return nil, nil
	}

	key := GenerateSessionLockKey(sessionID)
	token := uuid.NewString()
	ok, err := Rdb.SetNX(ctx, key, token, sessionLockTTL).Result()
	if err != nil {
		setAvailability(false)
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	return &SessionDistributedLock{
		Key:          key,
		Token:        token,
		stopRenew:    make(chan struct{}),
		renewStopped: make(chan struct{}),
	}, nil
}

// ReleaseSessionDistributedLock 释放 Redis 分布式锁。
// 这里用 Lua 做原子校验，避免锁过期后被其他实例重新拿到时发生误删。
func ReleaseSessionDistributedLock(ctx context.Context, lock *SessionDistributedLock) error {
	if lock == nil || !IsAvailable() {
		return nil
	}

	lock.stopWatchdog()

	if err := Rdb.Eval(ctx, releaseLockScript, []string{lock.Key}, lock.Token).Err(); err != nil {
		setAvailability(false)
		return err
	}
	return nil
}

// StartWatchdog 为分布式锁启动后台续约协程，避免长流式请求因 TTL 到期而失去锁资格。
// 如果续约时发现 token 已经不再匹配，说明当前执行流已经不是合法持锁者，此时通知上层取消请求。
func (l *SessionDistributedLock) StartWatchdog(parent context.Context, onLost func()) {
	if l == nil || l.stopRenew == nil || l.renewStopped == nil {
		return
	}

	go func() {
		defer close(l.renewStopped)

		ticker := time.NewTicker(sessionLockTTL / 3)
		defer ticker.Stop()

		for {
			select {
			case <-parent.Done():
				return
			case <-l.stopRenew:
				return
			case <-ticker.C:
				renewCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				renewed, err := l.renew(renewCtx)
				cancel()
				if err != nil {
					// Redis 不可用时进入 degraded mode，当前请求保留本地锁继续执行。
					continue
				}
				if renewed {
					observability.RecordSessionLockWatchdogRefresh()
					continue
				}
				observability.RecordSessionLockWatchdogLost()
				if onLost != nil {
					onLost()
				}
				return
			}
		}
	}()
}

func (l *SessionDistributedLock) renew(ctx context.Context) (bool, error) {
	if l == nil || !IsAvailable() {
		return false, nil
	}

	result, err := Rdb.Eval(ctx, renewLockScript, []string{l.Key}, l.Token, sessionLockTTL.Milliseconds()).Result()
	if err != nil {
		setAvailability(false)
		return false, err
	}

	value, ok := result.(int64)
	if !ok {
		return false, fmt.Errorf("unexpected redis session lock renew result type %T", result)
	}
	return value > 0, nil
}

func (l *SessionDistributedLock) stopWatchdog() {
	if l == nil || l.stopRenew == nil || l.renewStopped == nil {
		return
	}

	select {
	case <-l.stopRenew:
	default:
		close(l.stopRenew)
	}

	select {
	case <-l.renewStopped:
	case <-time.After(200 * time.Millisecond):
	}
}

// SaveSessionHotState 把 session 热状态快照写入 Redis。
// 这份快照只用于跨实例的快速恢复与共享，不替代 MySQL 真相源。
func SaveSessionHotState(ctx context.Context, state *model.SessionHotState) (SessionHotStateSaveResult, error) {
	if state == nil || !IsAvailable() {
		return SessionHotStateSaveUnavailable, nil
	}

	payload, err := json.Marshal(state)
	if err != nil {
		return "", err
	}

	result, err := Rdb.Eval(
		ctx,
		saveHotStateCASScript,
		[]string{GenerateSessionHotStateKey(state.SessionID)},
		string(payload),
		sessionHotStateTTL.Milliseconds(),
		state.Version,
	).Result()
	if err != nil {
		setAvailability(false)
		return "", err
	}

	resultText, ok := result.(string)
	if !ok {
		return "", fmt.Errorf("unexpected redis hot state save result type %T", result)
	}

	return SessionHotStateSaveResult(resultText), nil
}

// GetSessionHotState 读取 Redis 中的会话热状态快照。
// 如果 Redis 没有命中，返回 nil 交由上层继续走 DB 恢复逻辑。
func GetSessionHotState(ctx context.Context, sessionID string) (*model.SessionHotState, error) {
	if !IsAvailable() {
		return nil, nil
	}

	result, err := Rdb.Get(ctx, GenerateSessionHotStateKey(sessionID)).Result()
	if err != nil {
		if err == redisCli.Nil {
			return nil, nil
		}
		setAvailability(false)
		return nil, err
	}

	var state model.SessionHotState
	if err := json.Unmarshal([]byte(result), &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// DeleteSessionHotState 删除 Redis 中的热状态快照，避免已删除会话被旧快照复活。
func DeleteSessionHotState(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" || !IsAvailable() {
		return nil
	}

	if err := Rdb.Del(ctx, GenerateSessionHotStateKey(sessionID)).Err(); err != nil {
		setAvailability(false)
		return err
	}
	return nil
}

// DeleteSessionLock 强制删除会话锁，仅用于删除/失效后的联动清理。
func DeleteSessionLock(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" || !IsAvailable() {
		return nil
	}

	if err := Rdb.Del(ctx, GenerateSessionLockKey(sessionID)).Err(); err != nil {
		setAvailability(false)
		return err
	}
	return nil
}

// AllowRateLimit 尝试消费一个固定窗口限流令牌。
// 这里先提供最小可用版本：同一个 key 在指定窗口内最多允许 limit 次请求。
func AllowRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	if limit <= 0 {
		return true, nil
	}
	if window <= 0 {
		window = rateLimitDefaultTTL
	}
	if !IsAvailable() {
		return true, nil
	}

	counter, err := Rdb.Incr(ctx, key).Result()
	if err != nil {
		setAvailability(false)
		return true, err
	}
	if counter == 1 {
		if err := Rdb.Expire(ctx, key, window).Err(); err != nil {
			setAvailability(false)
			return true, err
		}
	}

	return counter <= int64(limit), nil
}

// BuildRateLimitKey 统一拼装 Redis 限流 key，便于后续观察和排查。
func BuildRateLimitKey(scope string, identifier string) string {
	return fmt.Sprintf("ratelimit:%s:%s", scope, identifier)
}
