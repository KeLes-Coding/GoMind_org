package session_folder

import (
	"GopherAI/common/mysql"
	"GopherAI/model"
)

func CreateSessionFolder(folder *model.SessionFolder) (*model.SessionFolder, error) {
	err := mysql.DB.Create(folder).Error
	return folder, err
}

func GetSessionFoldersByUserID(userID int64) ([]model.SessionFolder, error) {
	var folders []model.SessionFolder
	err := mysql.DB.Where("user_id = ?", userID).Order("created_at asc").Find(&folders).Error
	return folders, err
}

func GetSessionFolderByID(folderID int64) (*model.SessionFolder, error) {
	var folder model.SessionFolder
	err := mysql.DB.Where("id = ?", folderID).First(&folder).Error
	return &folder, err
}

func UpdateSessionFolderName(folderID int64, name string) error {
	return mysql.DB.Model(&model.SessionFolder{}).
		Where("id = ?", folderID).
		Update("name", name).Error
}

func DeleteSessionFolder(folderID int64) error {
	return mysql.DB.Delete(&model.SessionFolder{}, "id = ?", folderID).Error
}
