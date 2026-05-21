package oidc

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Signer wraps a single active RSA private key + its KID. Verification is
// done elsewhere (KeySet) because verifiers must accept any non-compromised
// key, not just the one currently signing.
type Signer struct {
	kid     string
	private *rsa.PrivateKey
}

// NewSigner constructs a Signer. kid must be non-empty (will be written
// into the JWT header so verifiers can look up the matching public key).
func NewSigner(kid string, private *rsa.PrivateKey) (*Signer, error) {
	if kid == "" {
		return nil, errors.New("kid must not be empty")
	}
	if private == nil {
		return nil, errors.New("private key must not be nil")
	}
	return &Signer{kid: kid, private: private}, nil
}

// KID returns the key identifier this signer writes into JWT headers.
func (s *Signer) KID() string { return s.kid }

// Sign issues an RS256 JWT with the given claims map. The `alg` and `kid`
// headers are set automatically; callers should not set them in claims.
func (s *Signer) Sign(claims jwt.MapClaims) (string, error) {
	if claims == nil {
		return "", errors.New("claims must not be nil")
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = s.kid
	signed, err := tok.SignedString(s.private)
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return signed, nil
}

// KeySet is the set of public keys a verifier will accept. Maps kid → key.
// Populate from oidc_signing_keys WHERE status IN ('active','retired').
type KeySet struct {
	keys map[string]*rsa.PublicKey
}

// NewKeySet constructs a verifier-side keyset.
func NewKeySet(keys map[string]*rsa.PublicKey) *KeySet {
	cloned := make(map[string]*rsa.PublicKey, len(keys))
	for k, v := range keys {
		cloned[k] = v
	}
	return &KeySet{keys: cloned}
}

// Verify parses the JWT, validates its signature against the key set
// matched by token header `kid`, and returns the claims. Expiry is enforced
// by jwt/v5's default validator (exp/nbf checks).
//
// Rejects: missing kid, unknown kid (e.g. revoked-and-removed key), signature
// mismatch, expired token. Does NOT validate audience, issuer, or scope —
// those are caller's responsibility because they vary per endpoint.
func (ks *KeySet) Verify(token string) (jwt.MapClaims, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{AlgorithmRS256}))
	parsed, err := parser.Parse(token, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("token header missing kid")
		}
		pub, ok := ks.keys[kid]
		if !ok {
			return nil, fmt.Errorf("unknown kid: %s", kid)
		}
		return pub, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
	}
	if parsed == nil || !parsed.Valid {
		return nil, ErrTokenInvalid
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

// Common verification errors.
var (
	ErrTokenExpired = errors.New("oidc token expired")
	ErrTokenInvalid = errors.New("oidc token invalid")
)

// NowFunc is injectable for tests; defaults to time.Now.
var NowFunc = func() time.Time { return time.Now().UTC() }
