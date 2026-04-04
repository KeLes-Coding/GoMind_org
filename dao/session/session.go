package session

import (
	"GopherAI/common/mysql"
	"GopherAI/model"

	"gorm.io/gorm"
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
