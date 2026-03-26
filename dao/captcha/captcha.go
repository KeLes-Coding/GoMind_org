package captcha

import (
	"GopherAI/common/mysql"
	"GopherAI/model"
	"errors"
	"time"

	"gorm.io/gorm"
)

func SaveCaptcha(email string, codeHash string, expiresAt time.Time) error {
	return mysql.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Where("email = ?", email).Delete(&model.EmailCaptcha{}).Error; err != nil {
			return err
		}

		record := &model.EmailCaptcha{
			Email:     email,
			CodeHash:  codeHash,
			ExpiresAt: expiresAt,
			CreatedAt: now,
			UpdatedAt: now,
		}
		return tx.Create(record).Error
	})
}

func GetActiveCaptchaByEmail(email string, now time.Time) (*model.EmailCaptcha, error) {
	var record model.EmailCaptcha
	err := mysql.DB.
		Where("email = ? AND used_at IS NULL AND expires_at > ?", email, now).
		Order("id DESC").
		First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func MarkCaptchaUsed(id uint, usedAt time.Time) error {
	return mysql.DB.Model(&model.EmailCaptcha{}).
		Where("id = ? AND used_at IS NULL", id).
		Updates(map[string]interface{}{
			"used_at":    usedAt,
			"updated_at": usedAt,
		}).Error
}

func IsCaptchaNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
