package user

import (
	"context"
	"log"
	"mime/multipart"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"GopherAI/common/code"
	myemail "GopherAI/common/email"
	myredis "GopherAI/common/redis"
	"GopherAI/common/storage"
	captchaDAO "GopherAI/dao/captcha"
	userDAO "GopherAI/dao/user"
	"GopherAI/model"
	"GopherAI/utils"
	"GopherAI/utils/myjwt"
)

const (
	captchaExpireDuration = 2 * time.Minute
	maxAvatarSize         = 2 << 20
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{3,19}$`)

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	Username     string
}

type UserProfile struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Bio       string `json:"bio"`
}

func Login(identifier, password string) (*TokenPair, code.Code) {
	userInformation, err := userDAO.GetByUsername(identifier)
	if err != nil {
		return nil, code.CodeServerBusy
	}
	if userInformation == nil {
		userInformation, err = userDAO.GetByEmail(identifier)
		if err != nil {
			return nil, code.CodeServerBusy
		}
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
	return &TokenPair{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		Username:     userInformation.Username,
	}, code.CodeSuccess
}

func Register(username, email, password, captcha string) (*TokenPair, code.Code) {
	if !isValidUsername(username) {
		return nil, code.CodeInvalidUsername
	}

	existingUser, err := userDAO.GetByEmail(email)
	if err != nil {
		return nil, code.CodeServerBusy
	}
	if existingUser != nil {
		return nil, code.CodeUserExist
	}

	existingUser, err = userDAO.GetByUsername(username)
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

	userInformation, err := userDAO.CreateUser(username, email, passwordHash)
	if err != nil {
		if err == userDAO.ErrDuplicateUsername {
			return nil, code.CodeUserExist
		}
		return nil, code.CodeServerBusy
	}

	if mailErr := myemail.SendCaptcha(email, username, userDAO.UserNameMsg); mailErr != nil {
		log.Printf("[register] user created but failed to send username email, email=%s username=%s err=%v", email, username, mailErr)
	}

	if err = consumeCaptcha(email); err != nil {
		log.Printf("[register] user created but failed to consume captcha, email=%s err=%v", email, err)
		return nil, code.CodeServerBusy
	}

	pair, err := myjwt.GenerateTokenPair(userInformation.ID, userInformation.Username, userInformation.TokenVersion)
	if err != nil {
		return nil, code.CodeServerBusy
	}

	return &TokenPair{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		Username:     userInformation.Username,
	}, code.CodeSuccess
}

func isValidUsername(username string) bool {
	return usernamePattern.MatchString(username)
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
	return &TokenPair{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		Username:     userInformation.Username,
	}, code.CodeSuccess
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

// GetProfile 读取当前用户资料，并转换成对外返回结构。
// GetProfile 读取当前用户资料，并转换成对外返回结构。
func GetProfile(userID int64) (*UserProfile, code.Code) {
	if userID <= 0 {
		return nil, code.CodeInvalidToken
	}

	userInformation, err := userDAO.GetByID(userID)
	if err != nil {
		return nil, code.CodeServerBusy
	}
	if userInformation == nil {
		return nil, code.CodeUserNotExist
	}

	return buildUserProfile(userInformation), code.CodeSuccess
}

// UpdateProfile 只允许更新展示层资料字段，避免资料接口越权修改认证信息。
// UpdateProfile 只允许更新展示层资料字段，避免资料接口越权修改认证信息。
func UpdateProfile(userID int64, name string, bio string) (*UserProfile, code.Code) {
	if userID <= 0 {
		return nil, code.CodeInvalidToken
	}

	name = strings.TrimSpace(name)
	bio = strings.TrimSpace(bio)
	if len(name) > 50 || len(bio) > 255 {
		return nil, code.CodeInvalidParams
	}

	updates := map[string]interface{}{
		"name": name,
		"bio":  bio,
	}
	if err := userDAO.UpdateProfile(userID, updates); err != nil {
		return nil, code.CodeServerBusy
	}

	return GetProfile(userID)
}

// UploadAvatar 负责校验头像文件并写入统一存储，再回写用户头像 key。
// UploadAvatar 负责校验头像文件并写入统一存储，再回写用户头像 key。
func UploadAvatar(userID int64, file *multipart.FileHeader) (*UserProfile, code.Code) {
	if userID <= 0 {
		return nil, code.CodeInvalidToken
	}
	if file == nil {
		return nil, code.CodeInvalidParams
	}
	if file.Size <= 0 || file.Size > maxAvatarSize {
		return nil, code.CodeInvalidParams
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
	default:
		return nil, code.CodeInvalidParams
	}

	src, err := file.Open()
	if err != nil {
		return nil, code.CodeServerBusy
	}
	defer src.Close()

	fileStorage, err := storage.GetStorage()
	if err != nil {
		return nil, code.CodeServerBusy
	}

	avatarKey := buildAvatarStorageKey(userID, ext)
	if err := fileStorage.Upload(context.Background(), avatarKey, src); err != nil {
		return nil, code.CodeServerBusy
	}

	if err := userDAO.UpdateProfile(userID, map[string]interface{}{"avatar_url": avatarKey}); err != nil {
		return nil, code.CodeServerBusy
	}

	return GetProfile(userID)
}

// GetAvatarStorageKeyByUserID 返回用户头像在存储层中的 key，供头像读取接口使用。
func GetAvatarStorageKeyByUserID(userID int64) (string, code.Code) {
	if userID <= 0 {
		return "", code.CodeInvalidParams
	}

	userInformation, err := userDAO.GetByID(userID)
	if err != nil {
		return "", code.CodeServerBusy
	}
	if userInformation == nil {
		return "", code.CodeUserNotExist
	}
	if strings.TrimSpace(userInformation.AvatarURL) == "" {
		return "", code.CodeRecordNotFound
	}

	return userInformation.AvatarURL, code.CodeSuccess
}

func buildUserProfile(userInformation *model.User) *UserProfile {
	if userInformation == nil {
		return nil
	}
	return &UserProfile{
		ID:        userInformation.ID,
		Name:      userInformation.Name,
		Email:     userInformation.Email,
		Username:  userInformation.Username,
		AvatarURL: buildAvatarAccessURL(userInformation),
		Bio:       userInformation.Bio,
	}
}

// buildAvatarAccessURL 把内部存储 key 转成前端可直接访问的头像地址。
func buildAvatarAccessURL(userInformation *model.User) string {
	if userInformation == nil || strings.TrimSpace(userInformation.AvatarURL) == "" {
		return ""
	}
	return "/api/user/avatar/" + strconv.FormatInt(userInformation.ID, 10)
}

// buildAvatarStorageKey 使用稳定目录加时间戳生成头像 key，避免文件名冲突。
func buildAvatarStorageKey(userID int64, ext string) string {
	return "avatars/" + strconv.FormatInt(userID, 10) + "/" + strconv.FormatInt(time.Now().UnixNano(), 10) + ext
}
