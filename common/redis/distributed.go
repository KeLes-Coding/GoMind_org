package redis

import (
	"GopherAI/common/observability"
	rt "GopherAI/common/runtime"
	"GopherAI/model"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	redisCli "github.com/redis/go-redis/v9"
)

const (
	sessionLockTTL       = 2 * time.Minute
	sessionHotStateTTL   = 30 * time.Minute
	sessionOwnerLeaseTTL = 90 * time.Second
	chatInstanceTTL      = 30 * time.Second
	rateLimitDefaultTTL  = 30 * time.Second
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

local _, _, storedFence = string.find(current, '"fence_token"%s*:%s*(-?%d+)')
if not storedFence then
	storedFence = "0"
end

if tonumber(ARGV[3]) < tonumber(storedVersion) then
	return "ignored_stale"
end
if tonumber(ARGV[3]) == tonumber(storedVersion) and tonumber(ARGV[4]) < tonumber(storedFence) then
	return "ignored_stale"
end

redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
return "applied"
`

var acquireOwnerLeaseScript = `
local current = redis.call("GET", KEYS[1])
if not current then
	local token = redis.call("INCR", KEYS[2])
	local value = ARGV[1] .. "|" .. token
	redis.call("SET", KEYS[1], value, "PX", ARGV[2])
	return value
end
local delimiter = string.find(current, "|")
if delimiter then
	local currentOwner = string.sub(current, 1, delimiter - 1)
	if currentOwner == ARGV[1] then
		redis.call("PEXPIRE", KEYS[1], ARGV[2])
		return current
	end
end
return current
`

var refreshOwnerLeaseScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
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

type SessionOwnerLease struct {
	SessionID  string
	OwnerID    string
	FenceToken int64
}

// ChatInstanceMeta 描述一个参与聊天 owner 选举的活跃实例元信息。
// 当前至少包含实例角色和路由权重，后续可继续扩展负载、地域等维度。
type ChatInstanceMeta struct {
	InstanceID string `json:"instance_id"`
	Role       string `json:"role"`
	Weight     int    `json:"weight"`
}

const (
	SessionHotStateSaveApplied      SessionHotStateSaveResult = "applied"
	SessionHotStateSaveIgnoredStale SessionHotStateSaveResult = "ignored_stale"
	SessionHotStateSaveUnavailable  SessionHotStateSaveResult = "unavailable"
)

func normalizeChatInstanceWeight(weight int) int {
	if weight <= 0 {
		return 100
	}
	return weight
}

func decodeChatInstanceMeta(instanceID string, raw string) ChatInstanceMeta {
	meta := ChatInstanceMeta{
		InstanceID: instanceID,
		Role:       raw,
		Weight:     100,
	}

	// 兼容旧版本心跳值：旧值只有 role 纯字符串，没有 JSON 结构。
	if !strings.HasPrefix(strings.TrimSpace(raw), "{") {
		return meta
	}

	var parsed ChatInstanceMeta
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return meta
	}
	if strings.TrimSpace(parsed.InstanceID) == "" {
		parsed.InstanceID = instanceID
	}
	if strings.TrimSpace(parsed.Role) == "" {
		parsed.Role = meta.Role
	}
	parsed.Weight = normalizeChatInstanceWeight(parsed.Weight)
	return parsed
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
		state.FenceToken,
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

// RegisterChatInstanceHeartbeat 刷新当前聊天实例的存活心跳。
// 第三阶段先用最小可用实现：实例只要持续刷新这个 key，就会被视为 hash routing 候选 owner。
func RegisterChatInstanceHeartbeat(ctx context.Context) error {
	if !IsAvailable() || !rt.IsOwnerEligible() {
		return nil
	}
	key := GenerateChatInstanceHeartbeatKey(rt.CurrentInstanceID())
	payload, err := json.Marshal(ChatInstanceMeta{
		InstanceID: rt.CurrentInstanceID(),
		Role:       rt.CurrentRole(),
		Weight:     rt.CurrentOwnerWeight(),
	})
	if err != nil {
		return err
	}
	if err := Rdb.Set(ctx, key, string(payload), chatInstanceTTL).Err(); err != nil {
		setAvailability(false)
		return err
	}
	return nil
}

// StartChatInstanceHeartbeat 在后台周期刷新当前实例的 owner 候选心跳。
func StartChatInstanceHeartbeat(ctx context.Context) {
	if !rt.IsOwnerEligible() {
		return
	}

	go func() {
		ticker := time.NewTicker(chatInstanceTTL / 3)
		defer ticker.Stop()
		for {
			if err := RegisterChatInstanceHeartbeat(context.Background()); err != nil {
				time.Sleep(2 * time.Second)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// ListActiveChatInstances 列出当前参与聊天 owner 选举的活跃实例。
func ListActiveChatInstances(ctx context.Context) ([]string, error) {
	metas, err := ListActiveChatInstanceMetas(ctx)
	if err != nil {
		return nil, err
	}
	instances := make([]string, 0, len(metas))
	for _, meta := range metas {
		if strings.TrimSpace(meta.InstanceID) == "" {
			continue
		}
		instances = append(instances, meta.InstanceID)
	}
	return instances, nil
}

// GetActiveChatInstanceMeta 读取指定实例当前的聊天心跳元数据。
// 第三阶段会在 resume 路由里用它判断“原 owner 是否仍在线”，从而决定是否应短退避重试。
func GetActiveChatInstanceMeta(ctx context.Context, instanceID string) (*ChatInstanceMeta, error) {
	if !IsAvailable() || strings.TrimSpace(instanceID) == "" {
		return nil, nil
	}

	rawValue, err := Rdb.Get(ctx, GenerateChatInstanceHeartbeatKey(instanceID)).Result()
	if err != nil {
		if err == redisCli.Nil {
			return nil, nil
		}
		setAvailability(false)
		return nil, err
	}

	meta := decodeChatInstanceMeta(instanceID, rawValue)
	return &meta, nil
}

// IsChatInstanceActive 判断指定实例是否仍在活跃心跳集合里。
// 这里不做更复杂的健康检查，只要 Redis 心跳键还在，就认为它仍可作为路由目标。
func IsChatInstanceActive(ctx context.Context, instanceID string) (bool, error) {
	meta, err := GetActiveChatInstanceMeta(ctx, instanceID)
	if err != nil {
		return false, err
	}
	return meta != nil, nil
}

// ListActiveChatInstanceMetas 列出当前活跃聊天实例及其用于路由的元信息。
func ListActiveChatInstanceMetas(ctx context.Context) ([]ChatInstanceMeta, error) {
	if !IsAvailable() {
		return nil, nil
	}

	keys, err := Rdb.Keys(ctx, GenerateChatInstanceHeartbeatKey("*")).Result()
	if err != nil {
		setAvailability(false)
		return nil, err
	}
	metas := make([]ChatInstanceMeta, 0, len(keys))
	prefix := "ai:instance:chat:"
	for _, key := range keys {
		instanceID := strings.TrimPrefix(key, prefix)
		if strings.TrimSpace(instanceID) == "" {
			continue
		}
		rawValue, readErr := Rdb.Get(ctx, key).Result()
		if readErr != nil {
			if readErr == redisCli.Nil {
				continue
			}
			setAvailability(false)
			return nil, readErr
		}
		metas = append(metas, decodeChatInstanceMeta(instanceID, rawValue))
	}
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].InstanceID < metas[j].InstanceID
	})
	return metas, nil
}

// scoreSessionOwnerHRW 为单个 (sessionID, instanceID) 组合计算加权 Rendezvous Hash 分数。
// 当前采用加权 HRW 近似公式：score = weight / -ln(u)。
// 这样权重越大，实例被选中的概率越高，同时仍保持 HRW 的局部重映射特性。
func scoreSessionOwnerHRW(sessionID string, instanceID string, weight int) float64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(sessionID))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(instanceID))

	hashValue := hasher.Sum64()
	// 将 64bit hash 映射到 (0,1] 区间，避免 ln(0)。
	u := (float64(hashValue) + 1) / (float64(math.MaxUint64) + 1)
	return float64(normalizeChatInstanceWeight(weight)) / -math.Log(u)
}

// selectPreferredSessionOwnerHRW 使用加权 HRW Rendezvous Hash 从活跃实例中选出首选 owner。
// 相比简单取模，HRW 在实例上下线时只会让少量 session 重映射，更符合粘性路由场景。
func selectPreferredSessionOwnerHRW(sessionID string, instances []ChatInstanceMeta) string {
	if len(instances) == 0 {
		return ""
	}

	bestInstance := ""
	bestScore := -1.0
	for _, instance := range instances {
		score := scoreSessionOwnerHRW(sessionID, instance.InstanceID, instance.Weight)
		// 分数更大时直接替换；
		// 分数相同时按实例 ID 字典序稳定打破平局，避免不同进程出现不同选择。
		if bestInstance == "" || score > bestScore || (score == bestScore && instance.InstanceID < bestInstance) {
			bestInstance = instance.InstanceID
			bestScore = score
		}
	}
	return bestInstance
}

// ResolvePreferredSessionOwner 根据当前活跃实例集合，为某个 session 选出稳定的 HRW owner。
func ResolvePreferredSessionOwner(ctx context.Context, sessionID string) (string, []string, error) {
	metas, err := ListActiveChatInstanceMetas(ctx)
	if err != nil {
		return "", nil, err
	}
	instances := make([]string, 0, len(metas))
	for _, meta := range metas {
		instances = append(instances, meta.InstanceID)
	}
	if len(metas) == 0 {
		return rt.CurrentInstanceID(), instances, nil
	}

	return selectPreferredSessionOwnerHRW(sessionID, metas), instances, nil
}

// GetSessionOwnerLease 返回当前 session 的 owner lease 持有者。
func GetSessionOwnerLease(ctx context.Context, sessionID string) (string, error) {
	if !IsAvailable() {
		return "", nil
	}
	result, err := Rdb.Get(ctx, GenerateSessionOwnerLeaseKey(sessionID)).Result()
	if err != nil {
		if err == redisCli.Nil {
			return "", nil
		}
		setAvailability(false)
		return "", err
	}
	return result, nil
}

func parseOwnerLeaseValue(value string) (*SessionOwnerLease, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.SplitN(value, "|", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid session owner lease value: %s", value)
	}
	fenceToken, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, err
	}
	return &SessionOwnerLease{
		OwnerID:    parts[0],
		FenceToken: fenceToken,
	}, nil
}

// GetSessionOwnerLeaseDetail 返回 session owner lease 的完整内容。
func GetSessionOwnerLeaseDetail(ctx context.Context, sessionID string) (*SessionOwnerLease, error) {
	value, err := GetSessionOwnerLease(ctx, sessionID)
	if err != nil || value == "" {
		return nil, err
	}
	lease, err := parseOwnerLeaseValue(value)
	if err != nil {
		return nil, err
	}
	if lease != nil {
		lease.SessionID = sessionID
	}
	return lease, nil
}

// AcquireOrRefreshSessionOwnerLease 获取或刷新 session owner lease。
// 这里的 lease 绑定到“实例”而不是“单次请求”，这样同一 owner 可以连续复用热状态，减少实例漂移。
func AcquireOrRefreshSessionOwnerLease(ctx context.Context, sessionID string, ownerID string) (*SessionOwnerLease, string, error) {
	if !IsAvailable() {
		return nil, "", nil
	}
	result, err := Rdb.Eval(
		ctx,
		acquireOwnerLeaseScript,
		[]string{GenerateSessionOwnerLeaseKey(sessionID), GenerateSessionOwnerFenceKey(sessionID)},
		ownerID,
		sessionOwnerLeaseTTL.Milliseconds(),
	).Result()
	if err != nil {
		setAvailability(false)
		return nil, "", err
	}
	text, ok := result.(string)
	if !ok {
		return nil, "", fmt.Errorf("unexpected session owner lease result type %T", result)
	}
	lease, err := parseOwnerLeaseValue(text)
	if err != nil {
		return nil, "", err
	}
	if lease != nil && lease.OwnerID == ownerID {
		lease.SessionID = sessionID
		return lease, text, nil
	}
	return nil, text, nil
}

// StartSessionOwnerWatchdog 在请求执行期间持续刷新 owner lease。
// 如果 lease 被其他实例接管，当前执行流需要尽快取消，避免旧 owner 继续推进状态。
func (l *SessionOwnerLease) StartSessionOwnerWatchdog(parent context.Context, onLost func()) {
	if l == nil || strings.TrimSpace(l.SessionID) == "" || strings.TrimSpace(l.OwnerID) == "" {
		return
	}

	go func() {
		ticker := time.NewTicker(sessionOwnerLeaseTTL / 3)
		defer ticker.Stop()
		for {
			select {
			case <-parent.Done():
				return
			case <-ticker.C:
				renewed, err := Rdb.Eval(
					context.Background(),
					refreshOwnerLeaseScript,
					[]string{GenerateSessionOwnerLeaseKey(l.SessionID)},
					fmt.Sprintf("%s|%d", l.OwnerID, l.FenceToken),
					sessionOwnerLeaseTTL.Milliseconds(),
				).Result()
				if err != nil {
					setAvailability(false)
					continue
				}
				if result, ok := renewed.(int64); ok && result > 0 {
					observability.RecordSessionOwnerLeaseRefresh()
					continue
				}
				observability.RecordSessionOwnerLeaseLost()
				if onLost != nil {
					onLost()
				}
				return
			}
		}
	}()
}

// ValidateSessionOwnerFence 检查当前请求持有的 owner/fence 是否仍然是最新写资格。
// 第三阶段最终闭环里，这一步用于在真正落 DB / 写热状态前再次做资格确认，避免旧 owner 尾写。
func ValidateSessionOwnerFence(ctx context.Context, sessionID string, ownerID string, fenceToken int64) (bool, error) {
	if !IsAvailable() || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(ownerID) == "" || fenceToken <= 0 {
		return true, nil
	}
	currentLease, err := GetSessionOwnerLeaseDetail(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if currentLease == nil {
		return false, nil
	}
	return currentLease.OwnerID == ownerID && currentLease.FenceToken == fenceToken, nil
}

// DeleteSessionOwnerLease 删除 session owner lease。
// 这主要用于删除会话后的联动清理，避免一个已经不存在的 session 还保留旧 owner 痕迹。
func DeleteSessionOwnerLease(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" || !IsAvailable() {
		return nil
	}
	if err := Rdb.Del(ctx, GenerateSessionOwnerLeaseKey(sessionID)).Err(); err != nil {
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
