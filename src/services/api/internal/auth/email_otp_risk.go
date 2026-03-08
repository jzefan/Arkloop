package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	emailOTPSendLimit          = 5
	emailOTPSendWindow         = 10 * time.Minute
	emailOTPVerifyFailLimit    = 8
	emailOTPVerifyLockDuration = 10 * time.Minute
	emailOTPRedisTimeout       = 100 * time.Millisecond

	emailOTPSendKeyPrefix       = "arkloop:auth:email_otp:send:"
	emailOTPVerifyFailKeyPrefix = "arkloop:auth:email_otp:verify_fail:"
	emailOTPVerifyLockKeyPrefix = "arkloop:auth:email_otp:verify_lock:"
)

type EmailOTPRiskControl interface {
	AllowSend(ctx context.Context, email string) error
	EnsureVerifyAllowed(ctx context.Context, email string) error
	RecordVerifyFailure(ctx context.Context, email string) error
	ResetVerifyState(ctx context.Context, email string) error
}

type OTPRateLimitedError struct{}

func (OTPRateLimitedError) Error() string {
	return "otp rate limited"
}

type OTPLockedError struct{}

func (OTPLockedError) Error() string {
	return "otp locked"
}

type OTPProtectionUnavailableError struct{}

func (OTPProtectionUnavailableError) Error() string {
	return "otp protection unavailable"
}

type redisEmailOTPRiskControl struct {
	client *redis.Client
}

func NewRedisEmailOTPRiskControl(client *redis.Client) EmailOTPRiskControl {
	return &redisEmailOTPRiskControl{client: client}
}

func (c *redisEmailOTPRiskControl) AllowSend(ctx context.Context, email string) error {
	count, err := c.incrWithTTL(ctx, emailOTPSendKeyPrefix+hashOTPEmail(email), emailOTPSendWindow)
	if err != nil {
		return OTPProtectionUnavailableError{}
	}
	if count > emailOTPSendLimit {
		return OTPRateLimitedError{}
	}
	return nil
}

func (c *redisEmailOTPRiskControl) EnsureVerifyAllowed(ctx context.Context, email string) error {
	if c == nil || c.client == nil {
		return OTPProtectionUnavailableError{}
	}
	ctx, cancel := context.WithTimeout(ctx, emailOTPRedisTimeout)
	defer cancel()

	locked, err := c.client.Exists(ctx, emailOTPVerifyLockKeyPrefix+hashOTPEmail(email)).Result()
	if err != nil {
		return OTPProtectionUnavailableError{}
	}
	if locked > 0 {
		return OTPLockedError{}
	}
	return nil
}

func (c *redisEmailOTPRiskControl) RecordVerifyFailure(ctx context.Context, email string) error {
	count, err := c.incrWithTTL(ctx, emailOTPVerifyFailKeyPrefix+hashOTPEmail(email), emailOTPVerifyLockDuration)
	if err != nil {
		return OTPProtectionUnavailableError{}
	}
	if count < emailOTPVerifyFailLimit {
		return nil
	}
	if c == nil || c.client == nil {
		return OTPProtectionUnavailableError{}
	}
	ctx, cancel := context.WithTimeout(ctx, emailOTPRedisTimeout)
	defer cancel()
	if err := c.client.Set(ctx, emailOTPVerifyLockKeyPrefix+hashOTPEmail(email), "1", emailOTPVerifyLockDuration).Err(); err != nil {
		return OTPProtectionUnavailableError{}
	}
	return nil
}

func (c *redisEmailOTPRiskControl) ResetVerifyState(ctx context.Context, email string) error {
	if c == nil || c.client == nil {
		return OTPProtectionUnavailableError{}
	}
	ctx, cancel := context.WithTimeout(ctx, emailOTPRedisTimeout)
	defer cancel()
	if err := c.client.Del(
		ctx,
		emailOTPVerifyFailKeyPrefix+hashOTPEmail(email),
		emailOTPVerifyLockKeyPrefix+hashOTPEmail(email),
	).Err(); err != nil {
		return OTPProtectionUnavailableError{}
	}
	return nil
}

func (c *redisEmailOTPRiskControl) incrWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if c == nil || c.client == nil {
		return 0, OTPProtectionUnavailableError{}
	}
	ctx, cancel := context.WithTimeout(ctx, emailOTPRedisTimeout)
	defer cancel()

	count, err := c.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if count == 1 {
		if err := c.client.Expire(ctx, key, ttl).Err(); err != nil {
			return 0, err
		}
		return count, nil
	}
	remaining, err := c.client.TTL(ctx, key).Result()
	if err == nil && remaining <= 0 {
		if err := c.client.Expire(ctx, key, ttl).Err(); err != nil {
			return 0, err
		}
	}
	return count, nil
}

func hashOTPEmail(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func emailOTPSendKey(email string) string {
	return emailOTPSendKeyPrefix + hashOTPEmail(email)
}

func emailOTPVerifyFailKey(email string) string {
	return emailOTPVerifyFailKeyPrefix + hashOTPEmail(email)
}

func emailOTPVerifyLockKey(email string) string {
	return emailOTPVerifyLockKeyPrefix + hashOTPEmail(email)
}

func formatOTPProtectionError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("otp protection: %w", err)
}
