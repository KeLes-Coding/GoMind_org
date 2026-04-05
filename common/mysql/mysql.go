package mysql

import (
	"GopherAI/config"
	"GopherAI/model"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitMysql() error {
	host := config.GetConfig().MysqlHost
	port := config.GetConfig().MysqlPort
	dbname := config.GetConfig().MysqlDatabaseName
	username := config.GetConfig().MysqlUser
	password := config.GetConfig().MysqlPassword
	charset := config.GetConfig().MysqlCharset

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=true&loc=Local", username, password, host, port, dbname, charset)

	var log logger.Interface
	if gin.Mode() == "debug" {
		log = logger.Default.LogMode(logger.Info)
	} else {
		log = logger.Default
	}

	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       dsn,
		DefaultStringSize:         256,
		DisableDatetimePrecision:  true,
		DontSupportRenameIndex:    true,
		DontSupportRenameColumn:   true,
		SkipInitializeWithVersion: false,
	}), &gorm.Config{
		Logger: log,
	})
	if err != nil {
		return err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	DB = db

	return migration()
}

func migration() error {
	if err := alignSessionSchema(); err != nil {
		return err
	}

	return DB.AutoMigrate(
		new(model.User),
		new(model.EmailCaptcha),
		new(model.SessionFolder),
		new(model.UserLLMConfig),
		new(model.Session),
		new(model.Message),
		new(model.MessageOutbox),
		new(model.FileAsset),
	)
}

func alignSessionSchema() error {
	if err := ensureVarchar36Column(config.GetConfig().MysqlDatabaseName, "session_folders", "id", false); err != nil {
		return err
	}
	if err := ensureVarchar36Column(config.GetConfig().MysqlDatabaseName, "sessions", "folder_id", true); err != nil {
		return err
	}
	return nil
}

func ensureVarchar36Column(schemaName string, tableName string, columnName string, nullable bool) error {
	if DB == nil {
		return errors.New("mysql DB is not initialized")
	}

	var dataType string
	err := DB.Raw(`
		SELECT DATA_TYPE
		FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ? AND column_name = ?
		LIMIT 1
	`, schemaName, tableName, columnName).Scan(&dataType).Error
	if err != nil {
		return err
	}
	if dataType == "" {
		return nil
	}

	normalized := strings.ToLower(dataType)
	if normalized == "varchar" || normalized == "char" {
		return nil
	}

	nullSQL := "NOT NULL"
	if nullable {
		nullSQL = "NULL"
	}

	alterSQL := fmt.Sprintf(
		"ALTER TABLE `%s` MODIFY COLUMN `%s` varchar(36) %s",
		tableName,
		columnName,
		nullSQL,
	)
	return DB.Exec(alterSQL).Error
}

func InsertUser(user *model.User) (*model.User, error) {
	err := DB.Create(&user).Error
	return user, err
}

func GetUserByID(id int64) (*model.User, error) {
	user := new(model.User)
	err := DB.Where("id = ?", id).First(user).Error
	return user, err
}

func GetUserByUsername(username string) (*model.User, error) {
	user := new(model.User)
	err := DB.Where("username = ?", username).First(user).Error
	return user, err
}

func GetUserByEmail(email string) (*model.User, error) {
	user := new(model.User)
	err := DB.Where("email = ?", email).First(user).Error
	return user, err
}

func UpdateUserPasswordByID(id int64, password string) error {
	return DB.Model(&model.User{}).Where("id = ?", id).Update("password", password).Error
}

func IncrementUserTokenVersion(id int64) error {
	return DB.Model(&model.User{}).Where("id = ?", id).Update("token_version", gorm.Expr("token_version + 1")).Error
}

// UpdateUserProfileByID 按用户 ID 更新可编辑资料字段。
// 这里显式限制可更新字段，避免把认证相关字段暴露给资料接口误改。
func UpdateUserProfileByID(id int64, updates map[string]interface{}) error {
	return DB.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}
