package session

import (
	"GopherAI/common/mysql"
	"GopherAI/model"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const sessionRepairRetryBackoff = 10 * time.Second

func GetSessionsByUserName(userName string) ([]model.Session, error) {
	var sessions []model.Session
	err := mysql.DB.Where("user_name = ?", userName).Order("updated_at desc").Find(&sessions).Error
	return sessions, err
}

func GetUngroupedSessionsByUserName(userName string) ([]model.Session, error) {
	var sessions []model.Session
	err := mysql.DB.Where("user_name = ? AND folder_id IS NULL", userName).Order("updated_at desc").Find(&sessions).Error
	return sessions, err
}

func GetSessionsByFolderID(userName string, folderID string) ([]model.Session, error) {
	var sessions []model.Session
	err := mysql.DB.Where("user_name = ? AND folder_id = ?", userName, folderID).Order("updated_at desc").Find(&sessions).Error
	return sessions, err
}

func CreateSession(session *model.Session) (*model.Session, error) {
	err := mysql.DB.Create(session).Error
	return session, err
}

func GetSessionByID(sessionID string) (*model.Session, error) {
	var session model.Session
	err := mysql.DB.Where("id = ?", sessionID).First(&session).Error
	return &session, err
}

func UpdateSessionSummary(sessionID string, summary string, summaryMessageCount int) error {
	return mysql.DB.Model(&model.Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"context_summary":       summary,
			"summary_message_count": summaryMessageCount,
		}).Error
}

func UpdateSessionChatSelection(sessionID string, llmConfigID *int64, chatMode string) error {
	return mysql.DB.Model(&model.Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"llm_config_id": llmConfigID,
			"chat_mode":     chatMode,
		}).Error
}

// UpdateSessionProgress 把摘要状态和正式 version 一起推进，避免两次更新之间出现中间态。
func UpdateSessionProgress(sessionID string, version int64, summary string, summaryMessageCount int) error {
	return mysql.DB.Model(&model.Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"context_summary":       summary,
			"summary_message_count": summaryMessageCount,
			"version":               version,
		}).Error
}

// UpdateSessionProgressAndPersistedVersion 在同步持久化路径里同时推进正式 version 与 persisted_version。
// 第四阶段开始，当 user/assistant 核心消息已经在 MySQL 内完成正式落库后，
// 这里会把 session 的逻辑进度和持久化水位一次性收敛，避免中间态暴露太久。
func UpdateSessionProgressAndPersistedVersion(sessionID string, version int64, summary string, summaryMessageCount int, persistedVersion int64) error {
	return mysql.DB.Model(&model.Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"context_summary":       summary,
			"summary_message_count": summaryMessageCount,
			"version":               version,
			"persisted_version":     persistedVersion,
		}).Error
}

func UpdateSessionTitle(userName string, sessionID string, title string) error {
	result := mysql.DB.Model(&model.Session{}).
		Where("id = ? AND user_name = ?", sessionID, userName).
		Update("title", title)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func UpdateSessionFolder(userName string, sessionID string, folderID *string) error {
	result := mysql.DB.Model(&model.Session{}).
		Where("id = ? AND user_name = ?", sessionID, userName).
		Update("folder_id", folderID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func ClearFolderIDByFolderID(userName string, folderID string) error {
	return mysql.DB.Model(&model.Session{}).
		Where("user_name = ? AND folder_id = ?", userName, folderID).
		Update("folder_id", nil).Error
}

func SoftDeleteSession(userName string, sessionID string) error {
	result := mysql.DB.Where("id = ? AND user_name = ?", sessionID, userName).Delete(&model.Session{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func buildSessionRepairTaskKey(sessionID string, taskType model.SessionRepairTaskType, targetVersion int64) string {
	return fmt.Sprintf("%s|%s|%d", sessionID, taskType, targetVersion)
}

// SaveHotStateRebuildTask 以幂等方式登记一条 Redis 热状态重建任务。
func SaveHotStateRebuildTask(sessionID string, selectionSignature string, targetVersion int64) error {
	if sessionID == "" || targetVersion <= 0 {
		return nil
	}

	task := &model.SessionRepairTask{
		TaskKey:            buildSessionRepairTaskKey(sessionID, model.SessionRepairTaskTypeHotStateRebuild, targetVersion),
		SessionID:          sessionID,
		TaskType:           model.SessionRepairTaskTypeHotStateRebuild,
		SelectionSignature: selectionSignature,
		TargetVersion:      targetVersion,
		Status:             model.SessionRepairTaskStatusPending,
		NextAttemptAt:      time.Now(),
	}

	return mysql.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "task_key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"selection_signature": selectionSignature,
			"target_version":      targetVersion,
			"status":              model.SessionRepairTaskStatusPending,
			"last_error":          "",
			"next_attempt_at":     time.Now(),
			"updated_at":          time.Now(),
		}),
	}).Create(task).Error
}

// ListPendingSessionRepairTasks 列出当前待执行的 repair task。
func ListPendingSessionRepairTasks(limit int) ([]model.SessionRepairTask, error) {
	var tasks []model.SessionRepairTask
	query := mysql.DB.
		Where("status = ? AND next_attempt_at <= ?", model.SessionRepairTaskStatusPending, time.Now()).
		Order("next_attempt_at asc").
		Order("id asc")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&tasks).Error
	return tasks, err
}

// MarkSessionRepairTaskCompleted 把 repair task 标记为完成。
func MarkSessionRepairTaskCompleted(taskID uint) error {
	if taskID == 0 {
		return nil
	}
	now := time.Now()
	return mysql.DB.Model(&model.SessionRepairTask{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":          model.SessionRepairTaskStatusCompleted,
			"last_error":      "",
			"completed_at":    now,
			"next_attempt_at": now,
		}).Error
}

// MarkSessionRepairTaskFailed 记录一次 repair 执行失败，并安排下一次重试时间。
func MarkSessionRepairTaskFailed(taskID uint, errText string) error {
	if taskID == 0 {
		return nil
	}
	now := time.Now()
	return mysql.DB.Model(&model.SessionRepairTask{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"retry_count":     clause.Expr{SQL: "retry_count + 1"},
			"last_error":      errText,
			"next_attempt_at": now.Add(sessionRepairRetryBackoff),
			"updated_at":      now,
		}).Error
}

// DeleteSessionRepairTasksBySessionID 在会话被删除时联动清理其 repair task。
func DeleteSessionRepairTasksBySessionID(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	return mysql.DB.Where("session_id = ?", sessionID).Delete(&model.SessionRepairTask{}).Error
}
