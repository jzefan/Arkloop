package auth

import (
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	jwtAlgorithmHS256 = "HS256"
	accessTokenType   = "access"
)

type TokenExpiredError struct {
	message string
}

func (e TokenExpiredError) Error() string {
	if e.message == "" {
		return "token expired"
	}
	return e.message
}

type TokenInvalidError struct {
	message string
}

func (e TokenInvalidError) Error() string {
	if e.message == "" {
		return "token invalid"
	}
	return e.message
}

type VerifiedAccessToken struct {
	UserID   uuid.UUID
	OrgID    uuid.UUID // uuid.Nil 表示旧 token 无 org claim
	IssuedAt time.Time
}

type JwtAccessTokenService struct {
	secret     []byte
	ttlSeconds int
}

func NewJwtAccessTokenService(secret string, ttlSeconds int) (*JwtAccessTokenService, error) {
	if secret == "" {
		return nil, errors.New("secret must not be empty")
	}
	if ttlSeconds <= 0 {
		return nil, errors.New("ttlSeconds must be positive")
	}
	return &JwtAccessTokenService{
		secret:     []byte(secret),
		ttlSeconds: ttlSeconds,
	}, nil
}

func (s *JwtAccessTokenService) Issue(userID uuid.UUID, orgID uuid.UUID, now time.Time) (string, error) {
	if userID == uuid.Nil {
		return "", errors.New("user_id must not be nil")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	issuedAt := now.UTC()
	expiresAt := issuedAt.Add(time.Duration(s.ttlSeconds) * time.Second)
	claims := jwt.MapClaims{
		"sub": userID.String(),
		"typ": accessTokenType,
		"iat": timestampFloatSeconds(issuedAt),
		"exp": expiresAt.Unix(),
	}
	if orgID != uuid.Nil {
		claims["org"] = orgID.String()
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", err
	}
	return signed, nil
}

func (s *JwtAccessTokenService) Verify(token string) (VerifiedAccessToken, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwtAlgorithmHS256}))
	parsed, err := parser.Parse(token, func(t *jwt.Token) (any, error) {
		return s.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return VerifiedAccessToken{}, TokenExpiredError{message: "token expired"}
		}
		return VerifiedAccessToken{}, TokenInvalidError{message: "token invalid"}
	}
	if parsed == nil || !parsed.Valid {
		return VerifiedAccessToken{}, TokenInvalidError{message: "token invalid"}
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return VerifiedAccessToken{}, TokenInvalidError{message: "token invalid"}
	}

	if _, ok := claims["exp"]; !ok {
		return VerifiedAccessToken{}, TokenInvalidError{message: "token invalid"}
	}

	typ, ok := claims["typ"]
	if !ok {
		return VerifiedAccessToken{}, TokenInvalidError{message: "token invalid"}
	}
	if typStr, ok := typ.(string); !ok || typStr != accessTokenType {
		return VerifiedAccessToken{}, TokenInvalidError{message: "token type incorrect"}
	}

	sub, ok := claims["sub"]
	if !ok {
		return VerifiedAccessToken{}, TokenInvalidError{message: "token invalid"}
	}
	subStr, ok := sub.(string)
	if !ok {
		return VerifiedAccessToken{}, TokenInvalidError{message: "token subject invalid"}
	}
	userID, err := uuid.Parse(subStr)
	if err != nil {
		return VerifiedAccessToken{}, TokenInvalidError{message: "token subject invalid"}
	}

	iat, ok := claims["iat"]
	if !ok {
		return VerifiedAccessToken{}, TokenInvalidError{message: "token invalid"}
	}
	issuedAt, err := parseIAT(iat)
	if err != nil {
		return VerifiedAccessToken{}, TokenInvalidError{message: "token iat invalid"}
	}

	var orgID uuid.UUID
	if orgRaw, exists := claims["org"]; exists {
		if orgStr, ok := orgRaw.(string); ok {
			if parsed, err := uuid.Parse(orgStr); err == nil {
				orgID = parsed
			}
		}
	}

	return VerifiedAccessToken{
		UserID:   userID,
		OrgID:    orgID,
		IssuedAt: issuedAt,
	}, nil
}

func timestampFloatSeconds(value time.Time) float64 {
	return float64(value.UnixNano()) / 1e9
}

func parseIAT(value any) (time.Time, error) {
	iat, ok := numericToFloat64(value)
	if !ok {
		return time.Time{}, errors.New("iat is not a number")
	}
	if math.IsNaN(iat) || math.IsInf(iat, 0) {
		return time.Time{}, errors.New("iat invalid")
	}
	sec, frac := math.Modf(iat)
	if sec < 0 {
		return time.Time{}, errors.New("iat invalid")
	}

	nsec := int64(frac * 1e9)
	if nsec < 0 {
		nsec = -nsec
	}
	return time.Unix(int64(sec), nsec).UTC(), nil
}

func numericToFloat64(value any) (float64, bool) {
	switch casted := value.(type) {
	case float64:
		return casted, true
	case float32:
		return float64(casted), true
	case int:
		return float64(casted), true
	case int64:
		return float64(casted), true
	case int32:
		return float64(casted), true
	case json.Number:
		parsed, err := casted.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
