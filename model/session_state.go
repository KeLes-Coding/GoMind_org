package model

import "time"

// SessionHotMessage 是“可共享热状态”里的轻量消息结构。
// 这里故意不把整个 AIHelper 直接序列化出去，而是只保留跨实例恢复真正需要的字段，
// 这样可以把“运行时对象”和“共享状态快照”拆开，降低后续分布式演进复杂度。
type SessionHotMessage struct {
	ID         uint      `json:"id"`
	MessageKey string    `json:"message_key"`
	SessionID  string    `json:"session_id"`
	UserName   string    `json:"user_name"`
	Content    string    `json:"content"`
	IsUser     bool      `json:"is_user"`
	Status     string    `json:"status,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// SessionHotState 是 Redis 中保存的会话热状态快照。
// 它只承载“最近窗口消息 + 摘要状态 + 版本号”这类共享数据，
// 不承载模型实例、锁、函数指针等运行时对象。
type SessionHotState struct {
	SessionID           string              `json:"session_id"`
	OwnerID             string              `json:"owner_id"`
	FenceToken          int64               `json:"fence_token"`
	Version             int64               `json:"version"`
	UpdatedAt           time.Time           `json:"updated_at"`
	ContextSummary      string              `json:"context_summary"`
	SummaryMessageCount int                 `json:"summary_message_count"`
	RecentMessages      []SessionHotMessage `json:"recent_messages"`
}

