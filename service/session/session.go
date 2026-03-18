package session

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	messageDAO "GopherAI/dao/message"
	sessionDAO "GopherAI/dao/session"
	"GopherAI/model"
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// buildAIConfig 统一构造模型初始化参数，避免同步和流式链路重复拼装配置。
func buildAIConfig(userName string) map[string]interface{} {
	return map[string]interface{}{
		"apiKey":   "your-api-key", // TODO: 后续从配置中心或环境变量读取
		"username": userName,       // RAG / MCP 等模型需要知道当前用户身份
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

// getOrCreateHelperWithHistory 优先复用当前进程中的 helper；
// 如果 helper 不存在，就从数据库回放历史消息，再把 helper 放回 manager 中缓存。
func getOrCreateHelperWithHistory(userName string, sessionID string, modelType string) (*aihelper.AIHelper, code.Code) {
	manager := aihelper.GetGlobalManager()
	if helper, exists := manager.GetAIHelper(userName, sessionID); exists {
		return helper, code.CodeSuccess
	}

	helper, err := manager.GetOrCreateAIHelper(userName, sessionID, modelType, buildAIConfig(userName))
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
func CreateSessionAndSendMessage(ctx context.Context, userName string, userQuestion string, modelType string) (string, string, code.Code) {
	newSession := &model.Session{
		ID:       uuid.New().String(),
		UserName: userName,
		// 先保持现有产品语义：用首条问题作为会话标题。
		Title: userQuestion,
	}
	createdSession, err := sessionDAO.CreateSession(newSession)
	if err != nil {
		log.Println("CreateSessionAndSendMessage CreateSession error:", err)
		return "", "", code.CodeServerBusy
	}

	helper, code_ := getOrCreateHelperWithHistory(userName, createdSession.ID, modelType)
	if code_ != code.CodeSuccess {
		return "", "", code_
	}

	aiResponse, err := helper.GenerateResponse(userName, ctx, userQuestion)
	if err != nil {
		log.Println("CreateSessionAndSendMessage GenerateResponse error:", err)
		return "", "", code.AIModelFail
	}

	return createdSession.ID, aiResponse.Content, code.CodeSuccess
}

// CreateStreamSessionOnly 只创建会话，不发送消息。
// 流式场景先下发 sessionID，再开始持续推流。
func CreateStreamSessionOnly(userName string, userQuestion string) (string, code.Code) {
	newSession := &model.Session{
		ID:       uuid.New().String(),
		UserName: userName,
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
	flusher, ok := writer.(http.Flusher)
	if !ok {
		log.Println("StreamMessageToExistingSession: streaming unsupported")
		return code.CodeServerBusy
	}

	if _, code_ := ensureOwnedSession(userName, sessionID); code_ != code.CodeSuccess {
		return code_
	}

	helper, code_ := getOrCreateHelperWithHistory(userName, sessionID, modelType)
	if code_ != code.CodeSuccess {
		return code_
	}

	cb := func(msg string) {
		// SSE 要求每个片段都按 "data: xxx\n\n" 输出，并在每次写入后立即 flush。
		_, err := writer.Write([]byte("data: " + msg + "\n\n"))
		if err != nil {
			log.Println("StreamMessageToExistingSession Write error:", err)
			return
		}
		flusher.Flush()
	}

	if _, err := helper.StreamResponse(userName, ctx, cb, userQuestion); err != nil {
		log.Println("StreamMessageToExistingSession StreamResponse error:", err)
		return code.AIModelFail
	}

	if _, err := writer.Write([]byte("data: [DONE]\n\n")); err != nil {
		log.Println("StreamMessageToExistingSession write DONE error:", err)
		return code.AIModelFail
	}
	flusher.Flush()

	return code.CodeSuccess
}

// CreateStreamSessionAndSendMessage 创建会话后立即走流式回复。
func CreateStreamSessionAndSendMessage(ctx context.Context, userName string, userQuestion string, modelType string, writer http.ResponseWriter) (string, code.Code) {
	sessionID, code_ := CreateStreamSessionOnly(userName, userQuestion)
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
	if _, code_ := ensureOwnedSession(userName, sessionID); code_ != code.CodeSuccess {
		return "", code_
	}

	helper, code_ := getOrCreateHelperWithHistory(userName, sessionID, modelType)
	if code_ != code.CodeSuccess {
		return "", code_
	}

	aiResponse, err := helper.GenerateResponse(userName, ctx, userQuestion)
	if err != nil {
		log.Println("ChatSend GenerateResponse error:", err)
		return "", code.AIModelFail
	}

	return aiResponse.Content, code.CodeSuccess
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
		})
	}

	return history, code.CodeSuccess
}

// ChatStreamSend 向已有会话发送流式消息。
func ChatStreamSend(ctx context.Context, userName string, sessionID string, userQuestion string, modelType string, writer http.ResponseWriter) code.Code {
	return StreamMessageToExistingSession(ctx, userName, sessionID, userQuestion, modelType, writer)
}
