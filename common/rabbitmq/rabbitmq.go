package rabbitmq

import (
	"GopherAI/common/observability"
	"GopherAI/config"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/streadway/amqp"
)

var conn *amqp.Connection

const (
	// 主业务消息超过最大重试次数后，不再无限回队列，而是转入死信队列等待人工处理。
	messageQueueName    = "Message"
	messageDLQName      = "Message.dlq"
	messageRetryHeader  = "x-retry-count"
	messageMaxRetryTime = 3
)

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

// retryCountFromHeaders 从消息头里提取当前已重试次数。
// MQ Header 的数值类型在不同驱动和 broker 路径下可能表现成多种整数类型，
// 这里统一做兼容转换，避免治理逻辑因为类型断言失败而失效。
func retryCountFromHeaders(headers amqp.Table) int {
	if headers == nil {
		return 0
	}

	value, exists := headers[messageRetryHeader]
	if !exists {
		return 0
	}

	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint8:
		return int(typed)
	case uint16:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(typed)
		if err == nil {
			return parsed
		}
	}

	return 0
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
	queue, err := channel.QueueDeclare(
		r.Key,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return amqp.Queue{}, err
	}

	// 每次声明/探活队列时顺手记录当前队列深度，代价低，但足够满足“可治理”阶段的巡检需求。
	observability.RecordMQQueueDepth(observability.MQQueueMain, queue.Messages)
	return queue, nil
}

// ensureDeadLetterQueue 保证死信队列存在。
// 这一步先用最直接的独立队列方案，不引入额外 exchange 拓扑，保持当前项目接入成本最低。
func (r *RabbitMQ) ensureDeadLetterQueue(channel *amqp.Channel) (amqp.Queue, error) {
	queue, err := channel.QueueDeclare(
		messageDLQName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return amqp.Queue{}, err
	}

	observability.RecordMQQueueDepth(observability.MQQueueDLQ, queue.Messages)
	return queue, nil
}

// publishWithHeaders 统一封装带 header 的持久化发布逻辑。
// 重试次数、死信来源等治理信息都通过这里进入 MQ。
func (r *RabbitMQ) publishWithHeaders(queueName string, message []byte, headers amqp.Table) error {
	r.publishMu.Lock()
	defer r.publishMu.Unlock()

	queue, err := r.publishChannel.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		return err
	}
	if queueName == messageDLQName {
		observability.RecordMQQueueDepth(observability.MQQueueDLQ, queue.Messages)
	} else if queueName == r.Key {
		observability.RecordMQQueueDepth(observability.MQQueueMain, queue.Messages)
	}

	return r.publishChannel.Publish(
		r.Exchange,
		queueName,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         message,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
			Headers:      headers,
		},
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
	if _, err := r.ensureDeadLetterQueue(r.consumeChannel); err != nil {
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

			retryCount := retryCountFromHeaders(msg.Headers)
			nextRetryCount := retryCount + 1
			headers := amqp.Table{}
			for key, value := range msg.Headers {
				headers[key] = value
			}
			headers[messageRetryHeader] = nextRetryCount

			// 这里不再直接无限 requeue。
			// 原因是无限重回主队列会形成“毒消息风暴”，让积压越来越深，而且很难观测。
			if nextRetryCount > messageMaxRetryTime {
				if publishErr := r.publishWithHeaders(messageDLQName, msg.Body, headers); publishErr != nil {
					log.Println("rabbitmq publish dead letter error:", publishErr)
					observability.RecordMQNack()
					_ = msg.Nack(false, true)
					continue
				}

				observability.RecordMQDeadLetter()
				if ackErr := msg.Ack(false); ackErr != nil {
					log.Println("rabbitmq ack after dead letter error:", ackErr)
					observability.RecordMQAckFail()
				}
				continue
			}

			if publishErr := r.publishWithHeaders(r.Key, msg.Body, headers); publishErr != nil {
				log.Println("rabbitmq republish retry error:", publishErr)
				observability.RecordMQNack()
				_ = msg.Nack(false, true)
				continue
			}

			observability.RecordMQRetry()
			if ackErr := msg.Ack(false); ackErr != nil {
				log.Println("rabbitmq ack after republish error:", ackErr)
				observability.RecordMQAckFail()
			}
			continue
		}
		observability.RecordMQConsume(true)

		if err := msg.Ack(false); err != nil {
			log.Println("rabbitmq ack error:", err)
			observability.RecordMQAckFail()
		}
	}
}
