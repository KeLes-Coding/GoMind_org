package router

import (
	"GopherAI/controller/user"
	jwtmiddleware "GopherAI/middleware/jwt"
	"GopherAI/middleware/ratelimit"

	"github.com/gin-gonic/gin"
)

func RegisterUserRouter(r *gin.RouterGroup) {
	r.POST("/register", user.Register)
	r.POST("/login", ratelimit.LimitLoginByIP(), user.Login)
	r.POST("/captcha", ratelimit.LimitCaptchaByIP(), user.HandleCaptcha)
	r.POST("/refresh", user.Refresh)
	r.GET("/avatar/:userID", user.GetAvatar)
	r.POST("/logout", jwtmiddleware.Auth(), user.Logout)
	r.GET("/profile", jwtmiddleware.Auth(), user.GetProfile)
	r.POST("/profile/update", jwtmiddleware.Auth(), user.UpdateProfile)
	r.POST("/avatar/upload", jwtmiddleware.Auth(), user.UploadAvatar)
}
