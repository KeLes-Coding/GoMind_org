package rabbitmq

import (
	"GopherAI/common/observability"
	"GopherAI/config"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/streadway/amqp"
)

var conn *amqp.Connection

func initConn() {
	c := config.GetConfig()
	mqURL := fmt.Sprintf(
		"amqp://%s:%s@%s:%d/%s",
		c.RabbitmqUsername, c.RabbitmqPassword, c.RabbitmqHost, c.RabbitmqPort, c.RabbitmqVhost,
	)

	var err error
	conn, err = amqp.Dial(mqURL)
	if err != nil {
		log.Fatalf("RabbitMQ connection failed: %v", err)
	}
}

// RabbitMQ 同时维护发布和消费两个 channel。
// 这样发布确认和消费确认就不会互相干扰。
type RabbitMQ struct {
	conn           *amqp.Connection
	publishChannel *amqp.Channel
	consumeChannel *amqp.Channel
	confirmChan    <-chan amqp.Confirmation
	publishMu      sync.Mutex
	Exchange       string
	Key            string
}

func NewRabbitMQ(exchange string, key string) *RabbitMQ {
	return &RabbitMQ{Exchange: exchange, Key: key}
}

// Destroy 关闭 channel 和 connection。
func (r *RabbitMQ) Destroy() {
	if r.publishChannel != nil {
		_ = r.publishChannel.Close()
	}
	if r.consumeChannel != nil {
		_ = r.consumeChannel.Close()
	}
	if r.conn != nil {
		_ = r.conn.Close()
	}
}

// NewWorkRabbitMQ 创建 work queue 模式实例。
func NewWorkRabbitMQ(queue string) *RabbitMQ {
	rabbitmq := NewRabbitMQ("", queue)

	if conn == nil {
		initConn()
	}
	rabbitmq.conn = conn

	var err error
	rabbitmq.publishChannel, err = rabbitmq.conn.Channel()
	if err != nil {
		panic(err.Error())
	}

	rabbitmq.consumeChannel, err = rabbitmq.conn.Channel()
	if err != nil {
		panic(err.Error())
	}

	// 打开发布确认；只有 broker 确认收到消息后，Publish 才返回成功。
	if err = rabbitmq.publishChannel.Confirm(false); err != nil {
		panic(err.Error())
	}
	rabbitmq.confirmChan = rabbitmq.publishChannel.NotifyPublish(make(chan amqp.Confirmation, 1))

	// 每次只预取一条，确保消费端只有在当前消息处理完成后才拿下一条。
	if err = rabbitmq.consumeChannel.Qos(1, 0, false); err != nil {
		panic(err.Error())
	}

	return rabbitmq
}

// ensureQueue 保证发布和消费看到的是同一个 durable queue。
func (r *RabbitMQ) ensureQueue(channel *amqp.Channel) (amqp.Queue, error) {
	return channel.QueueDeclare(
		r.Key,
		true,
		false,
		false,
		false,
		nil,
	)
}

// Publish 发送消息到队列，并等待 broker 的发布确认。
func (r *RabbitMQ) Publish(message []byte) error {
	r.publishMu.Lock()
	defer r.publishMu.Unlock()

	if _, err := r.ensureQueue(r.publishChannel); err != nil {
		observability.RecordMQPublish(false)
		return err
	}

	if err := r.publishChannel.Publish(r.Exchange, r.Key, false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         message,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
		},
	); err != nil {
		observability.RecordMQPublish(false)
		return err
	}

	select {
	case confirm, ok := <-r.confirmChan:
		if !ok {
			observability.RecordMQPublish(false)
			return fmt.Errorf("rabbitmq confirm channel closed")
		}
		if !confirm.Ack {
			observability.RecordMQPublish(false)
			return fmt.Errorf("rabbitmq broker nack message")
		}
		observability.RecordMQPublish(true)
		return nil
	case <-time.After(5 * time.Second):
		observability.RecordMQPublish(false)
		return fmt.Errorf("rabbitmq publish confirm timeout")
	}
}

// Consume 消费消息并使用手动 ack。
// 只有 handle 成功返回后才确认消费，失败则重新入队。
func (r *RabbitMQ) Consume(handle func(msg *amqp.Delivery) error) {
	q, err := r.ensureQueue(r.consumeChannel)
	if err != nil {
		panic(err)
	}

	msgs, err := r.consumeChannel.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		panic(err)
	}

	for msg := range msgs {
		if err := handle(&msg); err != nil {
			log.Println("rabbitmq consume handle error:", err)
			observability.RecordMQConsume(false)
			observability.RecordMQNack()
			_ = msg.Nack(false, true)
			continue
		}
		observability.RecordMQConsume(true)

		if err := msg.Ack(false); err != nil {
			log.Println("rabbitmq ack error:", err)
			observability.RecordMQAckFail()
		}
	}
}
