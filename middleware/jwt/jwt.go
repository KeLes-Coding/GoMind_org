package jwt

import (
	"GopherAI/common/code"
	"GopherAI/controller"
	userDAO "GopherAI/dao/user"
	"GopherAI/utils/myjwt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		res := new(controller.Response)

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
			c.Abort()
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if token == "" {
			c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
			c.Abort()
			return
		}

		claims, ok := myjwt.ParseAccessToken(token)
		if !ok {
			c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
			c.Abort()
			return
		}

		userInformation, err := userDAO.GetByID(claims.ID)
		if err != nil || userInformation == nil {
			c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
			c.Abort()
			return
		}
		if userInformation.TokenVersion != claims.TokenVersion || userInformation.Username != claims.Username {
			c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
			c.Abort()
			return
		}

		c.Set("userName", userInformation.Username)
		c.Set("userID", userInformation.ID)
		c.Set("tokenVersion", userInformation.TokenVersion)
		c.Next()
	}
}
