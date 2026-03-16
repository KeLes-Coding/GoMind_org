package user

import (
	"GopherAI/common/code"
	"GopherAI/controller"
	"GopherAI/service/user"
	"net/http"

	"github.com/gin-gonic/gin"
)

type (
	// LoginRequest 对应账号密码登录的请求体。
	LoginRequest struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	// LoginResponse 在统一响应结构上补充 token 字段。
	LoginResponse struct {
		controller.Response
		Token string `json:"token,omitempty"`
	}

	// RegisterRequest 对注册入口做第一层格式校验。
	RegisterRequest struct {
		Email    string `json:"email" binding:"required,email"`
		Captcha  string `json:"captcha" binding:"required,len=6"`
		Password string `json:"password" binding:"required,min=6"`
	}

	// RegisterResponse 注册成功后直接返回登录态 token。
	RegisterResponse struct {
		controller.Response
		Token string `json:"token,omitempty"`
	}

	// CaptchaRequest 仅接收邮箱，用于发送验证码。
	CaptchaRequest struct {
		Email string `json:"email" binding:"required,email"`
	}

	CaptchaResponse struct {
		controller.Response
	}
)

func Login(c *gin.Context) {
	req := new(LoginRequest)
	res := new(LoginResponse)
	// controller 只做参数绑定和响应封装，不承载业务决策。
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	token, code_ := user.Login(req.Username, req.Password)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	res.Token = token
	c.JSON(http.StatusOK, res)
}

func Register(c *gin.Context) {
	req := new(RegisterRequest)
	res := new(RegisterResponse)
	// 先拦截明显非法的输入，减少无效请求进入 service。
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	token, code_ := user.Register(req.Email, req.Password, req.Captcha)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	res.Token = token
	c.JSON(http.StatusOK, res)
}

func HandleCaptcha(c *gin.Context) {
	req := new(CaptchaRequest)
	res := new(CaptchaResponse)
	// 验证码接口只校验邮箱格式，具体发送流程交给 service。
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	code_ := user.SendCaptcha(req.Email)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	c.JSON(http.StatusOK, res)
}
