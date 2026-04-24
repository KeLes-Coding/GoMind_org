package notify

import (
	"GopherAI/common/observability"
	"GopherAI/model"
	"context"
	"fmt"
	"strings"
	"time"
)

const notificationSummaryLimit = 120

// ChatMessageReadyParams 描述聊天完成通知发布所需的最小参数集。
type ChatMessageReadyParams struct {
	UserID     int64
	SessionID  string
	MessageKey string
	Content    string
}

// PublishChatMessageReady 发布 assistant 完成态通知。
// 这条链路是典型旁路任务：
// 1. 不参与聊天核心消息是否成功落库的判定；
// 2. 只在主链路已经完成正式持久化后做 best-effort 投递；
// 3. 当前阶段消费端只打印终端日志，为后续前端通知预留统一事件格式。
func PublishChatMessageReady(ctx context.Context, params ChatMessageReadyParams) error {
	if params.UserID <= 0 {
		return fmt.Errorf("invalid user id")
	}
	if strings.TrimSpace(params.SessionID) == "" {
		return fmt.Errorf("session id is required")
	}
	if strings.TrimSpace(params.MessageKey) == "" {
		return fmt.Errorf("message key is required")
	}

	task := &model.NotificationTask{
		EventType:  model.NotificationEventChatMessageReady,
		UserID:     params.UserID,
		SessionID:  strings.TrimSpace(params.SessionID),
		MessageKey: strings.TrimSpace(params.MessageKey),
		Summary:    buildSummary(params.Content),
		CreatedAt:  time.Now(),
	}

	if err := publishTaskFunc(ctx, task); err != nil {
		// 通知链路是旁路任务，失败时只记观测并把错误回给调用方决定是否吞掉。
		observability.RecordNotificationPublishFail()
		return err
	}
	return nil
}

func buildSummary(content string) string {
	summary := strings.TrimSpace(content)
	summary = strings.ReplaceAll(summary, "\r", " ")
	summary = strings.ReplaceAll(summary, "\n", " ")
	summary = strings.Join(strings.Fields(summary), " ")
	if len(summary) <= notificationSummaryLimit {
		return summary
	}
	return summary[:notificationSummaryLimit]
}
