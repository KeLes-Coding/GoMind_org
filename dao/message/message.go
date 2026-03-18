package message

import (
	"GopherAI/common/mysql"
	"GopherAI/model"

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
