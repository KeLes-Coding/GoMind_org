package session

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	myredis "GopherAI/common/redis"
	"GopherAI/model"
	"context"
)

// mapContextErrorToCode 统一把上下文取消原因翻译成业务错误码。
// 这里单独抽出来，是为了避免 stop / timeout / 普通模型失败三种语义在各个调用点被混写成 AIModelFail。
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

// persistInterruptedAssistantMessage 用于在流式中断时，把已经生成出来的部分内容按“中断态消息”落库。
// 这一步不是完整的断点续传实现，但至少可以保证：
// 1. 用户已经看到的部分输出不至于完全丢失；
// 2. 历史接口能明确告诉前端，这是一条 cancelled / timeout / partial 消息。
func persistInterruptedAssistantMessage(helper *aihelper.AIHelper, userName string, content string, status model.MessageStatus) {
	if helper == nil || content == "" {
		return
	}
	helper.AddMessageWithStatus(content, userName, false, true, status)
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
