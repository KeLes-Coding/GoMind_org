package notify

import (
	"GopherAI/common/rabbitmq"
	"GopherAI/model"
	"context"
	"encoding/json"
	"fmt"
)

var publishTaskFunc = publishTaskToMQ

// publishTaskToMQ 把通知任务投递到独立通知队列。
// 这里沿用“共享 connection + 短生命周期 channel”的策略，避免影响常驻 worker。
func publishTaskToMQ(ctx context.Context, task *model.NotificationTask) error {
	if task == nil {
		return fmt.Errorf("notification task is nil")
	}

	taskData, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal notification task failed: %w", err)
	}

	mq, err := rabbitmq.NewWorkRabbitMQ(model.QueueNotification)
	if err != nil {
		return fmt.Errorf("create notification mq publisher failed: %w", err)
	}
	defer mq.CloseChannels()

	return mq.Publish(taskData)
}
