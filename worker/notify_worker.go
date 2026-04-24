package worker

import (
	"GopherAI/common/rabbitmq"
	"GopherAI/model"
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/streadway/amqp"
)

// StartNotifyWorker 启动通知占位 worker。
// 当前阶段它只负责消费通知任务并输出终端日志，
// 后续浏览器通知、系统通知、声音提醒都可以在这里继续分发。
func StartNotifyWorker(ctx context.Context) error {
	mq, err := rabbitmq.NewWorkRabbitMQ(model.QueueNotification)
	if err != nil {
		return fmt.Errorf("create notify worker mq failed: %w", err)
	}
	defer mq.Destroy()

	mq.Consume(func(msg *amqp.Delivery) error {
		return consumeNotificationTask(ctx, msg.Body)
	})

	return nil
}

func consumeNotificationTask(ctx context.Context, body []byte) error {
	var task model.NotificationTask
	if err := json.Unmarshal(body, &task); err != nil {
		return fmt.Errorf("unmarshal notification task failed: %w", err)
	}

	log.Printf(
		"notify worker consumed: event=%s userID=%d sessionID=%s messageKey=%s summary=%s",
		task.EventType,
		task.UserID,
		task.SessionID,
		task.MessageKey,
		task.Summary,
	)
	return nil
}
