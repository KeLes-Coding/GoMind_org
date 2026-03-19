package aihelper

import (
	"GopherAI/common/observability"
	"GopherAI/common/rabbitmq"
	"GopherAI/model"
	"GopherAI/utils"
	"context"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
)

// AIHelper 绑定一个具体会话的模型实例与消息上下文。
// 它是“运行时对象”，负责把当前会话组织成模型可消费的 message 列表。
type AIHelper struct {
	model    AIModel
	messages []*model.Message
	mu       sync.RWMutex
	// 一个 session 只绑定一个 AIHelper。
	SessionID string
	saveFunc  func(*model.Message) (*model.Message, error)
	// contextSummary 持久化“较早历史”的摘要；
	// summaryMessageCount 表示摘要已经覆盖了 messages 的前多少条。
	contextSummary      string
	summaryMessageCount int
}

const maxContextMessages = 20

// NewAIHelper 创建新的 AIHelper。
func NewAIHelper(model_ AIModel, SessionID string) *AIHelper {
	return &AIHelper{
		model:    model_,
		messages: make([]*model.Message, 0),
		// 默认通过 RabbitMQ 异步落库，降低聊天主链路上的数据库写入压力。
		saveFunc: func(msg *model.Message) (*model.Message, error) {
			data := rabbitmq.GenerateMessageMQParam(msg.MessageKey, msg.SessionID, msg.Content, msg.UserName, msg.IsUser)
			err := rabbitmq.RMQMessage.Publish(data)
			return msg, err
		},
		SessionID: SessionID,
	}
}

// AddMessage 把一条消息加入当前会话的内存上下文。
// Save=true 时继续走异步持久化；Save=false 时只做内存回放，不重复入库。
func (a *AIHelper) AddMessage(Content string, UserName string, IsUser bool, Save bool) {
	userMsg := model.Message{
		MessageKey: utils.GenerateUUID(),
		SessionID:  a.SessionID,
		Content:    Content,
		UserName:   UserName,
		IsUser:     IsUser,
	}

	// 写切片时必须加锁；否则同一个 session 并发写入会有数据竞争风险。
	a.mu.Lock()
	a.messages = append(a.messages, &userMsg)
	a.mu.Unlock()

	if Save {
		a.saveFunc(&userMsg)
	}
}

// LoadMessages 用数据库中的历史消息回放当前会话上下文。
// 这里不会再次触发 saveFunc，避免把旧消息重复投递到消息队列。
func (a *AIHelper) LoadMessages(messages []model.Message) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.messages = make([]*model.Message, 0, len(messages))
	for i := range messages {
		msg := messages[i]
		msg.SessionID = a.SessionID
		a.messages = append(a.messages, &msg)
	}
}

// SetSaveFunc 允许外部注入消息持久化策略，便于测试或切换持久化实现。
func (a *AIHelper) SetSaveFunc(saveFunc func(*model.Message) (*model.Message, error)) {
	a.saveFunc = saveFunc
}

// SetSummaryState 用持久化层保存的摘要状态覆盖当前 helper。
// 这让 helper 在重启、重建后也能继续复用已经生成过的摘要。
func (a *AIHelper) SetSummaryState(summary string, summaryMessageCount int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.contextSummary = summary
	if summaryMessageCount < 0 {
		summaryMessageCount = 0
	}
	if summaryMessageCount > len(a.messages) {
		summaryMessageCount = len(a.messages)
	}
	a.summaryMessageCount = summaryMessageCount
}

// GetSummaryState 返回当前 helper 的摘要快照，供 service 判断是否需要回写数据库。
func (a *AIHelper) GetSummaryState() (string, int) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.contextSummary, a.summaryMessageCount
}

// GetMessages 返回当前会话的内存消息快照，避免外部直接修改内部切片。
func (a *AIHelper) GetMessages() []*model.Message {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]*model.Message, len(a.messages))
	copy(out, a.messages)
	return out
}

// ensureContextSummary 保证“超过窗口之外的更早消息”都被压缩进摘要。
// 这样 buildModelMessages 就可以稳定地发送“摘要 + 最近 N 条”。
func (a *AIHelper) ensureContextSummary(ctx context.Context) error {
	a.mu.RLock()
	targetSummaryCount := len(a.messages) - maxContextMessages
	if targetSummaryCount < 0 {
		targetSummaryCount = 0
	}
	currentSummary := a.contextSummary
	currentSummaryCount := a.summaryMessageCount
	if targetSummaryCount <= currentSummaryCount {
		a.mu.RUnlock()
		return nil
	}

	messagesToSummarize := make([]*model.Message, targetSummaryCount-currentSummaryCount)
	copy(messagesToSummarize, a.messages[currentSummaryCount:targetSummaryCount])
	a.mu.RUnlock()

	summaryStart := time.Now()
	newSummary, err := a.model.GenerateSummary(ctx, currentSummary, utils.ConvertToSchemaMessages(messagesToSummarize))
	if err != nil {
		observability.RecordSummaryRefresh(false)
		observability.RecordModelCall("summary", a.model.GetModelType(), false, time.Since(summaryStart), len(messagesToSummarize), currentSummary != "")
		return err
	}
	observability.RecordSummaryRefresh(true)
	observability.RecordModelCall("summary", a.model.GetModelType(), true, time.Since(summaryStart), len(messagesToSummarize), currentSummary != "")

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.summaryMessageCount != currentSummaryCount {
		return nil
	}

	a.contextSummary = newSummary
	a.summaryMessageCount = targetSummaryCount
	return nil
}

// buildModelMessages 把当前会话消息裁剪成适合发送给模型的上下文窗口。
// 我们保留完整历史用于回放和查询，但真正发给模型时只发送“摘要 + 最近 N 条”，控制 token 成本和延迟。
func (a *AIHelper) buildModelMessages() []*schema.Message {
	a.mu.RLock()
	defer a.mu.RUnlock()

	start := a.summaryMessageCount
	if len(a.messages)-start > maxContextMessages {
		start = len(a.messages) - maxContextMessages
	}

	schemaMessages := make([]*schema.Message, 0, 1+len(a.messages[start:]))
	if a.contextSummary != "" {
		schemaMessages = append(schemaMessages, &schema.Message{
			Role: schema.System,
			Content: "以下是当前会话较早历史的摘要，请在回答时延续这些上下文信息：\n" +
				a.contextSummary,
		})
	}

	schemaMessages = append(schemaMessages, utils.ConvertToSchemaMessages(a.messages[start:])...)
	return schemaMessages
}

// GenerateResponse 走同步模型调用。
func (a *AIHelper) GenerateResponse(userName string, ctx context.Context, userQuestion string) (*model.Message, error) {
	// 先把用户本轮问题写入上下文，保证模型能看到完整多轮历史。
	a.AddMessage(userQuestion, userName, true, true)

	if err := a.ensureContextSummary(ctx); err != nil {
		return nil, err
	}

	messages := a.buildModelMessages()
	callStart := time.Now()
	usedSummary := false
	if summary, _ := a.GetSummaryState(); summary != "" {
		usedSummary = true
	}

	schemaMsg, err := a.model.GenerateResponse(ctx, messages)
	if err != nil {
		observability.RecordModelCall("generate", a.model.GetModelType(), false, time.Since(callStart), len(messages), usedSummary)
		return nil, err
	}
	observability.RecordModelCall("generate", a.model.GetModelType(), true, time.Since(callStart), len(messages), usedSummary)

	modelMsg := utils.ConvertToModelMessage(a.SessionID, userName, schemaMsg)

	// 再把模型回复写回上下文，这样下一轮问题就能带上当前回答。
	a.AddMessage(modelMsg.Content, userName, false, true)

	return modelMsg, nil
}

// StreamResponse 走流式模型调用。
func (a *AIHelper) StreamResponse(userName string, ctx context.Context, cb StreamCallback, userQuestion string) (*model.Message, error) {
	// 流式场景也要先把用户问题放入上下文，否则模型拿不到当前轮问题。
	a.AddMessage(userQuestion, userName, true, true)

	if err := a.ensureContextSummary(ctx); err != nil {
		return nil, err
	}

	messages := a.buildModelMessages()
	callStart := time.Now()
	usedSummary := false
	if summary, _ := a.GetSummaryState(); summary != "" {
		usedSummary = true
	}

	content, err := a.model.StreamResponse(ctx, messages, cb)
	if err != nil {
		observability.RecordModelCall("stream", a.model.GetModelType(), false, time.Since(callStart), len(messages), usedSummary)
		return nil, err
	}
	observability.RecordModelCall("stream", a.model.GetModelType(), true, time.Since(callStart), len(messages), usedSummary)

	modelMsg := &model.Message{
		SessionID: a.SessionID,
		UserName:  userName,
		Content:   content,
		IsUser:    false,
	}

	// 流式结束后把完整回复写回上下文，为下一轮对话做准备。
	a.AddMessage(modelMsg.Content, userName, false, true)

	return modelMsg, nil
}

// GetModelType 返回当前 helper 绑定的模型类型。
func (a *AIHelper) GetModelType() string {
	return a.model.GetModelType()
}
