package aihelper

import (
	"GopherAI/common/observability"
	"GopherAI/common/rabbitmq"
	messageDAO "GopherAI/dao/message"
	"GopherAI/model"
	"GopherAI/utils"
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

// AIHelper 绑定一个具体会话的模型实例与消息上下文。
// 它是“运行时对象”，负责把当前会话组织成模型可消费的 message 列表。
type AIHelper struct {
	model AIModel
	// fallbackModel 不是默认常驻主链路的模型，而是“主模型连续失败后”的兜底。
	// 这样可以把平时成本和稳定性做拆分：正常走主模型，异常时优先保可用。
	fallbackModel AIModel
	messages      []*model.Message
	mu            sync.RWMutex
	// 一个 session 只绑定一个 AIHelper。
	SessionID string
	saveFunc  func(*model.Message) (*model.Message, error)
	// contextSummary 持久化“较早历史”的摘要；
	// summaryMessageCount 表示摘要已经覆盖了 messages 的前多少条。
	contextSummary      string
	summaryMessageCount int
	// version 用于给“共享热状态快照”打版本号。
	// 这样后续把状态同步到 Redis 时，可以知道这是哪一次会话推进之后产生的新快照。
	version int64
}

const maxContextMessages = 20

// NewAIHelper 创建新的 AIHelper。
func NewAIHelper(model_ AIModel, SessionID string) *AIHelper {
	return &AIHelper{
		model:    model_,
		messages: make([]*model.Message, 0),
		// 默认通过 RabbitMQ 异步落库，降低聊天主链路上的数据库写入压力。
		saveFunc: func(msg *model.Message) (*model.Message, error) {
			// 先尝试走 MQ，只有在 MQ 当前不可用或发布失败时，才退回同步落库。
			// 这样主链路仍然以“异步削峰”为主，但在 MQ 故障场景下不会直接失去可用性。
			data := rabbitmq.GenerateMessageMQParam(msg.MessageKey, msg.SessionID, msg.Content, msg.UserName, msg.IsUser)
			if rabbitmq.RMQMessage != nil {
				if err := rabbitmq.RMQMessage.Publish(data); err == nil {
					return msg, nil
				}
			}

			observability.RecordMQFallback()
			persistedMsg, err := messageDAO.CreateMessage(msg)
			if err != nil && !errors.Is(err, gorm.ErrDuplicatedKey) {
				return msg, err
			}
			if persistedMsg != nil {
				*msg = *persistedMsg
			}
			return msg, nil
		},
		SessionID: SessionID,
		version:   1,
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
	a.version++
	a.mu.Unlock()

	if Save {
		if _, err := a.saveFunc(&userMsg); err != nil {
			// 持久化失败不应该把整个会话内存态直接回滚，否则会引入更复杂的上下文不一致。
			// 这里先明确记日志，后续可以继续把失败消息做补偿扫描。
			log.Println("AIHelper AddMessage persist message error:", err)
		}
	}
}

// LoadHotState 用共享热状态快照恢复 helper。
// 这一步不替代数据库真相，只是尽量减少“每次都全量读 DB 回放”的成本。
func (a *AIHelper) LoadHotState(state *model.SessionHotState) {
	if state == nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.messages = make([]*model.Message, 0, len(state.RecentMessages))
	for i := range state.RecentMessages {
		msg := state.RecentMessages[i]
		modelMsg := &model.Message{
			ID:         msg.ID,
			MessageKey: msg.MessageKey,
			SessionID:  msg.SessionID,
			UserName:   msg.UserName,
			Content:    msg.Content,
			IsUser:     msg.IsUser,
			CreatedAt:  msg.CreatedAt,
		}
		a.messages = append(a.messages, modelMsg)
	}

	a.contextSummary = state.ContextSummary
	a.summaryMessageCount = state.SummaryMessageCount
	if state.Version > 0 {
		a.version = state.Version
	}
}

// ExportHotState 把 helper 当前“适合共享”的状态导出成快照。
// 这里刻意只保留最近窗口消息，而不导出整个 messages 切片，
// 是为了让 Redis 保持轻量，避免共享状态无限膨胀。
func (a *AIHelper) ExportHotState() *model.SessionHotState {
	a.mu.RLock()
	defer a.mu.RUnlock()

	start := 0
	if len(a.messages) > maxContextMessages {
		start = len(a.messages) - maxContextMessages
	}

	recentMessages := make([]model.SessionHotMessage, 0, len(a.messages[start:]))
	for _, msg := range a.messages[start:] {
		recentMessages = append(recentMessages, model.SessionHotMessage{
			ID:         msg.ID,
			MessageKey: msg.MessageKey,
			SessionID:  msg.SessionID,
			UserName:   msg.UserName,
			Content:    msg.Content,
			IsUser:     msg.IsUser,
			CreatedAt:  msg.CreatedAt,
		})
	}

	return &model.SessionHotState{
		SessionID:           a.SessionID,
		Version:             a.version,
		UpdatedAt:           time.Now(),
		ContextSummary:      a.contextSummary,
		SummaryMessageCount: a.summaryMessageCount,
		RecentMessages:      recentMessages,
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
	a.version++
}

// SetSaveFunc 允许外部注入消息持久化策略，便于测试或切换持久化实现。
func (a *AIHelper) SetSaveFunc(saveFunc func(*model.Message) (*model.Message, error)) {
	a.saveFunc = saveFunc
}

// SetFallbackModel 注入备用模型。
// 备用模型只在主模型连续失败后使用，避免正常情况下平白增加额外调用成本。
func (a *AIHelper) SetFallbackModel(fallbackModel AIModel) {
	a.fallbackModel = fallbackModel
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
	a.version++
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

// GetLatestMessage 返回当前 helper 里最新的一条消息快照。
// 如果当前没有任何消息，则返回 nil。
func (a *AIHelper) GetLatestMessage() *model.Message {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if len(a.messages) == 0 {
		return nil
	}

	latest := *a.messages[len(a.messages)-1]
	return &latest
}

// GetLatestPersistedMessage 返回当前 helper 中“已经成功落过库”的最后一条消息。
// 这里通过 Message.ID>0 判断消息是否来自数据库持久化记录。
func (a *AIHelper) GetLatestPersistedMessage() *model.Message {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for i := len(a.messages) - 1; i >= 0; i-- {
		if a.messages[i].ID > 0 {
			msg := *a.messages[i]
			return &msg
		}
	}
	return nil
}

// GetPersistedMessageCount 返回当前 helper 中来自数据库持久化记录的消息数量。
func (a *AIHelper) GetPersistedMessageCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	count := 0
	for _, msg := range a.messages {
		if msg.ID > 0 {
			count++
		}
	}
	return count
}

// HasMessageKey 判断当前 helper 是否已经持有某条指定消息。
func (a *AIHelper) HasMessageKey(messageKey string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, msg := range a.messages {
		if msg.MessageKey == messageKey {
			return true
		}
	}
	return false
}

// ReconcileMessages 用数据库消息作为持久化基底，再把本地独有的未落库消息追加回去。
// 这样既不会让 DB 反向覆盖本地最新 buffer，也能在本地缺消息时补回。
func (a *AIHelper) ReconcileMessages(dbMessages []model.Message) {
	a.mu.Lock()
	defer a.mu.Unlock()

	dbMessageKeySet := make(map[string]struct{}, len(dbMessages))
	mergedMessages := make([]*model.Message, 0, len(dbMessages)+len(a.messages))

	for i := range dbMessages {
		msg := dbMessages[i]
		msg.SessionID = a.SessionID
		dbMessageKeySet[msg.MessageKey] = struct{}{}
		mergedMessages = append(mergedMessages, &msg)
	}

	// 本地独有消息通常是“已进入 buffer、但 MQ 异步落库还没追上”的尾部消息。
	for _, msg := range a.messages {
		if _, exists := dbMessageKeySet[msg.MessageKey]; exists {
			continue
		}
		localMsg := *msg
		localMsg.SessionID = a.SessionID
		mergedMessages = append(mergedMessages, &localMsg)
	}

	a.messages = mergedMessages
	a.version++
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
	releasePermit, err := globalModelConcurrencyManager.acquire(ctx, a.model.GetModelType())
	if err != nil {
		return fmt.Errorf("summary concurrency limited: %w", err)
	}
	defer releasePermit()

	newSummary, err := a.model.GenerateSummary(ctx, currentSummary, utils.ConvertToSchemaMessages(messagesToSummarize))
	if err != nil {
		// 摘要刷新失败时，不应把整个主链路直接打挂。
		// 此时系统仍然可以继续使用“最近窗口消息”完成回答，只是上下文压缩收益暂时下降。
		observability.RecordSummaryRefresh(false, time.Since(summaryStart))
		observability.RecordModelCall("summary", a.model.GetModelType(), false, time.Since(summaryStart), len(messagesToSummarize), currentSummary != "")
		log.Println("AIHelper ensureContextSummary generate summary error:", err)
		return nil
	}
	observability.RecordSummaryRefresh(true, time.Since(summaryStart))
	observability.RecordModelCall("summary", a.model.GetModelType(), true, time.Since(summaryStart), len(messagesToSummarize), currentSummary != "")

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.summaryMessageCount != currentSummaryCount {
		return nil
	}

	a.contextSummary = newSummary
	a.summaryMessageCount = targetSummaryCount
	a.version++
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

// generateWithRetryAndFallback 统一封装“同模型有限重试 + 备用模型降级”策略。
// 这里不把所有错误都吞掉，而是优先做一次低成本重试；只有确认主模型连续失败时，才切备用模型。
func (a *AIHelper) generateWithRetryAndFallback(
	ctx context.Context,
	operation string,
	messages []*schema.Message,
	usedSummary bool,
	invoke func(model AIModel) (*schema.Message, error),
) (*schema.Message, error) {
	callStart := time.Now()
	resp, err := invoke(a.model)
	observability.RecordModelCall(operation, a.model.GetModelType(), err == nil, time.Since(callStart), len(messages), usedSummary)
	if err == nil {
		return resp, nil
	}

	if ctx.Err() != nil {
		return nil, err
	}

	observability.RecordModelRetry()
	retryStart := time.Now()
	resp, retryErr := invoke(a.model)
	observability.RecordModelCall(operation+"_retry", a.model.GetModelType(), retryErr == nil, time.Since(retryStart), len(messages), usedSummary)
	if retryErr == nil {
		return resp, nil
	}

	if a.fallbackModel == nil || ctx.Err() != nil {
		return nil, retryErr
	}

	observability.RecordModelFallback()
	fallbackStart := time.Now()
	resp, fallbackErr := invoke(a.fallbackModel)
	observability.RecordModelCall(operation+"_fallback", a.fallbackModel.GetModelType(), fallbackErr == nil, time.Since(fallbackStart), len(messages), usedSummary)
	if fallbackErr == nil {
		return resp, nil
	}

	return nil, fallbackErr
}

// streamWithRetryAndFallback 统一封装流式场景下的重试与降级策略。
func (a *AIHelper) streamWithRetryAndFallback(
	ctx context.Context,
	operation string,
	messages []*schema.Message,
	usedSummary bool,
	allowRetry func() bool,
	invoke func(model AIModel) (string, error),
) (string, error) {
	callStart := time.Now()
	content, err := invoke(a.model)
	observability.RecordModelCall(operation, a.model.GetModelType(), err == nil, time.Since(callStart), len(messages), usedSummary)
	if err == nil {
		return content, nil
	}

	if ctx.Err() != nil {
		return "", err
	}

	// 流式场景下，一旦已经向客户端输出了部分 token，就不能再重试或切模型，
	// 否则前端会看到重复内容或语义断裂。
	if allowRetry != nil && !allowRetry() {
		return "", err
	}

	observability.RecordModelRetry()
	retryStart := time.Now()
	content, retryErr := invoke(a.model)
	observability.RecordModelCall(operation+"_retry", a.model.GetModelType(), retryErr == nil, time.Since(retryStart), len(messages), usedSummary)
	if retryErr == nil {
		return content, nil
	}

	if a.fallbackModel == nil || ctx.Err() != nil {
		return "", retryErr
	}

	observability.RecordModelFallback()
	fallbackStart := time.Now()
	content, fallbackErr := invoke(a.fallbackModel)
	observability.RecordModelCall(operation+"_fallback", a.fallbackModel.GetModelType(), fallbackErr == nil, time.Since(fallbackStart), len(messages), usedSummary)
	if fallbackErr == nil {
		return content, nil
	}

	return "", fallbackErr
}

// GenerateResponse 走同步模型调用。
func (a *AIHelper) GenerateResponse(userName string, ctx context.Context, userQuestion string) (*model.Message, error) {
	// 先把用户本轮问题写入上下文，保证模型能看到完整多轮历史。
	a.AddMessage(userQuestion, userName, true, true)

	if err := a.ensureContextSummary(ctx); err != nil {
		return nil, err
	}

	messages := a.buildModelMessages()
	usedSummary := false
	if summary, _ := a.GetSummaryState(); summary != "" {
		usedSummary = true
	}

	// 真正的模型调用前先申请并发令牌。
	// 这样即使外层接口层没有足够细的限流，实例内部也不会无限制地把请求全部压给模型。
	releasePermit, err := globalModelConcurrencyManager.acquire(ctx, a.model.GetModelType())
	if err != nil {
		return nil, fmt.Errorf("model concurrency limited: %w", err)
	}
	defer releasePermit()

	schemaMsg, err := a.generateWithRetryAndFallback(ctx, "generate", messages, usedSummary, func(model AIModel) (*schema.Message, error) {
		return model.GenerateResponse(ctx, messages)
	})
	if err != nil {
		return nil, err
	}

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
	usedSummary := false
	if summary, _ := a.GetSummaryState(); summary != "" {
		usedSummary = true
	}

	releasePermit, err := globalModelConcurrencyManager.acquire(ctx, a.model.GetModelType())
	if err != nil {
		return nil, fmt.Errorf("model concurrency limited: %w", err)
	}
	defer releasePermit()

	emitted := false
	content, err := a.streamWithRetryAndFallback(ctx, "stream", messages, usedSummary, func() bool {
		return !emitted
	}, func(model AIModel) (string, error) {
		return model.StreamResponse(ctx, messages, func(msg string) {
			emitted = true
			cb(msg)
		})
	})
	if err != nil {
		return nil, err
	}

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
