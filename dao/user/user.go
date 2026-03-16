package user

import (
	"GopherAI/common/mysql"
	"GopherAI/model"
	"GopherAI/utils"

	"gorm.io/gorm"
)

const (
	CodeMsg     = "GopherAI verification code: "
	UserNameMsg = "Your GopherAI username is: "
)

// IsExistUser 用于当前“用户名 + 密码”登录链路的存在性校验。
func IsExistUser(username string) (bool, *model.User) {
	user, err := mysql.GetUserByUsername(username)
	if err == gorm.ErrRecordNotFound || user == nil {
		return false, nil
	}
	return true, user
}

// ExistsByEmail 用于注册前检查邮箱是否已被占用。
func ExistsByEmail(email string) (bool, error) {
	user, err := mysql.GetUserByEmail(email)
	if err == gorm.ErrRecordNotFound || user == nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func Register(username, email, password string) (*model.User, error) {
	// DAO 只负责落库，service 负责组织注册流程。
	user, err := mysql.InsertUser(&model.User{
		Email:    email,
		Name:     username,
		Username: username,
		Password: utils.MD5(password),
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}
