package session

import (
	"GopherAI/common/mysql"
	"GopherAI/model"

	"gorm.io/gorm"
)

<<<<<<< HEAD
=======
// GetSessionsByUserName loads all sessions for a user ordered by creation time.
>>>>>>> 8b8125bb7c712b316afa9e1ad7389df2e321a22f
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

<<<<<<< HEAD
=======
// CreateSession persists a new session record.
>>>>>>> 8b8125bb7c712b316afa9e1ad7389df2e321a22f
func CreateSession(session *model.Session) (*model.Session, error) {
	err := mysql.DB.Create(session).Error
	return session, err
}

<<<<<<< HEAD
=======
// GetSessionByID loads a session by primary key.
>>>>>>> 8b8125bb7c712b316afa9e1ad7389df2e321a22f
func GetSessionByID(sessionID string) (*model.Session, error) {
	var session model.Session
	err := mysql.DB.Where("id = ?", sessionID).First(&session).Error
	return &session, err
}

<<<<<<< HEAD
=======
// UpdateSessionSummary persists the current summary state for a session.
>>>>>>> 8b8125bb7c712b316afa9e1ad7389df2e321a22f
func UpdateSessionSummary(sessionID string, summary string, summaryMessageCount int) error {
	return mysql.DB.Model(&model.Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"context_summary":       summary,
			"summary_message_count": summaryMessageCount,
		}).Error
}

<<<<<<< HEAD
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
=======
// UpdateSessionTitle updates a session title.
func UpdateSessionTitle(sessionID string, title string) error {
	return mysql.DB.Model(&model.Session{}).
		Where("id = ?", sessionID).
		Update("title", title).Error
}

// DeleteSession soft-deletes a session.
func DeleteSession(sessionID string) error {
	return mysql.DB.Delete(&model.Session{}, "id = ?", sessionID).Error
}

// UpdateSessionFolderID updates the folder association for a session.
func UpdateSessionFolderID(sessionID string, folderID *int64) error {
	return mysql.DB.Model(&model.Session{}).
		Where("id = ?", sessionID).
		Update("folder_id", folderID).Error
}

// ClearSessionFolderIDByFolderID clears folder references for all sessions under a folder.
func ClearSessionFolderIDByFolderID(folderID int64) error {
	return mysql.DB.Model(&model.Session{}).
		Where("folder_id = ?", folderID).
		Update("folder_id", nil).Error
}
>>>>>>> 8b8125bb7c712b316afa9e1ad7389df2e321a22f
