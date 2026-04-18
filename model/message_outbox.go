package model

import "time"

type MessageOutboxStatus string

const (
	// MessageOutboxStatusPending 表示事件已经持久化，但还没有成功发布到 MQ。
	MessageOutboxStatusPending MessageOutboxStatus = "pending"
	// MessageOutboxStatusPublished 表示事件已经成功发布到 MQ，正在等待消费端真正落库确认。
	MessageOutboxStatusPublished MessageOutboxStatus = "published"
	// MessageOutboxStatusDelivered 表示消息已经完成“发布 + 消费落库 + 回执确认”全链路。
	MessageOutboxStatusDelivered MessageOutboxStatus = "delivered"
)

// MessageOutbox 是聊天消息异步持久化链路的可靠投递账本。
// 第二阶段里，它承担两个职责：
// 1. 在主链路上持久化“待投递事件”；
// 2. 给 relay worker 提供可扫描、可重试、可确认的补偿基线。
type MessageOutbox struct {
	ID             uint                `gorm:"primaryKey;autoIncrement" json:"id"`
	MessageKey     string              `gorm:"type:varchar(64);uniqueIndex;not null" json:"message_key"`
	SessionID      string              `gorm:"index;index:idx_message_outboxes_session_order,priority:1;type:varchar(36);not null" json:"session_id"`
	SessionVersion int64               `gorm:"index;index:idx_message_outboxes_session_order,priority:2;not null;default:0" json:"session_version"`
	Status         MessageOutboxStatus `gorm:"type:varchar(20);not null;default:'pending';index" json:"status"`
	// Payload 直接保存将要投递到 MQ 的 JSON 负载，避免补偿时再依赖运行时对象重新拼装。
	Payload         string     `gorm:"type:longtext;not null" json:"payload"`
	PublishAttempts int        `gorm:"not null;default:0" json:"publish_attempts"`
	LastError       string     `gorm:"type:text" json:"last_error"`
	NextAttemptAt   time.Time  `gorm:"index;not null" json:"next_attempt_at"`
	PublishedAt     *time.Time `json:"published_at"`
	DeliveredAt     *time.Time `json:"delivered_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
