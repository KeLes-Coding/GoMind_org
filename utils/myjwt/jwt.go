package myjwt

import (
	"GopherAI/config"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

type Claims struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func GenerateToken(id int64, username string) (string, error) {
	claims := Claims{
		ID:       id,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(config.GetConfig().ExpireDuration) * time.Hour)),
			Issuer:    config.GetConfig().Issuer,
			Subject:   config.GetConfig().Subject,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	// 生成 token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.GetConfig().Key))
}

// ParseToken 解析Token
func ParseToken(token string) (string, bool) {
	claims := new(Claims)
	t, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(config.GetConfig().Key), nil
	})
	// 先判断 err 和 t 是否为空，再访问 t.Valid，避免解析失败时出现空指针。
	if err != nil || t == nil || !t.Valid || claims == nil {
		return "", false
	}
	return claims.Username, true
}
