package api

import (
	"testing"
	"time"
)

func TestNewAuthManager(t *testing.T) {
	auth := NewAuthManager()

	if auth == nil {
		t.Fatal("NewAuthManager returned nil")
	}

	if auth.tokens == nil {
		t.Error("tokens map is nil")
	}

	if auth.vaultUnlocked {
		t.Error("vaultUnlocked should be false initially")
	}
}

func TestGenerateToken(t *testing.T) {
	auth := NewAuthManager()

	token1 := auth.GenerateToken()
	if token1 == "" {
		t.Error("GenerateToken returned empty token")
	}

	token2 := auth.GenerateToken()
	if token1 == token2 {
		t.Error("GenerateToken should return unique tokens")
	}

	if !auth.ValidateToken(token1) {
		t.Error("Generated token should be valid")
	}
}

func TestValidateToken(t *testing.T) {
	auth := NewAuthManager()

	validToken := auth.GenerateToken()

	if !auth.ValidateToken(validToken) {
		t.Error("Valid token should pass validation")
	}

	invalidToken := "invalid-token-12345"
	if auth.ValidateToken(invalidToken) {
		t.Error("Invalid token should fail validation")
	}

	emptyToken := ""
	if auth.ValidateToken(emptyToken) {
		t.Error("Empty token should fail validation")
	}
}

func TestValidateTokenExpiration(t *testing.T) {
	auth := NewAuthManager()

	token := auth.GenerateToken()

	auth.mu.Lock()
	auth.tokens[token] = time.Now().Add(-1 * time.Hour)
	auth.mu.Unlock()

	if auth.ValidateToken(token) {
		t.Error("Expired token should fail validation")
	}
}

func TestSetVaultUnlocked(t *testing.T) {
	auth := NewAuthManager()

	if auth.IsVaultUnlocked() {
		t.Error("Vault should be locked initially")
	}

	auth.SetVaultUnlocked(true)
	if !auth.IsVaultUnlocked() {
		t.Error("Vault should be unlocked after SetVaultUnlocked(true)")
	}

	auth.SetVaultUnlocked(false)
	if auth.IsVaultUnlocked() {
		t.Error("Vault should be locked after SetVaultUnlocked(false)")
	}
}

func TestCleanupExpiredTokens(t *testing.T) {
	auth := NewAuthManager()

	validToken := auth.GenerateToken()

	auth.mu.Lock()
	auth.tokens["expired1"] = time.Now().Add(-1 * time.Hour)
	auth.tokens["expired2"] = time.Now().Add(-30 * time.Minute)
	auth.mu.Unlock()

	auth.CleanupExpiredTokens()

	if !auth.ValidateToken(validToken) {
		t.Error("Valid token should still be valid after cleanup")
	}

	auth.mu.RLock()
	_, exists1 := auth.tokens["expired1"]
	_, exists2 := auth.tokens["expired2"]
	auth.mu.RUnlock()

	if exists1 {
		t.Error("Expired token 1 should be removed")
	}
	if exists2 {
		t.Error("Expired token 2 should be removed")
	}
}

func TestTokenUniqueness(t *testing.T) {
	auth := NewAuthManager()

	tokens := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token := auth.GenerateToken()
		if tokens[token] {
			t.Errorf("Duplicate token generated at iteration %d", i)
		}
		tokens[token] = true
	}
}

func TestConcurrentTokenGeneration(t *testing.T) {
	auth := NewAuthManager()

	done := make(chan string, 100)
	for i := 0; i < 100; i++ {
		go func() {
			token := auth.GenerateToken()
			done <- token
		}()
	}

	tokens := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token := <-done
		if tokens[token] {
			t.Errorf("Duplicate token generated in concurrent test")
		}
		tokens[token] = true
	}
}

func TestTokenLength(t *testing.T) {
	auth := NewAuthManager()

	token := auth.GenerateToken()

	if len(token) < 40 {
		t.Errorf("Token length = %d, expected at least 40 characters", len(token))
	}
}
