package user

import (
	"GopherAI/common/mysql"
	"GopherAI/model"

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

// Register 只负责把已经处理好的用户信息落库，不在 DAO 层做密码加密。
func Register(username, email, passwordHash string) (*model.User, error) {
	user, err := mysql.InsertUser(&model.User{
		Email:    email,
		Name:     username,
		Username: username,
		Password: passwordHash,
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}

// UpdatePasswordHash 在历史 MD5 密码用户成功登录后，回写新的 bcrypt 哈希。
func UpdatePasswordHash(userID int64, passwordHash string) error {
	return mysql.UpdateUserPasswordByID(userID, passwordHash)
}
