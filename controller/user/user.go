package user

import (
	"GopherAI/common/code"
	"GopherAI/controller"
	userService "GopherAI/service/user"
	"net/http"

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
	}

	RegisterRequest struct {
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

	pair, code_ := userService.Register(req.Email, req.Password, req.Captcha)
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

func fillTokenResponse(res *TokenResponse, pair *userService.TokenPair) {
	res.Success()
	res.Token = pair.AccessToken
	res.AccessToken = pair.AccessToken
	res.RefreshToken = pair.RefreshToken
}
