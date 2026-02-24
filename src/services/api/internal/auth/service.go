package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

type InvalidCredentialsError struct{}

func (InvalidCredentialsError) Error() string {
	return "invalid_credentials"
}

type UserNotFoundError struct {
	UserID uuid.UUID
}

func (e UserNotFoundError) Error() string {
	return "user not found"
}

type SuspendedUserError struct {
	UserID uuid.UUID
	Status string // "suspended" | "deleted" | other non-active
}

func (e SuspendedUserError) Error() string {
	if e.Status == "deleted" {
		return "user deleted"
	}
	return "user suspended"
}

// IssuedTokenPair 包含一次认证/刷新操作签发的 Access + Refresh Token 对。
type IssuedTokenPair struct {
	AccessToken  string
	RefreshToken string
	UserID       uuid.UUID
}

type Service struct {
	userRepo         *data.UserRepository
	credentialRepo   *data.UserCredentialRepository
	membershipRepo   *data.OrgMembershipRepository
	passwordHasher   *BcryptPasswordHasher
	tokenService     *JwtAccessTokenService
	refreshTokenRepo *data.RefreshTokenRepository
}

func NewService(
	userRepo *data.UserRepository,
	credentialRepo *data.UserCredentialRepository,
	membershipRepo *data.OrgMembershipRepository,
	passwordHasher *BcryptPasswordHasher,
	tokenService *JwtAccessTokenService,
	refreshTokenRepo *data.RefreshTokenRepository,
) (*Service, error) {
	if userRepo == nil {
		return nil, errors.New("userRepo must not be nil")
	}
	if credentialRepo == nil {
		return nil, errors.New("credentialRepo must not be nil")
	}
	if membershipRepo == nil {
		return nil, errors.New("membershipRepo must not be nil")
	}
	if passwordHasher == nil {
		return nil, errors.New("passwordHasher must not be nil")
	}
	if tokenService == nil {
		return nil, errors.New("tokenService must not be nil")
	}
	if refreshTokenRepo == nil {
		return nil, errors.New("refreshTokenRepo must not be nil")
	}
	return &Service{
		userRepo:         userRepo,
		credentialRepo:   credentialRepo,
		membershipRepo:   membershipRepo,
		passwordHasher:   passwordHasher,
		tokenService:     tokenService,
		refreshTokenRepo: refreshTokenRepo,
	}, nil
}

func (s *Service) IssueAccessToken(ctx context.Context, login string, password string) (IssuedTokenPair, error) {
	credential, err := s.credentialRepo.GetByLogin(ctx, login)
	if err != nil {
		return IssuedTokenPair{}, err
	}
	if credential == nil {
		return IssuedTokenPair{}, InvalidCredentialsError{}
	}
	if !s.passwordHasher.VerifyPassword(password, credential.PasswordHash) {
		return IssuedTokenPair{}, InvalidCredentialsError{}
	}

	user, err := s.userRepo.GetByID(ctx, credential.UserID)
	if err != nil {
		return IssuedTokenPair{}, err
	}
	if user == nil {
		return IssuedTokenPair{}, UserNotFoundError{UserID: credential.UserID}
	}
	if user.Status != "active" {
		return IssuedTokenPair{}, SuspendedUserError{UserID: credential.UserID, Status: user.Status}
	}

	return s.issueTokenPair(ctx, credential.UserID)
}

// ConsumeRefreshToken 验证并轮换 Refresh Token，返回新的 token 对。
// 轮换是原子的：旧 token 在同一 UPDATE 中被吊销并返回 user_id；若 token 无效则返回 TokenInvalidError。
func (s *Service) ConsumeRefreshToken(ctx context.Context, plaintext string) (IssuedTokenPair, error) {
	if plaintext == "" {
		return IssuedTokenPair{}, TokenInvalidError{message: "refresh token required"}
	}

	tokenHash := sha256RefreshToken(plaintext)

	userID, ok, err := s.refreshTokenRepo.ConsumeByHash(ctx, tokenHash)
	if err != nil {
		return IssuedTokenPair{}, err
	}
	if !ok {
		return IssuedTokenPair{}, TokenInvalidError{message: "refresh token invalid or expired"}
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return IssuedTokenPair{}, err
	}
	if user == nil {
		return IssuedTokenPair{}, UserNotFoundError{UserID: userID}
	}
	if user.Status != "active" {
		return IssuedTokenPair{}, SuspendedUserError{UserID: userID, Status: user.Status}
	}

	return s.issueTokenPair(ctx, userID)
}

// issueTokenPair 为指定用户签发 Access Token + Refresh Token，并将 Refresh Token 持久化到 DB。
func (s *Service) issueTokenPair(ctx context.Context, userID uuid.UUID) (IssuedTokenPair, error) {
	now := time.Now().UTC()
	orgID := s.resolveDefaultOrgID(ctx, userID)

	accessToken, err := s.tokenService.Issue(userID, orgID, now)
	if err != nil {
		return IssuedTokenPair{}, err
	}

	plaintext, hash, expiresAt, err := s.tokenService.IssueRefreshToken(now)
	if err != nil {
		return IssuedTokenPair{}, err
	}

	if _, err = s.refreshTokenRepo.Create(ctx, userID, hash, expiresAt); err != nil {
		return IssuedTokenPair{}, err
	}

	return IssuedTokenPair{
		AccessToken:  accessToken,
		RefreshToken: plaintext,
		UserID:       userID,
	}, nil
}

// resolveDefaultOrgID 查用户的默认 org；失败时静默返回 uuid.Nil，不阻断认证流程。
func (s *Service) resolveDefaultOrgID(ctx context.Context, userID uuid.UUID) uuid.UUID {
	membership, err := s.membershipRepo.GetDefaultForUser(ctx, userID)
	if err != nil || membership == nil {
		return uuid.Nil
	}
	return membership.OrgID
}

func (s *Service) AuthenticateUser(ctx context.Context, token string) (*data.User, error) {
	verified, err := s.tokenService.Verify(token)
	if err != nil {
		return nil, err
	}

	user, err := s.userRepo.GetByID(ctx, verified.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, UserNotFoundError{UserID: verified.UserID}
	}

	if user.Status != "active" {
		return nil, SuspendedUserError{UserID: user.ID, Status: user.Status}
	}
	if verified.IssuedAt.Before(user.TokensInvalidBefore) {
		return nil, TokenInvalidError{message: "token revoked"}
	}
	return user, nil
}

func (s *Service) Logout(ctx context.Context, userID uuid.UUID, now time.Time) error {
	if userID == uuid.Nil {
		return errors.New("user_id must not be nil")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if err := s.refreshTokenRepo.RevokeAllForUser(ctx, userID); err != nil {
		return err
	}
	return s.userRepo.BumpTokensInvalidBefore(ctx, userID, now)
}

func sha256RefreshToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
