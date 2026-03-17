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

// GetByUsername 仅按用户名查询用户，供当前登录链路使用。
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

// GetByEmail 按邮箱查询用户，供注册查重使用。
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

// CreateUser 只负责落库；如果命中用户名唯一索引，则向上返回明确的重复用户名错误。
func CreateUser(username, email, passwordHash string) (*model.User, error) {
	user, err := mysql.InsertUser(&model.User{
		Email:    email,
		Name:     username,
		Username: username,
		Password: passwordHash,
	})
	if err != nil {
		if isDuplicateUsernameError(err) {
			return nil, ErrDuplicateUsername
		}
		return nil, err
	}
	return user, nil
}

// UpdatePasswordHash 在历史 MD5 密码用户成功登录后，回写新的 bcrypt 哈希。
func UpdatePasswordHash(userID int64, passwordHash string) error {
	return mysql.UpdateUserPasswordByID(userID, passwordHash)
}

func isDuplicateUsernameError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062 && strings.Contains(strings.ToLower(mysqlErr.Message), "username")
	}
	return false
}
