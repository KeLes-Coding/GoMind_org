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
}

// GenerateMessageMQParam 把一条消息序列化为 MQ 负载。
func GenerateMessageMQParam(messageKey string, sessionID string, content string, userName string, isUser bool) []byte {
	param := MessageMQParam{
		MessageKey: messageKey,
		SessionID:  sessionID,
		Content:    content,
		UserName:   userName,
		IsUser:     isUser,
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

	newMsg := &model.Message{
		MessageKey: param.MessageKey,
		SessionID:  param.SessionID,
		Content:    param.Content,
		UserName:   param.UserName,
		IsUser:     param.IsUser,
	}

	_, err := messageDAO.CreateMessage(newMsg)
	if err != nil && !errors.Is(err, gorm.ErrDuplicatedKey) {
		return err
	}

	return nil
}
