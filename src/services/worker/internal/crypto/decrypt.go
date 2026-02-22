package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

const (
	EncryptionKeyEnv = "ARKLOOP_ENCRYPTION_KEY"
	nonceSize        = 12
)

// DecryptGCM 解密 secrets 表中存储的密文。
// 格式：base64(nonce || ciphertext+tag)，与 API 服务 crypto 包保持一致。
func DecryptGCM(encoded string) ([]byte, error) {
	rawKey := strings.TrimSpace(os.Getenv(EncryptionKeyEnv))
	if rawKey == "" {
		return nil, fmt.Errorf("crypto: %s is not set", EncryptionKeyEnv)
	}

	keyBytes, err := hex.DecodeString(rawKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: invalid encryption key: %w", err)
	}
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("crypto: encryption key must be 32 bytes")
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("crypto: base64 decode: %w", err)
	}
	if len(data) < nonceSize {
		return nil, fmt.Errorf("crypto: ciphertext too short")
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}

	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}
