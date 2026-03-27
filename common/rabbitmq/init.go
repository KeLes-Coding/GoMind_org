package rabbitmq

var (
	RMQMessage *RabbitMQ
)

func InitRabbitMQ() {
	// 创建MQ并启动消费者
	// 无论调用多少次 NewWorkRabbitMQ，只会创建一次连接
	// 不同队列共用一个连接，可以保持不同队列消费消息的顺序

	// 统一使用包内常量，避免主队列名和治理逻辑里的死信队列名发生漂移。
	RMQMessage = NewWorkRabbitMQ(messageQueueName)
	go RMQMessage.Consume(MQMessage)

}

// DestroyRabbitMQ 销毁RabbitMQ
func DestroyRabbitMQ() {
	RMQMessage.Destroy()
}
