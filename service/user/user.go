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

	// 登录链路按用户名查用户，保持当前外部接口不变。
	if ok, userInformation = user.IsExistUser(username); !ok {
		return "", code.CodeUserNotExist
	}
	// 当前仍沿用 MD5，对外行为不变，后续再单独升级为 bcrypt。
	if userInformation.Password != utils.MD5(password) {
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

	// P0 修复：注册必须按邮箱查重，不能再把 email 当 username 去查。
	if ok, err := user.ExistsByEmail(email); err != nil {
		return "", code.CodeServerBusy
	} else if ok {
		return "", code.CodeUserExist
	}

	// 区分业务错误和系统错误：Redis 故障返回服务忙，验证码不匹配才返回验证码错误。
	if ok, err := myredis.CheckCaptchaForEmail(email, captcha); err != nil {
		return "", code.CodeServerBusy
	} else if !ok {
		return "", code.CodeInvalidCaptcha
	}

	// 当前用户名仍由随机数字生成，这一轮先不扩展外部行为。
	username := utils.GetRandomNumbers(11)

	var err error
	if userInformation, err = user.Register(username, email, password); err != nil {
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
