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
	// messageQueueName 是主业务消息队列。
	// 当前它主要服务站内消息相关逻辑，不直接参与文件向量化，
	// 但整个 RabbitMQ 基础设施是共享的，所以连接生命周期策略必须统一设计。
	messageQueueName = "Message"
	// messageDLQName 是主业务消息的死信队列。
	// 当同一条消息超过最大重试次数后，会被投递到这里等待人工排查。
	messageDLQName = "Message.dlq"
	// messageRetryHeader 记录当前消息已重试的次数。
	messageRetryHeader = "x-retry-count"
	// messageMaxRetryTime 限制单条消息的最大自动重试次数，避免毒消息无限回队。
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

// RabbitMQ 同时维护发布和消费两条 channel。
// 这样做的核心目的，是把“消息发布确认”和“消息消费确认”隔离开，
// 避免两个职责互相干扰，也便于后续独立观察问题。
type RabbitMQ struct {
	conn           *amqp.Connection
	publishChannel *amqp.Channel
	consumeChannel *amqp.Channel
	confirmChan    <-chan amqp.Confirmation
	publishMu      sync.Mutex
	Exchange       string
	Key            string
}

// retryCountFromHeaders 从消息头中解析当前已经重试的次数。
// 之所以单独做兼容解析，是因为 RabbitMQ Header 在不同驱动/路径下可能反序列化成不同数值类型。
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

// CloseChannels 只关闭当前实例自己打开的 channel。
// 这是这次修复的关键点：
// 1. 当前项目把 amqp.Connection 设计成包级共享单例；
// 2. 文件上传发布任务时，会临时创建一个 RabbitMQ 实例；
// 3. 如果该实例在请求结束时直接关闭共享 connection，就会把 worker 消费者的连接一起打断。
// 因此短生命周期发布器只能调用 CloseChannels，不能直接 Destroy。
func (r *RabbitMQ) CloseChannels() {
	if r.publishChannel != nil {
		_ = r.publishChannel.Close()
		r.publishChannel = nil
	}
	if r.consumeChannel != nil {
		_ = r.consumeChannel.Close()
		r.consumeChannel = nil
	}
}

// Destroy 用于销毁当前实例持有的全部资源。
// 这个方法保留给真正拥有连接生命周期的场景，例如应用退出或统一资源清理。
func (r *RabbitMQ) Destroy() {
	r.CloseChannels()
	if r.conn != nil {
		_ = r.conn.Close()
		if conn == r.conn {
			conn = nil
		}
		r.conn = nil
	}
}

// NewWorkRabbitMQ 创建 work queue 模式实例。
// 这里继续沿用共享 connection + 独立 channel 的结构：
// 1. 连接成本高，适合复用；
// 2. channel 成本低，适合按实例拆分职责。
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

	// 开启发布确认，只有 broker 确认收到消息后，Publish 才返回成功。
	if err = rabbitmq.publishChannel.Confirm(false); err != nil {
		panic(err.Error())
	}
	rabbitmq.confirmChan = rabbitmq.publishChannel.NotifyPublish(make(chan amqp.Confirmation, 1))

	// 每次只预取一条消息，确保消费端按“处理完成后再取下一条”的节奏前进。
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

	// 每次声明/探活队列时顺手记录当前队列深度，满足基础观测需求。
	observability.RecordMQQueueDepth(observability.MQQueueMain, queue.Messages)
	return queue, nil
}

// ensureDeadLetterQueue 保证死信队列存在。
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
// 消息重试次数等治理信息，都通过这个方法写回 MQ。
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
// 只有 handle 成功返回后才确认消费；失败则按有限重试策略重新投递，超过阈值后进入死信队列。
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

			// 不再对失败消息做无限 requeue，避免形成毒消息风暴。
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
