package api

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

type AuthManager struct {
	mu            sync.RWMutex
	tokens        map[string]time.Time
	vaultUnlocked bool
}

func NewAuthManager() *AuthManager {
	return &AuthManager{
		tokens: make(map[string]time.Time),
	}
}

func (a *AuthManager) GenerateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	token := base64.URLEncoding.EncodeToString(b)

	a.mu.Lock()
	a.tokens[token] = time.Now().Add(24 * time.Hour)
	a.mu.Unlock()

	return token
}

func (a *AuthManager) ValidateToken(token string) bool {
	a.mu.RLock()
	expiry, exists := a.tokens[token]
	a.mu.RUnlock()

	if !exists || time.Now().After(expiry) {
		return false
	}
	return true
}

func (a *AuthManager) SetVaultUnlocked(unlocked bool) {
	a.mu.Lock()
	a.vaultUnlocked = unlocked
	a.mu.Unlock()
}

func (a *AuthManager) IsVaultUnlocked() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.vaultUnlocked
}

func (a *AuthManager) CleanupExpiredTokens() {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for token, expiry := range a.tokens {
		if now.After(expiry) {
			delete(a.tokens, token)
		}
	}
}
