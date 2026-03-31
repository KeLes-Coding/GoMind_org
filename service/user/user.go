package user

import (
	"log"
	"time"

	"GopherAI/common/code"
	myemail "GopherAI/common/email"
	myredis "GopherAI/common/redis"
	captchaDAO "GopherAI/dao/captcha"
	userDAO "GopherAI/dao/user"
	"GopherAI/model"
	"GopherAI/utils"
	"GopherAI/utils/myjwt"
)

const (
	maxUsernameRetry      = 5
	captchaExpireDuration = 2 * time.Minute
)

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

func Login(username, password string) (*TokenPair, code.Code) {
	userInformation, err := userDAO.GetByUsername(username)
	if err != nil {
		return nil, code.CodeServerBusy
	}
	if userInformation == nil {
		return nil, code.CodeUserNotExist
	}

	passwordMatched := utils.VerifyPassword(userInformation.Password, password)
	if !passwordMatched && userInformation.Password == utils.MD5(password) {
		passwordMatched = true
		if passwordHash, err := utils.HashPassword(password); err == nil {
			_ = userDAO.UpdatePasswordHash(userInformation.ID, passwordHash)
		}
	}
	if !passwordMatched {
		return nil, code.CodeInvalidPassword
	}

	pair, err := myjwt.GenerateTokenPair(userInformation.ID, userInformation.Username, userInformation.TokenVersion)
	if err != nil {
		return nil, code.CodeServerBusy
	}
	return &TokenPair{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken}, code.CodeSuccess
}

func Register(email, password, captcha string) (*TokenPair, code.Code) {
	existingUser, err := userDAO.GetByEmail(email)
	if err != nil {
		return nil, code.CodeServerBusy
	}
	if existingUser != nil {
		return nil, code.CodeUserExist
	}

	ok, err := verifyCaptcha(email, captcha)
	if err != nil {
		return nil, code.CodeServerBusy
	}
	if !ok {
		return nil, code.CodeInvalidCaptcha
	}

	passwordHash, err := utils.HashPassword(password)
	if err != nil {
		return nil, code.CodeServerBusy
	}

	var userInformation *model.User
	for i := 0; i < maxUsernameRetry; i++ {
		username := utils.GetRandomNumbers(11)
		userInformation, err = userDAO.CreateUser(username, email, passwordHash)
		if err == nil {
			if mailErr := myemail.SendCaptcha(email, username, userDAO.UserNameMsg); mailErr != nil {
				log.Printf("[register] user created but failed to send username email, email=%s username=%s err=%v", email, username, mailErr)
			}
			break
		}
		if err != userDAO.ErrDuplicateUsername {
			return nil, code.CodeServerBusy
		}
	}

	if userInformation == nil {
		return nil, code.CodeServerBusy
	}

	if err := consumeCaptcha(email); err != nil {
		log.Printf("[register] user created but failed to consume captcha, email=%s err=%v", email, err)
		return nil, code.CodeServerBusy
	}

	pair, err := myjwt.GenerateTokenPair(userInformation.ID, userInformation.Username, userInformation.TokenVersion)
	if err != nil {
		return nil, code.CodeServerBusy
	}

	return &TokenPair{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken}, code.CodeSuccess
}

func RefreshToken(refreshToken string) (*TokenPair, code.Code) {
	claims, ok := myjwt.ParseRefreshToken(refreshToken)
	if !ok {
		return nil, code.CodeInvalidToken
	}

	userInformation, err := userDAO.GetByID(claims.ID)
	if err != nil {
		return nil, code.CodeServerBusy
	}
	if userInformation == nil {
		return nil, code.CodeInvalidToken
	}
	if userInformation.Username != claims.Username || userInformation.TokenVersion != claims.TokenVersion {
		return nil, code.CodeInvalidToken
	}

	pair, err := myjwt.GenerateTokenPair(userInformation.ID, userInformation.Username, userInformation.TokenVersion)
	if err != nil {
		return nil, code.CodeServerBusy
	}
	return &TokenPair{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken}, code.CodeSuccess
}

func Logout(userID int64) code.Code {
	if userID <= 0 {
		return code.CodeInvalidToken
	}
	if err := userDAO.IncrementTokenVersion(userID); err != nil {
		return code.CodeServerBusy
	}
	return code.CodeSuccess
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

	if err := myredis.SetCaptchaForEmail(email, sendCode); err != nil {
		log.Printf("[captcha] redis unavailable, fallback to mysql only, email=%s err=%v", email, err)
	}

	if err := myemail.SendCaptcha(email, sendCode, myemail.CodeMsg); err != nil {
		return code.CodeServerBusy
	}

	return code.CodeSuccess
}

func verifyCaptcha(email, input string) (bool, error) {
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
