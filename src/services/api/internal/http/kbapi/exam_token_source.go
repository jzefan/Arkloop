package kbapi

import (
	"context"
	"errors"
	"strings"
	"time"

	"arkloop/services/api/internal/data"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const defaultExamOAuthClientID = "exam-web"

type ExamTokenSource interface {
	IssueExamToken(ctx context.Context, userID uuid.UUID, scopes []string) (string, error)
}

type oidcTokenIssuer interface {
	IssueToken(ctx context.Context, claims jwt.MapClaims) (string, error)
}

type oidcExamTokenSource struct {
	issuer    oidcTokenIssuer
	users     *data.UserRepository
	issuerURL string
	clientID  string
	ttl       time.Duration
}

func NewOIDCExamTokenSource(
	issuer oidcTokenIssuer,
	users *data.UserRepository,
	issuerURL string,
	clientID string,
	ttl time.Duration,
) ExamTokenSource {
	if strings.TrimSpace(clientID) == "" {
		clientID = defaultExamOAuthClientID
	}
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &oidcExamTokenSource{
		issuer:    issuer,
		users:     users,
		issuerURL: strings.TrimRight(strings.TrimSpace(issuerURL), "/"),
		clientID:  clientID,
		ttl:       ttl,
	}
}

func (s *oidcExamTokenSource) IssueExamToken(ctx context.Context, userID uuid.UUID, scopes []string) (string, error) {
	if s == nil || s.issuer == nil {
		return "", errors.New("exam token source not configured")
	}
	if userID == uuid.Nil {
		return "", errors.New("exam token requires user context")
	}
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss":         s.issuerURL,
		"sub":         userID.String(),
		"aud":         s.clientID,
		"azp":         s.clientID,
		"client_id":   s.clientID,
		"scope":       strings.Join(scopes, " "),
		"iat":         now.Unix(),
		"exp":         now.Add(s.ttl).Unix(),
		"jti":         uuid.New().String(),
		"proxy_issue": true,
	}
	if s.users != nil {
		if user, err := s.users.GetByID(ctx, userID); err == nil && user != nil {
			if user.Email != nil {
				claims["email"] = *user.Email
				claims["email_verified"] = user.EmailVerifiedAt != nil
			}
			if user.Username != "" {
				claims["name"] = user.Username
				claims["preferred_username"] = user.Username
			}
			if user.AvatarURL != nil {
				claims["picture"] = *user.AvatarURL
			}
		}
	}
	return s.issuer.IssueToken(ctx, claims)
}
