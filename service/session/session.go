package session

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	"GopherAI/common/observability"
	myredis "GopherAI/common/redis"
	messageDAO "GopherAI/dao/message"
	sessionDAO "GopherAI/dao/session"
	"GopherAI/model"
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// buildAIConfig 统一构造模型初始化参数，避免同步和流式链路重复拼装配置。
func buildAIConfig(userName string, userID int64) map[string]interface{} {
	return map[string]interface{}{
		"apiKey":   "your-api-key", // TODO: 后续从配置中心或环境变量读取
		"username": userName,       // MCP 等模型需要知道当前用户身份
		"userID":   userID,         // RAG 模型需要 userID 查询文件
	}
}

// ensureOwnedSession 统一校验会话是否存在，以及是否属于当前用户。
// 数据库负责会话真相和权限边界，不能只靠运行时 helper 判断会话是否合法。
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

// persistSummaryIfChanged 只在摘要确实变化时回写数据库，避免每轮请求都更新 session。
func persistSummaryIfChanged(sessionID string, beforeSummary string, beforeCount int, helper *aihelper.AIHelper) code.Code {
	afterSummary, afterCount := helper.GetSummaryState()
	if beforeSummary == afterSummary && beforeCount == afterCount {
		return code.CodeSuccess
	}

	if err := sessionDAO.UpdateSessionSummary(sessionID, afterSummary, afterCount); err != nil {
		log.Println("persistSummaryIfChanged UpdateSessionSummary error:", err)
		return code.CodeServerBusy
	}

	return code.CodeSuccess
}

// persistHelperHotState 把 helper 当前的轻量热状态快照写入 Redis。
// 这里故意不把 Redis 当真相源，所以写失败只记日志，不阻断主聊天链路。
func persistHelperHotState(ctx context.Context, helper *aihelper.AIHelper) {
	if helper == nil {
		return
	}

	if err := myredis.SaveSessionHotState(ctx, helper.ExportHotState()); err != nil {
		observability.RecordRedisHotStateSaveFail()
		logSessionTrace(ctx, "hot_state_save_fail", "err=%v", err)
		log.Println("persistHelperHotState SaveSessionHotState error:", err)
	}
}

// syncHelperWithDatabase 在“继续某个会话前”校准本地 helper 与数据库消息状态。
// 规则是：
// 1. DB 最新消息已经在本地：说明本地至少不落后于 DB，保留本地 buffer。
// 2. 本地最新消息已落库，但 DB 最新消息不在本地：说明本地缺消息，按 DB 补回。
// 3. 本地最新消息未落库：说明本地可能领先；只有当 DB 最新消息也越过了本地最后一条已持久化消息时，才做保守重构。
func syncHelperWithDatabase(sessionID string, helper *aihelper.AIHelper) code.Code {
	latestDBMessage, err := messageDAO.GetLatestMessageBySessionID(sessionID)
	if err != nil {
		if messageDAO.IsMessageNotFoundError(err) {
			return code.CodeSuccess
		}
		log.Println("syncHelperWithDatabase GetLatestMessageBySessionID error:", err)
		return code.CodeServerBusy
	}

	// DB 最新消息已在本地，说明本地没有落后于数据库，不需要拿 DB 反向覆盖本地 buffer。
	if helper.HasMessageKey(latestDBMessage.MessageKey) {
		dbMessageCount, err := messageDAO.GetMessageCountBySessionID(sessionID)
		if err != nil {
			log.Println("syncHelperWithDatabase GetMessageCountBySessionID error:", err)
			return code.CodeServerBusy
		}

		// 即使“最后一条消息”对上了，也不代表中间没有缺口。
		// 如果本地已持久化消息数比 DB 少，仍然要做一次保守对齐。
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
		// 本地只有尚未落库的消息时，不能让 DB 反向覆盖本地。
		return code.CodeSuccess
	}

	localLatestExistsInDB, err := messageDAO.ExistsMessageKey(localLatestMessage.MessageKey)
	if err != nil {
		log.Println("syncHelperWithDatabase ExistsMessageKey latest local error:", err)
		return code.CodeServerBusy
	}

	// 本地最新消息已经在 DB，但 DB 最新消息却不在本地，说明本地 helper 缺了后续消息。
	if localLatestExistsInDB {
		dbMessages, err := messageDAO.GetMessagesBySessionID(sessionID)
		if err != nil {
			log.Println("syncHelperWithDatabase full DB reload error:", err)
			return code.CodeServerBusy
		}
		helper.ReconcileMessages(dbMessages)
		return code.CodeSuccess
	}

	// 走到这里说明：本地最新消息尚未落库，本地 buffer 领先于 DB。
	// 这时只有当 DB 最新消息已经不是“本地最后一条已持久化消息”时，
	// 才说明两边可能都各自缺了一部分，需要做保守重构。
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

// getOrSyncHelperWithHistory 优先复用当前进程中的 helper；
// 如果 helper 不存在，就从数据库回放历史消息；
// 如果 helper 已存在，就在继续会话前做一次本地/DB 的安全对齐。
func getOrSyncHelperWithHistory(ctx context.Context, userName string, sess *model.Session, modelType string) (*aihelper.AIHelper, code.Code) {
	if !aihelper.IsSupportedModelType(modelType) {
		return nil, code.CodeInvalidParams
	}

	sessionID := sess.ID
	manager := aihelper.GetGlobalManager()
	if helper, exists := manager.GetAIHelper(userName, sessionID); exists {
		observability.RecordHelperRecover(observability.HelperRecoverSourceProcess)
		helper.SetSummaryState(sess.ContextSummary, sess.SummaryMessageCount)
		if code_ := syncHelperWithDatabase(sessionID, helper); code_ != code.CodeSuccess {
			return nil, code_
		}
		return helper, code.CodeSuccess
	}

	helper, err := manager.GetOrCreateAIHelper(userName, sessionID, modelType, buildAIConfig(userName, sess.UserID))
	if err != nil {
		log.Println("getOrCreateHelperWithHistory GetOrCreateAIHelper error:", err)
		return nil, code.AIModelFail
	}

	// helper 首次进入当前进程时，需要从数据库回放消息历史，恢复会话上下文。
	msgs, err := messageDAO.GetMessagesBySessionID(sessionID)
	if err != nil {
		log.Println("getOrCreateHelperWithHistory GetMessagesBySessionID error:", err)
		return nil, code.CodeServerBusy
	}
	if len(msgs) > 0 {
		helper.LoadMessages(msgs)
	}

	// 第二轮升级里引入 Redis 热状态快照，但这里仍然保留 DB 回放作为兜底真相恢复。
	// 也就是说：先保证“至少能恢复”，再用 Redis 热快照把最近窗口状态补回来。
	hotState, err := myredis.GetSessionHotState(ctx, sessionID)
	if err != nil {
		observability.RecordRedisHotStateLookup(false)
		logSessionTrace(ctx, "hot_state_read_fail", "err=%v", err)
		log.Println("getOrCreateHelperWithHistory GetSessionHotState error:", err)
	} else if hotState != nil {
		observability.RecordRedisHotStateLookup(true)
		observability.RecordHelperRecover(observability.HelperRecoverSourceRedis)
		helper.LoadHotState(hotState)
	} else {
		observability.RecordRedisHotStateLookup(false)
	}
	observability.RecordHelperRecover(observability.HelperRecoverSourceDB)
	helper.SetSummaryState(sess.ContextSummary, sess.SummaryMessageCount)
	if code_ := syncHelperWithDatabase(sessionID, helper); code_ != code.CodeSuccess {
		return nil, code_
	}

	manager.SetAIHelper(userName, sessionID, helper)
	return helper, code.CodeSuccess
}

// GetUserSessionsByUserName 从数据库读取会话列表。
// 会话列表属于业务真相，不能依赖进程内 helper 的生命周期。
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

// CreateSessionAndSendMessage 创建新会话并发送第一条消息。
func CreateSessionAndSendMessage(ctx context.Context, userName string, userID int64, userQuestion string, modelType string) (string, string, code.Code) {
	requestStart := time.Now()
	if !allowChatRateLimit(ctx, userName) {
		observability.RecordRequest("create_sync", modelType, false, time.Since(requestStart))
		return "", "", code.CodeTooManyRequests
	}

	newSession := &model.Session{
		ID:       uuid.New().String(),
		UserName: userName,
		UserID:   userID,
		// 先保持现有产品语义：用首条问题作为会话标题。
		Title: userQuestion,
	}
	createdSession, err := sessionDAO.CreateSession(newSession)
	if err != nil {
		log.Println("CreateSessionAndSendMessage CreateSession error:", err)
		observability.RecordRequest("create_sync", modelType, false, time.Since(requestStart))
		return "", "", code.CodeServerBusy
	}

	ctx, trace := newSessionTrace(ctx, "create_sync", createdSession.ID, modelType)
	logSessionTrace(ctx, "start", "user=%s", userName)

	result := withSessionExecutionGuard(ctx, createdSession.ID, func() codeExecutorResult {
		helper, code_ := getOrSyncHelperWithHistory(ctx, userName, createdSession, modelType)
		if code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		beforeSummary, beforeCount := helper.GetSummaryState()
		aiResponse, err := helper.GenerateResponse(userName, ctx, userQuestion)
		if err != nil {
			log.Println("CreateSessionAndSendMessage GenerateResponse error:", err)
			return codeExecutorResult{code: code.AIModelFail}
		}
		if code_ = persistSummaryIfChanged(createdSession.ID, beforeSummary, beforeCount, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		persistHelperHotState(ctx, helper)
		return codeExecutorResult{
			code:       code.CodeSuccess,
			aiResponse: aiResponse.Content,
		}
	})
	if result.err != nil {
		log.Println("CreateSessionAndSendMessage execution guard error:", result.err)
		logSessionTrace(ctx, "failed", "err=%v", result.err)
		observability.RecordRequest("create_sync", modelType, false, time.Since(requestStart))
		return "", "", code.CodeServerBusy
	}
	if result.busy {
		logSessionTrace(ctx, "busy", "detail=distributed_lock_busy")
		observability.RecordRequest("create_sync", modelType, false, time.Since(requestStart))
		return "", "", code.CodeTooManyRequests
	}
	if result.code != code.CodeSuccess {
		logSessionTrace(ctx, "failed", "code=%d", result.code)
		observability.RecordRequest("create_sync", modelType, false, time.Since(requestStart))
		return "", "", result.code
	}
	logSessionTrace(ctx, "success", "response_chars=%d request_id=%s", len(result.aiResponse), trace.RequestID)
	observability.RecordRequest("create_sync", modelType, true, time.Since(requestStart))

	return createdSession.ID, result.aiResponse, code.CodeSuccess
}

// CreateStreamSessionOnly 只创建会话，不发送消息。
// 流式场景先下发 sessionID，再开始持续推流。
func CreateStreamSessionOnly(userName string, userID int64, userQuestion string) (string, code.Code) {
	newSession := &model.Session{
		ID:       uuid.New().String(),
		UserName: userName,
		UserID:   userID,
		Title:    userQuestion,
	}
	createdSession, err := sessionDAO.CreateSession(newSession)
	if err != nil {
		log.Println("CreateStreamSessionOnly CreateSession error:", err)
		return "", code.CodeServerBusy
	}
	return createdSession.ID, code.CodeSuccess
}

// StreamMessageToExistingSession 向已有会话发送一条流式消息。
func StreamMessageToExistingSession(ctx context.Context, userName string, sessionID string, userQuestion string, modelType string, writer http.ResponseWriter) code.Code {
	requestStart := time.Now()
	ctx, _ = newSessionTrace(ctx, "chat_stream", sessionID, modelType)
	logSessionTrace(ctx, "start", "user=%s", userName)
	observability.RecordStreamActiveDelta(1)
	defer observability.RecordStreamActiveDelta(-1)
	if !allowChatRateLimit(ctx, userName) {
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code.CodeTooManyRequests
	}

	flusher, ok := writer.(http.Flusher)
	if !ok {
		log.Println("StreamMessageToExistingSession: streaming unsupported")
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code.CodeServerBusy
	}

	sess, code_ := ensureOwnedSession(userName, sessionID)
	if code_ != code.CodeSuccess {
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code_
	}

	// 从这里开始走新的“会话执行保护 + 热状态回写”链路。
	result := withSessionExecutionGuard(ctx, sessionID, func() codeExecutorResult {
		helper, code_ := getOrSyncHelperWithHistory(ctx, userName, sess, modelType)
		if code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		cb := func(msg string) {
			// SSE 协议要求每个片段都按 data 行输出，并在每次写入后立刻 flush。
			_, err := writer.Write([]byte("data: " + msg + "\n\n"))
			if err != nil {
				log.Println("StreamMessageToExistingSession Write error:", err)
				return
			}
			flusher.Flush()
		}

		beforeSummary, beforeCount := helper.GetSummaryState()
		if _, err := helper.StreamResponse(userName, ctx, cb, userQuestion); err != nil {
			log.Println("StreamMessageToExistingSession StreamResponse error:", err)
			if ctx.Err() != nil {
				observability.RecordStreamDisconnect()
			}
			return codeExecutorResult{code: code.AIModelFail}
		}
		if code_ = persistSummaryIfChanged(sessionID, beforeSummary, beforeCount, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		persistHelperHotState(ctx, helper)
		return codeExecutorResult{code: code.CodeSuccess}
	})
	if result.err != nil {
		log.Println("StreamMessageToExistingSession execution guard error:", result.err)
		logSessionTrace(ctx, "failed", "err=%v", result.err)
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code.CodeServerBusy
	}
	if result.busy {
		logSessionTrace(ctx, "busy", "detail=distributed_lock_busy")
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code.CodeTooManyRequests
	}
	if result.code != code.CodeSuccess {
		logSessionTrace(ctx, "failed", "code=%d", result.code)
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return result.code
	}

	if _, err := writer.Write([]byte("data: [DONE]\n\n")); err != nil {
		log.Println("StreamMessageToExistingSession write DONE error:", err)
		if ctx.Err() != nil {
			observability.RecordStreamDisconnect()
		}
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code.AIModelFail
	}
	flusher.Flush()
	logSessionTrace(ctx, "success", "detail=stream_done")
	observability.RecordRequest("chat_stream", modelType, true, time.Since(requestStart))
	return code.CodeSuccess
}

// CreateStreamSessionAndSendMessage 创建会话后立即走流式回复。
func CreateStreamSessionAndSendMessage(ctx context.Context, userName string, userID int64, userQuestion string, modelType string, writer http.ResponseWriter) (string, code.Code) {
	sessionID, code_ := CreateStreamSessionOnly(userName, userID, userQuestion)
	if code_ != code.CodeSuccess {
		return "", code_
	}

	code_ = StreamMessageToExistingSession(ctx, userName, sessionID, userQuestion, modelType, writer)
	if code_ != code.CodeSuccess {
		return sessionID, code_
	}

	return sessionID, code.CodeSuccess
}

// ChatSend 向已有会话发送同步消息。
func ChatSend(ctx context.Context, userName string, sessionID string, userQuestion string, modelType string) (string, code.Code) {
	requestStart := time.Now()
	ctx, _ = newSessionTrace(ctx, "chat_sync", sessionID, modelType)
	logSessionTrace(ctx, "start", "user=%s", userName)
	if !allowChatRateLimit(ctx, userName) {
		observability.RecordRequest("chat_sync", modelType, false, time.Since(requestStart))
		return "", code.CodeTooManyRequests
	}

	sess, code_ := ensureOwnedSession(userName, sessionID)
	if code_ != code.CodeSuccess {
		observability.RecordRequest("chat_sync", modelType, false, time.Since(requestStart))
		return "", code_
	}

	// 新链路在这里提前返回，后面的旧逻辑仅保留作对照，实际不会再走到。
	result := withSessionExecutionGuard(ctx, sessionID, func() codeExecutorResult {
		helper, code_ := getOrSyncHelperWithHistory(ctx, userName, sess, modelType)
		if code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		beforeSummary, beforeCount := helper.GetSummaryState()
		aiResponse, err := helper.GenerateResponse(userName, ctx, userQuestion)
		if err != nil {
			log.Println("ChatSend GenerateResponse error:", err)
			return codeExecutorResult{code: code.AIModelFail}
		}
		if code_ = persistSummaryIfChanged(sessionID, beforeSummary, beforeCount, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		persistHelperHotState(ctx, helper)
		return codeExecutorResult{
			code:       code.CodeSuccess,
			aiResponse: aiResponse.Content,
		}
	})
	if result.err != nil {
		log.Println("ChatSend execution guard error:", result.err)
		logSessionTrace(ctx, "failed", "err=%v", result.err)
		observability.RecordRequest("chat_sync", modelType, false, time.Since(requestStart))
		return "", code.CodeServerBusy
	}
	if result.busy {
		logSessionTrace(ctx, "busy", "detail=distributed_lock_busy")
		observability.RecordRequest("chat_sync", modelType, false, time.Since(requestStart))
		return "", code.CodeTooManyRequests
	}
	if result.code != code.CodeSuccess {
		logSessionTrace(ctx, "failed", "code=%d", result.code)
		observability.RecordRequest("chat_sync", modelType, false, time.Since(requestStart))
		return "", result.code
	}
	logSessionTrace(ctx, "success", "response_chars=%d", len(result.aiResponse))
	observability.RecordRequest("chat_sync", modelType, true, time.Since(requestStart))
	return result.aiResponse, code.CodeSuccess
}

// GetChatHistory 从数据库读取历史消息。
// 历史接口强调可恢复性和一致性，因此必须以数据库中的消息记录为准。
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
		// 直接使用持久化的 IsUser 字段，避免再通过奇偶位猜测消息身份。
		history = append(history, model.History{
			IsUser:  msg.IsUser,
			Content: msg.Content,
			Status:  msg.Status,
		})
	}

	return history, code.CodeSuccess
}

// ChatStreamSend 向已有会话发送流式消息。
func ChatStreamSend(ctx context.Context, userName string, sessionID string, userQuestion string, modelType string, writer http.ResponseWriter) code.Code {
	return StreamMessageToExistingSession(ctx, userName, sessionID, userQuestion, modelType, writer)
}
