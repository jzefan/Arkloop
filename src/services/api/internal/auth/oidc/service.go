package oidc

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"sync"
	"time"

	"arkloop/services/api/internal/data"
	sharedencryption "arkloop/services/shared/encryption"

	"github.com/golang-jwt/jwt/v5"
)

// Service is the high-level facade other packages use to sign and verify
// OIDC tokens. It owns:
//
//   - the active Signer (rebuilt when the active key changes)
//   - a lazily-cached KeySet for verification (rebuilt on miss)
//
// All persistence flows through OIDCSigningKeyRepository; the master
// envelope key comes from KeyRing.
type Service struct {
	repo    *data.OIDCSigningKeyRepository
	keyring *sharedencryption.KeyRing

	mu       sync.RWMutex
	signer   *Signer       // current active signer; lazily loaded
	keySet   *KeySet       // verification keys for active + retired
	cachedAt time.Time     // when keySet was last rebuilt
	cacheTTL time.Duration // how long to cache before re-querying
}

// NewService wires the dependencies. cacheTTL defaults to 5 minutes if
// zero; passing a small value in tests lets you observe rotation faster.
func NewService(
	repo *data.OIDCSigningKeyRepository,
	keyring *sharedencryption.KeyRing,
	cacheTTL time.Duration,
) (*Service, error) {
	if repo == nil {
		return nil, errors.New("repo must not be nil")
	}
	if keyring == nil {
		return nil, errors.New("keyring must not be nil")
	}
	if cacheTTL <= 0 {
		cacheTTL = 5 * time.Minute
	}
	return &Service{repo: repo, keyring: keyring, cacheTTL: cacheTTL}, nil
}

// EnsureActiveKey is called once at process startup. If the database has
// no active key, generates a fresh RSA-4096 keypair, envelope-encrypts the
// private key, and inserts it. Idempotent — safe to call on every boot.
func (s *Service) EnsureActiveKey(ctx context.Context) (*data.OIDCSigningKey, error) {
	if existing, err := s.repo.GetActive(ctx); err != nil {
		return nil, fmt.Errorf("query active key: %w", err)
	} else if existing != nil {
		return existing, nil
	}

	key, err := GenerateRSAKey()
	if err != nil {
		return nil, err
	}
	kid, err := DeriveKID(&key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("derive kid: %w", err)
	}
	pubPEM, err := EncodePublicKeyPKIX(&key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("encode public key: %w", err)
	}
	privPEM, err := EncodePrivateKeyPKCS8(key)
	if err != nil {
		return nil, fmt.Errorf("encode private key: %w", err)
	}
	encrypted, keyVer, err := s.keyring.Encrypt(privPEM)
	if err != nil {
		return nil, fmt.Errorf("envelope encrypt: %w", err)
	}

	stored, err := s.repo.Insert(ctx, kid, AlgorithmRS256, string(pubPEM), encrypted, keyVer, true /*activate*/)
	if err != nil {
		return nil, fmt.Errorf("insert oidc key: %w", err)
	}
	return &stored, nil
}

// ActiveSigner returns the Signer backed by the current active key.
// Loaded lazily and cached until invalidateCache() is called.
func (s *Service) ActiveSigner(ctx context.Context) (*Signer, error) {
	s.mu.RLock()
	if s.signer != nil {
		signer := s.signer
		s.mu.RUnlock()
		return signer, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.signer != nil {
		return s.signer, nil
	}

	active, err := s.repo.GetActive(ctx)
	if err != nil {
		return nil, err
	}
	if active == nil {
		return nil, errors.New("no active oidc signing key (call EnsureActiveKey first)")
	}
	privPEM, err := s.keyring.Decrypt(active.PrivateKeyEncrypted, active.PrivateKeyEncryptionKeyVer)
	if err != nil {
		return nil, fmt.Errorf("decrypt private key: %w", err)
	}
	private, err := DecodePrivateKeyPKCS8(privPEM)
	if err != nil {
		return nil, err
	}
	signer, err := NewSigner(active.KID, private)
	if err != nil {
		return nil, err
	}
	s.signer = signer
	return signer, nil
}

// IssueToken signs the given claims with the active key.
//
// Required claims (caller's responsibility):
//   iss, sub, aud, iat, exp, jti, scope (space-separated string)
// Optional but recommended: nonce, auth_time (for id_token).
func (s *Service) IssueToken(ctx context.Context, claims jwt.MapClaims) (string, error) {
	signer, err := s.ActiveSigner(ctx)
	if err != nil {
		return "", err
	}
	return signer.Sign(claims)
}

// VerifyToken parses+verifies a token against all currently publishable
// keys (active + retired). Returns the claims map on success.
func (s *Service) VerifyToken(ctx context.Context, token string) (jwt.MapClaims, error) {
	ks, err := s.keySetCached(ctx)
	if err != nil {
		return nil, err
	}
	return ks.Verify(token)
}

// PublishableJWKs returns the JWK array suitable for the /.well-known/jwks.json
// endpoint. Cached together with the verification key set.
func (s *Service) PublishableJWKs(ctx context.Context) ([]JWK, error) {
	rows, err := s.repo.ListPublishable(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]JWK, 0, len(rows))
	for _, row := range rows {
		pub, err := DecodePublicKeyPKIX([]byte(row.PublicKeyPEM))
		if err != nil {
			return nil, fmt.Errorf("decode public key (kid=%s): %w", row.KID, err)
		}
		out = append(out, PublicKeyToJWK(pub, row.KID))
	}
	return out, nil
}

// keySetCached lazy-loads the KeySet from DB and caches it for cacheTTL.
// Concurrent callers behind the same expired cache share one DB roundtrip.
func (s *Service) keySetCached(ctx context.Context) (*KeySet, error) {
	s.mu.RLock()
	if s.keySet != nil && time.Since(s.cachedAt) < s.cacheTTL {
		ks := s.keySet
		s.mu.RUnlock()
		return ks, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keySet != nil && time.Since(s.cachedAt) < s.cacheTTL {
		return s.keySet, nil
	}

	rows, err := s.repo.ListPublishable(ctx)
	if err != nil {
		return nil, err
	}
	keys := make(map[string]*rsa.PublicKey, len(rows))
	for _, row := range rows {
		pub, err := DecodePublicKeyPKIX([]byte(row.PublicKeyPEM))
		if err != nil {
			return nil, fmt.Errorf("decode public key (kid=%s): %w", row.KID, err)
		}
		keys[row.KID] = pub
	}
	s.keySet = NewKeySet(keys)
	s.cachedAt = time.Now()
	return s.keySet, nil
}

// InvalidateCache forces the next Verify/JWKS call to re-fetch from DB.
// Call after rotation, retirement, or compromise.
func (s *Service) InvalidateCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keySet = nil
	s.signer = nil
	s.cachedAt = time.Time{}
}
