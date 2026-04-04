package worker

import (
	"GopherAI/common/rabbitmq"
	"GopherAI/model"
	"context"
	"log"

	"github.com/streadway/amqp"
)

// StartVectorizeWorker 启动文件向量化 worker。
// 这个 worker 只负责真正消费向量化任务并处理文件内容，不承担补偿扫描职责。
func StartVectorizeWorker(ctx context.Context) error {
	worker := NewVectorizeWorker()

	// worker 使用自己独立的 RabbitMQ channel。
	// 这样发布和消费职责边界更清晰，也更符合后续独立部署的方向。
	mq := rabbitmq.NewWorkRabbitMQ(model.QueueVectorize)
	defer mq.Destroy()

	// 当前一个 worker 进程内部仍按消息顺序消费。
	// 如果后面要提高吞吐，优先方式是增加 worker 副本，而不是在单进程里堆复杂并发逻辑。
	mq.Consume(func(msg *amqp.Delivery) error {
		return worker.ProcessTask(ctx, msg.Body)
	})

	return nil
}

// StartAllWorkers 用于单机一体化运行。
// 当前会顺带启动三类后台 worker：
// 1. 向量化 worker 负责真正处理文件；
// 2. 文件补偿 worker 负责把上传成功但没成功入队的文件补发到 MQ；
// 3. 聊天 outbox relay worker 负责补偿消息发布；
// 4. 会话补偿 worker 负责推进聊天链路的 persisted_version 水位。
func StartAllWorkers(ctx context.Context) {
	go func() {
		if err := StartVectorizeWorker(ctx); err != nil {
			log.Printf("Vectorize worker error: %v", err)
		}
	}()

	go func() {
		if err := StartVectorTaskCompensationWorker(ctx); err != nil {
			log.Printf("Vector task compensation worker error: %v", err)
		}
	}()

	go func() {
		if err := StartSessionPersistenceCompensationWorker(ctx); err != nil {
			log.Printf("Session persistence compensation worker error: %v", err)
		}
	}()

	go func() {
		if err := StartMessageOutboxRelayWorker(ctx); err != nil {
			log.Printf("Message outbox relay worker error: %v", err)
		}
	}()

	log.Println("All workers started")
}
