package session

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	"GopherAI/common/observability"
	"GopherAI/controller"
	"GopherAI/model"
	"GopherAI/service/session"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// validateModelType 在 controller 层提前拦截非法模型类型。
// 这样可以避免无效参数继续进入 service 和模型工厂，减少无意义的资源消耗。
func validateModelType(modelType string) bool {
	return aihelper.IsSupportedModelType(modelType)
}

type (
	GetUserSessionsResponse struct {
		controller.Response
		Sessions []model.SessionInfo `json:"sessions,omitempty"`
	}
	CreateSessionAndSendMessageRequest struct {
		UserQuestion string `json:"question" binding:"required"`
		ModelType    string `json:"modelType" binding:"required"`
	}

	CreateSessionAndSendMessageResponse struct {
		AiInformation string `json:"Information,omitempty"`
		SessionID     string `json:"sessionId,omitempty"`
		controller.Response
	}

	ChatSendRequest struct {
		UserQuestion string `json:"question" binding:"required"`
		ModelType    string `json:"modelType" binding:"required"`
		SessionID    string `json:"sessionId,omitempty" binding:"required"`
	}

	ChatSendResponse struct {
		AiInformation string `json:"Information,omitempty"`
		controller.Response
	}

	ChatHistoryRequest struct {
		SessionID string `json:"sessionId,omitempty" binding:"required"`
	}
	ChatHistoryResponse struct {
		History []model.History `json:"history"`
		controller.Response
	}
	StopStreamRequest struct {
		SessionID string `json:"sessionId" binding:"required"`
	}
	StopStreamResponse struct {
		PartialContent string `json:"partialContent,omitempty"`
		controller.Response
	}
	AIObservabilityResponse struct {
		controller.Response
		Data observability.AISnapshot `json:"data"`
	}
)

const (
	// syncChatTimeout 控制同步聊天接口的最长执行时间。
	// 同步接口没有流式回传，用户感知的是一次完整等待，因此超时值可以相对短一些。
	syncChatTimeout = 90 * time.Second
	// streamChatTimeout 控制流式聊天接口的最长执行时间。
	// 流式请求本身会持续输出，因此这里给比同步请求更宽松的窗口，避免正常长回答被过早中断。
	streamChatTimeout = 3 * time.Minute
)

// buildChatTimeoutContext 给聊天请求统一挂上 timeout。
// 这样 controller 层就能提供清晰的超时边界，不再完全依赖下游模型或网络层自己结束。
func buildChatTimeoutContext(c *gin.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), timeout)
}

// writeSSEError 用统一 JSON 结构把错误写回流式前端。
// 这里不用普通 JSON 响应，是因为流式接口已经进入 SSE 协议，前端需要继续按 data: 行解析。
func writeSSEError(c *gin.Context, code_ code.Code) {
	_, _ = c.Writer.WriteString(fmt.Sprintf("data: {\"type\":\"error\",\"status_code\":%d,\"message\":\"%s\"}\n\n", code_, code_.Msg()))
	c.Writer.Flush()
}

func writeSSEJSON(c *gin.Context, payload map[string]interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		writeSSEError(c, code.CodeServerBusy)
		return
	}
	_, _ = c.Writer.WriteString("data: " + string(data) + "\n\n")
	c.Writer.Flush()
}

func GetUserSessionsByUserName(c *gin.Context) {
	res := new(GetUserSessionsResponse)
	userName := c.GetString("userName")

	userSessions, err := session.GetUserSessionsByUserName(userName)
	if err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeServerBusy))
		return
	}

	res.Success()
	res.Sessions = userSessions
	c.JSON(http.StatusOK, res)
}

func CreateSessionAndSendMessage(c *gin.Context) {
	req := new(CreateSessionAndSendMessageRequest)
	res := new(CreateSessionAndSendMessageResponse)
	userName := c.GetString("userName")
	userID := c.GetInt64("userID")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}
	if !validateModelType(req.ModelType) {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	ctx, cancel := buildChatTimeoutContext(c, syncChatTimeout)
	defer cancel()

	sessionID, aiInformation, code_ := session.CreateSessionAndSendMessageWithControl(ctx, userName, userID, req.UserQuestion, req.ModelType)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	res.AiInformation = aiInformation
	res.SessionID = sessionID
	c.JSON(http.StatusOK, res)
}

func CreateStreamSessionAndSendMessage(c *gin.Context) {
	req := new(CreateSessionAndSendMessageRequest)
	userName := c.GetString("userName")
	userID := c.GetInt64("userID")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, gin.H{"error": "Invalid parameters"})
		return
	}
	if !validateModelType(req.ModelType) {
		c.JSON(http.StatusOK, gin.H{"error": "Invalid parameters"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Cache-Control", "no-cache, no-transform")

	ctx, cancel := buildChatTimeoutContext(c, streamChatTimeout)
	defer cancel()

	sessionID, code_ := session.CreateStreamSessionOnly(userName, userID, req.UserQuestion)
	if code_ != code.CodeSuccess {
		writeSSEError(c, code_)
		return
	}

	writeSSEJSON(c, map[string]interface{}{
		"type":      "session",
		"sessionId": sessionID,
	})
	writeSSEJSON(c, map[string]interface{}{
		"type": "ready",
	})

	code_ = session.ChatStreamSendWithControl(ctx, userName, sessionID, req.UserQuestion, req.ModelType, http.ResponseWriter(c.Writer))
	if code_ != code.CodeSuccess {
		writeSSEError(c, code_)
		return
	}
}

func ChatSend(c *gin.Context) {
	req := new(ChatSendRequest)
	res := new(ChatSendResponse)
	userName := c.GetString("userName")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}
	if !validateModelType(req.ModelType) {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	ctx, cancel := buildChatTimeoutContext(c, syncChatTimeout)
	defer cancel()

	aiInformation, code_ := session.ChatSendWithControl(ctx, userName, req.SessionID, req.UserQuestion, req.ModelType)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	res.AiInformation = aiInformation
	c.JSON(http.StatusOK, res)
}

func ChatStreamSend(c *gin.Context) {
	req := new(ChatSendRequest)
	userName := c.GetString("userName")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, gin.H{"error": "Invalid parameters"})
		return
	}
	if !validateModelType(req.ModelType) {
		c.JSON(http.StatusOK, gin.H{"error": "Invalid parameters"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Cache-Control", "no-cache, no-transform")

	ctx, cancel := buildChatTimeoutContext(c, streamChatTimeout)
	defer cancel()

	writeSSEJSON(c, map[string]interface{}{
		"type": "ready",
	})

	code_ := session.ChatStreamSendWithControl(ctx, userName, req.SessionID, req.UserQuestion, req.ModelType, http.ResponseWriter(c.Writer))
	if code_ != code.CodeSuccess {
		writeSSEError(c, code_)
		return
	}
}

func StopStream(c *gin.Context) {
	req := new(StopStreamRequest)
	res := new(StopStreamResponse)
	userName := c.GetString("userName")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	partialContent, code_ := session.StopStreamGeneration(userName, req.SessionID)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	res.PartialContent = partialContent
	c.JSON(http.StatusOK, res)
}

func ChatHistory(c *gin.Context) {
	req := new(ChatHistoryRequest)
	res := new(ChatHistoryResponse)
	userName := c.GetString("userName")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}
	history, code_ := session.GetChatHistory(userName, req.SessionID)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	res.History = history
	c.JSON(http.StatusOK, res)
}

func GetAIObservability(c *gin.Context) {
	res := new(AIObservabilityResponse)
	res.Success()
	res.Data = observability.SnapshotAI()
	c.JSON(http.StatusOK, res)
}
