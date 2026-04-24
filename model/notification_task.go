package model

import "time"

const (
	// NotificationEventChatMessageReady 表示 assistant 已经生成终态消息，可触发旁路通知。
	NotificationEventChatMessageReady = "chat_message_ready"
	// QueueNotification 表示通知任务的 RabbitMQ 队列名称。
	QueueNotification = "notification.dispatch"
)

// NotificationTask 是当前阶段通知旁路链路使用的统一任务结构。
// 这里先只覆盖聊天完成提醒，后续浏览器通知、系统通知、声音提醒都可以复用同一事件格式扩展。
type NotificationTask struct {
	EventType  string    `json:"event_type"`
	UserID     int64     `json:"user_id"`
	SessionID  string    `json:"session_id"`
	MessageKey string    `json:"message_key"`
	Summary    string    `json:"summary"`
	CreatedAt  time.Time `json:"created_at"`
}
