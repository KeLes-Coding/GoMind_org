package session

import (
	"GopherAI/common/code"
	"GopherAI/common/observability"
	"GopherAI/model"
	"context"
	"log"
	"net/http"
	"time"
)

// CreateSessionAndSendMessageWithControl 在保留原有同步链路的同时，补充了 timeout / cancelled 的错误映射。
// 原有实现内部大多把这类中断场景归并成 AIModelFail，这里在 controller 切换到新入口后，
// 可以把“主动取消”和“请求超时”单独返回给前端与面试稿。
func CreateSessionAndSendMessageWithControl(ctx context.Context, userName string, userID int64, userQuestion string, modelType string) (string, string, code.Code) {
	sessionID, answer, code_ := CreateSessionAndSendMessage(ctx, userName, userID, userQuestion, modelType)
	if code_ == code.AIModelFail && ctx != nil && ctx.Err() != nil {
		return sessionID, answer, mapContextErrorToCode(ctx)
	}
	return sessionID, answer, code_
}

// ChatSendWithControl 和 CreateSessionAndSendMessageWithControl 一样，
// 主要目的是把同步请求里的 timeout / cancelled 语义从 AIModelFail 中拆出来。
func ChatSendWithControl(ctx context.Context, userName string, sessionID string, userQuestion string, modelType string) (string, code.Code) {
	answer, code_ := ChatSend(ctx, userName, sessionID, userQuestion, modelType)
	if code_ == code.AIModelFail && ctx != nil && ctx.Err() != nil {
		return answer, mapContextErrorToCode(ctx)
	}
	return answer, code_
}

// CreateStreamSessionAndSendMessageWithControl 先创建会话，再走增强版流式发送链路。
func CreateStreamSessionAndSendMessageWithControl(ctx context.Context, userName string, userID int64, userQuestion string, modelType string, writer http.ResponseWriter) (string, code.Code) {
	sessionID, code_ := CreateStreamSessionOnly(userName, userID, userQuestion)
	if code_ != code.CodeSuccess {
		return "", code_
	}

	code_ = StreamMessageToExistingSessionWithControl(ctx, userName, sessionID, userQuestion, modelType, writer)
	if code_ != code.CodeSuccess {
		return sessionID, code_
	}
	return sessionID, code.CodeSuccess
}

// ChatStreamSendWithControl 对已有会话执行增强版流式发送。
func ChatStreamSendWithControl(ctx context.Context, userName string, sessionID string, userQuestion string, modelType string, writer http.ResponseWriter) code.Code {
	return StreamMessageToExistingSessionWithControl(ctx, userName, sessionID, userQuestion, modelType, writer)
}

// StreamMessageToExistingSessionWithControl 是新的流式主链路。
// 相比旧实现，这里补了三件事：
// 1. 为 stop 接口注册 session -> cancelFunc；
// 2. 在流式回调里持续缓存已输出内容，便于中断时回写 partial / cancelled / timeout；
// 3. 把 timeout / cancelled 单独映射成明确业务码，而不是一律 AIModelFail。
func StreamMessageToExistingSessionWithControl(ctx context.Context, userName string, sessionID string, userQuestion string, modelType string, writer http.ResponseWriter) code.Code {
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
		log.Println("StreamMessageToExistingSessionWithControl: streaming unsupported")
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code.CodeServerBusy
	}

	sess, code_ := ensureOwnedSession(userName, sessionID)
	if code_ != code.CodeSuccess {
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code_
	}

	// 这里额外包一层 WithCancel，是为了把“主动 stop”也并入同一套上下文取消链路。
	// 这样前端点 Stop 和用户直接断开连接，底层模型都能通过 ctx.Done() 收到停止信号。
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	activeTask := globalActiveStreamRegistry.register(userName, sessionID, cancel)
	defer globalActiveStreamRegistry.unregister(sessionID)

	result := withSessionExecutionGuard(runCtx, sessionID, func() codeExecutorResult {
		helper, code_ := getOrSyncHelperWithHistory(runCtx, userName, sess, modelType)
		if code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		cb := func(msg string) {
			// 先写入运行时 buffer，再向前端输出。
			// 这样即使写给前端时发生网络错误，当前这段内容也能留在 registry 中，供后续 partial 回写使用。
			activeTask.appendChunk(msg)

			if _, err := writer.Write([]byte("data: " + msg + "\n\n")); err != nil {
				log.Println("StreamMessageToExistingSessionWithControl Write error:", err)
				// 写前端失败大多意味着连接已经不可用。
				// 这里主动 cancel，确保底层模型流及时停掉，而不是继续白白生成 token。
				cancel()
				return
			}
			flusher.Flush()
		}

		beforeSummary, beforeCount := helper.GetSummaryState()
		if _, err := helper.StreamResponse(userName, runCtx, cb, userQuestion); err != nil {
			log.Println("StreamMessageToExistingSessionWithControl StreamResponse error:", err)

			partialContent := activeTask.snapshot()
			if runCtx.Err() != nil {
				observability.RecordStreamDisconnect()
				persistInterruptedAssistantMessage(helper, userName, partialContent, resolveInterruptedMessageStatus(runCtx))
				return codeExecutorResult{code: mapContextErrorToCode(runCtx)}
			}

			// 对于非 ctx 驱动的异常，这里仍然尽量保留已经产生的部分内容，
			// 但状态只能标成 partial，因为系统无法准确判断它是网络故障还是模型中途报错。
			persistInterruptedAssistantMessage(helper, userName, partialContent, model.MessageStatusPartial)
			return codeExecutorResult{code: code.AIModelFail}
		}

		if code_ = persistSummaryIfChanged(sessionID, beforeSummary, beforeCount, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		persistHelperHotState(runCtx, helper)
		return codeExecutorResult{code: code.CodeSuccess}
	})
	if result.err != nil {
		log.Println("StreamMessageToExistingSessionWithControl execution guard error:", result.err)
		logSessionTrace(runCtx, "failed", "err=%v", result.err)
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code.CodeServerBusy
	}
	if result.busy {
		logSessionTrace(runCtx, "busy", "detail=distributed_lock_busy")
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code.CodeTooManyRequests
	}
	if result.code != code.CodeSuccess {
		logSessionTrace(runCtx, "failed", "code=%d", result.code)
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return result.code
	}

	if _, err := writer.Write([]byte("data: [DONE]\n\n")); err != nil {
		log.Println("StreamMessageToExistingSessionWithControl write DONE error:", err)
		if runCtx.Err() != nil {
			observability.RecordStreamDisconnect()
			observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
			return mapContextErrorToCode(runCtx)
		}
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code.AIModelFail
	}
	flusher.Flush()

	logSessionTrace(runCtx, "success", "detail=stream_done")
	observability.RecordRequest("chat_stream", modelType, true, time.Since(requestStart))
	return code.CodeSuccess
}
