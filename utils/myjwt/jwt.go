package myjwt

import (
	"GopherAI/config"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

type Claims struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	TokenVersion int64  `json:"token_version"`
	TokenType    string `json:"token_type"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

func GenerateTokenPair(id int64, username string, tokenVersion int64) (*TokenPair, error) {
	accessToken, err := generateToken(id, username, tokenVersion, TokenTypeAccess, accessTokenTTL())
	if err != nil {
		return nil, err
	}
	refreshToken, err := generateToken(id, username, tokenVersion, TokenTypeRefresh, refreshTokenTTL())
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: accessToken, RefreshToken: refreshToken}, nil
}

func ParseAccessToken(token string) (*Claims, bool) {
	return parseToken(token, TokenTypeAccess)
}

func ParseRefreshToken(token string) (*Claims, bool) {
	return parseToken(token, TokenTypeRefresh)
}

func generateToken(id int64, username string, tokenVersion int64, tokenType string, ttl time.Duration) (string, error) {
	claims := Claims{
		ID:           id,
		Username:     username,
		TokenVersion: tokenVersion,
		TokenType:    tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			Issuer:    config.GetConfig().Issuer,
			Subject:   config.GetConfig().Subject,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.GetConfig().Key))
}

func parseToken(tokenString, expectedType string) (*Claims, bool) {
	claims := new(Claims)
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(config.GetConfig().Key), nil
	})
	if err != nil || token == nil || !token.Valid || claims == nil {
		return nil, false
	}
	if claims.TokenType != expectedType {
		return nil, false
	}
	return claims, true
}

func accessTokenTTL() time.Duration {
	cfg := config.GetConfig()
	if cfg.AccessExpireDuration > 0 {
		return time.Duration(cfg.AccessExpireDuration) * time.Hour
	}
	return 2 * time.Hour
}

func refreshTokenTTL() time.Duration {
	cfg := config.GetConfig()
	if cfg.RefreshExpireDuration > 0 {
		return time.Duration(cfg.RefreshExpireDuration) * time.Hour
	}
	if cfg.ExpireDuration > 0 {
		return time.Duration(cfg.ExpireDuration) * time.Hour
	}
	return 7 * 24 * time.Hour
}
