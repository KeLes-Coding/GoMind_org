package user

import (
	"log"
	"time"

	"GopherAI/common/code"
	myemail "GopherAI/common/email"
	myredis "GopherAI/common/redis"
	captchaDAO "GopherAI/dao/captcha"
	"GopherAI/dao/user"
	"GopherAI/model"
	"GopherAI/utils"
	"GopherAI/utils/myjwt"
)

const (
	maxUsernameRetry      = 5
	captchaExpireDuration = 2 * time.Minute
)

func Login(username, password string) (string, code.Code) {
	userInformation, err := user.GetByUsername(username)
	if err != nil {
		return "", code.CodeServerBusy
	}
	if userInformation == nil {
		return "", code.CodeUserNotExist
	}

	passwordMatched := utils.VerifyPassword(userInformation.Password, password)
	if !passwordMatched && userInformation.Password == utils.MD5(password) {
		passwordMatched = true
		if passwordHash, err := utils.HashPassword(password); err == nil {
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

	// Redis 可用时优先走 Redis；Redis 故障时回退到 MySQL 保底验证码记录。
	ok, err := verifyCaptcha(email, captcha)
	if err != nil {
		return "", code.CodeServerBusy
	}
	if !ok {
		return "", code.CodeInvalidCaptcha
	}

	passwordHash, err := utils.HashPassword(password)
	if err != nil {
		return "", code.CodeServerBusy
	}

	var userInformation *model.User
	for i := 0; i < maxUsernameRetry; i++ {
		username := utils.GetRandomNumbers(11)
		userInformation, err = user.CreateUser(username, email, passwordHash)
		if err == nil {
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

	// 注册完成后再消费验证码，避免验证码被重复使用。
	if err := consumeCaptcha(email); err != nil {
		log.Printf("[register] user created but failed to consume captcha, email=%s err=%v", email, err)
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

	codeHash, err := utils.HashPassword(sendCode)
	if err != nil {
		return code.CodeServerBusy
	}

	expiresAt := time.Now().Add(captchaExpireDuration)
	if err := captchaDAO.SaveCaptcha(email, codeHash, expiresAt); err != nil {
		return code.CodeServerBusy
	}

	// MySQL 是保底真相源；Redis 这里只做高频读取加速，失败不阻断发码。
	if err := myredis.SetCaptchaForEmail(email, sendCode); err != nil {
		log.Printf("[captcha] redis unavailable, fallback to mysql only, email=%s err=%v", email, err)
	}

	if err := myemail.SendCaptcha(email, sendCode, myemail.CodeMsg); err != nil {
		return code.CodeServerBusy
	}

	return code.CodeSuccess
}

func verifyCaptcha(email, input string) (bool, error) {
	// 读路径先尝试 Redis，命中时不打数据库；只有 Redis 不可用时才降级。
	if ok, err := myredis.ValidateCaptchaForEmail(email, input); err == nil {
		return ok, nil
	}

	record, err := captchaDAO.GetActiveCaptchaByEmail(email, time.Now())
	if err != nil {
		if captchaDAO.IsCaptchaNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return utils.VerifyPassword(record.CodeHash, input), nil
}

func consumeCaptcha(email string) error {
	// Redis 删除失败不影响主流程，MySQL 的 used 标记才是最终约束。
	if err := myredis.DeleteCaptchaForEmail(email); err != nil {
		log.Printf("[captcha] delete redis captcha skipped, email=%s err=%v", email, err)
	}

	record, err := captchaDAO.GetActiveCaptchaByEmail(email, time.Now())
	if err != nil {
		if captchaDAO.IsCaptchaNotFound(err) {
			return nil
		}
		return err
	}

	return captchaDAO.MarkCaptchaUsed(record.ID, time.Now())
}
