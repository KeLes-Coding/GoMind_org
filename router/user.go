package router

import (
	"GopherAI/controller/user"
	"GopherAI/middleware/ratelimit"

	"github.com/gin-gonic/gin"
)

func RegisterUserRouter(r *gin.RouterGroup) {
	{
		r.POST("/register", user.Register)
		r.POST("/login", ratelimit.LimitLoginByIP(), user.Login)
		r.POST("/captcha", ratelimit.LimitCaptchaByIP(), user.HandleCaptcha)
	}
}
