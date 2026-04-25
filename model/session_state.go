package model

import "time"

// SessionHotMessage 是“可共享热状态”里的轻量消息结构。
// 这里故意不把整个 AIHelper 直接序列化出去，而是只保留跨实例恢复真正需要的字段，
// 这样可以把“运行时对象”和“共享状态快照”拆开，降低后续分布式演进复杂度。
type SessionHotMessage struct {
	ID         uint   `json:"id"`
	MessageKey string `json:"message_key"`
	SessionID  string `json:"session_id"`
	// SessionVersion 让 Redis 热状态也能表达“这条消息属于哪一轮正式会话推进”。
	// 这样当 MySQL 同步写失败但 Redis 仍保存了最新热状态时，repair worker 才能准确回放到对应版本。
	SessionVersion   int64          `json:"session_version"`
	UserName         string         `json:"user_name"`
	Content          string         `json:"content"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ResponseMeta     map[string]any `json:"response_meta,omitempty"`
	Extra            map[string]any `json:"extra,omitempty"`
	IsUser           bool           `json:"is_user"`
	Status           string         `json:"status,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

// SessionHotState 是 Redis 中保存的会话热状态快照。
// 它只承载“最近窗口消息 + 摘要状态 + 版本号”这类共享数据，
// 不承载模型实例、锁、函数指针等运行时对象。
type SessionHotState struct {
	SessionID          string `json:"session_id"`
	SelectionSignature string `json:"selection_signature,omitempty"`
	OwnerID            string `json:"owner_id"`
	FenceToken         int64  `json:"fence_token"`
	Version            int64  `json:"version"`
	// PersistedVersion 表示当前热状态所对应的 MySQL 持久化水位。
	// 后续恢复时可以据此判断 Redis 热状态和 DB 正式状态之间的收敛程度。
	PersistedVersion int64 `json:"persisted_version"`
	// PendingPersist 用于标记“热状态已推进，但持久层仍待补偿”的场景。
	// 第一阶段先把字段放进协议，后续阶段再接入真实补偿逻辑。
	PendingPersist bool `json:"pending_persist,omitempty"`
	// HotStateDirty 用于标记“本次会话推进后，Redis 热状态可能未完全追平”的场景。
	// 它主要服务于降级观测和后续修复任务。
	HotStateDirty       bool      `json:"hot_state_dirty,omitempty"`
	UpdatedAt           time.Time `json:"updated_at"`
	ContextSummary      string    `json:"context_summary"`
	SummaryMessageCount int       `json:"summary_message_count"`
	// RecentMessagesStart 表示 recent_messages 在完整消息序列中的起始下标。
	// 这样 warm resume 时就能知道“当前窗口前面省略了多少条消息”，
	// 避免把 summary_message_count 误当成当前切片内的局部索引。
	RecentMessagesStart int                 `json:"recent_messages_start"`
	RecentMessages      []SessionHotMessage `json:"recent_messages"`
}
