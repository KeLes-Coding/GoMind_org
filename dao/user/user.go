package user

import (
	"errors"
	"strings"

	"GopherAI/common/mysql"
	"GopherAI/model"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

const (
	CodeMsg     = "GopherAI verification code: "
	UserNameMsg = "Your GopherAI username is: "
)

var ErrDuplicateUsername = errors.New("duplicate username")

func GetByID(id int64) (*model.User, error) {
	user, err := mysql.GetUserByID(id)
	if err == gorm.ErrRecordNotFound || user == nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

func GetByUsername(username string) (*model.User, error) {
	user, err := mysql.GetUserByUsername(username)
	if err == gorm.ErrRecordNotFound || user == nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

func GetByEmail(email string) (*model.User, error) {
	user, err := mysql.GetUserByEmail(email)
	if err == gorm.ErrRecordNotFound || user == nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

func CreateUser(username, email, passwordHash string) (*model.User, error) {
	user, err := mysql.InsertUser(&model.User{
		Email:        email,
		Name:         username,
		Username:     username,
		Password:     passwordHash,
		TokenVersion: 1,
	})
	if err != nil {
		if isDuplicateUsernameError(err) {
			return nil, ErrDuplicateUsername
		}
		return nil, err
	}
	return user, nil
}

func UpdatePasswordHash(userID int64, passwordHash string) error {
	return mysql.UpdateUserPasswordByID(userID, passwordHash)
}

func IncrementTokenVersion(userID int64) error {
	return mysql.IncrementUserTokenVersion(userID)
}

// UpdateProfile 只更新用户资料相关字段，不触碰密码和登录态版本。
func UpdateProfile(userID int64, updates map[string]interface{}) error {
	return mysql.UpdateUserProfileByID(userID, updates)
}

func isDuplicateUsernameError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062 && strings.Contains(strings.ToLower(mysqlErr.Message), "username")
	}
	return false
}
