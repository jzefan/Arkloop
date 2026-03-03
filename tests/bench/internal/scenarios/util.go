package scenarios

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func RandHex(bytesLen int) string {
	if bytesLen <= 0 {
		return ""
	}
	buf := make([]byte, bytesLen)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func UUIDString() string {
	var b [16]byte
	_, _ = rand.Read(b[:])

	// version 4
	b[6] = (b[6] & 0x0f) | 0x40
	// variant 10xxxxxx
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}
