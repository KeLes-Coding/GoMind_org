package session

import (
	"GopherAI/common/mysql"
	"GopherAI/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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

// ListSessionsWithPersistenceLag 列出“正式版本已推进，但持久化水位尚未追平”的会话。
func ListSessionsWithPersistenceLag(limit int) ([]model.Session, error) {
	var sessions []model.Session
	query := mysql.DB.
		Where("version > persisted_version").
		Order("updated_at asc")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&sessions).Error
	return sessions, err
}

// TryAdvancePersistedVersionIfReady 在消息已经可靠落库后尝试推进 persisted_version。
// 当前规则要求同一个 session_version 下至少已经持久化了一条用户消息和一条 assistant 消息，
// 避免只写入半轮对话时就把水位错误推进。
func TryAdvancePersistedVersionIfReady(sessionID string, targetVersion int64) (bool, error) {
	if sessionID == "" || targetVersion <= 0 {
		return false, nil
	}

	var advanced bool
	err := mysql.DB.Transaction(func(tx *gorm.DB) error {
		var session model.Session
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", sessionID).First(&session).Error; err != nil {
			return err
		}
		if session.PersistedVersion >= targetVersion || session.Version < targetVersion {
			return nil
		}

		var userCount int64
		if err := tx.Model(&model.Message{}).
			Where("session_id = ? AND session_version = ? AND is_user = ?", sessionID, targetVersion, true).
			Count(&userCount).Error; err != nil {
			return err
		}
		if userCount == 0 {
			return nil
		}

		var assistantCount int64
		if err := tx.Model(&model.Message{}).
			Where("session_id = ? AND session_version = ? AND is_user = ?", sessionID, targetVersion, false).
			Count(&assistantCount).Error; err != nil {
			return err
		}
		if assistantCount == 0 {
			return nil
		}

		result := tx.Model(&model.Session{}).
			Where("id = ? AND persisted_version < ?", sessionID, targetVersion).
			Update("persisted_version", targetVersion)
		if result.Error != nil {
			return result.Error
		}
		advanced = result.RowsAffected > 0
		return nil
	})
	return advanced, err
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
