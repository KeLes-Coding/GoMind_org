package session

import (
	"GopherAI/common/mysql"
	"GopherAI/model"
)

// GetSessionsByUserName 按用户名读取该用户的全部会话。
// 这里直接以数据库为真相来源，避免会话列表依赖进程内缓存。
func GetSessionsByUserName(userName string) ([]model.Session, error) {
	var sessions []model.Session
	err := mysql.DB.Where("user_name = ?", userName).Order("created_at desc").Find(&sessions).Error
	return sessions, err
}

// CreateSession 创建一条新的会话记录。
func CreateSession(session *model.Session) (*model.Session, error) {
	err := mysql.DB.Create(session).Error
	return session, err
}

// GetSessionByID 按主键读取会话，用于校验会话是否存在以及归属权。
func GetSessionByID(sessionID string) (*model.Session, error) {
	var session model.Session
	err := mysql.DB.Where("id = ?", sessionID).First(&session).Error
	return &session, err
}
