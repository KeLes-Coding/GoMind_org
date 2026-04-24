package session

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	"GopherAI/common/observability"
	"GopherAI/model"
	"context"
	"fmt"
	"log"
)

// getReusableExecutionHelper 尝试复用当前进程里已有的 helper。
// 注意这里的语义只是“复用执行对象”，不再代表跨请求恢复来源。
func getReusableExecutionHelper(userName string, sessionID string, selectionSignature string) (*aihelper.AIHelper, bool) {
	manager := aihelper.GetGlobalManager()
	helper, exists := manager.GetAIHelper(userName, sessionID)
	if !exists {
		return nil, false
	}
	if !helper.MatchesSelection(selectionSignature) {
		manager.RemoveAIHelper(userName, sessionID)
		return nil, false
	}
	observability.RecordHelperExecutionReuse()
	return helper, true
}

// newEphemeralHelper 创建一个新的短生命周期执行 helper。
// 它不直接从 manager 获取，避免把“本地缓存命中”误当成恢复来源。
func newEphemeralHelper(ctx context.Context, sessionID string, resolved *resolvedChatRequest) (*aihelper.AIHelper, error) {
	if resolved == nil {
		return nil, fmt.Errorf("resolved chat request is nil")
	}

	factory := aihelper.GetGlobalFactory()
	return factory.CreateAIHelper(ctx, resolved.ModelType, sessionID, resolved.RuntimeConfig)
}

// buildExecutionHelper 先尝试复用本地执行对象；如果没有可复用对象，则创建新的 helper。
// 这一步只负责“执行对象的准备”，不负责决定恢复真相源来自 Redis 还是 DB。
func buildExecutionHelper(ctx context.Context, userName string, sessionID string, resolved *resolvedChatRequest) (*aihelper.AIHelper, bool, error) {
	selectionSignature := resolved.RuntimeConfig.SelectionSignature(resolved.ModelType)
	if helper, reused := getReusableExecutionHelper(userName, sessionID, selectionSignature); reused {
		return helper, true, nil
	}

	helper, err := newEphemeralHelper(ctx, sessionID, resolved)
	if err != nil {
		return nil, false, err
	}
	return helper, false, nil
}

// BuildEphemeralHelperFromHotState 基于 Redis 热状态构造当前请求要使用的执行 helper。
// 这里即使复用了进程内对象，也只是为了少一次对象分配；真正的恢复语义仍然来自 Redis 热状态。
func BuildEphemeralHelperFromHotState(ctx context.Context, userName string, sess *model.Session, resolved *resolvedChatRequest, hotState *model.SessionHotState) (*aihelper.AIHelper, bool, error) {
	if sess == nil {
		return nil, false, fmt.Errorf("session is nil")
	}
	if hotState == nil {
		return nil, false, fmt.Errorf("session hot state is nil")
	}

	helper, reused, err := buildExecutionHelper(ctx, userName, sess.ID, resolved)
	if err != nil {
		return nil, false, err
	}

	helper.LoadHotState(hotState)
	applySessionMetadataToHelper(sess, helper)
	return helper, reused, nil
}

// BuildEphemeralHelperFromDB 基于 MySQL 全量消息重建当前请求要使用的执行 helper。
// 当 Redis 热状态缺失或不可信时，这里就是唯一兜底路径。
func BuildEphemeralHelperFromDB(ctx context.Context, userName string, sess *model.Session, resolved *resolvedChatRequest) (*aihelper.AIHelper, bool, codePathResult) {
	if sess == nil {
		return nil, false, codePathResult{ok: false, code: code.CodeInvalidParams}
	}

	helper, reused, err := buildExecutionHelper(ctx, userName, sess.ID, resolved)
	if err != nil {
		log.Println("BuildEphemeralHelperFromDB buildExecutionHelper error:", err)
		return nil, false, codePathResult{ok: false, code: code.AIModelFail}
	}

	code_ := fullReconcileHelperWithDatabase(sess, helper)
	if code_ != code.CodeSuccess {
		return nil, reused, codePathResult{ok: false, code: code_}
	}
	return helper, reused, codePathResult{ok: true}
}

type codePathResult struct {
	ok   bool
	code code.Code
}
