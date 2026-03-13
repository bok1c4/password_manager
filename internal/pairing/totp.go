// Package pairing provides TOTP-based pairing code generation and verification
package pairing

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"time"

	"golang.org/x/crypto/hkdf"
)

// deriveTOTPKey derives a dedicated TOTP key from vault master key using HKDF
// This isolates TOTP from the encryption key to prevent information leakage
func deriveTOTPKey(vaultMasterKey []byte, vaultID string) []byte {
	reader := hkdf.New(sha512.New, vaultMasterKey, []byte(vaultID), []byte("pwman-pairing-totp"))
	key := make([]byte, 32)
	reader.Read(key)
	return key
}

// GeneratePairingCode produces a 6-digit TOTP code seeded from vault master key
// The code changes every 60 seconds
func GeneratePairingCode(vaultMasterKey []byte, vaultID string) string {
	totpKey := deriveTOTPKey(vaultMasterKey, vaultID)
	window := time.Now().Unix() / 60

	mac := hmac.New(sha256.New, totpKey)
	mac.Write([]byte(vaultID))
	binary.Write(mac, binary.BigEndian, window)
	sum := mac.Sum(nil)

	// Dynamic truncation (RFC 4226)
	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1_000_000)
}

// VerifyPairingCode accepts codes from window-1, window, window+1
// to handle up to 60 seconds of clock skew
func VerifyPairingCode(vaultMasterKey []byte, vaultID, candidate string) bool {
	totpKey := deriveTOTPKey(vaultMasterKey, vaultID)
	window := time.Now().Unix() / 60

	for _, w := range []int64{window - 1, window, window + 1} {
		mac := hmac.New(sha256.New, totpKey)
		mac.Write([]byte(vaultID))
		binary.Write(mac, binary.BigEndian, w)
		sum := mac.Sum(nil)

		offset := sum[len(sum)-1] & 0x0f
		code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
		expected := fmt.Sprintf("%06d", code%1_000_000)

		if hmac.Equal([]byte(expected), []byte(candidate)) {
			return true
		}
	}
	return false
}
