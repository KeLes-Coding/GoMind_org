package worker

import (
	"GopherAI/common/rabbitmq"
	outboxDAO "GopherAI/dao/outbox"
	"context"
	"log"
	"time"
)

const (
	// messageOutboxRelayBatchSize 限制单次 relay 扫描最多处理多少条消息。
	// 这样即使短时间内积压较多，也不会一次性把 MQ 或数据库打满。
	messageOutboxRelayBatchSize = 100
)

// StartMessageOutboxRelayWorker 启动消息 outbox relay worker。
// 它负责把数据库里待补偿的消息事件重新发布到 MQ，直到消费端确认 delivered。
func StartMessageOutboxRelayWorker(ctx context.Context) error {
	ticker := time.NewTicker(sessionPersistenceScanInterval)
	defer ticker.Stop()

	relayPendingMessageOutboxes()

	for {
		select {
		case <-ctx.Done():
			log.Println("message outbox relay worker stopped")
			return nil
		case <-ticker.C:
			relayPendingMessageOutboxes()
		}
	}
}

// relayPendingMessageOutboxes 执行一次 outbox 补偿扫描。
// 这里允许重复发布，因为消费端已经用 message_key 做幂等，重复投递不会重复写坏消息表。
func relayPendingMessageOutboxes() {
	if rabbitmq.RMQMessage == nil {
		return
	}

	events, err := outboxDAO.ListMessageOutboxesReadyForRelay(messageOutboxRelayBatchSize)
	if err != nil {
		log.Printf("ListMessageOutboxesReadyForRelay failed: %v", err)
		return
	}
	if len(events) == 0 {
		return
	}

	for _, event := range events {
		if err := rabbitmq.RMQMessage.Publish([]byte(event.Payload)); err != nil {
			if markErr := outboxDAO.MarkMessageOutboxPublishFailed(event.MessageKey, err.Error()); markErr != nil {
				log.Printf("MarkMessageOutboxPublishFailed failed: messageKey=%s err=%v markErr=%v", event.MessageKey, err, markErr)
			}
			log.Printf("relay message outbox publish failed: messageKey=%s sessionID=%s err=%v", event.MessageKey, event.SessionID, err)
			continue
		}

		if err := outboxDAO.MarkMessageOutboxPublished(event.MessageKey); err != nil {
			log.Printf("MarkMessageOutboxPublished failed: messageKey=%s err=%v", event.MessageKey, err)
			continue
		}
		log.Printf("relay message outbox published: messageKey=%s sessionID=%s version=%d", event.MessageKey, event.SessionID, event.SessionVersion)
	}
}
