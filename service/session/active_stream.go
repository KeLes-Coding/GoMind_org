package session

import (
	"GopherAI/common/code"
	myredis "GopherAI/common/redis"
	"GopherAI/model"
	"context"
	"sync"
	"time"
)

const (
	// activeStreamResumeWindow 控制被动断开后的短续传窗口。
	// 当前实现里，它只表示“优先按实时流恢复”的建议窗口，不再用于窗口到期后自动取消生成。
	// 这样即使用户因为网络波动或关闭页面失去连接，后台仍会尽量把完整回复跑完并落库。
	activeStreamResumeWindow = 20 * time.Second
	// activeStreamBufferMaxChunks 控制本地环形缓冲区最多保留多少个 chunk。
	activeStreamBufferMaxChunks = 512
	// activeStreamBufferMaxBytes 控制本地缓冲区最多保留多少字符，避免长回答无限占内存。
	activeStreamBufferMaxBytes = 256 * 1024
)

type activeStreamEventType string

const (
	activeStreamEventChunk activeStreamEventType = "chunk"
	activeStreamEventDone  activeStreamEventType = "done"
)

// activeStreamEvent 是 active stream 广播给当前订阅连接的运行时事件。
// 这里不直接传 writer，而是先广播到订阅 channel，便于连接断开后重新 attach。
type activeStreamEvent struct {
	Type   activeStreamEventType
	Chunk  *model.StreamChunkSnapshot
	Status model.StreamRuntimeStatus
}

// activeStreamTask 表示一个仍在运行或短期可恢复的流式任务。
// 它同时维护：
// 1. 流式恢复所需的 seq / snapshot / ring buffer；
// 2. 当前实例上在线订阅者；
// 3. 被动断开后的 detached 窗口与取消逻辑。
type activeStreamTask struct {
	userName   string
	sessionID  string
	streamID   string
	messageID  string
	cancel     context.CancelFunc

	mu             sync.RWMutex
	status         model.StreamRuntimeStatus
	messageStatus  model.MessageStatus
	cancelStatus   model.MessageStatus
	content        string
	nextSeq        int64
	sessionVersion int64
	ownerID        string
	fenceToken     int64
	chunks         []model.StreamChunkSnapshot
	bufferBytes    int
	subscribers    map[string]chan activeStreamEvent
	resumeDeadline *time.Time
}

func newActiveStreamTask(userName string, sessionID string, streamID string, messageID string, cancel context.CancelFunc) *activeStreamTask {
	return &activeStreamTask{
		userName:    userName,
		sessionID:   sessionID,
		streamID:    streamID,
		messageID:   messageID,
		cancel:      cancel,
		status:      model.StreamStatusStreaming,
		messageStatus: model.MessageStatusStreaming,
		subscribers: make(map[string]chan activeStreamEvent),
	}
}

// exportMeta 导出当前流式恢复元数据，供 Redis 共享层使用。
func (t *activeStreamTask) exportMeta() *model.StreamResumeMeta {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return &model.StreamResumeMeta{
		StreamID:         t.streamID,
		SessionID:        t.sessionID,
		MessageID:        t.messageID,
		UserName:         t.userName,
		Status:           t.status,
		NextSeq:          t.nextSeq,
		UpdatedAt:        time.Now(),
		ResumeDeadlineAt: cloneTimePtr(t.resumeDeadline),
		OwnerID:          t.ownerID,
		FenceToken:       t.fenceToken,
	}
}

func (t *activeStreamTask) exportSnapshot() *model.StreamSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return &model.StreamSnapshot{
		StreamID:   t.streamID,
		SessionID:  t.sessionID,
		MessageID:  t.messageID,
		Content:    t.content,
		LastSeq:    t.nextSeq - 1,
		UpdatedAt:  time.Now(),
		StatusHint: string(t.status),
	}
}

func (t *activeStreamTask) snapshot() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.content
}

func (t *activeStreamTask) setOwnerGuard(ownerID string, fenceToken int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ownerID = ownerID
	t.fenceToken = fenceToken
}

func (t *activeStreamTask) setSessionVersion(version int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessionVersion = version
}

func (t *activeStreamTask) requestStop(status model.MessageStatus) {
	t.mu.Lock()
	t.cancelStatus = status
	t.mu.Unlock()
	t.cancel()
}

func (t *activeStreamTask) interruptedMessageStatus(ctx context.Context) model.MessageStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.cancelStatus != "" {
		return t.cancelStatus
	}
	switch ctx.Err() {
	case context.DeadlineExceeded:
		return model.MessageStatusTimeout
	case context.Canceled:
		return model.MessageStatusCancelled
	default:
		return model.MessageStatusPartial
	}
}

func (t *activeStreamTask) interruptedRuntimeStatus(ctx context.Context) model.StreamRuntimeStatus {
	switch t.interruptedMessageStatus(ctx) {
	case model.MessageStatusCancelled:
		return model.StreamStatusCancelled
	case model.MessageStatusTimeout:
		return model.StreamStatusTimeout
	case model.MessageStatusPartial:
		return model.StreamStatusPartial
	case model.MessageStatusFailed:
		return model.StreamStatusFailed
	default:
		return model.StreamStatusFailed
	}
}

func (t *activeStreamTask) buildAssistantMessage(status model.MessageStatus) *model.Message {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return &model.Message{
		MessageKey:     t.messageID,
		SessionID:      t.sessionID,
		SessionVersion: t.sessionVersion,
		UserName:       t.userName,
		Content:        t.content,
		IsUser:         false,
		Status:         status,
	}
}

// appendChunk 既更新本地环形缓冲区，也同步更新 Redis 恢复层。
// 这样同实例内可以直接走内存恢复，跨实例时仍能从 Redis 兜底。
func (t *activeStreamTask) appendChunk(chunk string) model.StreamChunkSnapshot {
	now := time.Now()

	t.mu.Lock()
	t.nextSeq++
	item := model.StreamChunkSnapshot{
		StreamID: t.streamID,
		Seq:      t.nextSeq - 1,
		Delta:    chunk,
		TsUnixMs: now.UnixMilli(),
	}
	t.content += chunk
	t.bufferBytes += len(chunk)
	t.chunks = append(t.chunks, item)
	for len(t.chunks) > activeStreamBufferMaxChunks || t.bufferBytes > activeStreamBufferMaxBytes {
		if len(t.chunks) == 0 {
			break
		}
		t.bufferBytes -= len(t.chunks[0].Delta)
		t.chunks = t.chunks[1:]
	}
	subscribers := make([]chan activeStreamEvent, 0, len(t.subscribers))
	for _, ch := range t.subscribers {
		subscribers = append(subscribers, ch)
	}
	t.mu.Unlock()

	// Redis 写入失败不应打断主链路，因此这里按 best effort 处理。
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_ = myredis.AppendActiveStreamChunk(ctx, t.streamID, &item, activeStreamBufferMaxChunks)
	_ = myredis.SaveActiveStreamSnapshot(ctx, t.exportSnapshot())
	_ = myredis.SaveActiveStreamMeta(ctx, t.exportMeta())
	cancel()

	event := activeStreamEvent{
		Type:  activeStreamEventChunk,
		Chunk: &item,
	}
	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
	}
	return item
}

func (t *activeStreamTask) attachSubscriber(lastSeq int64) (string, *model.StreamSnapshot, []model.StreamChunkSnapshot, model.StreamRuntimeStatus, <-chan activeStreamEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	subscriberID := time.Now().Format("20060102150405.000000000")
	ch := make(chan activeStreamEvent, 64)
	t.subscribers[subscriberID] = ch

	// 一旦有新的订阅者接入，就说明当前流重新回到 streaming 状态。
	if t.status == model.StreamStatusDetached {
		t.status = model.StreamStatusStreaming
		t.resumeDeadline = nil
	}

	backlog := make([]model.StreamChunkSnapshot, 0, len(t.chunks))
	for _, item := range t.chunks {
		if item.Seq > lastSeq {
			backlog = append(backlog, item)
		}
	}

	var snapshot *model.StreamSnapshot
	if len(backlog) > 0 && backlog[0].Seq > lastSeq+1 {
		snapshot = &model.StreamSnapshot{
			StreamID:   t.streamID,
			SessionID:  t.sessionID,
			MessageID:  t.messageID,
			Content:    t.content,
			LastSeq:    t.nextSeq - 1,
			UpdatedAt:  time.Now(),
			StatusHint: string(t.status),
		}
	}
	if len(backlog) == 0 && lastSeq < t.nextSeq-1 {
		snapshot = &model.StreamSnapshot{
			StreamID:   t.streamID,
			SessionID:  t.sessionID,
			MessageID:  t.messageID,
			Content:    t.content,
			LastSeq:    t.nextSeq - 1,
			UpdatedAt:  time.Now(),
			StatusHint: string(t.status),
		}
	}

	return subscriberID, snapshot, backlog, t.status, ch
}

func (t *activeStreamTask) removeSubscriber(subscriberID string) {
	var shouldDetach bool

	t.mu.Lock()
	ch, exists := t.subscribers[subscriberID]
	if exists {
		delete(t.subscribers, subscriberID)
		close(ch)
	}
	shouldDetach = len(t.subscribers) == 0 && !isTerminalStreamStatus(t.status)
	if shouldDetach {
		deadline := time.Now().Add(activeStreamResumeWindow)
		t.status = model.StreamStatusDetached
		t.resumeDeadline = &deadline
	}
	t.mu.Unlock()

	if shouldDetach {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = myredis.SaveActiveStreamMeta(ctx, t.exportMeta())
		cancel()
	}
}

func (t *activeStreamTask) finish(status model.StreamRuntimeStatus) {
	t.mu.Lock()
	t.status = status
	t.messageStatus = runtimeStatusToMessageStatus(status)
	t.resumeDeadline = nil
	subscribers := make([]chan activeStreamEvent, 0, len(t.subscribers))
	for _, ch := range t.subscribers {
		subscribers = append(subscribers, ch)
	}
	t.subscribers = make(map[string]chan activeStreamEvent)
	t.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_ = myredis.SaveActiveStreamMeta(ctx, t.exportMeta())
	_ = myredis.SaveActiveStreamSnapshot(ctx, t.exportSnapshot())
	_ = myredis.DeleteSessionActiveStream(ctx, t.sessionID)
	_ = myredis.DeleteActiveStreamStopSignal(ctx, t.streamID)
	cancel()

	event := activeStreamEvent{
		Type:   activeStreamEventDone,
		Status: status,
	}
	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
		close(ch)
	}
}

func isTerminalStreamStatus(status model.StreamRuntimeStatus) bool {
	switch status {
	case model.StreamStatusCompleted, model.StreamStatusCancelled, model.StreamStatusTimeout, model.StreamStatusFailed, model.StreamStatusPartial:
		return true
	default:
		return false
	}
}

func runtimeStatusToMessageStatus(status model.StreamRuntimeStatus) model.MessageStatus {
	switch status {
	case model.StreamStatusCompleted:
		return model.MessageStatusCompleted
	case model.StreamStatusCancelled:
		return model.MessageStatusCancelled
	case model.StreamStatusTimeout:
		return model.MessageStatusTimeout
	case model.StreamStatusFailed:
		return model.MessageStatusFailed
	case model.StreamStatusPartial:
		return model.MessageStatusPartial
	default:
		return model.MessageStatusStreaming
	}
}

func cloneTimePtr(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	copyValue := *v
	return &copyValue
}

// activeStreamRegistry 维护当前进程内所有活跃或短期可恢复的流式任务。
// 它一方面为 stop 和同实例 resume 提供极速路径，另一方面把跨实例恢复留给 Redis 兜底。
type activeStreamRegistry struct {
	mu           sync.RWMutex
	bySessionID  map[string]*activeStreamTask
	byStreamID   map[string]*activeStreamTask
}

func newActiveStreamRegistry() *activeStreamRegistry {
	return &activeStreamRegistry{
		bySessionID: make(map[string]*activeStreamTask),
		byStreamID:  make(map[string]*activeStreamTask),
	}
}

func (r *activeStreamRegistry) register(task *activeStreamTask) {
	if task == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bySessionID[task.sessionID] = task
	r.byStreamID[task.streamID] = task
}

func (r *activeStreamRegistry) unregister(task *activeStreamTask) {
	if task == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.bySessionID, task.sessionID)
	delete(r.byStreamID, task.streamID)
}

func (r *activeStreamRegistry) getBySessionID(sessionID string) *activeStreamTask {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.bySessionID[sessionID]
}

func (r *activeStreamRegistry) getByStreamID(streamID string) *activeStreamTask {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byStreamID[streamID]
}

func (r *activeStreamRegistry) stop(userName string, sessionID string) (string, code.Code) {
	task := r.getBySessionID(sessionID)
	if task == nil {
		return "", code.CodeChatNotRunning
	}
	if task.userName != userName {
		return "", code.CodeForbidden
	}

	partialContent := task.snapshot()
	task.requestStop(model.MessageStatusCancelled)
	return partialContent, code.CodeSuccess
}

var globalActiveStreamRegistry = newActiveStreamRegistry()

// resolveInterruptedMessageStatus 把上下文中断原因翻译成消息状态。
// 这里把“用户取消”和“请求超时”分开，后续历史查询和面试表达才有区分度。
func resolveInterruptedMessageStatus(ctx context.Context) model.MessageStatus {
	switch ctx.Err() {
	case context.DeadlineExceeded:
		return model.MessageStatusTimeout
	case context.Canceled:
		return model.MessageStatusCancelled
	default:
		return model.MessageStatusPartial
	}
}
