package session

import (
	"GopherAI/common/code"
	"GopherAI/model"
	"context"
	"sync"
)

// activeStreamTask 表示一个“当前仍在执行中的流式对话任务”。
// 这里单独维护一个运行时注册表，有两个目的：
// 1. 让 stop 接口可以跨 HTTP 请求拿到对应 session 的 cancelFunc；
// 2. 在流式生成过程中持续缓存已经产出的内容，便于在取消/超时场景下回写 partial 内容。
type activeStreamTask struct {
	userName  string
	sessionID string
	cancel    context.CancelFunc

	mu      sync.RWMutex
	content string
}

func (t *activeStreamTask) appendChunk(chunk string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.content += chunk
}

func (t *activeStreamTask) snapshot() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.content
}

// activeStreamRegistry 维护当前进程内所有活跃流式任务。
// 之所以只做进程内，而不直接放 Redis，是因为 cancelFunc 本身不可跨进程序列化，
// stop 能力首先需要的是“当前实例内可取消”，而不是分布式协调。
type activeStreamRegistry struct {
	mu    sync.RWMutex
	tasks map[string]*activeStreamTask
}

func newActiveStreamRegistry() *activeStreamRegistry {
	return &activeStreamRegistry{
		tasks: make(map[string]*activeStreamTask),
	}
}

func (r *activeStreamRegistry) register(userName string, sessionID string, cancel context.CancelFunc) *activeStreamTask {
	r.mu.Lock()
	defer r.mu.Unlock()

	task := &activeStreamTask{
		userName:  userName,
		sessionID: sessionID,
		cancel:    cancel,
	}
	r.tasks[sessionID] = task
	return task
}

func (r *activeStreamRegistry) unregister(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tasks, sessionID)
}

func (r *activeStreamRegistry) stop(userName string, sessionID string) (string, code.Code) {
	r.mu.RLock()
	task, exists := r.tasks[sessionID]
	r.mu.RUnlock()
	if !exists {
		return "", code.CodeChatNotRunning
	}
	if task.userName != userName {
		return "", code.CodeForbidden
	}

	// 先抓取当前已经输出的内容，再执行 cancel。
	// 这样 stop 接口可以把“已生成多少内容”一并返回给前端，方便后续做 UI 同步。
	partialContent := task.snapshot()
	task.cancel()
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
