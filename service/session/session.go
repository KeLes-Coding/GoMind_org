package session

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	"GopherAI/common/observability"
	myredis "GopherAI/common/redis"
	messageDAO "GopherAI/dao/message"
	sessionDAO "GopherAI/dao/session"
	"GopherAI/model"
	notifyservice "GopherAI/service/notify"
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// maxWarmResumeVersionLag 控制主链路允许直接采用热恢复的最大 version 滞后窗口。
	// 如果 session.version 明显领先于 persisted_version，说明当前更多依赖“未完全落库”的运行态，
	// 此时宁可退回全量对账，也不在主链路继续信任可能不完整的热快照。
	maxWarmResumeVersionLag int64 = 8
)

var publishChatMessageReadyFunc = notifyservice.PublishChatMessageReady

// ensureOwnedSession 校验会话是否存在且归当前用户所有。
// 后续所有读写操作都应先经过该检查，避免越权访问其他用户会话。
func ensureOwnedSession(userName string, sessionID string) (*model.Session, code.Code) {
	sess, err := sessionDAO.GetSessionByID(sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, code.CodeRecordNotFound
		}
		log.Println("ensureOwnedSession GetSessionByID error:", err)
		return nil, code.CodeServerBusy
	}

	if sess.UserName != userName {
		return nil, code.CodeForbidden
	}

	return sess, code.CodeSuccess
}

// ensureSessionWriteOwnership 在真正写 DB / Redis 前再次校验 owner fencing 资格。
// 这样即使旧 owner 的执行流还没完全退出，只要 owner lease 已被新实例接管，它也不能继续尾写。
func ensureSessionWriteOwnership(ctx context.Context, sessionID string) code.Code {
	guard := sessionOwnerGuardFromContext(ctx)
	if guard == nil || guard.SessionID != sessionID {
		return code.CodeSuccess
	}
	valid, err := myredis.ValidateSessionOwnerFence(ctx, sessionID, guard.OwnerID, guard.FenceToken)
	if err != nil {
		logSessionTrace(ctx, "owner_fence_validate_fail", "err=%v", err)
		log.Println("ensureSessionWriteOwnership ValidateSessionOwnerFence error:", err)
		return code.CodeServerBusy
	}
	if !valid {
		logSessionTrace(ctx, "owner_fence_reject", "owner=%s fence=%d", guard.OwnerID, guard.FenceToken)
		return code.AIModelCancelled
	}
	return code.CodeSuccess
}

// persistSessionProgress 将 helper 当前的摘要状态和版本号持久化到 session。
// 这个方法保留给“消息正式落库与 session 进度推进暂时分离”的路径使用。
func persistSessionProgress(ctx context.Context, sessionID string, helper *aihelper.AIHelper) code.Code {
	if code_ := ensureSessionWriteOwnership(ctx, sessionID); code_ != code.CodeSuccess {
		return code_
	}
	afterSummary, afterCount := helper.GetSummaryState()
	nextVersion := helper.GetVersion() + 1
	if err := sessionDAO.UpdateSessionProgress(sessionID, nextVersion, afterSummary, afterCount); err != nil {
		observability.RecordDBPersistFail()
		log.Println("persistSessionProgress UpdateSessionProgress error:", err)
		return code.CodeServerBusy
	}

	helper.SetVersion(nextVersion)
	return code.CodeSuccess
}

// persistSessionProgressWithPersistedVersion 在核心消息已经同步写入 MySQL 后，
// 同步推进 session.version 与 session.persisted_version。
func persistSessionProgressWithPersistedVersion(ctx context.Context, sessionID string, helper *aihelper.AIHelper) code.Code {
	if code_ := ensureSessionWriteOwnership(ctx, sessionID); code_ != code.CodeSuccess {
		return code_
	}
	afterSummary, afterCount := helper.GetSummaryState()
	nextVersion := helper.GetVersion() + 1
	if err := sessionDAO.UpdateSessionProgressAndPersistedVersion(sessionID, nextVersion, afterSummary, afterCount, nextVersion); err != nil {
		observability.RecordDBPersistFail()
		log.Println("persistSessionProgressWithPersistedVersion UpdateSessionProgressAndPersistedVersion error:", err)
		return code.CodeServerBusy
	}

	helper.SetVersion(nextVersion)
	helper.SetPersistedVersion(nextVersion)
	return code.CodeSuccess
}

// commitHelperHotState 将 helper 的热状态同步提交到 Redis。
// 第三阶段开始，这个方法用于关键提交点：
// 1. 用户消息进入上下文后
// 2. assistant 终态完成后
// 3. 其他需要让外部请求立刻可见的正式热状态推进点
func commitHelperHotState(ctx context.Context, helper *aihelper.AIHelper) code.Code {
	if helper == nil {
		return code.CodeInvalidParams
	}
	guard := sessionOwnerGuardFromContext(ctx)
	if guard != nil && guard.SessionID == helper.SessionID {
		valid, err := myredis.ValidateSessionOwnerFence(ctx, helper.SessionID, guard.OwnerID, guard.FenceToken)
		if err != nil {
			observability.RecordRedisHotStateSaveFail()
			logSessionTrace(ctx, "hot_state_owner_validate_fail", "err=%v", err)
			log.Println("persistHelperHotState ValidateSessionOwnerFence error:", err)
			return code.CodeServerBusy
		}
		if !valid {
			logSessionTrace(ctx, "hot_state_owner_reject", "owner=%s fence=%d", guard.OwnerID, guard.FenceToken)
			return code.AIModelCancelled
		}
	}

	hotState := helper.ExportHotState()
	if guard != nil && guard.SessionID == helper.SessionID {
		hotState.OwnerID = guard.OwnerID
		hotState.FenceToken = guard.FenceToken
	}
	result, err := myredis.SaveSessionHotState(ctx, hotState)
	if err != nil {
		observability.RecordRedisHotStateSaveFail()
		logSessionTrace(ctx, "hot_state_save_fail", "err=%v", err)
		log.Println("persistHelperHotState SaveSessionHotState error:", err)
		return code.CodeServerBusy
	}
	if result == myredis.SessionHotStateSaveUnavailable {
		observability.RecordRedisHotStateSaveFail()
		logSessionTrace(ctx, "hot_state_save_fail", "detail=redis_unavailable")
		return code.CodeServerBusy
	}
	if result == myredis.SessionHotStateSaveIgnoredStale {
		logSessionTrace(ctx, "hot_state_save_ignored", "detail=stale_snapshot")
		return code.AIModelCancelled
	}
	return code.CodeSuccess
}

// appendUserMessageAndCommitHotState 负责把用户消息先写入 helper，再同步提交 Redis 热状态。
// 这样模型真正开始执行前，其他请求就能从 Redis 看见这次会话推进。
func appendUserMessageAndCommitHotState(ctx context.Context, helper *aihelper.AIHelper, userName string, userQuestion string) (*model.Message, code.Code) {
	if helper == nil {
		return nil, code.CodeInvalidParams
	}
	message := helper.AddMessageWithStatus(userQuestion, userName, true, false, model.MessageStatusCompleted)
	if code_ := commitHelperHotState(ctx, helper); code_ != code.CodeSuccess {
		return nil, code_
	}
	return message, code.CodeSuccess
}

// finalizeAssistantMessageAndCommitHotState 负责把 assistant 终态写回 helper，并同步提交 Redis 热状态。
// 第三阶段开始，assistant 完整回复、partial、cancelled、timeout 都应在这里形成正式可恢复热状态。
func finalizeAssistantMessageAndCommitHotState(ctx context.Context, helper *aihelper.AIHelper, message *model.Message) code.Code {
	if helper == nil || message == nil {
		return code.CodeInvalidParams
	}
	helper.AppendExistingMessage(message)
	return commitHelperHotState(ctx, helper)
}

// persistMessageSync 把聊天核心消息同步写入 MySQL。
// 第四阶段开始，user/assistant 主消息都应通过这个入口完成正式落库，
// 而不是继续依赖 AIHelper.saveFunc 触发的 outbox/MQ 主链路。
func persistMessageSync(ctx context.Context, message *model.Message) code.Code {
	if message == nil {
		return code.CodeInvalidParams
	}
	if code_ := ensureSessionWriteOwnership(ctx, message.SessionID); code_ != code.CodeSuccess {
		return code_
	}
	if _, err := messageDAO.CreateMessage(message); err != nil {
		observability.RecordDBPersistFail()
		log.Println("persistMessageSync CreateMessage error:", err)
		return code.CodeServerBusy
	}
	return code.CodeSuccess
}

// publishAssistantReadyNotificationBestEffort 在 assistant 完整回复正式落库后投递旁路通知。
// 这里明确采用 best-effort 策略：
// 1. 通知失败不回滚主链路；
// 2. 只有 completed assistant 才发送，避免 partial/cancelled/timeout 产生噪音；
// 3. 这样 MQ 就只承担旁路价值，而不再影响聊天核心一致性。
func publishAssistantReadyNotificationBestEffort(ctx context.Context, sess *model.Session, message *model.Message) {
	if sess == nil || message == nil {
		return
	}
	if message.IsUser || message.Status != model.MessageStatusCompleted {
		return
	}

	err := publishChatMessageReadyFunc(ctx, notifyservice.ChatMessageReadyParams{
		UserID:     sess.UserID,
		SessionID:  sess.ID,
		MessageKey: message.MessageKey,
		Content:    message.Content,
	})
	if err != nil {
		log.Println("publishAssistantReadyNotificationBestEffort error:", err)
	}
}

// applySessionMetadataToHelper 用 session 表中的正式元信息刷新 helper。
// 这里不碰消息内容，只覆盖 version / summary 这类“会话级正式状态”。
func applySessionMetadataToHelper(sess *model.Session, helper *aihelper.AIHelper) {
	if sess == nil || helper == nil {
		return
	}
	helper.SetVersion(sess.Version)
	helper.SetPersistedVersion(sess.PersistedVersion)
	helper.SetSummaryState(sess.ContextSummary, sess.SummaryMessageCount)
}

// lightReconcileHelperWithSession 只做轻量元信息对齐，不读取全量消息。
// 它用于进程内 helper 命中或 warm resume 命中后的主链路快速恢复。
func lightReconcileHelperWithSession(sess *model.Session, helper *aihelper.AIHelper) code.Code {
	if sess == nil || helper == nil {
		return code.CodeInvalidParams
	}
	applySessionMetadataToHelper(sess, helper)
	return code.CodeSuccess
}

// fullReconcileHelperWithDatabase 使用数据库完整消息序列重建 helper。
// 这是异常兜底路径，只有热态不可信或缺失时才进入。
func fullReconcileHelperWithDatabase(sess *model.Session, helper *aihelper.AIHelper) code.Code {
	if sess == nil || helper == nil {
		return code.CodeInvalidParams
	}

	start := time.Now()
	msgs, err := messageDAO.GetMessagesBySessionID(sess.ID)
	if err != nil {
		log.Println("fullReconcileHelperWithDatabase GetMessagesBySessionID error:", err)
		return code.CodeServerBusy
	}

	if len(msgs) > 0 {
		helper.LoadMessages(msgs)
	} else {
		helper.LoadMessages(nil)
	}
	applySessionMetadataToHelper(sess, helper)
	observability.RecordHelperFullReconcile(len(msgs), time.Since(start))
	return code.CodeSuccess
}

// canWarmResumeFromHotState 判断当前 Redis 热状态是否足以直接支撑“继续服务”。
// 这里刻意要求条件偏保守，宁可回退全量对账，也不让错误热态进入主路径。
func canWarmResumeFromHotState(sess *model.Session, hotState *model.SessionHotState, currentLease *myredis.SessionOwnerLease, selectionSignature string) bool {
	if sess == nil || hotState == nil {
		return false
	}
	if hotState.SelectionSignature == "" || hotState.SelectionSignature != selectionSignature {
		return false
	}
	if hotState.Version < sess.Version {
		return false
	}
	if currentLease != nil && hotState.FenceToken < currentLease.FenceToken {
		return false
	}
	if len(hotState.RecentMessages) == 0 && strings.TrimSpace(hotState.ContextSummary) == "" {
		return false
	}
	// recent window 的起点不能落在 summary 覆盖范围之后，否则中间会出现“既不在摘要里，也不在 recent 里”的空洞。
	if hotState.RecentMessagesStart > hotState.SummaryMessageCount {
		return false
	}
	if sess.Version-sess.PersistedVersion > maxWarmResumeVersionLag {
		return false
	}
	return true
}

func resolveBusyResultCode(result codeExecutorResult) code.Code {
	if result.code != 0 {
		return result.code
	}
	return code.CodeTooManyRequests
}

// syncHelperWithDatabase 将内存中的 helper 与数据库消息状态对齐。
// 对齐策略：
// 1. 如果 DB 最新消息已经存在于 helper 中，仅在消息数量落后时补齐缺口。
// 2. 如果 helper 还没有加载任何消息，则直接从 DB 全量加载。
// 3. 如果 helper 与 DB 的最新消息不一致，则按情况做全量对账，保证顺序和内容一致。
func syncHelperWithDatabase(sessionID string, helper *aihelper.AIHelper) code.Code {
	latestDBMessage, err := messageDAO.GetLatestMessageBySessionID(sessionID)
	if err != nil {
		if messageDAO.IsMessageNotFoundError(err) {
			return code.CodeSuccess
		}
		log.Println("syncHelperWithDatabase GetLatestMessageBySessionID error:", err)
		return code.CodeServerBusy
	}

	// DB 最新消息已经在 helper 中，说明本地缓冲至少包含 DB 的最新状态。
	if helper.HasMessageKey(latestDBMessage.MessageKey) {
		dbMessageCount, err := messageDAO.GetMessageCountBySessionID(sessionID)
		if err != nil {
			log.Println("syncHelperWithDatabase GetMessageCountBySessionID error:", err)
			return code.CodeServerBusy
		}

		// 若 DB 记录数更多，说明有消息已写库但未进入当前 helper，需要重新对账补齐。
		// 这里统一走 ReconcileMessages，避免依赖局部增量修补的顺序假设。
		if int64(helper.GetPersistedMessageCount()) < dbMessageCount {
			dbMessages, err := messageDAO.GetMessagesBySessionID(sessionID)
			if err != nil {
				log.Println("syncHelperWithDatabase hole reconcile GetMessagesBySessionID error:", err)
				return code.CodeServerBusy
			}
			helper.ReconcileMessages(dbMessages)
		}
		return code.CodeSuccess
	}

	localLatestMessage := helper.GetLatestMessage()
	if localLatestMessage == nil {
		dbMessages, err := messageDAO.GetMessagesBySessionID(sessionID)
		if err != nil {
			log.Println("syncHelperWithDatabase GetMessagesBySessionID error:", err)
			return code.CodeServerBusy
		}
		helper.LoadMessages(dbMessages)
		return code.CodeSuccess
	}

	localLatestPersistedMessage := helper.GetLatestPersistedMessage()
	if localLatestPersistedMessage == nil {
		// helper 中还没有已持久化消息，当前无需做增量比较。
		return code.CodeSuccess
	}

	localLatestExistsInDB, err := messageDAO.ExistsMessageKey(localLatestMessage.MessageKey)
	if err != nil {
		log.Println("syncHelperWithDatabase ExistsMessageKey latest local error:", err)
		return code.CodeServerBusy
	}

	// helper 最新消息已落库，但与 DB 最新消息不一致，直接全量对账即可恢复一致性。
	if localLatestExistsInDB {
		dbMessages, err := messageDAO.GetMessagesBySessionID(sessionID)
		if err != nil {
			log.Println("syncHelperWithDatabase full DB reload error:", err)
			return code.CodeServerBusy
		}
		helper.ReconcileMessages(dbMessages)
		return code.CodeSuccess
	}

	// helper 最新消息尚未落库时，如果 DB 最新消息正好等于 helper 的最新已持久化消息，
	// 说明未落库部分只是本地新增缓冲内容，不需要用 DB 覆盖。
	// 其他情况则以 DB 为准重新对账，避免出现分叉。
	if localLatestPersistedMessage.MessageKey == latestDBMessage.MessageKey {
		return code.CodeSuccess
	}

	dbMessages, err := messageDAO.GetMessagesBySessionID(sessionID)
	if err != nil {
		log.Println("syncHelperWithDatabase final reconcile GetMessagesBySessionID error:", err)
		return code.CodeServerBusy
	}
	helper.ReconcileMessages(dbMessages)
	return code.CodeSuccess
}

// getOrSyncHelperWithHistory 获取当前会话的 helper，并按“Redis 热恢复优先、全量对账兜底”的顺序恢复。
// 本地 helper 只作为 execution cache 复用对象，不再承担主恢复入口语义。
func getOrSyncHelperWithHistory(ctx context.Context, userName string, sess *model.Session, resolved *resolvedChatRequest) (*aihelper.AIHelper, code.Code) {
	if resolved == nil || !aihelper.IsSupportedModelType(resolved.ModelType) {
		return nil, code.CodeInvalidParams
	}

	sessionID := sess.ID
	selectionSignature := resolved.RuntimeConfig.SelectionSignature(resolved.ModelType)
	manager := aihelper.GetGlobalManager()

	if !myredis.IsAvailable() {
		// Redis 降级时明确声明：跨请求恢复不再尝试热状态，直接回退 DB rebuild。
		logSessionTrace(ctx, "hot_state_disabled", "detail=redis_degraded_fallback_db")
		observability.RecordRedisHotStateLookup(false)
		observability.RecordHelperWarmResumeFallbackDB()
		observability.RecordHelperRecover(observability.HelperRecoverSourceDB)
		helper, reused, dbResult := BuildEphemeralHelperFromDB(ctx, userName, sess, resolved)
		if !dbResult.ok {
			return nil, dbResult.code
		}
		if reused {
			logSessionTrace(ctx, "process_helper_reused", "runtime_source=%s", SessionRuntimeSourceProcessEphemeral)
		}
		manager.SetAIHelper(userName, sessionID, helper)
		return helper, code.CodeSuccess
	}

	hotState, err := myredis.GetSessionHotState(ctx, sessionID)
	if err != nil {
		observability.RecordRedisHotStateLookup(false)
		logSessionTrace(ctx, "hot_state_read_fail", "err=%v", err)
		log.Println("getOrSyncHelperWithHistory GetSessionHotState error:", err)
	} else if hotState != nil {
		observability.RecordHelperWarmResumeCandidate()
		currentLease, leaseErr := myredis.GetSessionOwnerLeaseDetail(ctx, sessionID)
		if leaseErr != nil {
			observability.RecordHelperWarmResumeFallbackDB()
			observability.RecordRedisHotStateLookup(false)
			logSessionTrace(ctx, "owner_lease_read_fail", "err=%v", leaseErr)
			log.Println("getOrCreateHelperWithHistory GetSessionOwnerLeaseDetail error:", leaseErr)
		} else if canWarmResumeFromHotState(sess, hotState, currentLease, selectionSignature) {
			warmResumeStart := time.Now()
			observability.RecordRedisHotStateLookup(true)
			observability.RecordHelperRecover(observability.HelperRecoverSourceRedis)
			helper, reused, buildErr := BuildEphemeralHelperFromHotState(ctx, userName, sess, resolved, hotState)
			if buildErr != nil {
				log.Println("getOrSyncHelperWithHistory BuildEphemeralHelperFromHotState error:", buildErr)
				return nil, code.AIModelFail
			}
			if reused {
				logSessionTrace(ctx, "process_helper_reused", "runtime_source=%s", SessionRuntimeSourceProcessEphemeral)
			}
			observability.RecordHelperWarmResumeApplied(time.Since(warmResumeStart))
			manager.SetAIHelper(userName, sessionID, helper)
			return helper, code.CodeSuccess
		} else {
			observability.RecordHelperWarmResumeFallbackDB()
			observability.RecordRedisHotStateLookup(false)
			logSessionTrace(ctx, "hot_state_rejected", "redis_version=%d db_version=%d redis_fence=%d current_fence=%d redis_start=%d summary_count=%d", hotState.Version, sess.Version, hotState.FenceToken, func() int64 {
				if currentLease == nil {
					return 0
				}
				return currentLease.FenceToken
			}(), hotState.RecentMessagesStart, hotState.SummaryMessageCount)
		}
	} else {
		observability.RecordRedisHotStateLookup(false)
	}

	// 热态未命中或不可信时，再回退到全量 DB 对账。
	observability.RecordHelperRecover(observability.HelperRecoverSourceDB)
	helper, reused, dbResult := BuildEphemeralHelperFromDB(ctx, userName, sess, resolved)
	if !dbResult.ok {
		return nil, dbResult.code
	}
	if reused {
		logSessionTrace(ctx, "process_helper_reused", "runtime_source=%s", SessionRuntimeSourceProcessEphemeral)
	}
	manager.SetAIHelper(userName, sessionID, helper)
	return helper, code.CodeSuccess
}

// GetUserSessionsByUserName 获取用户的会话列表，只返回前端展示所需字段。
// 这里不暴露内部 helper 或持久化细节。
func GetUserSessionsByUserName(userName string) ([]model.SessionInfo, error) {
	sessions, err := sessionDAO.GetSessionsByUserName(userName)
	if err != nil {
		return nil, err
	}

	sessionInfos := make([]model.SessionInfo, 0, len(sessions))
	for _, sess := range sessions {
		sessionInfos = append(sessionInfos, model.SessionInfo{
			SessionID: sess.ID,
			Title:     sess.Title,
		})
	}

	return sessionInfos, nil
}

// GetSessionInfo 返回单个会话的基础信息和当前绑定的配置摘要。
// 这里用于聊天页回显当前会话正在使用的配置和模式。
func GetSessionInfo(userName string, sessionID string) (*model.SessionInfo, code.Code) {
	sess, code_ := ensureOwnedSession(userName, sessionID)
	if code_ != code.CodeSuccess {
		return nil, code_
	}

	info := buildSessionInfo(*sess)
	return &info, code.CodeSuccess
}

// CreateSessionAndSendMessage 创建新会话并同步返回首轮回复。
func CreateSessionAndSendMessage(ctx context.Context, userName string, userID int64, userQuestion string, req ChatRequest) (string, string, code.Code) {
	resolved, code_ := resolveChatRequest(userName, userID, req, nil)
	if code_ != code.CodeSuccess {
		return "", "", code_
	}

	requestStart := time.Now()
	if !allowChatRateLimit(ctx, userName) {
		observability.RecordRequest("create_sync", resolved.ModelType, false, time.Since(requestStart))
		return "", "", code.CodeTooManyRequests
	}

	newSession := &model.Session{
		ID:          uuid.New().String(),
		UserName:    userName,
		UserID:      userID,
		LLMConfigID: resolved.RuntimeConfig.LLMConfigID,
		ChatMode:    resolved.ChatMode,
		// 默认使用用户首问作为会话标题，后续可由用户自行重命名。
		Title: userQuestion,
	}
	createdSession, err := sessionDAO.CreateSession(newSession)
	if err != nil {
		log.Println("CreateSessionAndSendMessage CreateSession error:", err)
		observability.RecordRequest("create_sync", resolved.ModelType, false, time.Since(requestStart))
		return "", "", code.CodeServerBusy
	}

	ctx, trace := newSessionTrace(ctx, "create_sync", createdSession.ID, resolved.ModelType)
	logSessionTrace(ctx, "start", "user=%s", userName)
	logResolvedSelection(ctx, resolved)

	result := withSessionExecutionGuard(ctx, createdSession.ID, func(execCtx context.Context) codeExecutorResult {
		helper, code_ := getOrSyncHelperWithHistory(execCtx, userName, createdSession, resolved)
		if code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		userMessage, code_ := appendUserMessageAndCommitHotState(execCtx, helper, userName, userQuestion)
		if code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}
		if code_ = persistMessageSync(execCtx, userMessage); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		aiResponse, err := helper.GenerateResponseForPreparedUserMessage(userName, execCtx)
		if err != nil {
			log.Println("CreateSessionAndSendMessage GenerateResponse error:", err)
			return codeExecutorResult{code: code.AIModelFail}
		}
		if code_ = persistMessageSync(execCtx, aiResponse); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}
		if code_ = persistSessionProgressWithPersistedVersion(execCtx, createdSession.ID, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}
		if code_ = commitHelperHotState(execCtx, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}
		publishAssistantReadyNotificationBestEffort(execCtx, createdSession, aiResponse)

		return codeExecutorResult{
			code:       code.CodeSuccess,
			aiResponse: aiResponse.Content,
		}
	})
	if result.err != nil {
		log.Println("CreateSessionAndSendMessage execution guard error:", result.err)
		logSessionTrace(ctx, "failed", "err=%v", result.err)
		observability.RecordRequest("create_sync", resolved.ModelType, false, time.Since(requestStart))
		return "", "", code.CodeServerBusy
	}
	if result.busy {
		logSessionTrace(ctx, "busy", "code=%d retry_after_ms=%d", resolveBusyResultCode(result), result.retryAfterMs)
		observability.RecordRequest("create_sync", resolved.ModelType, false, time.Since(requestStart))
		return "", "", resolveBusyResultCode(result)
	}
	if result.code != code.CodeSuccess {
		logSessionTrace(ctx, "failed", "code=%d", result.code)
		observability.RecordRequest("create_sync", resolved.ModelType, false, time.Since(requestStart))
		return "", "", result.code
	}
	logSessionTrace(ctx, "success", "response_chars=%d request_id=%s", len(result.aiResponse), trace.RequestID)
	observability.RecordRequest("create_sync", resolved.ModelType, true, time.Since(requestStart))

	return createdSession.ID, result.aiResponse, code.CodeSuccess
}

// CreateStreamSessionOnly 仅创建流式会话，不立即触发模型回复。
// 适用于需要先拿到 sessionID，再分步推送流式内容的场景。
func CreateStreamSessionOnly(userName string, userID int64, userQuestion string, req ChatRequest) (string, code.Code) {
	resolved, code_ := resolveChatRequest(userName, userID, req, nil)
	if code_ != code.CodeSuccess {
		return "", code_
	}

	newSession := &model.Session{
		ID:          uuid.New().String(),
		UserName:    userName,
		UserID:      userID,
		LLMConfigID: resolved.RuntimeConfig.LLMConfigID,
		ChatMode:    resolved.ChatMode,
		Title:       userQuestion,
	}
	createdSession, err := sessionDAO.CreateSession(newSession)
	if err != nil {
		log.Println("CreateStreamSessionOnly CreateSession error:", err)
		return "", code.CodeServerBusy
	}
	return createdSession.ID, code.CodeSuccess
}

// StreamMessageToExistingSession 向已有会话发送消息，并以 SSE 方式流式返回结果。
func StreamMessageToExistingSession(ctx context.Context, userName string, sessionID string, userQuestion string, req ChatRequest, writer http.ResponseWriter) code.Code {
	sess, code_ := ensureOwnedSession(userName, sessionID)
	if code_ != code.CodeSuccess {
		return code_
	}
	resolved, code_ := resolveChatRequest(userName, sess.UserID, req, sess)
	if code_ != code.CodeSuccess {
		return code_
	}

	requestStart := time.Now()
	ctx, _ = newSessionTrace(ctx, "chat_stream", sessionID, resolved.ModelType)
	logSessionTrace(ctx, "start", "user=%s", userName)
	logResolvedSelection(ctx, resolved)
	observability.RecordStreamActiveDelta(1)
	defer observability.RecordStreamActiveDelta(-1)
	if !allowChatRateLimit(ctx, userName) {
		observability.RecordRequest("chat_stream", resolved.ModelType, false, time.Since(requestStart))
		return code.CodeTooManyRequests
	}

	flusher, ok := writer.(http.Flusher)
	if !ok {
		log.Println("StreamMessageToExistingSession: streaming unsupported")
		observability.RecordRequest("chat_stream", resolved.ModelType, false, time.Since(requestStart))
		return code.CodeServerBusy
	}
	if code_ = persistResolvedChatSelection(sess, resolved); code_ != code.CodeSuccess {
		observability.RecordRequest("chat_stream", resolved.ModelType, false, time.Since(requestStart))
		return code_
	}

	// 同一会话的流式生成也需要串行执行，避免并发写入造成上下文错乱。
	result := withSessionExecutionGuard(ctx, sessionID, func(execCtx context.Context) codeExecutorResult {
		helper, code_ := getOrSyncHelperWithHistory(execCtx, userName, sess, resolved)
		if code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		userMessage, code_ := appendUserMessageAndCommitHotState(execCtx, helper, userName, userQuestion)
		if code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}
		if code_ = persistMessageSync(execCtx, userMessage); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		cb := func(msg string) {
			// SSE 需要按 data: <chunk> 后跟两个换行的格式持续写出，并在每次写后 flush。
			_, err := writer.Write([]byte("data: " + msg + "\n\n"))
			if err != nil {
				log.Println("StreamMessageToExistingSession Write error:", err)
				return
			}
			flusher.Flush()
		}

		if _, err := helper.StreamResponseForPreparedUserMessage(userName, execCtx, cb); err != nil {
			log.Println("StreamMessageToExistingSession StreamResponse error:", err)
			if execCtx.Err() != nil {
				observability.RecordStreamDisconnect()
			}
			return codeExecutorResult{code: code.AIModelFail}
		}
		assistantMessage := helper.GetLatestMessage()
		if code_ = persistMessageSync(execCtx, assistantMessage); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}
		if code_ = persistSessionProgressWithPersistedVersion(execCtx, sessionID, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}
		if code_ = commitHelperHotState(execCtx, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}
		publishAssistantReadyNotificationBestEffort(execCtx, sess, assistantMessage)

		return codeExecutorResult{code: code.CodeSuccess}
	})
	if result.err != nil {
		log.Println("StreamMessageToExistingSession execution guard error:", result.err)
		logSessionTrace(ctx, "failed", "err=%v", result.err)
		observability.RecordRequest("chat_stream", resolved.ModelType, false, time.Since(requestStart))
		return code.CodeServerBusy
	}
	if result.busy {
		logSessionTrace(ctx, "busy", "code=%d retry_after_ms=%d", resolveBusyResultCode(result), result.retryAfterMs)
		observability.RecordRequest("chat_stream", resolved.ModelType, false, time.Since(requestStart))
		return resolveBusyResultCode(result)
	}
	if result.code != code.CodeSuccess {
		logSessionTrace(ctx, "failed", "code=%d", result.code)
		observability.RecordRequest("chat_stream", resolved.ModelType, false, time.Since(requestStart))
		return result.code
	}

	if _, err := writer.Write([]byte("data: [DONE]\n\n")); err != nil {
		log.Println("StreamMessageToExistingSession write DONE error:", err)
		if ctx.Err() != nil {
			observability.RecordStreamDisconnect()
		}
		observability.RecordRequest("chat_stream", resolved.ModelType, false, time.Since(requestStart))
		return code.AIModelFail
	}
	flusher.Flush()
	logSessionTrace(ctx, "success", "detail=stream_done")
	observability.RecordRequest("chat_stream", resolved.ModelType, true, time.Since(requestStart))
	return code.CodeSuccess
}

// CreateStreamSessionAndSendMessage 创建会话并立即开始流式回复。
func CreateStreamSessionAndSendMessage(ctx context.Context, userName string, userID int64, userQuestion string, req ChatRequest, writer http.ResponseWriter) (string, code.Code) {
	sessionID, code_ := CreateStreamSessionOnly(userName, userID, userQuestion, req)
	if code_ != code.CodeSuccess {
		return "", code_
	}

	code_ = StreamMessageToExistingSession(ctx, userName, sessionID, userQuestion, req, writer)
	if code_ != code.CodeSuccess {
		return sessionID, code_
	}

	return sessionID, code.CodeSuccess
}

// ChatSend 向已有会话发送消息，并同步返回完整回复。
func ChatSend(ctx context.Context, userName string, sessionID string, userQuestion string, req ChatRequest) (string, code.Code) {
	sess, code_ := ensureOwnedSession(userName, sessionID)
	if code_ != code.CodeSuccess {
		return "", code_
	}
	resolved, code_ := resolveChatRequest(userName, sess.UserID, req, sess)
	if code_ != code.CodeSuccess {
		return "", code_
	}

	requestStart := time.Now()
	ctx, _ = newSessionTrace(ctx, "chat_sync", sessionID, resolved.ModelType)
	logSessionTrace(ctx, "start", "user=%s", userName)
	logResolvedSelection(ctx, resolved)
	if !allowChatRateLimit(ctx, userName) {
		observability.RecordRequest("chat_sync", resolved.ModelType, false, time.Since(requestStart))
		return "", code.CodeTooManyRequests
	}
	if code_ = persistResolvedChatSelection(sess, resolved); code_ != code.CodeSuccess {
		observability.RecordRequest("chat_sync", resolved.ModelType, false, time.Since(requestStart))
		return "", code_
	}

	// 同一会话的同步生成同样通过执行保护串行化，避免状态竞争。
	result := withSessionExecutionGuard(ctx, sessionID, func(execCtx context.Context) codeExecutorResult {
		helper, code_ := getOrSyncHelperWithHistory(execCtx, userName, sess, resolved)
		if code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		userMessage, code_ := appendUserMessageAndCommitHotState(execCtx, helper, userName, userQuestion)
		if code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}
		if code_ = persistMessageSync(execCtx, userMessage); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		aiResponse, err := helper.GenerateResponseForPreparedUserMessage(userName, execCtx)
		if err != nil {
			log.Println("ChatSend GenerateResponse error:", err)
			return codeExecutorResult{code: code.AIModelFail}
		}
		if code_ = persistMessageSync(execCtx, aiResponse); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}
		if code_ = persistSessionProgressWithPersistedVersion(execCtx, sessionID, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}
		if code_ = commitHelperHotState(execCtx, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}
		publishAssistantReadyNotificationBestEffort(execCtx, sess, aiResponse)

		return codeExecutorResult{
			code:       code.CodeSuccess,
			aiResponse: aiResponse.Content,
		}
	})
	if result.err != nil {
		log.Println("ChatSend execution guard error:", result.err)
		logSessionTrace(ctx, "failed", "err=%v", result.err)
		observability.RecordRequest("chat_sync", resolved.ModelType, false, time.Since(requestStart))
		return "", code.CodeServerBusy
	}
	if result.busy {
		logSessionTrace(ctx, "busy", "code=%d retry_after_ms=%d", resolveBusyResultCode(result), result.retryAfterMs)
		observability.RecordRequest("chat_sync", resolved.ModelType, false, time.Since(requestStart))
		return "", resolveBusyResultCode(result)
	}
	if result.code != code.CodeSuccess {
		logSessionTrace(ctx, "failed", "code=%d", result.code)
		observability.RecordRequest("chat_sync", resolved.ModelType, false, time.Since(requestStart))
		return "", result.code
	}
	logSessionTrace(ctx, "success", "response_chars=%d", len(result.aiResponse))
	observability.RecordRequest("chat_sync", resolved.ModelType, true, time.Since(requestStart))
	return result.aiResponse, code.CodeSuccess
}

// GetChatHistory 获取会话历史消息，并转换为接口返回结构。
// 这里只透出前端所需字段，避免把内部存储结构直接暴露出去。
func GetChatHistory(userName string, sessionID string) ([]model.History, code.Code) {
	if _, code_ := ensureOwnedSession(userName, sessionID); code_ != code.CodeSuccess {
		return nil, code_
	}

	messages, err := messageDAO.GetMessagesBySessionID(sessionID)
	if err != nil {
		log.Println("GetChatHistory GetMessagesBySessionID error:", err)
		return nil, code.CodeServerBusy
	}

	history := make([]model.History, 0, len(messages))
	for _, msg := range messages {
		// 保留 IsUser 字段，供前端区分用户消息与模型消息。
		history = append(history, model.History{
			IsUser:  msg.IsUser,
			Content: msg.Content,
			Status:  msg.Status,
		})
	}

	return history, code.CodeSuccess
}

// ChatStreamSend 是 StreamMessageToExistingSession 的简单封装。
func ChatStreamSend(ctx context.Context, userName string, sessionID string, userQuestion string, req ChatRequest, writer http.ResponseWriter) code.Code {
	return StreamMessageToExistingSession(ctx, userName, sessionID, userQuestion, req, writer)
}

// RenameSession renames a session owned by the current user.
