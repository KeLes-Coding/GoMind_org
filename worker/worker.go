package worker

import (
	"GopherAI/common/rabbitmq"
	"GopherAI/model"
	"context"
	"log"

	"github.com/streadway/amqp"
)

// StartVectorizeWorker 启动向量化 Worker
func StartVectorizeWorker(ctx context.Context) error {
	worker := NewVectorizeWorker()

	// 创建 RabbitMQ 实例
	mq := rabbitmq.NewWorkRabbitMQ(model.QueueVectorize)
	defer mq.Destroy()

	// 消费队列
	mq.Consume(func(msg *amqp.Delivery) error {
		return worker.ProcessTask(ctx, msg.Body)
	})

	return nil
}

// StartAllWorkers 启动所有 Worker（可在 main.go 中调用）
func StartAllWorkers(ctx context.Context) {
	go func() {
		if err := StartVectorizeWorker(ctx); err != nil {
			log.Printf("Vectorize worker error: %v", err)
		}
	}()

	log.Println("All workers started")
}
