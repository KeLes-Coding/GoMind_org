package session

import (
	"GopherAI/common/applog"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type traceContextKey string

const sessionTraceContextKey traceContextKey = "session-trace"

// sessionTrace 用于把一次会话请求的关键字段串起来。
// 这里先用最轻量的日志 Trace 方案，不引入额外链路追踪组件，也能把核心排障信息串起来。
type sessionTrace struct {
	RequestID          string
	Operation          string
	SessionID          string
	RequestedModelType string
	StartTime          time.Time
}

func newSessionTrace(ctx context.Context, operation string, sessionID string, requestedModelType string) (context.Context, *sessionTrace) {
	trace := &sessionTrace{
		RequestID:          uuid.NewString(),
		Operation:          operation,
		SessionID:          sessionID,
		RequestedModelType: requestedModelType,
		StartTime:          time.Now(),
	}

	return context.WithValue(ctx, sessionTraceContextKey, trace), trace
}

func traceFromContext(ctx context.Context) *sessionTrace {
	if ctx == nil {
		return nil
	}

	trace, _ := ctx.Value(sessionTraceContextKey).(*sessionTrace)
	return trace
}

func logSessionTrace(ctx context.Context, stage string, format string, args ...interface{}) {
	trace := traceFromContext(ctx)
	if trace == nil {
		applog.Userf("session_trace stage=%s "+format, append([]interface{}{stage}, args...)...)
		return
	}

	prefixArgs := []interface{}{
		stage,
		trace.RequestID,
		trace.Operation,
		trace.SessionID,
		trace.RequestedModelType,
		time.Since(trace.StartTime).Milliseconds(),
	}
	applog.Userf(
		"session_trace stage=%s request_id=%s operation=%s session_id=%s requested_model=%s elapsed_ms=%d "+format,
		append(prefixArgs, args...)...,
	)
}

func logResolvedSelection(ctx context.Context, resolved *resolvedChatRequest) {
	if resolved == nil {
		return
	}

	configID := "nil"
	if resolved.RuntimeConfig.LLMConfigID != nil {
		configID = fmt.Sprintf("%d", *resolved.RuntimeConfig.LLMConfigID)
	}

	logSessionTrace(
		ctx,
		"resolved_selection",
		"chat_mode=%s provider=%s config_id=%s model=%s base_url=%s",
		resolved.ChatMode,
		resolved.RuntimeConfig.Provider,
		configID,
		resolved.RuntimeConfig.ModelName,
		resolved.RuntimeConfig.BaseURL,
	)
}
