package task

import (
	"GopherAI/common/rabbitmq"
	"GopherAI/model"
	"context"
	"encoding/json"
	"fmt"
)

// PublishVectorizeTask 发布向量化任务到 RabbitMQ
func PublishVectorizeTask(ctx context.Context, fileID string, version int) error {
	task := model.VectorizeTask{
		FileID:  fileID,
		Version: version,
	}

	// 序列化任务
	taskData, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	// 创建 RabbitMQ 实例并发布
	mq := rabbitmq.NewWorkRabbitMQ(model.QueueVectorize)
	defer mq.Destroy()

	return mq.Publish(taskData)
}
