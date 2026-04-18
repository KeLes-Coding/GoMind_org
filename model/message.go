package model

import (
	"time"
)

// MessageStatus 用于描述一条消息在当前系统里的最终状态。
// 这里刻意把“成功完成”“被用户取消”“请求超时”“执行失败”拆开，
// 这样后续历史查询、前端展示、面试表述都会更清晰，而不是所有异常都混成一个 failed。
type MessageStatus string

const (
	// MessageStatusStreaming 主要给前端和运行时内存态使用，表示消息仍在流式生成中。
	MessageStatusStreaming MessageStatus = "streaming"
	// MessageStatusCompleted 表示消息已经完整生成并成功进入持久化链路。
	MessageStatusCompleted MessageStatus = "completed"
	// MessageStatusCancelled 表示消息被用户主动停止。
	MessageStatusCancelled MessageStatus = "cancelled"
	// MessageStatusTimeout 表示消息因为请求超时而中断。
	MessageStatusTimeout MessageStatus = "timeout"
	// MessageStatusFailed 表示消息因为模型或系统故障失败。
	MessageStatusFailed MessageStatus = "failed"
	// MessageStatusPartial 表示消息只保存了部分输出内容，通常是流式中途中断后的兜底状态。
	MessageStatusPartial MessageStatus = "partial"
)

type Message struct {
	// ID 是数据库自增主键，只承担持久化层面的排序和关联职责。
	ID uint `gorm:"primaryKey;autoIncrement" json:"id"`
	// MessageKey 是消息幂等键。
	// 之所以显式保留这个字段，是因为 MQ 重试、重复写入、状态回写都需要一个稳定键来做 upsert。
	MessageKey string `gorm:"type:varchar(64);uniqueIndex;not null" json:"message_key"`
	// SessionID 关联所属会话。
	SessionID string `gorm:"index;index:idx_messages_session_order,priority:1;not null;type:varchar(36)" json:"session_id"`
	// SessionVersion 表示这条消息属于会话的哪一次正式推进版本。
	// 第二阶段用它来判断 persisted_version 能否安全推进，而不是只看消息总数猜测。
	SessionVersion int64 `gorm:"index;index:idx_messages_session_order,priority:2;not null;default:0" json:"session_version"`
	// UserName 保留消息所属用户，便于排障和辅助查询。
	UserName string `gorm:"type:varchar(20)" json:"username"`
	// Content 是消息正文。
	Content string `gorm:"type:text" json:"content"`
	// IsUser 标识该消息是否由用户发送。
	IsUser bool `gorm:"index:idx_messages_session_order,priority:3;not null" json:"is_user"`
	// Status 描述该消息在当前系统中的最终状态。
	// 默认值给 completed，是为了兼容历史“同步完整写入”的老逻辑。
	Status    MessageStatus `gorm:"type:varchar(20);not null;default:'completed'" json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

type History struct {
	// History 直接面向前端会话历史接口，因此除了内容本身，还要把状态带出去，
	// 这样前端才能区分 completed / cancelled / timeout / failed，而不是一律当普通 assistant 消息展示。
	IsUser  bool          `json:"is_user"`
	Content string        `json:"content"`
	Status  MessageStatus `json:"status"`
}
