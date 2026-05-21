package oidc

import (
	"crypto/rsa"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestEncodeDecodePrivateKeyRoundTrip(t *testing.T) {
	key, err := GenerateRSAKey()
	if err != nil {
		t.Fatalf("GenerateRSAKey: %v", err)
	}
	pemBytes, err := EncodePrivateKeyPKCS8(key)
	if err != nil {
		t.Fatalf("EncodePrivateKeyPKCS8: %v", err)
	}
	if !strings.Contains(string(pemBytes), "PRIVATE KEY") {
		t.Fatalf("PEM missing header: %s", pemBytes[:64])
	}
	decoded, err := DecodePrivateKeyPKCS8(pemBytes)
	if err != nil {
		t.Fatalf("DecodePrivateKeyPKCS8: %v", err)
	}
	if decoded.N.Cmp(key.N) != 0 || decoded.E != key.E {
		t.Fatal("decoded key does not match original")
	}
}

func TestEncodeDecodePublicKeyRoundTrip(t *testing.T) {
	key, _ := GenerateRSAKey()
	pemBytes, err := EncodePublicKeyPKIX(&key.PublicKey)
	if err != nil {
		t.Fatalf("EncodePublicKeyPKIX: %v", err)
	}
	decoded, err := DecodePublicKeyPKIX(pemBytes)
	if err != nil {
		t.Fatalf("DecodePublicKeyPKIX: %v", err)
	}
	if decoded.N.Cmp(key.N) != 0 || decoded.E != key.E {
		t.Fatal("decoded public key does not match original")
	}
}

func TestDeriveKIDIsStableAndDistinct(t *testing.T) {
	keyA, _ := GenerateRSAKey()
	keyB, _ := GenerateRSAKey()

	kidA1, _ := DeriveKID(&keyA.PublicKey)
	kidA2, _ := DeriveKID(&keyA.PublicKey)
	kidB, _ := DeriveKID(&keyB.PublicKey)

	if kidA1 == "" || kidA1 != kidA2 {
		t.Fatalf("KID for same key is unstable: %q vs %q", kidA1, kidA2)
	}
	if kidA1 == kidB {
		t.Fatal("KIDs for different keys should not collide")
	}
	// We use 16-byte SHA256 prefix → base64url no-pad → 22 chars.
	if len(kidA1) != 22 {
		t.Fatalf("unexpected KID length %d (want 22): %q", len(kidA1), kidA1)
	}
}

func TestPublicKeyToJWKShape(t *testing.T) {
	key, _ := GenerateRSAKey()
	kid, _ := DeriveKID(&key.PublicKey)
	jwk := PublicKeyToJWK(&key.PublicKey, kid)

	if jwk.Kty != "RSA" || jwk.Use != "sig" || jwk.Alg != "RS256" {
		t.Fatalf("unexpected JWK metadata: %+v", jwk)
	}
	if jwk.Kid != kid {
		t.Fatalf("kid mismatch: %s vs %s", jwk.Kid, kid)
	}
	if jwk.N == "" || jwk.E == "" {
		t.Fatalf("n/e must be base64url encoded: %+v", jwk)
	}
	// Standard public exponent is 65537 → 0x010001 → "AQAB" in base64url.
	if jwk.E != "AQAB" {
		t.Fatalf("unexpected public exponent encoding: %s (want AQAB)", jwk.E)
	}
}

func TestSignerVerifyHappyPath(t *testing.T) {
	key, _ := GenerateRSAKey()
	kid, _ := DeriveKID(&key.PublicKey)
	signer, err := NewSigner(kid, key)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}

	now := time.Now().UTC()
	token, err := signer.Sign(jwt.MapClaims{
		"sub":   "user-123",
		"iss":   "https://arkloop.example.com",
		"aud":   "exam-web",
		"scope": "openid exam:write",
		"iat":   now.Unix(),
		"exp":   now.Add(15 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	ks := NewKeySet(map[string]*rsa.PublicKey{kid: &key.PublicKey})
	claims, err := ks.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims["sub"] != "user-123" || claims["scope"] != "openid exam:write" {
		t.Fatalf("claims roundtrip mismatch: %+v", claims)
	}
}

func TestKeySetRejectsUnknownKID(t *testing.T) {
	keyA, _ := GenerateRSAKey()
	keyB, _ := GenerateRSAKey()
	kidA, _ := DeriveKID(&keyA.PublicKey)
	kidB, _ := DeriveKID(&keyB.PublicKey)

	signer, _ := NewSigner(kidA, keyA)
	tok, _ := signer.Sign(jwt.MapClaims{
		"sub": "u",
		"exp": time.Now().Add(time.Minute).Unix(),
	})

	// KeySet only holds keyB → tok signed with keyA must be rejected.
	ks := NewKeySet(map[string]*rsa.PublicKey{kidB: &keyB.PublicKey})
	if _, err := ks.Verify(tok); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestKeySetRejectsExpiredToken(t *testing.T) {
	key, _ := GenerateRSAKey()
	kid, _ := DeriveKID(&key.PublicKey)
	signer, _ := NewSigner(kid, key)

	tok, _ := signer.Sign(jwt.MapClaims{
		"sub": "u",
		"iat": time.Now().Add(-time.Hour).Unix(),
		"exp": time.Now().Add(-time.Minute).Unix(),
	})

	ks := NewKeySet(map[string]*rsa.PublicKey{kid: &key.PublicKey})
	_, err := ks.Verify(tok)
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestKeySetRejectsTamperedSignature(t *testing.T) {
	key, _ := GenerateRSAKey()
	kid, _ := DeriveKID(&key.PublicKey)
	signer, _ := NewSigner(kid, key)
	tok, _ := signer.Sign(jwt.MapClaims{
		"sub": "u",
		"exp": time.Now().Add(time.Minute).Unix(),
	})

	// Flip the last char of the signature segment.
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("malformed jwt: %s", tok)
	}
	if parts[2][len(parts[2])-1] == 'A' {
		parts[2] = parts[2][:len(parts[2])-1] + "B"
	} else {
		parts[2] = parts[2][:len(parts[2])-1] + "A"
	}
	tampered := strings.Join(parts, ".")

	ks := NewKeySet(map[string]*rsa.PublicKey{kid: &key.PublicKey})
	if _, err := ks.Verify(tampered); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected ErrTokenInvalid for tampered token, got %v", err)
	}
}
