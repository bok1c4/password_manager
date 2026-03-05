package main

import (
	"sync"
	"testing"
	"time"

	"github.com/bok1c4/pwman/internal/state"
)

func TestPairingCodeValidation(t *testing.T) {
	pairingCodes := make(map[string]state.PairingCode)
	var pairingLock sync.Mutex

	validCode := "VALID12"
	expiredCode := "EXPIRED1"
	usedCode := "USEDCODE1"
	nonexistentCode := "NOTEXIST"

	validPairingCode := state.PairingCode{
		Code:       validCode,
		VaultID:    "vault-1",
		DeviceID:   "device-1",
		DeviceName: "New Device",
		ExpiresAt:  time.Now().Add(10 * time.Minute),
		Used:       false,
	}

	expiredPairingCode := state.PairingCode{
		Code:      expiredCode,
		VaultID:   "vault-1",
		DeviceID:  "device-2",
		ExpiresAt: time.Now().Add(-1 * time.Minute),
		Used:      false,
	}

	usedPairingCode := state.PairingCode{
		Code:      usedCode,
		VaultID:   "vault-1",
		DeviceID:  "device-3",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Used:      true,
	}

	pairingCodes[validCode] = validPairingCode
	pairingCodes[expiredCode] = expiredPairingCode
	pairingCodes[usedCode] = usedPairingCode

	tests := []struct {
		name        string
		code        string
		wantErr     bool
		wantErrType string
	}{
		{
			name:        "valid code",
			code:        validCode,
			wantErr:     false,
			wantErrType: "",
		},
		{
			name:        "nonexistent code",
			code:        nonexistentCode,
			wantErr:     true,
			wantErrType: "invalid_code",
		},
		{
			name:        "expired code",
			code:        expiredCode,
			wantErr:     true,
			wantErrType: "code_expired",
		},
		{
			name:        "already used code",
			code:        usedCode,
			wantErr:     true,
			wantErrType: "code_already_used",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pairingLock.Lock()
			code, exists := pairingCodes[tt.code]

			var errType string
			if !exists {
				errType = "invalid_code"
			} else if code.Used {
				errType = "code_already_used"
			} else if time.Now().After(code.ExpiresAt) {
				errType = "code_expired"
			} else {
				code.Used = true
				pairingCodes[tt.code] = code
			}
			pairingLock.Unlock()

			if tt.wantErr && errType != tt.wantErrType {
				t.Errorf("errType = %q, want %q", errType, tt.wantErrType)
			}
			if !tt.wantErr && errType != "" {
				t.Errorf("expected no error, got %q", errType)
			}
		})
	}
}

func TestPairingCodeMarksAsUsed(t *testing.T) {
	pairingCodes := make(map[string]state.PairingCode)
	var pairingLock sync.Mutex

	code := "TESTCODE1"
	pairingCodes[code] = state.PairingCode{
		Code:      code,
		VaultID:   "vault-1",
		DeviceID:  "device-1",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Used:      false,
	}

	pairingLock.Lock()
	storedCode := pairingCodes[code]
	if storedCode.Used {
		t.Error("Code should not be used initially")
	}
	storedCode.Used = true
	pairingCodes[code] = storedCode
	pairingLock.Unlock()

	pairingLock.Lock()
	updatedCode := pairingCodes[code]
	pairingLock.Unlock()

	if !updatedCode.Used {
		t.Error("Code should be marked as used after validation")
	}
}

func TestPairingCodeExpirationCheck(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		expiresAt time.Time
		isExpired bool
	}{
		{
			name:      "not expired",
			expiresAt: now.Add(10 * time.Minute),
			isExpired: false,
		},
		{
			name:      "expired",
			expiresAt: now.Add(-1 * time.Minute),
			isExpired: true,
		},
		{
			name:      "exactly now",
			expiresAt: now,
			isExpired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := state.PairingCode{
				Code:      "test",
				ExpiresAt: tt.expiresAt,
			}
			isExpired := time.Now().After(code.ExpiresAt)
			if isExpired != tt.isExpired {
				t.Errorf("isExpired = %v, want %v", isExpired, tt.isExpired)
			}
		})
	}
}

func TestPairingCodeConcurrentAccess(t *testing.T) {
	pairingCodes := make(map[string]state.PairingCode)
	var pairingLock sync.Mutex

	code := "CONCURRENT1"
	pairingCodes[code] = state.PairingCode{
		Code:      code,
		VaultID:   "vault-1",
		DeviceID:  "device-1",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Used:      false,
	}

	var wg sync.WaitGroup
	numGoroutines := 100
	successChan := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pairingLock.Lock()
			c, exists := pairingCodes[code]
			if !exists {
				pairingLock.Unlock()
				return
			}
			if c.Used {
				pairingLock.Unlock()
				return
			}
			c.Used = true
			pairingCodes[code] = c
			successChan <- true
			pairingLock.Unlock()
		}()
	}

	wg.Wait()
	close(successChan)

	successCount := 0
	for range successChan {
		successCount++
	}

	if successCount == 0 {
		t.Error("At least one goroutine should have succeeded")
	}

	if successCount > 1 {
		t.Errorf("Expected at most 1 successful validation, got %d", successCount)
	}
}
