package model

import "time"

// StreamRuntimeStatus 表示一轮流式生成任务在恢复协议里的运行态。
// 它和消息最终状态并不完全等价：例如 detached 只表示“连接断了但流仍可能继续跑”，
// 还没有收敛成最终的 assistant 消息状态。
type StreamRuntimeStatus string

const (
	StreamStatusStreaming StreamRuntimeStatus = "streaming"
	StreamStatusDetached  StreamRuntimeStatus = "detached"
	StreamStatusCompleted StreamRuntimeStatus = "completed"
	StreamStatusCancelled StreamRuntimeStatus = "cancelled"
	StreamStatusTimeout   StreamRuntimeStatus = "timeout"
	StreamStatusFailed    StreamRuntimeStatus = "failed"
	StreamStatusPartial   StreamRuntimeStatus = "partial"
)

// StreamChunkSnapshot 是 Redis/本地环形缓冲区里的单个 chunk 快照。
// 这里保留 seq，便于前端按水位恢复和去重。
type StreamChunkSnapshot struct {
	StreamID       string         `json:"stream_id"`
	Seq            int64          `json:"seq"`
	Delta          string         `json:"delta"`
	ReasoningDelta string         `json:"reasoning_delta,omitempty"`
	ResponseMeta   map[string]any `json:"response_meta,omitempty"`
	Extra          map[string]any `json:"extra,omitempty"`
	TsUnixMs       int64          `json:"ts_unix_ms"`
}

// StreamSnapshot 是当前整段 assistant 已生成文本的快照。
// 当环形缓冲区已经覆盖不住历史 seq 时，前端可以用它直接覆盖当前内容，再继续追实时流。
type StreamSnapshot struct {
	StreamID         string         `json:"stream_id"`
	Content          string         `json:"content"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ResponseMeta     map[string]any `json:"response_meta,omitempty"`
	Extra            map[string]any `json:"extra,omitempty"`
	LastSeq          int64          `json:"last_seq"`
	UpdatedAt        time.Time      `json:"updated_at"`
	MessageID        string         `json:"message_id"`
	SessionID        string         `json:"session_id"`
	StatusHint       string         `json:"status_hint,omitempty"`
}

// StreamResumeMeta 是 Redis 恢复层共享的流式元数据。
// 它不承载模型实例，只描述恢复所需的最小状态。
type StreamResumeMeta struct {
	StreamID         string              `json:"stream_id"`
	SessionID        string              `json:"session_id"`
	MessageID        string              `json:"message_id"`
	UserName         string              `json:"user_name"`
	Status           StreamRuntimeStatus `json:"status"`
	NextSeq          int64               `json:"next_seq"`
	UpdatedAt        time.Time           `json:"updated_at"`
	ResumeDeadlineAt *time.Time          `json:"resume_deadline_at,omitempty"`
	OwnerID          string              `json:"owner_id,omitempty"`
	FenceToken       int64               `json:"fence_token,omitempty"`
}
