package session

import (
	"GopherAI/common/code"
	"GopherAI/common/observability"
	myredis "GopherAI/common/redis"
	messageDAO "GopherAI/dao/message"
	"GopherAI/model"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const activeStreamExecutionTimeout = 3 * time.Minute

type streamAttachOptions struct {
	includeSessionEvent bool
	lastSeq             int64
}

// CreateSessionAndSendMessageWithControl 在保留原有同步链路的同时，补充了 timeout / cancelled 的错误映射。
// 原有实现内部大多把这类中断场景归并成 AIModelFail，这里在 controller 切换到新入口后，
// 可以把“主动取消”和“请求超时”单独返回给前端与面试稿。
func CreateSessionAndSendMessageWithControl(ctx context.Context, userName string, userID int64, userQuestion string, req ChatRequest) (string, string, code.Code) {
	sessionID, answer, code_ := CreateSessionAndSendMessage(ctx, userName, userID, userQuestion, req)
	if code_ == code.AIModelFail && ctx != nil && ctx.Err() != nil {
		return sessionID, answer, mapContextErrorToCode(ctx)
	}
	return sessionID, answer, code_
}

// ChatSendWithControl 和 CreateSessionAndSendMessageWithControl 一样，
// 主要目的是把同步请求里的 timeout / cancelled 语义从 AIModelFail 中拆出来。
func ChatSendWithControl(ctx context.Context, userName string, sessionID string, userQuestion string, req ChatRequest) (string, code.Code) {
	answer, code_ := ChatSend(ctx, userName, sessionID, userQuestion, req)
	if code_ == code.AIModelFail && ctx != nil && ctx.Err() != nil {
		return answer, mapContextErrorToCode(ctx)
	}
	return answer, code_
}

// CreateStreamSessionAndSendMessageWithControl 先创建会话，再启动 active stream，并把当前连接挂上去。
func CreateStreamSessionAndSendMessageWithControl(ctx context.Context, userName string, userID int64, userQuestion string, req ChatRequest, writer http.ResponseWriter) (string, code.Code) {
	sessionID, code_ := CreateStreamSessionOnly(userName, userID, userQuestion, req)
	if code_ != code.CodeSuccess {
		return "", code_
	}

	code_ = StreamMessageToExistingSessionWithControl(ctx, userName, sessionID, userQuestion, req, writer, streamAttachOptions{
		includeSessionEvent: true,
		lastSeq:             0,
	})
	if code_ != code.CodeSuccess {
		return sessionID, code_
	}
	return sessionID, code.CodeSuccess
}

// ChatStreamSendWithControl 对已有会话执行增强版流式发送。
func ChatStreamSendWithControl(ctx context.Context, userName string, sessionID string, userQuestion string, req ChatRequest, writer http.ResponseWriter) code.Code {
	return StreamMessageToExistingSessionWithControl(ctx, userName, sessionID, userQuestion, req, writer, streamAttachOptions{
		includeSessionEvent: true,
		lastSeq:             0,
	})
}

// ResumeStreamWithControl 用于在用户网络波动后重新把当前连接挂到 active stream 上。
// 如果本地存在任务，优先走进程内订阅；否则回退到 Redis 恢复层轮询。
func ResumeStreamWithControl(ctx context.Context, userName string, sessionID string, streamID string, lastSeq int64, writer http.ResponseWriter) code.Code {
	if _, code_ := ensureOwnedSession(userName, sessionID); code_ != code.CodeSuccess {
		return code_
	}

	if task := globalActiveStreamRegistry.getByStreamID(streamID); task != nil {
		return attachToActiveStream(ctx, writer, task, streamAttachOptions{
			includeSessionEvent: false,
			lastSeq:             lastSeq,
		})
	}
	return attachToRedisBackedStream(ctx, writer, userName, sessionID, streamID, lastSeq)
}

// StreamMessageToExistingSessionWithControl 是新的流式主链路。
// 核心变化包括：
// 1. 先启动独立 active stream 任务，让模型生成与 HTTP 连接解耦；
// 2. 当前连接只是订阅者，断开后 active stream 可短时间保持 detached；
// 3. active stream 同时写本地环形缓冲区和 Redis 恢复层，支撑后续 resume。
func StreamMessageToExistingSessionWithControl(ctx context.Context, userName string, sessionID string, userQuestion string, req ChatRequest, writer http.ResponseWriter, attach streamAttachOptions) code.Code {
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
	if code_ = persistResolvedChatSelection(sess, resolved); code_ != code.CodeSuccess {
		observability.RecordRequest("chat_stream", resolved.ModelType, false, time.Since(requestStart))
		return code_
	}

	task, code_ := startActiveStream(userName, sess, resolved, userQuestion)
	if code_ != code.CodeSuccess {
		observability.RecordRequest("chat_stream", resolved.ModelType, false, time.Since(requestStart))
		return code_
	}

	code_ = attachToActiveStream(ctx, writer, task, attach)
	if code_ != code.CodeSuccess {
		observability.RecordRequest("chat_stream", resolved.ModelType, false, time.Since(requestStart))
		return code_
	}
	observability.RecordRequest("chat_stream", resolved.ModelType, true, time.Since(requestStart))
	return code.CodeSuccess
}

func startActiveStream(userName string, sess *model.Session, resolved *resolvedChatRequest, userQuestion string) (*activeStreamTask, code.Code) {
	if sess == nil || resolved == nil {
		return nil, code.CodeInvalidParams
	}

	// 同一 session 同时只允许一个 active stream；恢复请走 resume 接口，而不是再发起一轮 send-stream。
	if task := globalActiveStreamRegistry.getBySessionID(sess.ID); task != nil && !isTerminalStreamStatus(task.exportMeta().Status) {
		return nil, code.CodeTooManyRequests
	}
	if streamID, err := myredis.GetSessionActiveStream(context.Background(), sess.ID); err == nil && streamID != "" {
		if meta, metaErr := myredis.GetActiveStreamMeta(context.Background(), streamID); metaErr == nil && meta != nil && !isTerminalStreamStatus(meta.Status) {
			return nil, code.CodeTooManyRequests
		}
	}

	runCtx, cancel := context.WithTimeout(context.Background(), activeStreamExecutionTimeout)
	task := newActiveStreamTask(userName, sess.ID, uuid.NewString(), uuid.NewString(), cancel)
	task.setSessionVersion(sess.Version + 1)
	globalActiveStreamRegistry.register(task)

	ctx, cancelPersist := context.WithTimeout(context.Background(), 2*time.Second)
	_ = myredis.SaveSessionActiveStream(ctx, sess.ID, task.streamID)
	_ = myredis.SaveActiveStreamMeta(ctx, task.exportMeta())
	_ = myredis.SaveActiveStreamSnapshot(ctx, task.exportSnapshot())
	cancelPersist()

	go func() {
		defer globalActiveStreamRegistry.unregister(task)
		defer cancel()

		requestStart := time.Now()
		traceCtx, _ := newSessionTrace(runCtx, "chat_stream", sess.ID, resolved.ModelType)
		result := withSessionExecutionGuard(traceCtx, sess.ID, func(execCtx context.Context) codeExecutorResult {
			if guard := sessionOwnerGuardFromContext(execCtx); guard != nil {
				task.setOwnerGuard(guard.OwnerID, guard.FenceToken)
				ctx, cancelPersist := context.WithTimeout(context.Background(), 2*time.Second)
				_ = myredis.SaveActiveStreamMeta(ctx, task.exportMeta())
				cancelPersist()
			}
			helper, code_ := getOrSyncHelperWithHistory(execCtx, userName, sess, resolved)
			if code_ != code.CodeSuccess {
				return codeExecutorResult{code: code_}
			}
			assistantPlaceholder := task.buildAssistantMessage(model.MessageStatusStreaming)
			if _, err := messageDAO.CreateMessage(assistantPlaceholder); err != nil {
				log.Println("startActiveStream create assistant placeholder error:", err)
				return codeExecutorResult{code: code.CodeServerBusy}
			}

			go watchRemoteStopSignal(execCtx, task)

			if _, err := helper.StreamResponseWithExistingAssistant(userName, execCtx, func(msg string) {
				task.appendChunk(msg)
			}, userQuestion, assistantPlaceholder); err != nil {
				log.Println("startActiveStream StreamResponse error:", err)
				if execCtx.Err() != nil {
					observability.RecordStreamDisconnect()
					finalStatus := task.interruptedMessageStatus(execCtx)
					if persistErr := persistActiveStreamAssistantMessage(execCtx, task, finalStatus); persistErr != nil {
						log.Println("startActiveStream persist interrupted assistant placeholder error:", persistErr)
					}
					helper.AppendExistingMessage(task.buildAssistantMessage(finalStatus))
					return codeExecutorResult{code: mapContextErrorToCode(execCtx)}
				}
				if persistErr := persistActiveStreamAssistantMessage(execCtx, task, model.MessageStatusPartial); persistErr != nil {
					log.Println("startActiveStream persist partial assistant placeholder error:", persistErr)
				}
				helper.AppendExistingMessage(task.buildAssistantMessage(model.MessageStatusPartial))
				return codeExecutorResult{code: code.AIModelFail}
			}

			if persistErr := persistActiveStreamAssistantMessage(execCtx, task, model.MessageStatusCompleted); persistErr != nil {
				log.Println("startActiveStream persist completed assistant placeholder error:", persistErr)
				return codeExecutorResult{code: code.CodeServerBusy}
			}

			if code_ = persistSessionProgress(execCtx, sess.ID, helper); code_ != code.CodeSuccess {
				return codeExecutorResult{code: code_}
			}
			persistHelperHotState(execCtx, helper)
			return codeExecutorResult{code: code.CodeSuccess}
		})

		switch {
		case result.err != nil:
			log.Println("startActiveStream execution guard error:", result.err)
			logSessionTrace(traceCtx, "failed", "err=%v", result.err)
			task.finish(model.StreamStatusFailed)
		case result.busy:
			logSessionTrace(traceCtx, "busy", "detail=distributed_lock_busy")
			task.finish(model.StreamStatusFailed)
		case result.code == code.CodeSuccess:
			logSessionTrace(traceCtx, "success", "detail=stream_done")
			task.finish(model.StreamStatusCompleted)
		case result.code == code.AIModelCancelled:
			task.finish(task.interruptedRuntimeStatus(runCtx))
		case result.code == code.CodeRequestTimeout:
			task.finish(model.StreamStatusTimeout)
		default:
			// 非 timeout / cancelled 的中途中断一律按 partial 收敛。
			// 这样前端至少能恢复用户已经看到的内容，而不是直接变成 failed 丢失文本。
			task.finish(model.StreamStatusPartial)
		}

		observability.RecordRequest("chat_stream_background", resolved.ModelType, result.code == code.CodeSuccess, time.Since(requestStart))
	}()

	return task, code.CodeSuccess
}

func watchRemoteStopSignal(ctx context.Context, task *activeStreamTask) {
	if task == nil {
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stopped, err := myredis.HasActiveStreamStopSignal(context.Background(), task.streamID)
			if err != nil {
				continue
			}
			if stopped {
				task.requestStop(model.MessageStatusCancelled)
				return
			}
		}
	}
}

// persistActiveStreamAssistantMessage 把 active stream 对应的 assistant 占位消息更新为最新正文和终态。
// 这里直接走 message_key upsert，确保同一条消息不会因为重试而生成重复记录。
func persistActiveStreamAssistantMessage(ctx context.Context, task *activeStreamTask, status model.MessageStatus) error {
	if task == nil {
		return nil
	}
	if code_ := ensureSessionWriteOwnership(ctx, task.sessionID); code_ != code.CodeSuccess {
		return nil
	}
	message := task.buildAssistantMessage(status)
	_, err := messageDAO.CreateMessage(message)
	return err
}

func attachToActiveStream(ctx context.Context, writer http.ResponseWriter, task *activeStreamTask, options streamAttachOptions) code.Code {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		log.Println("attachToActiveStream: streaming unsupported")
		return code.CodeServerBusy
	}

	if options.includeSessionEvent {
		if err := writeStreamJSON(writer, flusher, map[string]interface{}{
			"type":      "session",
			"sessionId": task.sessionID,
			"streamId":  task.streamID,
			"messageId": task.messageID,
		}); err != nil {
			return code.CodeServerBusy
		}
	}
	if err := writeStreamJSON(writer, flusher, map[string]interface{}{
		"type":     "ready",
		"streamId": task.streamID,
	}); err != nil {
		return code.CodeServerBusy
	}

	subscriberID, snapshot, backlog, status, ch := task.attachSubscriber(options.lastSeq)
	defer task.removeSubscriber(subscriberID)

	if snapshot != nil {
		if err := writeStreamJSON(writer, flusher, map[string]interface{}{
			"type":      "snapshot",
			"streamId":  snapshot.StreamID,
			"messageId": snapshot.MessageID,
			"content":   snapshot.Content,
			"lastSeq":   snapshot.LastSeq,
		}); err != nil {
			return code.CodeSuccess
		}
	}
	for _, chunk := range backlog {
		if err := writeStreamJSON(writer, flusher, map[string]interface{}{
			"type":     "chunk",
			"streamId": chunk.StreamID,
			"seq":      chunk.Seq,
			"delta":    chunk.Delta,
		}); err != nil {
			return code.CodeSuccess
		}
	}

	// 若 attach 时流已经收敛为终态，则只需要补齐 backlog，再发 done 即可。
	if isTerminalStreamStatus(status) {
		return writeDoneEvent(writer, flusher, task.streamID, status, task.exportSnapshot().LastSeq)
	}

	for {
		select {
		case <-ctx.Done():
			// 连接结束只解除订阅，不直接 cancel 模型生成，让 detached 窗口接管后续恢复。
			return code.CodeSuccess
		case event, ok := <-ch:
			if !ok {
				return writeDoneEvent(writer, flusher, task.streamID, task.exportMeta().Status, task.exportSnapshot().LastSeq)
			}
			switch event.Type {
			case activeStreamEventChunk:
				if event.Chunk == nil {
					continue
				}
				if err := writeStreamJSON(writer, flusher, map[string]interface{}{
					"type":     "chunk",
					"streamId": event.Chunk.StreamID,
					"seq":      event.Chunk.Seq,
					"delta":    event.Chunk.Delta,
				}); err != nil {
					return code.CodeSuccess
				}
			case activeStreamEventDone:
				return writeDoneEvent(writer, flusher, task.streamID, event.Status, task.exportSnapshot().LastSeq)
			}
		}
	}
}

func attachToRedisBackedStream(ctx context.Context, writer http.ResponseWriter, userName string, sessionID string, streamID string, lastSeq int64) code.Code {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		return code.CodeServerBusy
	}

	if err := writeStreamJSON(writer, flusher, map[string]interface{}{
		"type":     "ready",
		"streamId": streamID,
	}); err != nil {
		return code.CodeServerBusy
	}

	cursor := lastSeq
	snapshotSent := false

	for {
		select {
		case <-ctx.Done():
			return code.CodeSuccess
		default:
		}

		meta, err := myredis.GetActiveStreamMeta(context.Background(), streamID)
		if err != nil {
			return code.CodeServerBusy
		}
		if meta == nil {
			return code.CodeChatNotRunning
		}
		if meta.UserName != "" && meta.UserName != userName {
			return code.CodeForbidden
		}
		if meta.SessionID != sessionID {
			return code.CodeForbidden
		}

		chunks, chunkErr := myredis.GetActiveStreamChunks(context.Background(), streamID)
		if chunkErr != nil {
			return code.CodeServerBusy
		}

		filtered := make([]model.StreamChunkSnapshot, 0, len(chunks))
		for _, chunk := range chunks {
			if chunk.Seq > cursor {
				filtered = append(filtered, chunk)
			}
		}

		if !snapshotSent {
			needSnapshot := false
			if len(filtered) > 0 && filtered[0].Seq > cursor+1 {
				needSnapshot = true
			}
			if len(filtered) == 0 && meta.NextSeq-1 > cursor {
				needSnapshot = true
			}
			if needSnapshot {
				snapshot, snapshotErr := myredis.GetActiveStreamSnapshot(context.Background(), streamID)
				if snapshotErr != nil {
					return code.CodeServerBusy
				}
				if snapshot != nil {
					if err := writeStreamJSON(writer, flusher, map[string]interface{}{
						"type":      "snapshot",
						"streamId":  snapshot.StreamID,
						"messageId": snapshot.MessageID,
						"content":   snapshot.Content,
						"lastSeq":   snapshot.LastSeq,
					}); err != nil {
						return code.CodeSuccess
					}
					cursor = snapshot.LastSeq
					snapshotSent = true
				}
			}
		}

		for _, chunk := range filtered {
			if err := writeStreamJSON(writer, flusher, map[string]interface{}{
				"type":     "chunk",
				"streamId": chunk.StreamID,
				"seq":      chunk.Seq,
				"delta":    chunk.Delta,
			}); err != nil {
				return code.CodeSuccess
			}
			cursor = chunk.Seq
		}

		if isTerminalStreamStatus(meta.Status) {
			return writeDoneEvent(writer, flusher, streamID, meta.Status, cursor)
		}

		time.Sleep(400 * time.Millisecond)
	}
}

func writeStreamJSON(writer http.ResponseWriter, flusher http.Flusher, payload map[string]interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeDoneEvent(writer http.ResponseWriter, flusher http.Flusher, streamID string, status model.StreamRuntimeStatus, lastSeq int64) code.Code {
	if err := writeStreamJSON(writer, flusher, map[string]interface{}{
		"type":     "done",
		"streamId": streamID,
		"status":   status,
		"lastSeq":  lastSeq,
	}); err != nil {
		return code.CodeSuccess
	}
	return code.CodeSuccess
}
