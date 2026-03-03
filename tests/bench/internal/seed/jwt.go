package seed

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// MakeJWT 生成一个 HS256 签名的 JWT，包含 org 和 sub claims。
func MakeJWT(secret string, orgID string, userID string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": userID,
		"org": orgID,
		"iat": now.Unix(),
		"exp": now.Add(expiry).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
