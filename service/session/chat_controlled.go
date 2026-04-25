package session

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	"GopherAI/common/observability"
	myredis "GopherAI/common/redis"
	rt "GopherAI/common/runtime"
	messageDAO "GopherAI/dao/message"
	"GopherAI/model"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const activeStreamExecutionTimeout = 3 * time.Minute

type streamAttachOptions struct {
	includeSessionEvent bool
	lastSeq             int64
}

// mapContextErrorToCode 统一把上下文取消原因翻译成业务错误码。
// 这样 stop / timeout / 普通模型失败三种语义不会在不同调用点再次混成 AIModelFail。
func mapContextErrorToCode(ctx context.Context) code.Code {
	if ctx == nil {
		return code.AIModelFail
	}

	switch ctx.Err() {
	case context.DeadlineExceeded:
		return code.CodeRequestTimeout
	case context.Canceled:
		return code.AIModelCancelled
	default:
		return code.AIModelFail
	}
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
	if !myredis.IsAvailable() {
		// Redis 降级模式下，当前实例既拿不到共享恢复层，也不应继续做 detached resume/takeover 推断。
		// 这里直接返回 server busy，让客户端保留当前已看到内容并等待 Redis 恢复或重新发起请求。
		observability.RecordStreamResumeRedisDegraded()
		log.Println("ResumeStreamWithControl redis degraded: redis-backed resume disabled")
		return code.CodeServerBusy
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

	if code_ := ensureResumeRoutingOwnership(context.Background(), meta); code_ != code.CodeSuccess {
		return code_
	}
	if claimedMeta, code_ := tryTakeoverDetachedStreamResume(context.Background(), meta); code_ != code.CodeSuccess {
		return code_
	} else if claimedMeta != nil {
		meta = claimedMeta
	}

	return attachToRedisBackedStream(ctx, writer, userName, sessionID, meta, lastSeq)
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
	observability.RecordStreamCreated()

	if err := persistActiveStreamRecoveryState(task, sess.ID); err != nil {
		globalActiveStreamRegistry.unregister(task)
		cancel()
		log.Println("startActiveStream persistActiveStreamRecoveryState error:", err)
		return nil, code.CodeServerBusy
	}

	go func() {
		defer cancel()

		requestStart := time.Now()
		traceCtx, _ := newSessionTrace(runCtx, "chat_stream", sess.ID, resolved.ModelType)
		result := withSessionExecutionGuard(traceCtx, sess.ID, func(execCtx context.Context) codeExecutorResult {
			if guard := sessionOwnerGuardFromContext(execCtx); guard != nil {
				task.setOwnerGuard(guard.OwnerID, guard.FenceToken)
				ctx, cancelPersist := context.WithTimeout(context.Background(), 2*time.Second)
				if err := myredis.SaveActiveStreamMeta(ctx, task.exportMeta()); err != nil {
					observability.RecordStreamMetaSyncFail()
					cancelPersist()
					log.Println("startActiveStream SaveActiveStreamMeta owner guard error:", err)
					return codeExecutorResult{code: code.CodeServerBusy}
				}
				cancelPersist()
			}
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
			assistantPlaceholder := task.buildAssistantMessage(model.MessageStatusStreaming)
			if _, err := messageDAO.CreateMessage(assistantPlaceholder); err != nil {
				log.Println("startActiveStream create assistant placeholder error:", err)
				return codeExecutorResult{code: code.CodeServerBusy}
			}

			go watchRemoteStopSignal(execCtx, task)

			if _, err := helper.StreamResponseWithExistingAssistantForPreparedUserMessage(userName, execCtx, func(msg string) {
				if commitErr := task.appendChunkAndCommit(msg); commitErr != nil {
					task.setCommitError(commitErr)
					task.requestStop(model.MessageStatusPartial)
				}
			}, assistantPlaceholder); err != nil {
				log.Println("startActiveStream StreamResponse error:", err)
				if commitErr := task.getCommitError(); commitErr != nil {
					log.Println("startActiveStream stream chunk commit error:", commitErr)
					return settleActiveStreamTerminalFailure(execCtx, sess, helper, task, model.MessageStatusPartial, code.CodeServerBusy, "startActiveStream persist partial assistant placeholder after commit fail")
				}
				if execCtx.Err() != nil {
					observability.RecordStreamDisconnect()
					finalStatus := task.interruptedMessageStatus(execCtx)
					return settleActiveStreamTerminalFailure(execCtx, sess, helper, task, finalStatus, mapContextErrorToCode(execCtx), "startActiveStream persist interrupted assistant placeholder")
				}
				return settleActiveStreamTerminalFailure(execCtx, sess, helper, task, model.MessageStatusPartial, code.AIModelFail, "startActiveStream persist partial assistant placeholder")
			}

			return settleActiveStreamTerminalSuccess(execCtx, sess, helper, task)
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
		globalActiveStreamRegistry.markRetained(task, activeStreamTerminalRetention)
	}()

	return task, code.CodeSuccess
}

// persistActiveStreamRecoveryState 把新建流的 session-active-stream、meta、snapshot 一次性写入恢复层。
// 这里单独抽出来，避免 startActiveStream 里继续堆三段相似的初始化提交代码。
func persistActiveStreamRecoveryState(task *activeStreamTask, sessionID string) error {
	if task == nil || sessionID == "" {
		return nil
	}

	ctx, cancelPersist := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelPersist()

	if err := myredis.SaveSessionActiveStream(ctx, sessionID, task.streamID); err != nil {
		return err
	}
	if err := myredis.SaveActiveStreamMeta(ctx, task.exportMeta()); err != nil {
		observability.RecordStreamMetaSyncFail()
		return err
	}
	if err := myredis.SaveActiveStreamSnapshot(ctx, task.exportSnapshot()); err != nil {
		observability.RecordStreamSnapshotSyncFail()
		return err
	}
	return nil
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

// settleActiveStreamTerminalFailure 在 active stream 失败、中断或 partial 场景下统一收敛 assistant 终态。
// 这里复用同一套“写 assistant 终态 -> 推进 session -> 提交热状态”的流程，避免重复分支继续膨胀。
func settleActiveStreamTerminalFailure(ctx context.Context, sess *model.Session, helper *aihelper.AIHelper, task *activeStreamTask, status model.MessageStatus, resultCode code.Code, logPrefix string) codeExecutorResult {
	if persistErr := persistActiveStreamAssistantMessage(ctx, task, status); persistErr != nil {
		log.Printf("%s error: %v", logPrefix, persistErr)
		savePendingPersistHotStateBestEffort(ctx, helper)
	}
	if code_ := finalizeAssistantMessageAndCommitHotState(ctx, helper, task.buildAssistantMessage(status)); code_ != code.CodeSuccess {
		enqueueHotStateRebuildRepairBestEffort(helper)
		return codeExecutorResult{code: code_}
	}
	if code_ := persistSessionProgressWithPersistedVersion(ctx, sess.ID, helper); code_ != code.CodeSuccess {
		savePendingPersistHotStateBestEffort(ctx, helper)
		return codeExecutorResult{code: code_}
	}
	if code_ := commitHelperHotState(ctx, helper); code_ != code.CodeSuccess {
		enqueueHotStateRebuildRepairBestEffort(helper)
		return codeExecutorResult{code: code_}
	}
	return codeExecutorResult{code: resultCode}
}

// settleActiveStreamTerminalSuccess 在 active stream 正常完成后统一收敛 assistant 正式状态。
// 成功路径里 assistant 已经写回 helper，因此这里只需要正式写库、推进 version、提交热状态并发通知。
func settleActiveStreamTerminalSuccess(ctx context.Context, sess *model.Session, helper *aihelper.AIHelper, task *activeStreamTask) codeExecutorResult {
	if persistErr := persistActiveStreamAssistantMessage(ctx, task, model.MessageStatusCompleted); persistErr != nil {
		log.Println("startActiveStream persist completed assistant placeholder error:", persistErr)
		savePendingPersistHotStateBestEffort(ctx, helper)
		return codeExecutorResult{code: code.CodeServerBusy}
	}
	if code_ := persistSessionProgressWithPersistedVersion(ctx, sess.ID, helper); code_ != code.CodeSuccess {
		savePendingPersistHotStateBestEffort(ctx, helper)
		return codeExecutorResult{code: code_}
	}
	if code_ := commitHelperHotState(ctx, helper); code_ != code.CodeSuccess {
		enqueueHotStateRebuildRepairBestEffort(helper)
		return codeExecutorResult{code: code_}
	}
	publishAssistantReadyNotificationBestEffort(ctx, sess, task.buildAssistantMessage(model.MessageStatusCompleted))
	return codeExecutorResult{code: code.CodeSuccess}
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
	if err != nil {
		observability.RecordDBPersistFail()
	}
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
	if !options.includeSessionEvent {
		observability.RecordStreamResumeLocal()
	}

	if snapshot != nil {
		observability.RecordStreamResumeSnapshotFallback()
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

func attachToRedisBackedStream(ctx context.Context, writer http.ResponseWriter, userName string, sessionID string, meta *model.StreamResumeMeta, lastSeq int64) code.Code {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		return code.CodeServerBusy
	}
	if meta == nil {
		return code.CodeChatNotRunning
	}
	streamID := meta.StreamID
	observability.RecordStreamResumeRedis()

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
				observability.RecordStreamResumeSnapshotFallback()
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

// ensureResumeRoutingOwnership 在恢复请求进入时优先检查 owner 是否就在别的活跃实例上。
// 如果原 owner 仍在线，当前实例直接返回 owner mismatch，让前端按短退避重试到更稳定的节点。
func ensureResumeRoutingOwnership(ctx context.Context, meta *model.StreamResumeMeta) code.Code {
	if meta == nil {
		return code.CodeChatNotRunning
	}
	if !myredis.IsAvailable() {
		return code.CodeSuccess
	}

	ownerID := strings.TrimSpace(meta.OwnerID)
	if ownerID == "" || ownerID == rt.CurrentInstanceID() {
		return code.CodeSuccess
	}

	active, err := myredis.IsChatInstanceActive(ctx, ownerID)
	if err != nil {
		return code.CodeSuccess
	}
	if active {
		observability.RecordStreamResumeOwnerRedirect()
		return code.CodeOwnerMismatch
	}
	return code.CodeSuccess
}

// tryTakeoverDetachedStreamResume 在 detached 流的旧 owner 已离线时，尝试让当前实例接管恢复层 owner。
// 这里接管的是“恢复协议的 owner 信息”，不是直接迁移底层模型生成任务。
// 这样至少能减少后续 resume 继续漂移到已经离线的实例。
func tryTakeoverDetachedStreamResume(ctx context.Context, meta *model.StreamResumeMeta) (*model.StreamResumeMeta, code.Code) {
	if meta == nil || meta.Status != model.StreamStatusDetached || !rt.IsOwnerEligible() {
		return nil, code.CodeSuccess
	}
	if !myredis.IsAvailable() {
		// Redis 已经降级时，不再尝试接管 detached 恢复层 owner，避免在不完整共享状态上继续漂移。
		observability.RecordStreamResumeRedisDegraded()
		return nil, code.CodeServerBusy
	}

	ownerID := strings.TrimSpace(meta.OwnerID)
	if ownerID != "" {
		active, err := myredis.IsChatInstanceActive(ctx, ownerID)
		if err != nil {
			return nil, code.CodeSuccess
		}
		if active {
			return nil, code.CodeSuccess
		}
	}

	observability.RecordStreamResumeTakeoverAttempt()
	lease, _, err := myredis.AcquireOrRefreshSessionOwnerLease(ctx, meta.SessionID, rt.CurrentInstanceID())
	if err != nil {
		return nil, code.CodeServerBusy
	}
	if lease == nil {
		return nil, code.CodeOwnerMismatch
	}

	claimed := *meta
	claimed.OwnerID = lease.OwnerID
	claimed.FenceToken = lease.FenceToken
	claimed.UpdatedAt = time.Now()
	if err := myredis.SaveActiveStreamMeta(ctx, &claimed); err != nil {
		return nil, code.CodeServerBusy
	}
	observability.RecordStreamResumeTakeoverSuccess()
	return &claimed, code.CodeSuccess
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

// StopStreamGeneration 负责给当前会话发送“主动停止”信号。
// 它只对“正在执行中的流式任务”生效；如果当前会话没有活跃流式任务，会返回 CodeChatNotRunning。
func StopStreamGeneration(userName string, sessionID string) (string, code.Code) {
	if _, code_ := ensureOwnedSession(userName, sessionID); code_ != code.CodeSuccess {
		return "", code_
	}

	if partialContent, code_ := globalActiveStreamRegistry.stop(userName, sessionID); code_ == code.CodeSuccess {
		if task := globalActiveStreamRegistry.getBySessionID(sessionID); task != nil {
			_ = myredis.SaveActiveStreamStopSignal(context.Background(), task.streamID)
		}
		return partialContent, code.CodeSuccess
	}

	streamID, err := myredis.GetSessionActiveStream(context.Background(), sessionID)
	if err != nil {
		return "", code.CodeServerBusy
	}
	if streamID == "" {
		return "", code.CodeChatNotRunning
	}

	meta, err := myredis.GetActiveStreamMeta(context.Background(), streamID)
	if err != nil {
		return "", code.CodeServerBusy
	}
	if meta == nil || meta.UserName != "" && meta.UserName != userName {
		return "", code.CodeChatNotRunning
	}

	if err := myredis.SaveActiveStreamStopSignal(context.Background(), streamID); err != nil {
		return "", code.CodeServerBusy
	}
	snapshot, snapshotErr := myredis.GetActiveStreamSnapshot(context.Background(), streamID)
	if snapshotErr != nil || snapshot == nil {
		return "", code.CodeSuccess
	}
	return snapshot.Content, code.CodeSuccess
}
