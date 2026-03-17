package user

import (
	"GopherAI/common/code"
	myemail "GopherAI/common/email"
	myredis "GopherAI/common/redis"
	"GopherAI/dao/user"
	"GopherAI/model"
	"GopherAI/utils"
	"GopherAI/utils/myjwt"
)

func Login(username, password string) (string, code.Code) {
	var userInformation *model.User
	var ok bool

	// 登录链路仍按用户名查询，保持现有外部接口不变。
	if ok, userInformation = user.IsExistUser(username); !ok {
		return "", code.CodeUserNotExist
	}

	// 第二轮改造后优先校验 bcrypt；如果命中历史 MD5，则在成功登录后自动升级。
	passwordMatched := utils.VerifyPassword(userInformation.Password, password)
	if !passwordMatched && userInformation.Password == utils.MD5(password) {
		passwordMatched = true
		if passwordHash, err := utils.HashPassword(password); err == nil {
			// 这里只做平滑升级，不影响本次登录结果。
			_ = user.UpdatePasswordHash(userInformation.ID, passwordHash)
		}
	}
	if !passwordMatched {
		return "", code.CodeInvalidPassword
	}

	token, err := myjwt.GenerateToken(userInformation.ID, userInformation.Username)
	if err != nil {
		return "", code.CodeServerBusy
	}
	return token, code.CodeSuccess
}

func Register(email, password, captcha string) (string, code.Code) {
	var userInformation *model.User

	// 注册必须按邮箱查重，不能再把 email 当成 username 去查。
	if ok, err := user.ExistsByEmail(email); err != nil {
		return "", code.CodeServerBusy
	} else if ok {
		return "", code.CodeUserExist
	}

	// 业务错误和基础设施错误分开处理，避免 Redis 故障被误报成验证码错误。
	if ok, err := myredis.CheckCaptchaForEmail(email, captcha); err != nil {
		return "", code.CodeServerBusy
	} else if !ok {
		return "", code.CodeInvalidCaptcha
	}

	username := utils.GetRandomNumbers(11)

	// 新注册用户统一写入 bcrypt 哈希，不再继续写 MD5。
	passwordHash, err := utils.HashPassword(password)
	if err != nil {
		return "", code.CodeServerBusy
	}

	if userInformation, err = user.Register(username, email, passwordHash); err != nil {
		return "", code.CodeServerBusy
	}

	if err := myemail.SendCaptcha(email, username, user.UserNameMsg); err != nil {
		return "", code.CodeServerBusy
	}

	token, err := myjwt.GenerateToken(userInformation.ID, userInformation.Username)
	if err != nil {
		return "", code.CodeServerBusy
	}

	return token, code.CodeSuccess
}

func SendCaptcha(email string) code.Code {
	sendCode := utils.GetRandomNumbers(6)
	// 先写 Redis，再发邮件，保证后续注册时有可校验的验证码。
	if err := myredis.SetCaptchaForEmail(email, sendCode); err != nil {
		return code.CodeServerBusy
	}

	if err := myemail.SendCaptcha(email, sendCode, myemail.CodeMsg); err != nil {
		return code.CodeServerBusy
	}

	return code.CodeSuccess
}
