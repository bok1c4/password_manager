package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/bok1c4/pwman/internal/api"
	"github.com/bok1c4/pwman/internal/state"
)

func setupTestEnvironment(t *testing.T) (string, func()) {
	tmpDir, err := os.MkdirTemp("", "pwman-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)

	cleanup := func() {
		os.Setenv("HOME", oldHome)
		os.RemoveAll(tmpDir)
	}

	return tmpDir, cleanup
}

func TestAuthHandlers_Init(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	s := state.NewServerState()
	am := api.NewAuthManager()
	h := NewAuthHandlers(s, am)

	tests := []struct {
		name       string
		body       map[string]interface{}
		expectCode int
	}{
		{
			name: "valid initialization",
			body: map[string]interface{}{
				"name":     "Test Device",
				"password": "testpassword123",
			},
			expectCode: http.StatusOK,
		},
		{
			name: "missing name",
			body: map[string]interface{}{
				"password": "testpassword123",
			},
			expectCode: http.StatusBadRequest,
		},
		{
			name: "short password",
			body: map[string]interface{}{
				"name":     "Test Device",
				"password": "short",
			},
			expectCode: http.StatusBadRequest,
		},
		{
			name: "missing password",
			body: map[string]interface{}{
				"name": "Test Device",
			},
			expectCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/api/init", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			h.Init(rr, req)

			if rr.Code != tt.expectCode {
				t.Errorf("expected status %d, got %d. Body: %s", tt.expectCode, rr.Code, rr.Body.String())
			}

			if tt.expectCode == http.StatusOK {
				var resp map[string]interface{}
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to parse response: %v", err)
				}

				data, ok := resp["data"].(map[string]interface{})
				if !ok {
					t.Fatal("expected data object in response")
				}

				if _, ok := data["device_id"]; !ok {
					t.Error("expected device_id in response")
				}

				if !s.IsUnlocked() {
					t.Error("expected vault to be unlocked after init")
				}
			}
		})
	}
}

func TestAuthHandlers_InitAlreadyInitialized(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	s := state.NewServerState()
	am := api.NewAuthManager()
	h := NewAuthHandlers(s, am)

	// First init
	body := map[string]interface{}{
		"name":     "Test Device",
		"password": "testpassword123",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/init", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Init(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("first init failed: %d", rr.Code)
	}

	// Clear vault to test re-init
	s.ClearVault()

	// Try to init again
	req2 := httptest.NewRequest("POST", "/api/init", bytes.NewReader(bodyBytes))
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()

	h.Init(rr2, req2)

	if rr2.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for already initialized vault, got %d", rr2.Code)
	}
}

func TestAuthHandlers_UnlockLock(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	s := state.NewServerState()
	am := api.NewAuthManager()
	h := NewAuthHandlers(s, am)

	// Initialize vault first
	initBody := map[string]interface{}{
		"name":     "Test Device",
		"password": "testpassword123",
	}
	initBytes, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest("POST", "/api/init", bytes.NewReader(initBytes))
	initReq.Header.Set("Content-Type", "application/json")
	initRR := httptest.NewRecorder()
	h.Init(initRR, initReq)

	if initRR.Code != http.StatusOK {
		t.Fatalf("init failed: %d", initRR.Code)
	}

	// Lock the vault
	s.ClearVault()
	am.SetVaultUnlocked(false)

	// Test unlock
	tests := []struct {
		name       string
		password   string
		expectCode int
	}{
		{
			name:       "correct password",
			password:   "testpassword123",
			expectCode: http.StatusOK,
		},
		{
			name:       "wrong password",
			password:   "wrongpassword",
			expectCode: http.StatusUnauthorized,
		},
		{
			name:       "empty password",
			password:   "",
			expectCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unlockBody := map[string]interface{}{
				"password": tt.password,
			}
			unlockBytes, _ := json.Marshal(unlockBody)
			req := httptest.NewRequest("POST", "/api/unlock", bytes.NewReader(unlockBytes))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			h.Unlock(rr, req)

			if rr.Code != tt.expectCode {
				t.Errorf("expected status %d, got %d. Body: %s", tt.expectCode, rr.Code, rr.Body.String())
			}

			if tt.expectCode == http.StatusOK {
				var resp map[string]interface{}
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to parse response: %v", err)
				}

				data, ok := resp["data"].(map[string]interface{})
				if !ok {
					t.Fatal("expected data object in response")
				}

				if _, ok := data["token"]; !ok {
					t.Error("expected token in response")
				}

				if !s.IsUnlocked() {
					t.Error("expected vault to be unlocked")
				}

				// Lock for next test
				s.ClearVault()
				am.SetVaultUnlocked(false)
			}
		})
	}
}

func TestAuthHandlers_Lock(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	s := state.NewServerState()
	am := api.NewAuthManager()
	h := NewAuthHandlers(s, am)

	// Initialize and unlock
	initBody := map[string]interface{}{
		"name":     "Test Device",
		"password": "testpassword123",
	}
	initBytes, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest("POST", "/api/init", bytes.NewReader(initBytes))
	initReq.Header.Set("Content-Type", "application/json")
	initRR := httptest.NewRecorder()
	h.Init(initRR, initReq)

	if !s.IsUnlocked() {
		t.Fatal("vault should be unlocked after init")
	}

	// Lock
	req := httptest.NewRequest("POST", "/api/lock", nil)
	rr := httptest.NewRecorder()

	h.Lock(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	if s.IsUnlocked() {
		t.Error("expected vault to be locked")
	}
}

func TestAuthHandlers_IsUnlocked(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	s := state.NewServerState()
	am := api.NewAuthManager()
	h := NewAuthHandlers(s, am)

	// Check unlocked status when locked
	req := httptest.NewRequest("GET", "/api/is_unlocked", nil)
	rr := httptest.NewRecorder()

	h.IsUnlocked(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data object in response")
	}

	if unlocked, ok := data["unlocked"].(bool); !ok || unlocked {
		t.Error("expected unlocked to be false")
	}

	// Initialize and check again
	initBody := map[string]interface{}{
		"name":     "Test Device",
		"password": "testpassword123",
	}
	initBytes, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest("POST", "/api/init", bytes.NewReader(initBytes))
	initReq.Header.Set("Content-Type", "application/json")
	initRR := httptest.NewRecorder()
	h.Init(initRR, initReq)

	req2 := httptest.NewRequest("GET", "/api/is_unlocked", nil)
	rr2 := httptest.NewRecorder()

	h.IsUnlocked(rr2, req2)

	json.Unmarshal(rr2.Body.Bytes(), &resp)
	data = resp["data"].(map[string]interface{})

	if unlocked, ok := data["unlocked"].(bool); !ok || !unlocked {
		t.Error("expected unlocked to be true after init")
	}
}

func TestAuthHandlers_IsInitialized(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	s := state.NewServerState()
	am := api.NewAuthManager()
	h := NewAuthHandlers(s, am)

	// Check when not initialized
	req := httptest.NewRequest("GET", "/api/is_initialized", nil)
	rr := httptest.NewRecorder()

	h.IsInitialized(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data object in response")
	}

	if initialized, ok := data["initialized"].(bool); !ok || initialized {
		t.Error("expected initialized to be false")
	}

	// Initialize and check again
	initBody := map[string]interface{}{
		"name":     "Test Device",
		"password": "testpassword123",
	}
	initBytes, _ := json.Marshal(initBody)
	initReq := httptest.NewRequest("POST", "/api/init", bytes.NewReader(initBytes))
	initReq.Header.Set("Content-Type", "application/json")
	initRR := httptest.NewRecorder()
	h.Init(initRR, initReq)

	req2 := httptest.NewRequest("GET", "/api/is_initialized", nil)
	rr2 := httptest.NewRecorder()

	h.IsInitialized(rr2, req2)

	json.Unmarshal(rr2.Body.Bytes(), &resp)
	data = resp["data"].(map[string]interface{})

	if initialized, ok := data["initialized"].(bool); !ok || !initialized {
		t.Error("expected initialized to be true after init")
	}
}
