package jwt

import (
	"GopherAI/common/code"
	"GopherAI/controller"
	"GopherAI/utils/myjwt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Auth 统一从 Authorization 头中读取 Bearer Token，不再兼容 query 参数传 token。
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		res := new(controller.Response)

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
			c.Abort()
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" {
			c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
			c.Abort()
			return
		}

		// 第二轮去掉明文 token 日志，避免凭证进入日志系统。
		userName, ok := myjwt.ParseToken(token)
		if !ok {
			c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
			c.Abort()
			return
		}

		c.Set("userName", userName)
		c.Next()
	}
}
