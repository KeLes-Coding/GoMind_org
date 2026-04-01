package user

import (
	"GopherAI/common/code"
	"GopherAI/common/storage"
	"GopherAI/controller"
	userService "GopherAI/service/user"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
)

type (
	LoginRequest struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	TokenResponse struct {
		controller.Response
		Token        string `json:"token,omitempty"`
		AccessToken  string `json:"access_token,omitempty"`
		RefreshToken string `json:"refresh_token,omitempty"`
		Username     string `json:"username,omitempty"`
	}

	RegisterRequest struct {
		Username string `json:"username" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Captcha  string `json:"captcha" binding:"required,len=6"`
		Password string `json:"password" binding:"required,min=6"`
	}

	CaptchaRequest struct {
		Email string `json:"email" binding:"required,email"`
	}

	RefreshRequest struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	CaptchaResponse struct {
		controller.Response
	}

	UserProfileResponse struct {
		controller.Response
		Profile *userService.UserProfile `json:"profile,omitempty"`
	}

	UpdateProfileRequest struct {
		Name string `json:"name"`
		Bio  string `json:"bio"`
	}
)

func Login(c *gin.Context) {
	req := new(LoginRequest)
	res := new(TokenResponse)
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	pair, code_ := userService.Login(req.Username, req.Password)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	fillTokenResponse(res, pair)
	c.JSON(http.StatusOK, res)
}

func Register(c *gin.Context) {
	req := new(RegisterRequest)
	res := new(TokenResponse)
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	pair, code_ := userService.Register(req.Username, req.Email, req.Password, req.Captcha)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	fillTokenResponse(res, pair)
	c.JSON(http.StatusOK, res)
}

func Refresh(c *gin.Context) {
	req := new(RefreshRequest)
	res := new(TokenResponse)
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	pair, code_ := userService.RefreshToken(req.RefreshToken)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	fillTokenResponse(res, pair)
	c.JSON(http.StatusOK, res)
}

func Logout(c *gin.Context) {
	res := new(controller.Response)
	userID := c.GetInt64("userID")
	code_ := userService.Logout(userID)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}
	res.Success()
	c.JSON(http.StatusOK, res)
}

func HandleCaptcha(c *gin.Context) {
	req := new(CaptchaRequest)
	res := new(CaptchaResponse)
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	code_ := userService.SendCaptcha(req.Email)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	c.JSON(http.StatusOK, res)
}

func GetProfile(c *gin.Context) {
	res := new(UserProfileResponse)
	userID := c.GetInt64("userID")

	profile, code_ := userService.GetProfile(userID)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	res.Profile = profile
	c.JSON(http.StatusOK, res)
}

func UpdateProfile(c *gin.Context) {
	req := new(UpdateProfileRequest)
	res := new(UserProfileResponse)
	userID := c.GetInt64("userID")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	profile, code_ := userService.UpdateProfile(userID, req.Name, req.Bio)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	res.Profile = profile
	c.JSON(http.StatusOK, res)
}

func UploadAvatar(c *gin.Context) {
	res := new(UserProfileResponse)
	userID := c.GetInt64("userID")
	uploadedFile, err := c.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	profile, code_ := userService.UploadAvatar(userID, uploadedFile)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	res.Profile = profile
	c.JSON(http.StatusOK, res)
}

func GetAvatar(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("userID"), 10, 64)
	if err != nil || userID <= 0 {
		c.Status(http.StatusNotFound)
		return
	}

	avatarKey, code_ := userService.GetAvatarStorageKeyByUserID(userID)
	if code_ != code.CodeSuccess {
		c.Status(http.StatusNotFound)
		return
	}

	fileStorage, err := storage.GetStorage()
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	reader, err := fileStorage.Download(c.Request.Context(), avatarKey)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	contentType := mime.TypeByExtension(filepath.Ext(avatarKey))
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}
	c.Data(http.StatusOK, contentType, content)
}

func fillTokenResponse(res *TokenResponse, pair *userService.TokenPair) {
	res.Success()
	res.Token = pair.AccessToken
	res.AccessToken = pair.AccessToken
	res.RefreshToken = pair.RefreshToken
	res.Username = pair.Username
}
