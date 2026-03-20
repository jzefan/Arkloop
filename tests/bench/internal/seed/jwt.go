package seed

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// MakeJWT 生成 HS256 JWT，含 account、sub。
func MakeJWT(secret string, accountID string, userID string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":     userID,
		"account": accountID,
		"iat":     now.Unix(),
		"exp":     now.Add(expiry).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
