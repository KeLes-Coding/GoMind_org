<<<<<<< HEAD
package sessionfolder
=======
package session_folder
>>>>>>> 8b8125bb7c712b316afa9e1ad7389df2e321a22f

import (
	"GopherAI/common/mysql"
	"GopherAI/model"
<<<<<<< HEAD
	"strings"

	"gorm.io/gorm"
)

func CreateFolder(folder *model.SessionFolder) (*model.SessionFolder, error) {
=======
)

func CreateSessionFolder(folder *model.SessionFolder) (*model.SessionFolder, error) {
>>>>>>> 8b8125bb7c712b316afa9e1ad7389df2e321a22f
	err := mysql.DB.Create(folder).Error
	return folder, err
}

<<<<<<< HEAD
func GetFoldersByUserName(userName string) ([]model.SessionFolder, error) {
	var folders []model.SessionFolder
	err := mysql.DB.Where("user_name = ?", userName).Order("created_at asc").Find(&folders).Error
	return folders, err
}

func GetFolderByID(folderID string) (*model.SessionFolder, error) {
=======
func GetSessionFoldersByUserID(userID int64) ([]model.SessionFolder, error) {
	var folders []model.SessionFolder
	err := mysql.DB.Where("user_id = ?", userID).Order("created_at asc").Find(&folders).Error
	return folders, err
}

func GetSessionFolderByID(folderID int64) (*model.SessionFolder, error) {
>>>>>>> 8b8125bb7c712b316afa9e1ad7389df2e321a22f
	var folder model.SessionFolder
	err := mysql.DB.Where("id = ?", folderID).First(&folder).Error
	return &folder, err
}

<<<<<<< HEAD
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
=======
func UpdateSessionFolderName(folderID int64, name string) error {
	return mysql.DB.Model(&model.SessionFolder{}).
		Where("id = ?", folderID).
		Update("name", name).Error
}

func DeleteSessionFolder(folderID int64) error {
	return mysql.DB.Delete(&model.SessionFolder{}, "id = ?", folderID).Error
>>>>>>> 8b8125bb7c712b316afa9e1ad7389df2e321a22f
}
