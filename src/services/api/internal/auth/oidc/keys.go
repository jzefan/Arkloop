// Package oidc implements RS256-signed JWT issuance and verification for
// the ArkLoop OIDC Identity Provider. It is intentionally separate from
// the existing internal/auth package, which uses HS256 symmetric tokens
// for first-party sessions. The two paths coexist:
//
//   - HS256 (internal/auth): /v1/auth/login, refresh, worker service token
//   - RS256 (this package):  /v1/auth/oauth/token (third-party OIDC clients)
package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
)

// RSAKeyBits is the modulus size for newly generated keys. 4096 gives
// ~150 years of security margin and matches the OIDC IdP design doc.
const RSAKeyBits = 4096

// AlgorithmRS256 is the JWT algorithm name used everywhere in this package.
const AlgorithmRS256 = "RS256"

// GenerateRSAKey produces a fresh RSA-4096 keypair suitable for signing
// OIDC tokens. Reads from crypto/rand for the random source.
func GenerateRSAKey() (*rsa.PrivateKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, RSAKeyBits)
	if err != nil {
		return nil, fmt.Errorf("generate rsa key: %w", err)
	}
	return key, nil
}

// EncodePrivateKeyPKCS8 marshals the private key as PKCS#8 PEM. The bytes
// returned are intended to be envelope-encrypted before being persisted.
func EncodePrivateKeyPKCS8(key *rsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal pkcs8: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// DecodePrivateKeyPKCS8 is the inverse of EncodePrivateKeyPKCS8.
func DecodePrivateKeyPKCS8(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	if block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("unexpected PEM block type %q", block.Type)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse pkcs8: %w", err)
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return rsaKey, nil
}

// EncodePublicKeyPKIX marshals the public key as SubjectPublicKeyInfo PEM
// (the standard "BEGIN PUBLIC KEY" format readable by exam-side python-jose
// and most other JWKS-aware libraries).
func EncodePublicKeyPKIX(key *rsa.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal pkix: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}

// DecodePublicKeyPKIX is the inverse of EncodePublicKeyPKIX. Required when
// rebuilding JWKS entries from the database.
func DecodePublicKeyPKIX(pemBytes []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse pkix: %w", err)
	}
	rsaKey, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("public key is not RSA")
	}
	return rsaKey, nil
}

// DeriveKID computes a stable Key ID from the public key's PKIX DER bytes.
// Truncates SHA-256 to 16 bytes hex (32 chars) — long enough to never
// collide in our table, short enough to be ergonomic in JWT headers.
func DeriveKID(pub *rsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("marshal pkix: %w", err)
	}
	sum := sha256.Sum256(der)
	return base64.RawURLEncoding.EncodeToString(sum[:16]), nil
}

// JWK is the JSON shape returned by the JWKS endpoint for an RSA public key.
// Conforms to RFC 7517 §4. Only `kty`, `use`, `alg`, `kid`, `n`, `e` are
// emitted; clients that need x5c/x5t can fetch directly from the PEM.
type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// PublicKeyToJWK turns an RSA public key into its JWK representation.
// `n` and `e` are big-endian, base64url-encoded with no padding (RFC 7518 §6.3.1).
func PublicKeyToJWK(pub *rsa.PublicKey, kid string) JWK {
	return JWK{
		Kty: "RSA",
		Use: "sig",
		Alg: AlgorithmRS256,
		Kid: kid,
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(bigEndianBytes(pub.E)),
	}
}

func bigEndianBytes(e int) []byte {
	return new(big.Int).SetInt64(int64(e)).Bytes()
}
