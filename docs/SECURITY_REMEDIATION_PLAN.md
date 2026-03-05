# Security & Architecture Remediation Plan

**Project**: Password Manager  
**Document Version**: 1.0  
**Last Updated**: 2026-03-05  
**Priority**: CRITICAL  

---

## Executive Summary

This document outlines critical security vulnerabilities and architectural issues discovered during a comprehensive audit of the password manager codebase. The issues range from **CRITICAL** (must fix immediately) to **LOW** (backlog). 

**Status**: This codebase is NOT production-ready due to multiple CRITICAL security vulnerabilities.

---

## Phase 1: CRITICAL — Fix Immediately (Week 1)

These issues pose immediate security risks and must be addressed before any production deployment.

### 1.1 CORS Misconfiguration

**Severity**: CRITICAL  
**Files**: `cmd/server/main.go:140-142`, `cmd/server/main.go:148-150`  
**Issue**: API server accepts requests from any origin (`*`)

```go
// CURRENT (VULNERABLE)
w.Header().Set("Access-Control-Allow-Origin", "*")
w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
```

**Impact**: 
- Any malicious website can make requests to localhost:18475
- CSRF attacks possible
- Password theft via XSS from any domain

**Implementation**:
```go
// SECURE - Only allow Tauri app origins
allowedOrigins := []string{
    "tauri://localhost",
    "https://tauri.localhost",
    "http://localhost:1420", // Dev server
}

func corsHandler(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        origin := r.Header.Get("Origin")
        allowed := false
        for _, o := range allowedOrigins {
            if origin == o {
                allowed = true
                w.Header().Set("Access-Control-Allow-Origin", origin)
                break
            }
        }
        
        if !allowed && origin != "" {
            http.Error(w, "Forbidden", http.StatusForbidden)
            return
        }
        
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        w.Header().Set("Vary", "Origin")
        
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        
        next(w, r)
    }
}
```

**Testing**:
- Test from browser console: `fetch('http://localhost:18475/api/entries')` should fail from random origin
- Test from Tauri app should succeed

---

### 1.2 API Authentication Missing

**Severity**: CRITICAL  
**Files**: All `/api/*` handlers in `cmd/server/main.go`  
**Issue**: No authentication on any API endpoint

**Impact**: Any process on the machine can:
- Read all encrypted passwords
- Decrypt passwords
- Delete vaults
- Approve malicious devices

**Implementation**:

1. **Create auth middleware** (new file: `internal/api/auth.go`):
```go
package api

import (
    "crypto/rand"
    "encoding/base64"
    "net/http"
    "sync"
    "time"
)

type AuthManager struct {
    mu        sync.RWMutex
    tokens    map[string]time.Time // token -> expiry
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
```

2. **Update handlers to require auth**:
```go
func requireAuth(auth *api.AuthManager, next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Skip auth for unlock and init endpoints
        if r.URL.Path == "/api/unlock" || r.URL.Path == "/api/init" {
            next(w, r)
            return
        }
        
        token := r.Header.Get("Authorization")
        if token == "" {
            jsonResponse(w, Response{Success: false, Error: "authentication required"})
            return
        }
        
        // Remove "Bearer " prefix if present
        if len(token) > 7 && token[:7] == "Bearer " {
            token = token[7:]
        }
        
        if !auth.ValidateToken(token) {
            jsonResponse(w, Response{Success: false, Error: "invalid token"})
            return
        }
        
        next(w, r)
    }
}

// In handleUnlock, generate and return token on success:
func handleUnlock(w http.ResponseWriter, r *http.Request) {
    // ... existing unlock logic ...
    
    if err != nil {
        jsonResponse(w, Response{Success: false, Error: "wrong password"})
        return
    }
    
    token := authManager.GenerateToken()
    authManager.SetVaultUnlocked(true)
    
    jsonResponse(w, Response{
        Success: true, 
        Data: map[string]string{"token": token},
    })
}
```

3. **Frontend must send token**:
```typescript
// src/lib/api.ts
let authToken: string | null = null;

async function apiCall(endpoint: string, options?: RequestInit): Promise<any> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };
  
  if (authToken) {
    headers['Authorization'] = `Bearer ${authToken}`;
  }
  
  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    headers,
  });
  // ... rest
}

export function setAuthToken(token: string) {
  authToken = token;
}

export function clearAuthToken() {
  authToken = null;
}
```

---

### 1.3 Remove Password from P2P Protocol

**Severity**: CRITICAL  
**Files**: 
- `internal/p2p/messages.go:214-220`
- `cmd/server/main.go:1593`
- `cmd/server/main.go:1668`

**Issue**: Vault password transmitted over P2P network

**Implementation**:

1. **Remove password field from PairingRequestPayload**:
```go
// internal/p2p/messages.go
// REMOVE: Password string from PairingRequestPayload

type PairingRequestPayload struct {
    Code       string `json:"code"`
    DeviceID   string `json:"device_id"`
    DeviceName string `json:"device_name"`
    PublicKey  string `json:"public_key"`
    // REMOVED: Password field
}
```

2. **Update CreatePairingRequestMessage**:
```go
func CreatePairingRequestMessage(code, deviceID, deviceName, publicKey string) (*Message, error) {
    payload := PairingRequestPayload{
        Code:       code,
        DeviceID:   deviceID,
        DeviceName: deviceName,
        PublicKey:  publicKey,
    }
    return NewMessage(MsgTypePairingRequest, payload)
}
```

3. **Update all call sites**:
- Remove password parameter from `CreatePairingRequestMessage` calls
- Remove `joiningPassword` from pairing request message creation
- Use public key verification only for authentication

4. **Alternative authentication**: Use PAKE (Password Authenticated Key Exchange) or QR code scanning for pairing instead of password transmission.

**Testing**:
- Verify password never appears in P2P message logs
- Verify pairing still works with public key exchange only

---

### 1.4 Fix Vault Password Logging

**Severity**: CRITICAL  
**File**: `cmd/server/main.go:1593`

**Current**:
```go
log.Printf("[Pairing Join] Looking for code: '%s' via P2P (password provided: %v)", req.Code, req.Password != "")
```

**Fix**:
```go
// Remove password presence logging entirely
log.Printf("[Pairing Join] Looking for code: '%s' via P2P", req.Code)
```

**Also check**:
- Search for any `log.Printf` with password, key, or secret terms
- Ensure no debug logs expose sensitive data

---

### 1.5 Increase RSA Key Size

**Severity**: CRITICAL  
**Files**: 
- `cmd/server/main.go:230`
- `cmd/server/main.go:1758`

**Current**:
```go
keyPair, err := crypto.GenerateRSAKeyPair(2048)
```

**Fix**:
```go
keyPair, err := crypto.GenerateRSAKeyPair(4096)
```

**Considerations**:
- 4096-bit RSA is ~5x slower than 2048-bit
- For new systems, consider X25519 (ECDH) which is faster and more secure
- Migration path needed for existing 2048-bit keys

**Alternative (Future)**:
```go
// Consider migrating to Ed25519 for signatures + X25519 for key exchange
// This would require protocol changes but is more modern
```

---

## Phase 2: HIGH — Fix This Week (Week 2)

### 2.1 Fix Race Conditions in Vault Access

**Severity**: HIGH  
**Files**: `cmd/server/main.go` (multiple locations)  

**Issues**:
1. `vault.privateKey` accessed without lock in some paths
2. `vault.storage` accessed outside critical sections
3. Channel operations may race with vault state changes

**Implementation**:

1. **Create vault manager** (new file: `internal/vault/manager.go`):
```go
package vault

import (
    "sync"
    "github.com/bok1c4/pwman/internal/config"
    "github.com/bok1c4/pwman/internal/crypto"
    "github.com/bok1c4/pwman/internal/storage"
)

type Manager struct {
    mu         sync.RWMutex
    vault      *Vault
    isUnlocked bool
}

type Vault struct {
    privateKey *crypto.KeyPair
    storage    *storage.SQLite
    cfg        *config.VaultConfig
    vaultName  string
}

func NewManager() *Manager {
    return &Manager{}
}

func (m *Manager) Lock() {
    m.mu.Lock()
}

func (m *Manager) Unlock() {
    m.mu.Unlock()
}

func (m *Manager) RLock() {
    m.mu.RLock()
}

func (m *Manager) RUnlock() {
    m.mu.RUnlock()
}

func (m *Manager) GetVault() (*Vault, bool) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    if m.vault == nil {
        return nil, false
    }
    return m.vault, true
}

func (m *Manager) SetVault(v *Vault) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.vault = v
    m.isUnlocked = v != nil
}

func (m *Manager) IsUnlocked() bool {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.isUnlocked
}

func (m *Manager) Close() error {
    m.mu.Lock()
    defer m.mu.Unlock()
    if m.vault != nil && m.vault.storage != nil {
        err := m.vault.storage.Close()
        m.vault = nil
        m.isUnlocked = false
        return err
    }
    return nil
}

// Vault methods (no locking - caller must hold lock)
func (v *Vault) GetPrivateKey() *crypto.KeyPair {
    return v.privateKey
}

func (v *Vault) GetStorage() *storage.SQLite {
    return v.storage
}

func (v *Vault) GetConfig() *config.VaultConfig {
    return v.cfg
}

func (v *Vault) GetVaultName() string {
    return v.vaultName
}
```

2. **Update server to use manager**:
```go
// Replace global vault var with manager
var vaultManager = vault.NewManager()

// Update all handlers to use proper locking
func handleUnlock(w http.ResponseWriter, r *http.Request) {
    // ... unlock logic ...
    
    v := &vault.Vault{
        privateKey: keyPair,
        storage:    db,
        cfg:        cfg,
        vaultName:  vaultName,
    }
    vaultManager.SetVault(v)
}

func handleGetPassword(w http.ResponseWriter, r *http.Request) {
    v, ok := vaultManager.GetVault()
    if !ok {
        jsonResponse(w, Response{Success: false, Error: "vault not unlocked"})
        return
    }
    
    // Access vault only through v, no lock needed during operation
    // but be careful about concurrent access to storage
    entry, err := v.GetStorage().GetEntry(entryID)
    // ... rest of logic ...
}
```

---

### 2.2 Fix Goroutine Leaks

**Severity**: HIGH  
**Files**: `cmd/server/main.go:1079-1101` and similar

**Issue**: Goroutines spawned for P2P handlers never stop

**Implementation**:

1. **Use context for cancellation**:
```go
// In handleP2PStart
ctx, cancel := context.WithCancel(context.Background())
p2pCancel = cancel // Store globally

go func(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case peer := <-p2pManager.ConnectedChan():
            // handle peer
        // ... other cases ...
        }
    }
}(ctx)
```

2. **Cancel on stop**:
```go
func handleP2PStop(w http.ResponseWriter, r *http.Request) {
    if p2pCancel != nil {
        p2pCancel()
    }
    // ... rest of stop logic ...
}
```

3. **Close channels properly**:
```go
// Ensure all channels are closed in Stop()
func (p *P2PManager) Stop() {
    p.cancel()
    
    if p.discovery != nil {
        p.discovery.Close()
    }
    
    if p.host != nil {
        p.host.Close()
    }
    
    // Close channels safely
    close(p.messageChan)
    // ... close other channels ...
}
```

---

### 2.3 Add Input Validation

**Severity**: HIGH  
**Files**: All API handlers in `cmd/server/main.go`

**Implementation**:

1. **Create validation utilities** (new file: `internal/api/validation.go`):
```go
package api

import (
    "errors"
    "regexp"
    "strings"
)

const (
    MaxSiteLength     = 256
    MaxUsernameLength = 256
    MaxPasswordLength = 4096
    MaxNotesLength    = 10000
    MaxDeviceName     = 100
    MinPasswordLength = 1
)

var (
    ErrInvalidSite     = errors.New("invalid site")
    ErrInvalidUsername = errors.New("invalid username")
    ErrInvalidPassword = errors.New("invalid password")
    ErrInvalidNotes    = errors.New("invalid notes")
    ErrSiteTooLong     = errors.New("site name too long")
    ErrUsernameTooLong = errors.New("username too long")
    ErrPasswordTooLong = errors.New("password too long")
    ErrNotesTooLong    = errors.New("notes too long")
)

// SanitizeString removes control characters and trims whitespace
func SanitizeString(s string, maxLen int) (string, error) {
    s = strings.TrimSpace(s)
    if len(s) > maxLen {
        return "", errors.New("string too long")
    }
    // Remove control characters except newlines and tabs in notes
    sanitized := strings.Map(func(r rune) rune {
        if r < 32 && r != '\n' && r != '\t' {
            return -1
        }
        return r
    }, s)
    return sanitized, nil
}

func ValidateSite(site string) (string, error) {
    if site == "" {
        return "", ErrInvalidSite
    }
    return SanitizeString(site, MaxSiteLength)
}

func ValidateUsername(username string) (string, error) {
    return SanitizeString(username, MaxUsernameLength)
}

func ValidatePassword(password string) error {
    if len(password) < MinPasswordLength {
        return ErrInvalidPassword
    }
    if len(password) > MaxPasswordLength {
        return ErrPasswordTooLong
    }
    return nil
}

func ValidateNotes(notes string) (string, error) {
    return SanitizeString(notes, MaxNotesLength)
}

func ValidateDeviceName(name string) (string, error) {
    if name == "" {
        return "", errors.New("device name required")
    }
    return SanitizeString(name, MaxDeviceName)
}
```

2. **Apply validation in handlers**:
```go
func handleAddEntry(w http.ResponseWriter, r *http.Request) {
    // ... unlock check ...
    
    type AddRequest struct {
        Site     string `json:"site"`
        Username string `json:"username"`
        Password string `json:"password"`
        Notes    string `json:"notes"`
    }
    
    var req AddRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        jsonResponse(w, Response{Success: false, Error: err.Error()})
        return
    }
    
    // Validate inputs
    site, err := api.ValidateSite(req.Site)
    if err != nil {
        jsonResponse(w, Response{Success: false, Error: err.Error()})
        return
    }
    
    username, err := api.ValidateUsername(req.Username)
    if err != nil {
        jsonResponse(w, Response{Success: false, Error: err.Error()})
        return
    }
    
    if err := api.ValidatePassword(req.Password); err != nil {
        jsonResponse(w, Response{Success: false, Error: err.Error()})
        return
    }
    
    notes, err := api.ValidateNotes(req.Notes)
    if err != nil {
        jsonResponse(w, Response{Success: false, Error: err.Error()})
        return
    }
    
    // Use validated values
    req.Site = site
    req.Username = username
    req.Notes = notes
    
    // ... rest of handler ...
}
```

---

### 2.4 Fix Clipboard Clearing

**Severity**: HIGH  
**File**: `internal/cli/get.go:59-71`

**Current**:
```go
go func() {
    time.Sleep(30 * time.Second)
    clipboard.WriteAll("")
}()
```

**Fix**:
```go
func clearClipboardAfter(password string, duration time.Duration) {
    time.Sleep(duration)
    // Only clear if clipboard still contains our password
    current, _ := clipboard.ReadAll()
    if current == password {
        clipboard.WriteAll("")
    }
}

// In handler:
if getClipboard {
    if err := clipboard.WriteAll(pass); err != nil {
        fmt.Printf("[ERROR] Failed to copy to clipboard: %v\n", err)
        os.Exit(1)
    }
    fmt.Printf("[INFO] Password for %s copied to clipboard (clears in 30 seconds)\n", site)
    
    // Create cancellable cleanup
    ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
    defer cancel()
    
    go func() {
        select {
        case <-ctx.Done():
            clearClipboardAfter(pass, 0) // Clear immediately
        case <-time.After(30 * time.Second):
            clearClipboardAfter(pass, 0)
        }
    }()
} else {
    fmt.Printf("[INFO] Password for %s:\n%s\n", site, pass)
}
```

**Alternative**: Use `defer` with panic recovery to ensure cleanup:
```go
func copyToClipboard(password string) error {
    if err := clipboard.WriteAll(password); err != nil {
        return err
    }
    
    // Ensure cleanup happens even on panic
    defer func() {
        if r := recover(); r != nil {
            clipboard.WriteAll("")
            panic(r)
        }
    }()
    
    go func() {
        time.Sleep(30 * time.Second)
        current, _ := clipboard.ReadAll()
        if current == password {
            clipboard.WriteAll("")
        }
    }()
    
    return nil
}
```

---

### 2.5 Make Server Port Configurable

**Severity**: HIGH  
**File**: `cmd/server/main.go:26-33`

**Current**:
```go
var serverPort = os.Getenv("PWMAN_PORT")

func init() {
    if serverPort == "" {
        serverPort = "18475"
    }
}
```

**Fix**:
```go
var (
    serverPort      = os.Getenv("PWMAN_PORT")
    serverPortRange = os.Getenv("PWMAN_PORT_RANGE") // e.g., "18475-18575"
)

func findAvailablePort() (string, error) {
    if serverPort != "" {
        // Check if specified port is available
        ln, err := net.Listen("tcp", ":"+serverPort)
        if err != nil {
            return "", fmt.Errorf("port %s is not available: %w", serverPort, err)
        }
        ln.Close()
        return serverPort, nil
    }
    
    // Try to find available port in range
    if serverPortRange != "" {
        parts := strings.Split(serverPortRange, "-")
        if len(parts) == 2 {
            start, _ := strconv.Atoi(parts[0])
            end, _ := strconv.Atoi(parts[1])
            for port := start; port <= end; port++ {
                ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
                if err == nil {
                    ln.Close()
                    return strconv.Itoa(port), nil
                }
            }
        }
    }
    
    // Use random high port
    ln, err := net.Listen("tcp", ":0")
    if err != nil {
        return "", err
    }
    port := ln.Addr().(*net.TCPAddr).Port
    ln.Close()
    return strconv.Itoa(port), nil
}

// Store the actual port for frontend to use
var actualServerPort string

func main() {
    port, err := findAvailablePort()
    if err != nil {
        log.Fatalf("Failed to find available port: %v", err)
    }
    actualServerPort = port
    
    log.Printf("Starting pwman API server on :%s", actualServerPort)
    log.Fatal(http.ListenAndServe(":"+actualServerPort, nil))
}
```

**Frontend update**:
```typescript
// src/lib/api.ts
// Read port from environment or config
const API_PORT = import.meta.env.VITE_API_PORT || '18475';
const API_BASE = `http://localhost:${API_PORT}/api`;
```

---

## Phase 3: MEDIUM — Fix This Sprint (Weeks 3-4)

### 3.1 Implement Rate Limiting

**Priority**: MEDIUM  
**Scope**: All API endpoints

**Implementation**:

1. **Create rate limiter** (new file: `internal/api/ratelimit.go`):
```go
package api

import (
    "net/http"
    "sync"
    "time"
    
    "golang.org/x/time/rate"
)

type RateLimiter struct {
    mu       sync.RWMutex
    limiters map[string]*rate.Limiter
    
    // Config
    rps    rate.Limit
    burst  int
    expiry time.Duration
}

func NewRateLimiter(rps rate.Limit, burst int) *RateLimiter {
    rl := &RateLimiter{
        limiters: make(map[string]*rate.Limiter),
        rps:      rps,
        burst:    burst,
        expiry:   10 * time.Minute,
    }
    
    // Cleanup old entries periodically
    go rl.cleanup()
    
    return rl
}

func (rl *RateLimiter) cleanup() {
    ticker := time.NewTicker(rl.expiry)
    defer ticker.Stop()
    
    for range ticker.C {
        rl.mu.Lock()
        // In production, track last access time and remove old entries
        rl.mu.Unlock()
    }
}

func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    
    limiter, exists := rl.limiters[key]
    if !exists {
        limiter = rate.NewLimiter(rl.rps, rl.burst)
        rl.limiters[key] = limiter
    }
    
    return limiter
}

func (rl *RateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Use IP + endpoint as key
        key := r.RemoteAddr + r.URL.Path
        
        limiter := rl.getLimiter(key)
        if !limiter.Allow() {
            w.WriteHeader(http.StatusTooManyRequests)
            json.NewEncoder(w).Encode(Response{
                Success: false,
                Error:   "rate limit exceeded",
            })
            return
        }
        
        next(w, r)
    }
}
```

2. **Apply to specific endpoints**:
```go
// Different limits for different endpoints
var (
    generalLimiter = api.NewRateLimiter(10, 20)     // 10 req/sec
    authLimiter    = api.NewRateLimiter(1, 5)       // 1 req/sec (brute force protection)
    p2pLimiter     = api.NewRateLimiter(5, 10)      // 5 req/sec
)

http.HandleFunc("/api/unlock", corsHandler(authLimiter.Middleware(handleUnlock)))
http.HandleFunc("/api/entries/add", corsHandler(generalLimiter.Middleware(handleAddEntry)))
```

---

### 3.2 Fix Pairing Code Reuse

**Priority**: MEDIUM  
**File**: `cmd/server/main.go:2234-2241`

**Current Issue**: `code.Used` is set but not always checked

**Fix**:
```go
pairingLock.Lock()
code, exists := pairingCodes[pairingReq.Code]
if exists && code.Used {
    pairingLock.Unlock()
    response = p2p.PairingResponsePayload{
        Success: false, 
        Error: "code_already_used",
    }
    // send response and return
    return
}

if !exists {
    pairingLock.Unlock()
    response = p2p.PairingResponsePayload{Success: false, Error: "invalid_code"}
} else if time.Now().After(code.ExpiresAt) {
    pairingLock.Unlock()
    response = p2p.PairingResponsePayload{Success: false, Error: "code_expired"}
} else {
    code.Used = true
    pairingCodes[pairingReq.Code] = code
    pairingLock.Unlock()
    // proceed with approval
}
```

**Additional improvement**: Add attempt tracking:
```go
type PairingCode struct {
    Code        string
    VaultID     string
    DeviceID    string
    DeviceName  string
    PublicKey   string
    Fingerprint string
    ExpiresAt   time.Time
    Used        bool
    Attempts    int           // Track failed attempts
    MaxAttempts int           // Lock out after N failures
}

// Check attempts
if code.Attempts >= code.MaxAttempts {
    response = p2p.PairingResponsePayload{
        Success: false, 
        Error: "too_many_attempts",
    }
    return
}

// Increment on failure
code.Attempts++
pairingCodes[pairingReq.Code] = code
```

---

### 3.3 Encrypt Vault Metadata

**Priority**: MEDIUM  
**Files**: 
- `cmd/server/main.go:254-255`
- `internal/config/config.go`

**Current**: Vault config stored in plaintext JSON

**Implementation**:

1. **Create encrypted config** (new file: `internal/config/encrypted.go`):
```go
package config

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "io"
    "os"
    
    "golang.org/x/crypto/pbkdf2"
)

type EncryptedConfig struct {
    Salt      string `json:"salt"`
    Nonce     string `json:"nonce"`
    Ciphertext string `json:"ciphertext"`
}

func SaveEncryptedVaultConfig(vaultName string, cfg *VaultConfig, password string) error {
    // Serialize config
    data, err := json.Marshal(cfg)
    if err != nil {
        return err
    }
    
    // Generate salt
    salt := make([]byte, 32)
    if _, err := io.ReadFull(rand.Reader, salt); err != nil {
        return err
    }
    
    // Derive key from password
    key := pbkdf2.Key([]byte(password), salt, 100000, 32, sha256.New)
    
    // Encrypt
    block, err := aes.NewCipher(key)
    if err != nil {
        return err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return err
    }
    
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return err
    }
    
    ciphertext := gcm.Seal(nonce, nonce, data, nil)
    
    // Save encrypted config
    encConfig := EncryptedConfig{
        Salt:       base64.StdEncoding.EncodeToString(salt),
        Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
    }
    
    encData, _ := json.Marshal(encConfig)
    configPath := VaultConfigPath(vaultName) + ".enc"
    return os.WriteFile(configPath, encData, 0600)
}

func LoadEncryptedVaultConfig(vaultName string, password string) (*VaultConfig, error) {
    configPath := VaultConfigPath(vaultName) + ".enc"
    
    data, err := os.ReadFile(configPath)
    if err != nil {
        return nil, err
    }
    
    var encConfig EncryptedConfig
    if err := json.Unmarshal(data, &encConfig); err != nil {
        return nil, err
    }
    
    salt, _ := base64.StdEncoding.DecodeString(encConfig.Salt)
    ciphertext, _ := base64.StdEncoding.DecodeString(encConfig.Ciphertext)
    
    // Derive key
    key := pbkdf2.Key([]byte(password), salt, 100000, 32, sha256.New)
    
    // Decrypt
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    
    nonceSize := gcm.NonceSize()
    if len(ciphertext) < nonceSize {
        return nil, fmt.Errorf("ciphertext too short")
    }
    
    nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
    plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return nil, fmt.Errorf("wrong password")
    }
    
    var cfg VaultConfig
    if err := json.Unmarshal(plaintext, &cfg); err != nil {
        return nil, err
    }
    
    return &cfg, nil
}
```

---

### 3.4 Strengthen Scrypt Parameters

**Priority**: MEDIUM  
**File**: `internal/crypto/crypto.go:216-223`

**Current**:
```go
const (
    N            = 16384
    R            = 8
    P            = 1
    SCRYPTKeyLen = 32
)
```

**Recommended**:
```go
const (
    N            = 32768  // Increase from 16384
    R            = 8
    P            = 1
    SCRYPTKeyLen = 32
)

// Alternative: Higher security (slower)
// N = 65536  // ~400ms on modern hardware
```

**Migration consideration**: 
- Store scrypt params with salt
- On unlock, check params and re-encrypt if outdated
- Gradual migration path for existing vaults

---

### 3.5 Implement Database Integrity Checks

**Priority**: MEDIUM  
**File**: `internal/storage/sqlite.go`

**Implementation**:

1. **Add integrity check method**:
```go
func (s *SQLite) CheckIntegrity() error {
    var result string
    err := s.db.QueryRow("PRAGMA integrity_check").Scan(&result)
    if err != nil {
        return fmt.Errorf("integrity check failed: %w", err)
    }
    if result != "ok" {
        return fmt.Errorf("database corruption detected: %s", result)
    }
    return nil
}
```

2. **Run on startup**:
```go
func NewSQLite(path string) (*SQLite, error) {
    // ... existing code ...
    
    // Run integrity check
    if err := db.CheckIntegrity(); err != nil {
        db.Close()
        return nil, err
    }
    
    // Enable foreign keys
    if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
        db.Close()
        return nil, err
    }
    
    return &SQLite{db: db}, nil
}
```

3. **Add periodic checks**:
```go
func (s *SQLite) StartIntegrityMonitor(interval time.Duration) {
    ticker := time.NewTicker(interval)
    go func() {
        for range ticker.C {
            if err := s.CheckIntegrity(); err != nil {
                log.Printf("[CRITICAL] Database integrity check failed: %v", err)
                // Alert user, initiate recovery
            }
        }
    }()
}
```

---

### 3.6 Fix Public Key Fingerprint

**Priority**: MEDIUM  
**File**: `internal/crypto/crypto.go:84-86`

**Current**:
```go
func GetFingerprint(key *rsa.PublicKey) string {
    return base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(key))
}
```

**Fix** (use proper cryptographic hash):
```go
func GetFingerprint(key *rsa.PublicKey) string {
    keyBytes := x509.MarshalPKCS1PublicKey(key)
    hash := sha256.Sum256(keyBytes)
    return hex.EncodeToString(hash[:])
}
```

**Migration**:
- Existing fingerprints in database will need migration
- Can maintain backward compatibility by checking both formats

---

### 3.7 Implement Soft Delete Purge

**Priority**: MEDIUM  
**File**: `internal/storage/sqlite.go:304-307`

**Implementation**:

1. **Add purge method**:
```go
func (s *SQLite) PurgeDeletedEntries(olderThan time.Duration) (int, error) {
    cutoff := time.Now().Add(-olderThan).Format("2006-01-02 15:04:05")
    
    result, err := s.db.Exec(
        `DELETE FROM entries WHERE deleted_at IS NOT NULL AND deleted_at < ?`,
        cutoff,
    )
    if err != nil {
        return 0, err
    }
    
    rows, _ := result.RowsAffected()
    return int(rows), nil
}
```

2. **Add scheduled cleanup**:
```go
func (s *SQLite) StartCleanupScheduler(interval time.Duration, retention time.Duration) {
    ticker := time.NewTicker(interval)
    go func() {
        for range ticker.C {
            purged, err := s.PurgeDeletedEntries(retention)
            if err != nil {
                log.Printf("[ERROR] Failed to purge deleted entries: %v", err)
            } else if purged > 0 {
                log.Printf("[INFO] Purged %d soft-deleted entries", purged)
            }
        }
    }()
}
```

3. **CLI command**:
```go
var purgeCmd = &cobra.Command{
    Use:   "purge",
    Short: "Permanently delete soft-deleted entries",
    Run: func(cmd *cobra.Command, args []string) {
        // ... unlock vault ...
        purged, err := storage.PurgeDeletedEntries(30 * 24 * time.Hour)
        if err != nil {
            fmt.Printf("[ERROR] Failed to purge: %v\n", err)
            return
        }
        fmt.Printf("[INFO] Purged %d entries\n", purged)
    },
}
```

---

### 3.8 Add Comprehensive Tests

**Priority**: MEDIUM  
**Scope**: Critical paths

**Test Coverage Targets**:

1. **API Tests** (new file: `cmd/server/main_test.go`):
```go
func TestHandleUnlock(t *testing.T) {
    // Setup test server
    // Test successful unlock
    // Test wrong password
    // Test rate limiting
}

func TestHandleAddEntry(t *testing.T) {
    // Test add entry
    // Test validation
    // Test unauthorized access
}

func TestCORS(t *testing.T) {
    // Test blocked origins
    // Test allowed origins
}
```

2. **P2P Tests** (new file: `internal/p2p/p2p_test.go`):
```go
func TestPairing(t *testing.T) {
    // Test pairing flow
    // Test code expiry
    // Test code reuse prevention
}

func TestSync(t *testing.T) {
    // Test entry sync
    // Test conflict resolution
    // Test device approval
}
```

3. **Integration Tests** (new file: `tests/integration_test.go`):
```go
func TestFullWorkflow(t *testing.T) {
    // Initialize vault
    // Add entries
    // P2P pairing
    // Sync
    // Verify both devices can decrypt
}
```

**Coverage Requirements**:
- Crypto package: >90%
- Storage package: >80%
- API endpoints: >70%
- P2P: >60% (harder to test)

---

## Phase 4: LOW — Backlog

### 4.1 Refactor Monolithic Server

**Priority**: LOW  
**File**: `cmd/server/main.go` (2576 lines)

**Proposed Structure**:
```
cmd/server/
├── main.go              # Entry point only
├── handlers/
│   ├── vault.go         # Vault operations
│   ├── entries.go       # Password entries
│   ├── devices.go       # Device management
│   ├── p2p.go          # P2P endpoints
│   └── pairing.go       # Pairing flow
├── middleware/
│   ├── auth.go          # Authentication
│   ├── cors.go          # CORS handling
│   └── ratelimit.go     # Rate limiting
└── routes.go            # Route registration
```

---

### 4.2 Migrate to OAEP Padding

**Priority**: LOW  
**File**: `internal/crypto/crypto.go`

**Current**:
```go
ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, publicKey, plaintext)
```

**Future**:
```go
// OAEP is more secure but requires hashing
label := []byte("")
ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, plaintext, label)
```

**Challenge**: Breaking change requiring all devices to update

---

### 4.3 Add Telemetry/Monitoring

**Priority**: LOW

**Implementation**:
- Health check endpoint
- Metrics collection
- Error reporting
- Optional anonymous usage stats

---

### 4.4 Modern Crypto (X25519)

**Priority**: LOW

**Consideration**:
```go
// X25519 is faster and more modern than RSA
// Would require significant protocol changes
// Consider for v2.0
```

---

## Testing Checklist

Before marking each phase complete:

### Phase 1 (Critical)
- [ ] CORS wildcard removed
- [ ] API requires authentication
- [ ] Password removed from P2P protocol
- [ ] No sensitive data in logs
- [ ] RSA keys are 4096-bit

### Phase 2 (High)
- [ ] No race conditions detected by `go test -race`
- [ ] No goroutine leaks (`go tool pprof`)
- [ ] Input validation rejects malicious input
- [ ] Clipboard always cleared after timeout
- [ ] Server handles port conflicts gracefully

### Phase 3 (Medium)
- [ ] Rate limiting blocks excessive requests
- [ ] Pairing codes can't be reused
- [ ] Config files are encrypted
- [ ] Scrypt uses N=32768
- [ ] Database integrity checks pass
- [ ] Fingerprints use SHA-256
- [ ] Soft deletes are purged after retention period
- [ ] Test coverage >80% for critical paths

### Phase 4 (Backlog)
- [ ] Server file <500 lines
- [ ] OAEP padding implemented
- [ ] Health endpoint working
- [ ] All tests pass

---

## Migration Guide for Existing Users

### Vault Compatibility

**Version 1.0 → 1.1 (Critical fixes)**:
- RSA keys remain 2048-bit (no breaking change)
- API adds auth (breaking change for clients)
- Frontend must be updated
- P2P protocol changes (breaking for old clients)

**Version 1.1 → 1.2 (Strengthening)**:
- New vaults use 4096-bit RSA
- Existing vaults continue working
- Config encryption optional
- Scrypt params stored per-vault

**Version 1.2 → 2.0 (Modern crypto)**:
- Breaking change
- Migration tool required
- All devices must update

---

## Security Audit Log

| Date | Auditor | Findings | Status |
|------|---------|----------|--------|
| 2026-03-05 | AI Audit | 5 CRITICAL, 5 HIGH, 8 MEDIUM issues | Open |
| | | | |

---

## Appendix: Quick Reference

### Critical Files to Modify

1. `cmd/server/main.go` - API auth, CORS, rate limiting
2. `internal/p2p/messages.go` - Remove password field
3. `internal/crypto/crypto.go` - RSA key size, fingerprint hash
4. `internal/storage/sqlite.go` - Integrity checks, purge
5. `internal/config/` - Encrypted config
6. `src/lib/api.ts` - Auth token handling

### Commands for Testing

```bash
# Run race detector
go test -race ./...

# Check goroutines
go tool pprof http://localhost:6060/debug/pprof/goroutine

# Security scan
gosec ./...

# Test coverage
go test -cover ./...

# Static analysis
staticcheck ./...
```

---

**Document Owner**: Development Team  
**Review Schedule**: Weekly during remediation  
**Next Review**: After Phase 1 completion
