package task

import (
	"GopherAI/common/rabbitmq"
	"GopherAI/model"
	"context"
	"encoding/json"
	"fmt"
)

// PublishVectorizeTask 发布文件向量化任务到 RabbitMQ。
// 这里虽然只是一个很小的发布动作，但资源释放策略非常关键：
// 1. 当前项目里的 RabbitMQ connection 是包级共享的；
// 2. worker 消费向量化任务时，也会复用这条共享 connection；
// 3. 因此发布结束后只能关闭本次临时创建的 channel，不能顺手把 connection 一起关掉。
func PublishVectorizeTask(ctx context.Context, fileID string, version int) error {
	task := model.VectorizeTask{
		FileID:  fileID,
		Version: version,
	}

	// 先把任务结构序列化成 JSON，作为工作队列里的消息体。
	taskData, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	// 这里只释放本次发布新开的 channel，不销毁共享 connection。
	// 否则上传接口每发布一次任务，都可能把 worker 正在使用的连接一起关掉。
	mq := rabbitmq.NewWorkRabbitMQ(model.QueueVectorize)
	defer mq.CloseChannels()

	return mq.Publish(taskData)
}
