package session

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	"GopherAI/common/observability"
	myredis "GopherAI/common/redis"
	messageDAO "GopherAI/dao/message"
	outboxDAO "GopherAI/dao/outbox"
	sessionDAO "GopherAI/dao/session"
	"GopherAI/model"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
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
func persistSessionProgress(ctx context.Context, sessionID string, helper *aihelper.AIHelper) code.Code {
	if code_ := ensureSessionWriteOwnership(ctx, sessionID); code_ != code.CodeSuccess {
		return code_
	}
	afterSummary, afterCount := helper.GetSummaryState()
	nextVersion := helper.GetVersion() + 1
	if err := sessionDAO.UpdateSessionProgress(sessionID, nextVersion, afterSummary, afterCount); err != nil {
		log.Println("persistSessionProgress UpdateSessionProgress error:", err)
		return code.CodeServerBusy
	}

	helper.SetVersion(nextVersion)
	return code.CodeSuccess
}

// persistHelperHotState 将 helper 的热状态写入 Redis。
// 写入失败只记录观测信息，不影响主流程返回结果。
func persistHelperHotState(ctx context.Context, helper *aihelper.AIHelper) {
	if helper == nil {
		return
	}
	guard := sessionOwnerGuardFromContext(ctx)
	if guard != nil && guard.SessionID == helper.SessionID {
		valid, err := myredis.ValidateSessionOwnerFence(ctx, helper.SessionID, guard.OwnerID, guard.FenceToken)
		if err != nil {
			observability.RecordRedisHotStateSaveFail()
			logSessionTrace(ctx, "hot_state_owner_validate_fail", "err=%v", err)
			log.Println("persistHelperHotState ValidateSessionOwnerFence error:", err)
			return
		}
		if !valid {
			logSessionTrace(ctx, "hot_state_owner_reject", "owner=%s fence=%d", guard.OwnerID, guard.FenceToken)
			return
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
		return
	}
	if result == myredis.SessionHotStateSaveIgnoredStale {
		logSessionTrace(ctx, "hot_state_save_ignored", "detail=stale_snapshot")
	}
}

// applySessionMetadataToHelper 用 session 表中的正式元信息刷新 helper。
// 这里不碰消息内容，只覆盖 version / summary 这类“会话级正式状态”。
func applySessionMetadataToHelper(sess *model.Session, helper *aihelper.AIHelper) {
	if sess == nil || helper == nil {
		return
	}
	helper.SetVersion(sess.Version)
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

// getOrSyncHelperWithHistory 获取当前会话的 helper，并按“热恢复优先、全量对账兜底”的顺序恢复。
// 主链路先尝试进程内 helper，再尝试 Redis warm resume，只有热态不可信时才回退 DB 全量对账。
func getOrSyncHelperWithHistory(ctx context.Context, userName string, sess *model.Session, resolved *resolvedChatRequest) (*aihelper.AIHelper, code.Code) {
	if resolved == nil || !aihelper.IsSupportedModelType(resolved.ModelType) {
		return nil, code.CodeInvalidParams
	}

	sessionID := sess.ID
	selectionSignature := resolved.RuntimeConfig.SelectionSignature(resolved.ModelType)
	manager := aihelper.GetGlobalManager()
	if helper, exists := manager.GetAIHelper(userName, sessionID); exists {
		if helper.MatchesSelection(selectionSignature) {
			observability.RecordHelperRecover(observability.HelperRecoverSourceProcess)
			if code_ := lightReconcileHelperWithSession(sess, helper); code_ != code.CodeSuccess {
				return nil, code_
			}
			return helper, code.CodeSuccess
		}
		manager.RemoveAIHelper(userName, sessionID)
	}

	helper, err := manager.GetOrCreateAIHelper(userName, sessionID, resolved.ModelType, resolved.RuntimeConfig)
	if err != nil {
		log.Println("getOrCreateHelperWithHistory GetOrCreateAIHelper error:", err)
		return nil, code.AIModelFail
	}
	applySessionMetadataToHelper(sess, helper)

	hotState, err := myredis.GetSessionHotState(ctx, sessionID)
	if err != nil {
		observability.RecordRedisHotStateLookup(false)
		logSessionTrace(ctx, "hot_state_read_fail", "err=%v", err)
		log.Println("getOrCreateHelperWithHistory GetSessionHotState error:", err)
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
			helper.LoadHotState(hotState)
			if code_ := lightReconcileHelperWithSession(sess, helper); code_ != code.CodeSuccess {
				return nil, code_
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
	if code_ := fullReconcileHelperWithDatabase(sess, helper); code_ != code.CodeSuccess {
		return nil, code_
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

		aiResponse, err := helper.GenerateResponse(userName, execCtx, userQuestion)
		if err != nil {
			log.Println("CreateSessionAndSendMessage GenerateResponse error:", err)
			return codeExecutorResult{code: code.AIModelFail}
		}
		if code_ = persistSessionProgress(execCtx, createdSession.ID, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		persistHelperHotState(execCtx, helper)
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

		cb := func(msg string) {
			// SSE 需要按 data: <chunk> 后跟两个换行的格式持续写出，并在每次写后 flush。
			_, err := writer.Write([]byte("data: " + msg + "\n\n"))
			if err != nil {
				log.Println("StreamMessageToExistingSession Write error:", err)
				return
			}
			flusher.Flush()
		}

		if _, err := helper.StreamResponse(userName, execCtx, cb, userQuestion); err != nil {
			log.Println("StreamMessageToExistingSession StreamResponse error:", err)
			if execCtx.Err() != nil {
				observability.RecordStreamDisconnect()
			}
			return codeExecutorResult{code: code.AIModelFail}
		}
		if code_ = persistSessionProgress(execCtx, sessionID, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		persistHelperHotState(execCtx, helper)
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

		aiResponse, err := helper.GenerateResponse(userName, execCtx, userQuestion)
		if err != nil {
			log.Println("ChatSend GenerateResponse error:", err)
			return codeExecutorResult{code: code.AIModelFail}
		}
		if code_ = persistSessionProgress(execCtx, sessionID, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		persistHelperHotState(execCtx, helper)
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
	messages, code_ := mergeUndeliveredOutboxMessages(sessionID, messages)
	if code_ != code.CodeSuccess {
		return nil, code_
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

func mergeUndeliveredOutboxMessages(sessionID string, messages []model.Message) ([]model.Message, code.Code) {
	events, err := outboxDAO.ListUndeliveredMessageOutboxesBySessionID(sessionID)
	if err != nil {
		log.Println("mergeUndeliveredOutboxMessages ListUndeliveredMessageOutboxesBySessionID error:", err)
		return nil, code.CodeServerBusy
	}
	if len(events) == 0 {
		return messages, code.CodeSuccess
	}

	messageKeys := make(map[string]struct{}, len(messages))
	for _, msg := range messages {
		messageKeys[msg.MessageKey] = struct{}{}
	}

	for _, event := range events {
		if _, exists := messageKeys[event.MessageKey]; exists {
			continue
		}
		var payload struct {
			MessageKey     string `json:"message_key"`
			SessionID      string `json:"session_id"`
			SessionVersion int64  `json:"session_version"`
			Content        string `json:"content"`
			UserName       string `json:"user_name"`
			IsUser         bool   `json:"is_user"`
			Status         string `json:"status"`
		}
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
			log.Println("mergeUndeliveredOutboxMessages payload unmarshal error:", err)
			continue
		}
		if payload.SessionID != sessionID {
			continue
		}
		status := model.MessageStatus(payload.Status)
		if status == "" {
			status = model.MessageStatusCompleted
		}
		messages = append(messages, model.Message{
			MessageKey:     payload.MessageKey,
			SessionID:      payload.SessionID,
			SessionVersion: payload.SessionVersion,
			UserName:       payload.UserName,
			Content:        payload.Content,
			IsUser:         payload.IsUser,
			Status:         status,
			CreatedAt:      event.CreatedAt,
			UpdatedAt:      event.UpdatedAt,
		})
		messageKeys[payload.MessageKey] = struct{}{}
	}

	sort.SliceStable(messages, func(i, j int) bool {
		if messages[i].SessionVersion != messages[j].SessionVersion {
			return messages[i].SessionVersion < messages[j].SessionVersion
		}
		if messages[i].IsUser != messages[j].IsUser {
			return messages[i].IsUser
		}
		if !messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			return messages[i].CreatedAt.Before(messages[j].CreatedAt)
		}
		return messages[i].MessageKey < messages[j].MessageKey
	})

	return messages, code.CodeSuccess
}

// ChatStreamSend 是 StreamMessageToExistingSession 的简单封装。
func ChatStreamSend(ctx context.Context, userName string, sessionID string, userQuestion string, req ChatRequest, writer http.ResponseWriter) code.Code {
	return StreamMessageToExistingSession(ctx, userName, sessionID, userQuestion, req, writer)
}

// RenameSession renames a session owned by the current user.
