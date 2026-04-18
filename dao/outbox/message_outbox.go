package outbox

import (
	"GopherAI/common/mysql"
	"GopherAI/model"
	"time"

	"gorm.io/gorm/clause"
)

const (
	// messageOutboxPublishBackoff 控制“发布失败后多久重试一次”。
	messageOutboxPublishBackoff = 5 * time.Second
	// messageOutboxDeliveryCheckDelay 控制“发布成功后多久再次检查是否已经被消费确认”。
	// 如果消费确认迟迟没回来，relay worker 会重新投递；重复投递由 message_key 幂等兜底。
	messageOutboxDeliveryCheckDelay = 30 * time.Second
)

// SaveMessageOutbox 持久化一条消息 outbox 事件。
// 这里使用 message_key 做幂等键，保证同一条消息不会因为重复写入产生多条 outbox。
func SaveMessageOutbox(event *model.MessageOutbox) error {
	if event == nil {
		return nil
	}
	if event.NextAttemptAt.IsZero() {
		event.NextAttemptAt = time.Now()
	}

	return mysql.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "message_key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"session_id":      event.SessionID,
			"session_version": event.SessionVersion,
			"payload":         event.Payload,
			"status":          event.Status,
			"last_error":      event.LastError,
			"next_attempt_at": event.NextAttemptAt,
			"updated_at":      time.Now(),
		}),
	}).Create(event).Error
}

// MarkMessageOutboxPublished 把 outbox 标记为“已成功发布到 MQ，等待消费确认”。
func MarkMessageOutboxPublished(messageKey string) error {
	now := time.Now()
	return mysql.DB.Model(&model.MessageOutbox{}).
		Where("message_key = ?", messageKey).
		Updates(map[string]interface{}{
			"status":           model.MessageOutboxStatusPublished,
			"publish_attempts": clause.Expr{SQL: "publish_attempts + 1"},
			"last_error":       "",
			"next_attempt_at":  now.Add(messageOutboxDeliveryCheckDelay),
			"published_at":     now,
		}).Error
}

// MarkMessageOutboxPublishFailed 记录一次发布失败，并安排下一次补偿重试时间。
func MarkMessageOutboxPublishFailed(messageKey string, publishErr string) error {
	now := time.Now()
	return mysql.DB.Model(&model.MessageOutbox{}).
		Where("message_key = ?", messageKey).
		Updates(map[string]interface{}{
			"status":           model.MessageOutboxStatusPending,
			"publish_attempts": clause.Expr{SQL: "publish_attempts + 1"},
			"last_error":       publishErr,
			"next_attempt_at":  now.Add(messageOutboxPublishBackoff),
		}).Error
}

// MarkMessageOutboxDelivered 记录“消费端已成功落库确认”。
func MarkMessageOutboxDelivered(messageKey string) error {
	now := time.Now()
	return mysql.DB.Model(&model.MessageOutbox{}).
		Where("message_key = ?", messageKey).
		Updates(map[string]interface{}{
			"status":          model.MessageOutboxStatusDelivered,
			"last_error":      "",
			"next_attempt_at": now,
			"delivered_at":    now,
		}).Error
}

// ListMessageOutboxesReadyForRelay 列出当前需要 relay 补偿的消息事件。
func ListMessageOutboxesReadyForRelay(limit int) ([]model.MessageOutbox, error) {
	var events []model.MessageOutbox
	now := time.Now()
	query := mysql.DB.
		Where("status <> ? AND next_attempt_at <= ?", model.MessageOutboxStatusDelivered, now).
		Order("next_attempt_at asc").
		Order("id asc")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&events).Error
	return events, err
}

// ListUndeliveredMessageOutboxesBySessionID 返回指定会话里尚未完成消费落库的消息事件。
// 历史接口会用它补齐“主链路已写 outbox，但 MQ 消费还没追上”的短暂窗口。
func ListUndeliveredMessageOutboxesBySessionID(sessionID string) ([]model.MessageOutbox, error) {
	var events []model.MessageOutbox
	err := mysql.DB.
		Where("session_id = ? AND status <> ?", sessionID, model.MessageOutboxStatusDelivered).
		Order("session_version asc").
		Order("id asc").
		Find(&events).Error
	return events, err
}
