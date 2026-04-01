package session

import (
	"GopherAI/common/mysql"
	"GopherAI/model"
)

// GetSessionsByUserName loads all sessions for a user ordered by creation time.
func GetSessionsByUserName(userName string) ([]model.Session, error) {
	var sessions []model.Session
	err := mysql.DB.Where("user_name = ?", userName).Order("created_at desc").Find(&sessions).Error
	return sessions, err
}

// CreateSession persists a new session record.
func CreateSession(session *model.Session) (*model.Session, error) {
	err := mysql.DB.Create(session).Error
	return session, err
}

// GetSessionByID loads a session by primary key.
func GetSessionByID(sessionID string) (*model.Session, error) {
	var session model.Session
	err := mysql.DB.Where("id = ?", sessionID).First(&session).Error
	return &session, err
}

// UpdateSessionSummary persists the current summary state for a session.
func UpdateSessionSummary(sessionID string, summary string, summaryMessageCount int) error {
	return mysql.DB.Model(&model.Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"context_summary":       summary,
			"summary_message_count": summaryMessageCount,
		}).Error
}

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
