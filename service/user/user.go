package user

import (
	"log"

	"GopherAI/common/code"
	myemail "GopherAI/common/email"
	myredis "GopherAI/common/redis"
	"GopherAI/dao/user"
	"GopherAI/model"
	"GopherAI/utils"
	"GopherAI/utils/myjwt"
)

const maxUsernameRetry = 5

func Login(username, password string) (string, code.Code) {
	userInformation, err := user.GetByUsername(username)
	if err != nil {
		return "", code.CodeServerBusy
	}
	if userInformation == nil {
		return "", code.CodeUserNotExist
	}

	// 登录优先校验 bcrypt；如果命中历史 MD5，则在成功登录后自动升级。
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
	existingUser, err := user.GetByEmail(email)
	if err != nil {
		return "", code.CodeServerBusy
	}
	if existingUser != nil {
		return "", code.CodeUserExist
	}

	// 业务错误和基础设施错误分开处理，避免 Redis 故障被误报成验证码错误。
	if ok, err := myredis.CheckCaptchaForEmail(email, captcha); err != nil {
		return "", code.CodeServerBusy
	} else if !ok {
		return "", code.CodeInvalidCaptcha
	}

	// 新注册用户统一写入 bcrypt 哈希，不再继续写 MD5。
	passwordHash, err := utils.HashPassword(password)
	if err != nil {
		return "", code.CodeServerBusy
	}

	// 随机用户名有碰撞概率，这里做有限重试，避免一次碰撞就直接注册失败。
	var userInformation *model.User
	for i := 0; i < maxUsernameRetry; i++ {
		username := utils.GetRandomNumbers(11)
		userInformation, err = user.CreateUser(username, email, passwordHash)
		if err == nil {
			// 邮件发送属于附属通知链路，失败记录日志即可，不影响注册成功。
			if mailErr := myemail.SendCaptcha(email, username, user.UserNameMsg); mailErr != nil {
				log.Printf("[register] user created but failed to send username email, email=%s username=%s err=%v", email, username, mailErr)
			}
			break
		}
		if err != user.ErrDuplicateUsername {
			return "", code.CodeServerBusy
		}
	}

	if userInformation == nil {
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
