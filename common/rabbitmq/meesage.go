package rabbitmq

import (
	messageDAO "GopherAI/dao/message"
	"GopherAI/model"
	"encoding/json"
	"errors"

	"github.com/streadway/amqp"
	"gorm.io/gorm"
)

// MessageMQParam 是写入 RabbitMQ 的消息体。
// 这里显式携带 MessageKey，用它作为消费端幂等键。
type MessageMQParam struct {
	MessageKey string `json:"message_key"`
	SessionID  string `json:"session_id"`
	Content    string `json:"content"`
	UserName   string `json:"user_name"`
	IsUser     bool   `json:"is_user"`
	// Status 跟随消息一并进入 MQ，保证异步落库链路不会丢失状态语义。
	Status string `json:"status"`
}

// GenerateMessageMQParam 把一条消息序列化为 MQ 负载。
func GenerateMessageMQParam(messageKey string, sessionID string, content string, userName string, isUser bool, status string) []byte {
	param := MessageMQParam{
		MessageKey: messageKey,
		SessionID:  sessionID,
		Content:    content,
		UserName:   userName,
		IsUser:     isUser,
		Status:     status,
	}
	data, _ := json.Marshal(param)
	return data
}

// MQMessage 是消息队列消费者的业务处理函数。
// 只有数据库写入成功后，外层消费逻辑才会 ack；失败则会走 nack / 重试。
func MQMessage(msg *amqp.Delivery) error {
	var param MessageMQParam
	if err := json.Unmarshal(msg.Body, &param); err != nil {
		return err
	}
	if param.Status == "" {
		// 兼容老版本生产出的 MQ 消息；这些消息默认视作完整完成态。
		param.Status = string(model.MessageStatusCompleted)
	}

	newMsg := &model.Message{
		MessageKey: param.MessageKey,
		SessionID:  param.SessionID,
		Content:    param.Content,
		UserName:   param.UserName,
		IsUser:     param.IsUser,
		Status:     model.MessageStatus(param.Status),
	}

	_, err := messageDAO.CreateMessage(newMsg)
	if err != nil && !errors.Is(err, gorm.ErrDuplicatedKey) {
		return err
	}

	return nil
}
