package session

import (
	"GopherAI/common/code"
	myredis "GopherAI/common/redis"
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
