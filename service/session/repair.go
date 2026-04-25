package session

import (
	"GopherAI/common/aihelper"
	myredis "GopherAI/common/redis"
	messageDAO "GopherAI/dao/message"
	sessionDAO "GopherAI/dao/session"
	"GopherAI/model"
	"context"
	"log"
	"sort"
	"time"
)

const repairHotStateMessageWindow = 20

// savePendingPersistHotStateBestEffort 在 MySQL 正式收敛失败后，把 repair 标记回写到 Redis 热状态。
// 这里刻意做 best-effort：
// 1. 主请求已经要返回失败，不能再额外阻塞；
// 2. 只要 Redis 还在，就尽量把“待修复”语义留下来；
// 3. 后续 repair worker 会基于这个标记继续补偿。
func savePendingPersistHotStateBestEffort(ctx context.Context, helper *aihelper.AIHelper) {
	saveRepairHotStateBestEffort(ctx, helper, true, false)
}

// saveRepairHotStateBestEffort 把 repair 语义附着到 Redis 热状态上。
func saveRepairHotStateBestEffort(ctx context.Context, helper *aihelper.AIHelper, pendingPersist bool, hotStateDirty bool) {
	if helper == nil {
		return
	}

	hotState := helper.ExportHotState()
	if pendingPersist {
		hotState.PendingPersist = true
	}
	if hotStateDirty {
		hotState.HotStateDirty = true
	}

	guard := sessionOwnerGuardFromContext(ctx)
	if guard != nil && guard.SessionID == helper.SessionID {
		hotState.OwnerID = guard.OwnerID
		hotState.FenceToken = guard.FenceToken
	}

	if _, err := myredis.SaveSessionHotState(ctx, hotState); err != nil {
		log.Println("saveRepairHotStateBestEffort SaveSessionHotState error:", err)
	}
}

// enqueueHotStateRebuildRepairBestEffort 在“数据库已成功，但 Redis 热状态提交失败”时登记一条 repair task。
func enqueueHotStateRebuildRepairBestEffort(helper *aihelper.AIHelper) {
	if helper == nil {
		return
	}

	hotState := helper.ExportHotState()
	targetVersion := hotState.Version
	if hotState.PersistedVersion > targetVersion {
		targetVersion = hotState.PersistedVersion
	}
	for _, msg := range hotState.RecentMessages {
		if msg.SessionVersion > targetVersion {
			targetVersion = msg.SessionVersion
		}
	}
	if err := sessionDAO.SaveHotStateRebuildTask(hotState.SessionID, hotState.SelectionSignature, targetVersion); err != nil {
		log.Println("enqueueHotStateRebuildRepairBestEffort SaveHotStateRebuildTask error:", err)
	}
}

// RepairPendingSessionPersistenceFromHotState 尝试用 Redis 热状态回放最近一轮尚未正式写入 MySQL 的消息。
// 这条 repair 只处理“Redis 已成功推进、MySQL 在终态阶段失败”的场景；
// 对于只有 user message 的半轮失败请求，这里不会强行把不完整轮次落库。
func RepairPendingSessionPersistenceFromHotState(ctx context.Context, hotState *model.SessionHotState) (bool, error) {
	if hotState == nil || !hotState.PendingPersist {
		return false, nil
	}

	sess, err := sessionDAO.GetSessionByID(hotState.SessionID)
	if err != nil {
		return false, err
	}

	versionSet := make(map[int64]struct{})
	for _, msg := range hotState.RecentMessages {
		if msg.SessionVersion <= sess.PersistedVersion {
			continue
		}
		versionSet[msg.SessionVersion] = struct{}{}
	}
	if len(versionSet) == 0 {
		return false, nil
	}

	versions := make([]int64, 0, len(versionSet))
	for version := range versionSet {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] < versions[j]
	})

	repairedVersion := sess.PersistedVersion
	for _, version := range versions {
		var repairMessages []model.SessionHotMessage
		hasUser := false
		hasAssistant := false

		for _, msg := range hotState.RecentMessages {
			if msg.SessionVersion != version {
				continue
			}
			repairMessages = append(repairMessages, msg)
			if msg.IsUser {
				hasUser = true
			} else {
				hasAssistant = true
			}
		}

		// 只修复完整的一轮对话，避免把“请求已失败、只有 user message 的半轮状态”误落到正式历史里。
		if !hasUser || !hasAssistant {
			continue
		}

		sort.SliceStable(repairMessages, func(i, j int) bool {
			if repairMessages[i].IsUser != repairMessages[j].IsUser {
				return repairMessages[i].IsUser
			}
			if !repairMessages[i].CreatedAt.Equal(repairMessages[j].CreatedAt) {
				return repairMessages[i].CreatedAt.Before(repairMessages[j].CreatedAt)
			}
			return repairMessages[i].MessageKey < repairMessages[j].MessageKey
		})

		for _, hotMessage := range repairMessages {
			_, err := messageDAO.CreateMessage(&model.Message{
				MessageKey:     hotMessage.MessageKey,
				SessionID:      hotMessage.SessionID,
				SessionVersion: hotMessage.SessionVersion,
				UserName:       hotMessage.UserName,
				Content:        hotMessage.Content,
				IsUser:         hotMessage.IsUser,
				Status:         model.MessageStatus(hotMessage.Status),
			})
			if err != nil {
				return false, err
			}
		}
		repairedVersion = version
	}

	if repairedVersion <= sess.PersistedVersion {
		return false, nil
	}

	nextSessionVersion := repairedVersion
	if sess.Version > nextSessionVersion {
		nextSessionVersion = sess.Version
	}
	if err := sessionDAO.UpdateSessionProgressAndPersistedVersion(
		sess.ID,
		nextSessionVersion,
		hotState.ContextSummary,
		hotState.SummaryMessageCount,
		repairedVersion,
	); err != nil {
		return false, err
	}

	hotState.PendingPersist = false
	hotState.HotStateDirty = false
	if hotState.Version < nextSessionVersion {
		hotState.Version = nextSessionVersion
	}
	hotState.PersistedVersion = repairedVersion
	hotState.UpdatedAt = time.Now()
	if _, err := myredis.SaveSessionHotState(ctx, hotState); err != nil {
		return false, err
	}

	return true, nil
}

// RebuildSessionHotStateFromDatabase 用 MySQL 正式状态重建一份 Redis 热状态。
func RebuildSessionHotStateFromDatabase(ctx context.Context, sessionID string, selectionSignature string) error {
	sess, err := sessionDAO.GetSessionByID(sessionID)
	if err != nil {
		return err
	}

	messages, err := messageDAO.GetMessagesBySessionID(sessionID)
	if err != nil {
		return err
	}

	hotState := buildSessionHotStateFromDatabase(sess, messages, selectionSignature)
	_, err = myredis.SaveSessionHotState(ctx, hotState)
	return err
}

func buildSessionHotStateFromDatabase(sess *model.Session, messages []model.Message, selectionSignature string) *model.SessionHotState {
	start := 0
	if len(messages) > repairHotStateMessageWindow {
		start = len(messages) - repairHotStateMessageWindow
	}

	recentMessages := make([]model.SessionHotMessage, 0, len(messages[start:]))
	for i := range messages[start:] {
		msg := messages[start:][i]
		recentMessages = append(recentMessages, model.SessionHotMessage{
			ID:             msg.ID,
			MessageKey:     msg.MessageKey,
			SessionID:      msg.SessionID,
			SessionVersion: msg.SessionVersion,
			UserName:       msg.UserName,
			Content:        msg.Content,
			IsUser:         msg.IsUser,
			Status:         string(msg.Status),
			CreatedAt:      msg.CreatedAt,
		})
	}

	return &model.SessionHotState{
		SessionID:           sess.ID,
		SelectionSignature:  selectionSignature,
		Version:             sess.Version,
		PersistedVersion:    sess.PersistedVersion,
		UpdatedAt:           time.Now(),
		ContextSummary:      sess.ContextSummary,
		SummaryMessageCount: sess.SummaryMessageCount,
		RecentMessagesStart: start,
		RecentMessages:      recentMessages,
	}
}
