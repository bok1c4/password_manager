package pairing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTOTPGeneration(t *testing.T) {
	masterKey := []byte("test-master-key-32bytes-long!!")
	vaultID := "test-vault-123"

	code := GeneratePairingCode(masterKey, vaultID)

	// Should be 6 digits
	assert.Equal(t, 6, len(code))
	assert.True(t, isAllDigits(code))

	// Should verify correctly
	assert.True(t, VerifyPairingCode(masterKey, vaultID, code))

	// Wrong code should fail
	assert.False(t, VerifyPairingCode(masterKey, vaultID, "000000"))
}

func TestTOTPClockSkew(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping clock-dependent test")
	}

	masterKey := []byte("test-master-key-32bytes-long!!")
	vaultID := "test-vault-123"

	// Generate code at current time
	code := GeneratePairingCode(masterKey, vaultID)

	// Should still verify 30 seconds later (within same window or next)
	time.Sleep(30 * time.Second)
	assert.True(t, VerifyPairingCode(masterKey, vaultID, code))
}

func TestTOTPDifferentVaults(t *testing.T) {
	masterKey := []byte("test-master-key-32bytes-long!!")
	vaultID1 := "vault-1"
	vaultID2 := "vault-2"

	code1 := GeneratePairingCode(masterKey, vaultID1)
	code2 := GeneratePairingCode(masterKey, vaultID2)

	// Codes should be different for different vaults
	assert.NotEqual(t, code1, code2)

	// Each code should only verify for its vault
	assert.True(t, VerifyPairingCode(masterKey, vaultID1, code1))
	assert.False(t, VerifyPairingCode(masterKey, vaultID1, code2))
	assert.True(t, VerifyPairingCode(masterKey, vaultID2, code2))
	assert.False(t, VerifyPairingCode(masterKey, vaultID2, code1))
}

func TestTOTPCodeRotation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping clock-dependent test")
	}

	masterKey := []byte("test-master-key-32bytes-long!!")
	vaultID := "test-vault-123"

	code1 := GeneratePairingCode(masterKey, vaultID)

	// Wait for next window (60 seconds)
	time.Sleep(60 * time.Second)

	code2 := GeneratePairingCode(masterKey, vaultID)

	// Codes should be different in different windows
	assert.NotEqual(t, code1, code2)

	// Old code should still verify (window-1 tolerance)
	assert.True(t, VerifyPairingCode(masterKey, vaultID, code1))
	assert.True(t, VerifyPairingCode(masterKey, vaultID, code2))
}

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
