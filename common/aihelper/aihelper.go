package aihelper

import (
	"GopherAI/common/observability"
	"GopherAI/common/rabbitmq"
	outboxDAO "GopherAI/dao/outbox"
	"GopherAI/model"
	"GopherAI/utils"
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
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
	// messageWindowStart 表示当前 messages 切片在“完整消息序列”中的起始下标。
	// Full 模式下它恒为 0；Warm 模式下它用于表达“当前内存只保留了最近窗口”。
	messageWindowStart int
	// version 用于给“共享热状态快照”打版本号。
	// 这样后续把状态同步到 Redis 时，可以知道这是哪一次会话推进之后产生的新快照。
	version int64
	// selectionSignature 用于标识当前 helper 绑定的模型/配置选择。
	// 当会话切换 llmConfigId 或 chatMode 时，service 可以据此判断是否必须重建 helper。
	selectionSignature string
	// recoveryMode 显式区分当前 helper 是“热恢复服务态”还是“全量对账态”。
	// 这样 service 可以在不读全量 DB 的情况下安全恢复最近窗口。
	recoveryMode RecoveryMode
}

const maxContextMessages = 20

// RecoveryMode 表示 helper 当前采用的恢复语义。
// Warm 只要求“足够继续服务”，Full 则表示当前 messages 已视作完整对账基底。
type RecoveryMode string

const (
	RecoveryModeWarm RecoveryMode = "warm"
	RecoveryModeFull RecoveryMode = "full"
)

// NewAIHelper 创建新的 AIHelper。
func NewAIHelper(model_ AIModel, SessionID string, selectionSignature string) *AIHelper {
	return &AIHelper{
		model:    model_,
		messages: make([]*model.Message, 0),
		// 第二阶段开始，消息先写入 outbox，再尝试即时发布到 MQ。
		// 这样既保留“主链路尽量异步”的体验，也把“发布失败后还能补偿重放”的基线落到了数据库里。
		saveFunc: func(msg *model.Message) (*model.Message, error) {
			data := rabbitmq.GenerateMessageMQParam(
				msg.MessageKey,
				msg.SessionID,
				msg.SessionVersion,
				msg.Content,
				msg.UserName,
				msg.IsUser,
				string(msg.Status),
			)
			now := time.Now()
			outboxEvent := &model.MessageOutbox{
				MessageKey:     msg.MessageKey,
				SessionID:      msg.SessionID,
				SessionVersion: msg.SessionVersion,
				Status:         model.MessageOutboxStatusPending,
				Payload:        string(data),
				NextAttemptAt:  now,
			}
			if err := outboxDAO.SaveMessageOutbox(outboxEvent); err != nil {
				return msg, err
			}

			// 先做一次即时发布，尽量维持原来的低延迟异步体验。
			// 即时发布失败时不把消息丢掉，而是把失败信息留在 outbox，交给 relay worker 后续重试。
			if rabbitmq.RMQMessage != nil {
				if err := rabbitmq.RMQMessage.Publish(data); err == nil {
					if markErr := outboxDAO.MarkMessageOutboxPublished(msg.MessageKey); markErr != nil {
						log.Println("AIHelper save message mark outbox published error:", markErr)
					}
					return msg, nil
				} else if markErr := outboxDAO.MarkMessageOutboxPublishFailed(msg.MessageKey, err.Error()); markErr != nil {
					log.Println("AIHelper save message mark outbox publish failed error:", markErr)
				}
			}

			// 这里不再直接同步落 message 表，而是明确交给 outbox 补偿链路处理。
			// 这样消息最终是通过同一条“发布 -> 消费 -> 落库 -> 回执”路径收敛，避免出现多套真相源。
			observability.RecordMQFallback()
			return msg, nil
		},
		SessionID:          SessionID,
		version:            1,
		selectionSignature: selectionSignature,
		recoveryMode:       RecoveryModeFull,
	}
}

// AddMessage 把一条消息加入当前会话的内存上下文。
// Save=true 时继续走异步持久化；Save=false 时只做内存回放，不重复入库。
func (a *AIHelper) AddMessage(Content string, UserName string, IsUser bool, Save bool) {
	a.AddMessageWithStatus(Content, UserName, IsUser, Save, model.MessageStatusCompleted)
}

func (a *AIHelper) appendMessage(message *model.Message, save bool) *model.Message {
	if message == nil {
		return nil
	}

	// 写切片时必须加锁；否则同一个 session 并发写入会有数据竞争风险。
	a.mu.Lock()
	a.messages = append(a.messages, message)
	a.mu.Unlock()

	if save && a.saveFunc != nil {
		if _, err := a.saveFunc(message); err != nil {
			log.Println("AIHelper appendMessage persist message error:", err)
		}
	}
	return message
}

// AddMessageWithStatus 把一条消息写入当前 helper，并带上明确状态。
// 之所以新增这个方法，而不是继续只保留 AddMessage，是因为 stop / timeout / partial
// 这类中断场景需要把“消息内容”和“消息最终状态”一起落到持久化层。
func (a *AIHelper) AddMessageWithStatus(Content string, UserName string, IsUser bool, Save bool, status model.MessageStatus) *model.Message {
	userMsg := model.Message{
		MessageKey: utils.GenerateUUID(),
		SessionID:  a.SessionID,
		// 当前一轮对话里的消息都归属到“下一次正式会话推进版本”。
		// 这样消息异步落库后，就可以按 session_version 推进 persisted_version。
		SessionVersion: a.GetVersion() + 1,
		Content:        Content,
		UserName:       UserName,
		IsUser:         IsUser,
		Status:         status,
	}

	return a.appendMessage(&userMsg, Save)
}

// AppendExistingMessage 把一个已经存在固定 message_key 的消息补回 helper。
// 它主要给流式占位消息使用：同一条 assistant 消息会先落占位，再在终态时补内容。
func (a *AIHelper) AppendExistingMessage(message *model.Message) *model.Message {
	return a.appendMessage(message, false)
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
			Status:     model.MessageStatus(msg.Status),
			CreatedAt:  msg.CreatedAt,
		}
		a.messages = append(a.messages, modelMsg)
	}

	a.contextSummary = state.ContextSummary
	a.summaryMessageCount = state.SummaryMessageCount
	a.messageWindowStart = state.RecentMessagesStart
	if state.Version > 0 {
		a.version = state.Version
	}
	a.recoveryMode = RecoveryModeWarm
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
			Status:     string(msg.Status),
			CreatedAt:  msg.CreatedAt,
		})
	}

	return &model.SessionHotState{
		SessionID:           a.SessionID,
		SelectionSignature:  a.selectionSignature,
		Version:             a.version,
		UpdatedAt:           time.Now(),
		ContextSummary:      a.contextSummary,
		SummaryMessageCount: a.summaryMessageCount,
		RecentMessagesStart: a.messageWindowStart + start,
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
	a.messageWindowStart = 0
	a.recoveryMode = RecoveryModeFull
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
	a.summaryMessageCount = summaryMessageCount
}

// GetSummaryState 返回当前 helper 的摘要快照，供 service 判断是否需要回写数据库。
func (a *AIHelper) GetSummaryState() (string, int) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.contextSummary, a.summaryMessageCount
}

// GetRecoveryMode 返回 helper 当前的恢复模式，供 service 做观测和分支控制。
func (a *AIHelper) GetRecoveryMode() RecoveryMode {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.recoveryMode
}

// GetVersion 返回当前 helper 绑定的会话正式版本号。
func (a *AIHelper) GetVersion() int64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.version
}

// SetVersion 用显式版本覆盖 helper 当前版本，避免恢复或持久化时继续沿用旧值。
func (a *AIHelper) SetVersion(version int64) {
	if version <= 0 {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.version = version
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
	a.messageWindowStart = 0
	a.recoveryMode = RecoveryModeFull
}

// ensureContextSummary 保证“超过窗口之外的更早消息”都被压缩进摘要。
// 这样 buildModelMessages 就可以稳定地发送“摘要 + 最近 N 条”。
func (a *AIHelper) ensureContextSummary(ctx context.Context) error {
	a.mu.RLock()
	// targetSummaryCount 使用“完整消息序列的绝对下标”表达，
	// 这样 warm 模式下即使当前只保留 recent window，也能继续推进摘要覆盖范围。
	targetSummaryCount := a.messageWindowStart + len(a.messages) - maxContextMessages
	if targetSummaryCount < 0 {
		targetSummaryCount = 0
	}
	currentSummary := a.contextSummary
	currentSummaryCount := a.summaryMessageCount
	if targetSummaryCount <= currentSummaryCount {
		a.mu.RUnlock()
		return nil
	}

	localSummaryStart := currentSummaryCount - a.messageWindowStart
	if localSummaryStart < 0 {
		// 当前窗口前存在“未被摘要覆盖、但又未保留在内存”的历史空洞时，
		// 说明 helper 只能继续服务，不能再安全增量生成新摘要。
		a.mu.RUnlock()
		return nil
	}
	localSummaryEnd := targetSummaryCount - a.messageWindowStart
	if localSummaryEnd > len(a.messages) {
		localSummaryEnd = len(a.messages)
	}
	if localSummaryStart >= localSummaryEnd {
		a.mu.RUnlock()
		return nil
	}

	messagesToSummarize := make([]*model.Message, localSummaryEnd-localSummaryStart)
	copy(messagesToSummarize, a.messages[localSummaryStart:localSummaryEnd])
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
	return nil
}

// buildModelMessages 把当前会话消息裁剪成适合发送给模型的上下文窗口。
// 我们保留完整历史用于回放和查询，但真正发给模型时只发送“摘要 + 最近 N 条”，控制 token 成本和延迟。
func (a *AIHelper) buildModelMessages() []*schema.Message {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// start 仍然使用当前切片内的局部下标，但 summaryMessageCount 本身表达的是绝对覆盖位置。
	// 因此 Warm 模式下需要先减去 messageWindowStart，才能算出当前切片里该从哪里开始拼接。
	start := a.summaryMessageCount - a.messageWindowStart
	if start < 0 {
		start = 0
	}
	if start > len(a.messages) {
		start = len(a.messages)
	}
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
	log.Printf("AIHelper %s primary call failed: session=%s model=%s err=%v", operation, a.SessionID, a.model.GetModelType(), err)

	if ctx.Err() != nil {
		return nil, err
	}

	observability.RecordModelRetry()
	log.Printf("AIHelper %s retrying primary model: session=%s model=%s", operation, a.SessionID, a.model.GetModelType())
	retryStart := time.Now()
	resp, retryErr := invoke(a.model)
	observability.RecordModelCall(operation+"_retry", a.model.GetModelType(), retryErr == nil, time.Since(retryStart), len(messages), usedSummary)
	if retryErr == nil {
		return resp, nil
	}
	log.Printf("AIHelper %s retry failed: session=%s model=%s err=%v", operation, a.SessionID, a.model.GetModelType(), retryErr)

	if a.fallbackModel == nil || ctx.Err() != nil {
		return nil, retryErr
	}

	observability.RecordModelFallback()
	log.Printf("AIHelper %s falling back: session=%s from=%s to=%s", operation, a.SessionID, a.model.GetModelType(), a.fallbackModel.GetModelType())
	fallbackStart := time.Now()
	resp, fallbackErr := invoke(a.fallbackModel)
	observability.RecordModelCall(operation+"_fallback", a.fallbackModel.GetModelType(), fallbackErr == nil, time.Since(fallbackStart), len(messages), usedSummary)
	if fallbackErr == nil {
		return resp, nil
	}
	log.Printf("AIHelper %s fallback failed: session=%s model=%s err=%v", operation, a.SessionID, a.fallbackModel.GetModelType(), fallbackErr)

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
	log.Printf("AIHelper %s primary stream failed: session=%s model=%s err=%v", operation, a.SessionID, a.model.GetModelType(), err)

	if ctx.Err() != nil {
		return "", err
	}

	// 流式场景下，一旦已经向客户端输出了部分 token，就不能再重试或切模型，
	// 否则前端会看到重复内容或语义断裂。
	if allowRetry != nil && !allowRetry() {
		return "", err
	}

	observability.RecordModelRetry()
	log.Printf("AIHelper %s retrying primary stream model: session=%s model=%s", operation, a.SessionID, a.model.GetModelType())
	retryStart := time.Now()
	content, retryErr := invoke(a.model)
	observability.RecordModelCall(operation+"_retry", a.model.GetModelType(), retryErr == nil, time.Since(retryStart), len(messages), usedSummary)
	if retryErr == nil {
		return content, nil
	}
	log.Printf("AIHelper %s retry stream failed: session=%s model=%s err=%v", operation, a.SessionID, a.model.GetModelType(), retryErr)

	if a.fallbackModel == nil || ctx.Err() != nil {
		return "", retryErr
	}

	observability.RecordModelFallback()
	log.Printf("AIHelper %s falling back stream: session=%s from=%s to=%s", operation, a.SessionID, a.model.GetModelType(), a.fallbackModel.GetModelType())
	fallbackStart := time.Now()
	content, fallbackErr := invoke(a.fallbackModel)
	observability.RecordModelCall(operation+"_fallback", a.fallbackModel.GetModelType(), fallbackErr == nil, time.Since(fallbackStart), len(messages), usedSummary)
	if fallbackErr == nil {
		return content, nil
	}
	log.Printf("AIHelper %s fallback stream failed: session=%s model=%s err=%v", operation, a.SessionID, a.fallbackModel.GetModelType(), fallbackErr)

	return "", fallbackErr
}

// GenerateResponse 走同步模型调用。
func (a *AIHelper) GenerateResponse(userName string, ctx context.Context, userQuestion string) (*model.Message, error) {
	// 先把用户本轮问题写入上下文，保证模型能看到完整多轮历史。
	a.AddMessage(userQuestion, userName, true, true)

	if err := a.ensureContextSummary(ctx); err != nil {
		log.Printf("AIHelper GenerateResponse ensureContextSummary failed: session=%s model=%s err=%v", a.SessionID, a.model.GetModelType(), err)
		return nil, err
	}

	messages := a.buildModelMessages()
	usedSummary := false
	if summary, _ := a.GetSummaryState(); summary != "" {
		usedSummary = true
	}

	// 真正的模型调用前先申请并发令牌。
	// 这样即使外层接口层没有足够细的限流，实例内部也不会无限制地把请求全部压给模型。
	log.Printf("AIHelper GenerateResponse acquiring permit: session=%s model=%s messages=%d", a.SessionID, a.model.GetModelType(), len(messages))
	releasePermit, err := globalModelConcurrencyManager.acquire(ctx, a.model.GetModelType())
	if err != nil {
		log.Printf("AIHelper GenerateResponse acquire permit failed: session=%s model=%s err=%v", a.SessionID, a.model.GetModelType(), err)
		return nil, fmt.Errorf("model concurrency limited: %w", err)
	}
	defer releasePermit()

	log.Printf("AIHelper GenerateResponse invoking model: session=%s model=%s", a.SessionID, a.model.GetModelType())
	schemaMsg, err := a.generateWithRetryAndFallback(ctx, "generate", messages, usedSummary, func(model AIModel) (*schema.Message, error) {
		return model.GenerateResponse(ctx, messages)
	})
	if err != nil {
		log.Printf("AIHelper GenerateResponse model failed: session=%s model=%s err=%v", a.SessionID, a.model.GetModelType(), err)
		return nil, err
	}
	log.Printf("AIHelper GenerateResponse model success: session=%s model=%s response_chars=%d", a.SessionID, a.model.GetModelType(), len(schemaMsg.Content))

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
		log.Printf("AIHelper StreamResponse ensureContextSummary failed: session=%s model=%s err=%v", a.SessionID, a.model.GetModelType(), err)
		return nil, err
	}

	messages := a.buildModelMessages()
	usedSummary := false
	if summary, _ := a.GetSummaryState(); summary != "" {
		usedSummary = true
	}

	log.Printf("AIHelper StreamResponse acquiring permit: session=%s model=%s messages=%d", a.SessionID, a.model.GetModelType(), len(messages))
	releasePermit, err := globalModelConcurrencyManager.acquire(ctx, a.model.GetModelType())
	if err != nil {
		log.Printf("AIHelper StreamResponse acquire permit failed: session=%s model=%s err=%v", a.SessionID, a.model.GetModelType(), err)
		return nil, fmt.Errorf("model concurrency limited: %w", err)
	}
	defer releasePermit()

	emitted := false
	log.Printf("AIHelper StreamResponse invoking model: session=%s model=%s", a.SessionID, a.model.GetModelType())
	content, err := a.streamWithRetryAndFallback(ctx, "stream", messages, usedSummary, func() bool {
		return !emitted
	}, func(model AIModel) (string, error) {
		return model.StreamResponse(ctx, messages, func(msg string) {
			emitted = true
			cb(msg)
		})
	})
	if err != nil {
		log.Printf("AIHelper StreamResponse model failed: session=%s model=%s err=%v", a.SessionID, a.model.GetModelType(), err)
		return nil, err
	}
	log.Printf("AIHelper StreamResponse model success: session=%s model=%s response_chars=%d", a.SessionID, a.model.GetModelType(), len(content))

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

// StreamResponseWithExistingAssistant 和 StreamResponse 类似，但最终 assistant 消息使用外部已确定的 message_key。
// 这主要用于流式占位消息：先创建 streaming 占位，再在流结束时把完整内容补回同一条消息。
func (a *AIHelper) StreamResponseWithExistingAssistant(userName string, ctx context.Context, cb StreamCallback, userQuestion string, assistantMessage *model.Message) (*model.Message, error) {
	// 流式场景也要先把用户问题放入上下文，否则模型拿不到当前轮问题。
	a.AddMessage(userQuestion, userName, true, true)

	if err := a.ensureContextSummary(ctx); err != nil {
		log.Printf("AIHelper StreamResponseWithExistingAssistant ensureContextSummary failed: session=%s model=%s err=%v", a.SessionID, a.model.GetModelType(), err)
		return nil, err
	}

	messages := a.buildModelMessages()
	usedSummary := false
	if summary, _ := a.GetSummaryState(); summary != "" {
		usedSummary = true
	}

	log.Printf("AIHelper StreamResponseWithExistingAssistant acquiring permit: session=%s model=%s messages=%d", a.SessionID, a.model.GetModelType(), len(messages))
	releasePermit, err := globalModelConcurrencyManager.acquire(ctx, a.model.GetModelType())
	if err != nil {
		log.Printf("AIHelper StreamResponseWithExistingAssistant acquire permit failed: session=%s model=%s err=%v", a.SessionID, a.model.GetModelType(), err)
		return nil, fmt.Errorf("model concurrency limited: %w", err)
	}
	defer releasePermit()

	emitted := false
	log.Printf("AIHelper StreamResponseWithExistingAssistant invoking model: session=%s model=%s", a.SessionID, a.model.GetModelType())
	content, err := a.streamWithRetryAndFallback(ctx, "stream", messages, usedSummary, func() bool {
		return !emitted
	}, func(model AIModel) (string, error) {
		return model.StreamResponse(ctx, messages, func(msg string) {
			emitted = true
			cb(msg)
		})
	})
	if err != nil {
		log.Printf("AIHelper StreamResponseWithExistingAssistant model failed: session=%s model=%s err=%v", a.SessionID, a.model.GetModelType(), err)
		return nil, err
	}
	log.Printf("AIHelper StreamResponseWithExistingAssistant model success: session=%s model=%s response_chars=%d", a.SessionID, a.model.GetModelType(), len(content))

	finalAssistant := &model.Message{
		MessageKey:     assistantMessage.MessageKey,
		SessionID:      a.SessionID,
		SessionVersion: assistantMessage.SessionVersion,
		UserName:       userName,
		Content:        content,
		IsUser:         false,
		Status:         model.MessageStatusCompleted,
	}

	a.AppendExistingMessage(finalAssistant)
	return finalAssistant, nil
}

// GetModelType 返回当前 helper 绑定的模型类型。
func (a *AIHelper) GetModelType() string {
	return a.model.GetModelType()
}

func (a *AIHelper) MatchesSelection(selectionSignature string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.selectionSignature == selectionSignature
}
