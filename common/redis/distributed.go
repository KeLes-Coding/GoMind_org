package redis

import (
	"GopherAI/model"
	"context"
	"encoding/json"
	"fmt"
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

// SessionDistributedLock 表示一次成功获取到的 Redis 分布式锁。
// token 用于释放锁时校验“谁加的锁谁释放”，避免误删其他实例的锁。
type SessionDistributedLock struct {
	Key   string
	Token string
}

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
		redisAvailable.Store(false)
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	return &SessionDistributedLock{Key: key, Token: token}, nil
}

// ReleaseSessionDistributedLock 释放 Redis 分布式锁。
// 这里用 Lua 做原子校验，避免锁过期后被其他实例重新拿到时发生误删。
func ReleaseSessionDistributedLock(ctx context.Context, lock *SessionDistributedLock) error {
	if lock == nil || !IsAvailable() {
		return nil
	}

	if err := Rdb.Eval(ctx, releaseLockScript, []string{lock.Key}, lock.Token).Err(); err != nil {
		redisAvailable.Store(false)
		return err
	}
	return nil
}

// SaveSessionHotState 把 session 热状态快照写入 Redis。
// 这份快照只用于跨实例的快速恢复与共享，不替代 MySQL 真相源。
func SaveSessionHotState(ctx context.Context, state *model.SessionHotState) error {
	if state == nil || !IsAvailable() {
		return nil
	}

	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}

	if err := Rdb.Set(ctx, GenerateSessionHotStateKey(state.SessionID), payload, sessionHotStateTTL).Err(); err != nil {
		redisAvailable.Store(false)
		return err
	}
	return nil
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
		redisAvailable.Store(false)
		return nil, err
	}

	var state model.SessionHotState
	if err := json.Unmarshal([]byte(result), &state); err != nil {
		return nil, err
	}
	return &state, nil
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
		redisAvailable.Store(false)
		return true, err
	}
	if counter == 1 {
		if err := Rdb.Expire(ctx, key, window).Err(); err != nil {
			redisAvailable.Store(false)
			return true, err
		}
	}

	return counter <= int64(limit), nil
}

// BuildRateLimitKey 统一拼装 Redis 限流 key，便于后续观察和排查。
func BuildRateLimitKey(scope string, identifier string) string {
	return fmt.Sprintf("ratelimit:%s:%s", scope, identifier)
}
