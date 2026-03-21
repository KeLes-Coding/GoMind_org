package message

import (
	"GopherAI/common/mysql"
	"GopherAI/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func GetMessagesBySessionID(sessionID string) ([]model.Message, error) {
	var msgs []model.Message
	err := mysql.DB.Where("session_id = ?", sessionID).Order("created_at asc").Find(&msgs).Error
	return msgs, err
}

func GetMessagesBySessionIDs(sessionIDs []string) ([]model.Message, error) {
	var msgs []model.Message
	if len(sessionIDs) == 0 {
		return msgs, nil
	}
	err := mysql.DB.Where("session_id IN ?", sessionIDs).Order("created_at asc").Find(&msgs).Error
	return msgs, err
}

func CreateMessage(message *model.Message) (*model.Message, error) {
	// 以 message_key 作为幂等键；如果消息因为 MQ 重投被重复消费，这里直接忽略重复写入。
	err := mysql.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "message_key"}},
		DoNothing: true,
	}).Create(message).Error
	return message, err
}

func GetAllMessages() ([]model.Message, error) {
	var msgs []model.Message
	err := mysql.DB.Order("created_at asc").Find(&msgs).Error
	return msgs, err
}

// GetLatestMessageBySessionID 读取某个会话当前最新的一条持久化消息。
func GetLatestMessageBySessionID(sessionID string) (*model.Message, error) {
	var msg model.Message
	err := mysql.DB.Where("session_id = ?", sessionID).Order("id desc").First(&msg).Error
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// ExistsMessageKey 判断某条消息是否已经成功落库。
func ExistsMessageKey(messageKey string) (bool, error) {
	var count int64
	err := mysql.DB.Model(&model.Message{}).Where("message_key = ?", messageKey).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// IsMessageNotFoundError 用于让上层识别“会话还没有任何落库消息”这种正常场景。
func IsMessageNotFoundError(err error) bool {
	return err == gorm.ErrRecordNotFound
}

// GetMessageCountBySessionID 统计某个会话当前已经持久化了多少条消息。
func GetMessageCountBySessionID(sessionID string) (int64, error) {
	var count int64
	err := mysql.DB.Model(&model.Message{}).Where("session_id = ?", sessionID).Count(&count).Error
	return count, err
}
