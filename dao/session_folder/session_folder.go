package sessionfolder

import (
	"GopherAI/common/mysql"
	"GopherAI/model"
	"strings"

	"gorm.io/gorm"
)

func CreateFolder(folder *model.SessionFolder) (*model.SessionFolder, error) {
	err := mysql.DB.Create(folder).Error
	return folder, err
}

func GetFoldersByUserName(userName string) ([]model.SessionFolder, error) {
	var folders []model.SessionFolder
	err := mysql.DB.Where("user_name = ?", userName).Order("created_at asc").Find(&folders).Error
	return folders, err
}

func GetFolderByID(folderID string) (*model.SessionFolder, error) {
	var folder model.SessionFolder
	err := mysql.DB.Where("id = ?", folderID).First(&folder).Error
	return &folder, err
}

func GetFolderByUserAndName(userID int64, name string) (*model.SessionFolder, error) {
	var folder model.SessionFolder
	err := mysql.DB.Where("user_id = ? AND name = ?", userID, strings.TrimSpace(name)).First(&folder).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &folder, nil
}

func RenameFolder(userID int64, folderID string, name string) error {
	result := mysql.DB.Model(&model.SessionFolder{}).
		Where("id = ? AND user_id = ?", folderID, userID).
		Update("name", strings.TrimSpace(name))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func DeleteFolder(userID int64, folderID string) error {
	result := mysql.DB.Where("id = ? AND user_id = ?", folderID, userID).Delete(&model.SessionFolder{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
