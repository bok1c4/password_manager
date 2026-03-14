# Production Remediation Plan: pwman Security Overhaul

**Based on:** Production Readiness Review (Score: 3.8/10 → Target: 7.5/10)  
**Priority:** 🔴 CRITICAL - Do not use until fixed  
**Timeline:** 4 weeks (estimated)  
**Status:** Action Plan

---

## Executive Summary

The pwman password manager has **7 critical security vulnerabilities** that make it unsafe for production use. This plan provides step-by-step remediation guidance to fix all critical issues, address high-priority bugs, and prepare the system for production deployment.

**Key Statistics:**
- Critical Issues: 7 (must fix)
- High-Priority Issues: 7 (should fix)
- Frontend Blockers: 4
- Testing Gaps: 7
- Estimated Effort: 3-4 weeks
- Team Size Recommended: 2-3 engineers

---

## Phase 1: Critical Security Fixes (Week 1)

### Day 1-2: Private Key Encryption

**Issue:** Private keys stored in plaintext on disk  
**File:** `internal/identity/identity.go`  
**Severity:** 🔴 CRITICAL

#### TODO List

**Task 1.1: Modify identity.Save() to encrypt private keys**
```go
// internal/identity/identity.go
// Add import: "golang.org/x/crypto/argon2"

func (id *DeviceIdentity) SaveEncrypted(path string, masterPassword string) error {
    // 1. Derive encryption key from master password using Argon2id
    salt := make([]byte, 16)
    if _, err := rand.Read(salt); err != nil {
        return err
    }
    
    encryptionKey := argon2.IDKey(
        []byte(masterPassword),
        salt,
        3,      // time
        64*1024, // memory
        4,      // threads
        32,     // key length
    )
    
    // 2. Encrypt the Ed25519 seed
    seed := id.SignPrivateKey.Seed()
    encryptedSeed, err := aesGCMEncrypt(seed, encryptionKey)
    if err != nil {
        return err
    }
    
    // 3. Store: salt + IV + ciphertext as PEM
    // Format: salt(16) + iv(12) + ciphertext
    data := append(salt, encryptedSeed...)
    
    privPEM := pem.EncodeToMemory(&pem.Block{
        Type:  "ENCRYPTED PRIVATE KEY",
        Bytes: data,
    })
    
    // 4. Write with strict permissions
    if err := os.WriteFile(path, privPEM, 0600); err != nil {
        return err
    }
    
    // 5. Save public key unencrypted
    pubPEM := pem.EncodeToMemory(&pem.Block{
        Type:  "ED25519 PUBLIC KEY",
        Bytes: id.SignPublicKey,
    })
    
    return os.WriteFile(path+".pub", pubPEM, 0644)
}
```

**Task 1.2: Modify identity.Load() to decrypt private keys**
```go
func LoadIdentityEncrypted(path string, masterPassword string) (*DeviceIdentity, error) {
    // 1. Read encrypted key
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    block, _ := pem.Decode(data)
    if block == nil || block.Type != "ENCRYPTED PRIVATE KEY" {
        return nil, fmt.Errorf("invalid key format")
    }
    
    // 2. Extract salt and encrypted data
    if len(block.Bytes) < 16+12 { // salt + minimum IV
        return nil, fmt.Errorf("invalid encrypted key")
    }
    
    salt := block.Bytes[:16]
    encryptedData := block.Bytes[16:]
    
    // 3. Derive key and decrypt
    encryptionKey := argon2.IDKey([]byte(masterPassword), salt, 3, 64*1024, 4, 32)
    seed, err := aesGCMDecrypt(encryptedData, encryptionKey)
    if err != nil {
        return nil, fmt.Errorf("decryption failed (wrong password?)")
    }
    
    // 4. Reconstruct identity
    priv := ed25519.NewKeyFromSeed(seed)
    // ... rest of derivation as before
}
```

**Task 1.3: Update auth handlers to pass master password**
- File: `cmd/server/handlers/auth.go`
- Modify `Init()` and `Unlock()` to call `SaveEncrypted()` and `LoadIdentityEncrypted()`
- Pass master password from request

**Task 1.4: Add secure memory wiping**
```go
// internal/crypto/secure.go

import "runtime"

// ClearBytes overwrites a byte slice with zeros
func ClearBytes(b []byte) {
    for i := range b {
        b[i] = 0
    }
    runtime.KeepAlive(b) // Prevent optimization
}
```

**Task 1.5: Unit tests for encryption/decrypt**
- Test: Correct password decrypts successfully
- Test: Wrong password fails
- Test: Tampered ciphertext fails
- Test: Different salt produces different encryption

**Verification:**
```bash
# Check file is encrypted
head -1 ~/.pwman/vaults/test/identity.key
# Should show: -----BEGIN ENCRYPTED PRIVATE KEY-----
# NOT: -----BEGIN ED25519 PRIVATE KEY-----
```

---

### Day 2-3: Rate Limiter Wiring

**Issue:** Rate limiter implemented but never applied to endpoints  
**File:** `internal/api/ratelimit.go`, `cmd/server/main.go`  
**Severity:** 🔴 CRITICAL

#### TODO List

**Task 2.1: Create rate limiter middleware**
```go
// internal/api/ratelimit.go

func (rl *RateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        clientID := getClientID(r) // IP or token
        
        if !rl.Allow(clientID) {
            w.Header().Set("Retry-After", "30")
            http.Error(w, "Too many requests", http.StatusTooManyRequests)
            return
        }
        
        next(w, r)
    }
}

func getClientID(r *http.Request) string {
    // Try to get from header first (for proxied requests)
    if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
        return strings.Split(fwd, ",")[0]
    }
    // Fall back to remote address
    host, _, _ := net.SplitHostPort(r.RemoteAddr)
    return host
}
```

**Task 2.2: Apply to unlock endpoint in main.go**
```go
// cmd/server/main.go

rateLimiter := api.NewRateLimiter()

// Wrap unlock handler with rate limiter
router.POST("/api/unlock", rateLimiter.Middleware(authHandlers.Unlock))
```

**Task 2.3: Add structured logging**
```go
// In unlock handler
log := logger.WithFields(logrus.Fields{
    "handler": "unlock",
    "client_ip": clientIP,
    "vault": vaultName,
})

if err != nil {
    log.WithError(err).Warn("Unlock failed")
    api.Unauthorized(w, "wrong password")
    return
}

log.Info("Unlock successful")
```

**Task 2.4: Integration test**
```go
func TestUnlockRateLimiting(t *testing.T) {
    // Make 5 successful requests
    for i := 0; i < 5; i++ {
        resp := makeUnlockRequest()
        assert.Equal(t, 200, resp.StatusCode)
    }
    
    // 6th request should be rate limited
    resp := makeUnlockRequest()
    assert.Equal(t, 429, resp.StatusCode)
    
    // Wait 30s, should work again
    time.Sleep(30 * time.Second)
    resp = makeUnlockRequest()
    assert.Equal(t, 200, resp.StatusCode)
}
```

---

### Day 3-4: Fix Rate Limiter Logic

**Issue:** Rate limiter resets count after lockout, allowing unlimited retries  
**File:** `internal/state/server.go:89-105`  
**Severity:** 🔴 CRITICAL

#### TODO List

**Task 3.1: Implement exponential backoff**
```go
// internal/state/server.go

type PairingAttempt struct {
    Count         int
    ConsecutiveFailures int  // Track consecutive failures
    LastAttempt   time.Time
    LockedUntil   *time.Time
    LockoutCount  int        // Track number of lockouts
}

func (s *ServerState) RecordPairingAttempt(peerID string) bool {
    s.pairingAttemptsMu.Lock()
    defer s.pairingAttemptsMu.Unlock()

    att, exists := s.pairingAttempts[peerID]
    if !exists {
        s.pairingAttempts[peerID] = &PairingAttempt{
            Count:       1,
            LastAttempt: time.Now(),
        }
        return true
    }

    // Check if still locked
    if att.LockedUntil != nil && time.Now().Before(*att.LockedUntil) {
        return false
    }

    att.Count++
    att.LastAttempt = time.Now()

    // Lock after MORE than 5 attempts
    if att.Count > 5 {
        // Exponential backoff: 30s, 60s, 120s, 240s, 300s max
        baseDelay := 30 * time.Second
        multiplier := 1 << att.LockoutCount // 2^lockoutCount
        if multiplier > 10 {
            multiplier = 10 // Cap at 5 minutes
        }
        
        delay := time.Duration(multiplier) * baseDelay
        until := time.Now().Add(delay)
        att.LockedUntil = &until
        att.LockoutCount++
        att.Count = 0 // Reset attempt count but NOT lockout count
        return false
    }

    return true
}
```

**Task 3.2: Add persistence**
```go
// Save attempts to disk periodically
func (s *ServerState) persistAttempts() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        s.pairingAttemptsMu.Lock()
        data, _ := json.Marshal(s.pairingAttempts)
        s.pairingAttemptsMu.Unlock()
        
        os.WriteFile("/tmp/pwman_attempts.json", data, 0600)
    }
}
```

---

### Day 4-5: TLS Certificate Verification

**Issue:** TLS `InsecureSkipVerify: true` and no fingerprint check  
**File:** `internal/p2p/p2p.go:447`  
**Severity:** 🔴 CRITICAL

#### TODO List

**Task 4.1: Remove InsecureSkipVerify**
```go
// internal/p2p/p2p.go

// OLD:
config := &tls.Config{
    InsecureSkipVerify: true,
}

// NEW:
config := &tls.Config{
    InsecureSkipVerify: false,
    VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
        if len(rawCerts) == 0 {
            return fmt.Errorf("no certificate presented")
        }
        
        // Compute fingerprint of peer certificate
        fingerprint := sha256.Sum256(rawCerts[0])
        fpStr := hex.EncodeToString(fingerprint[:])
        
        // Check if this fingerprint is trusted
        if !p.peerStore.IsTrusted(fpStr) {
            return fmt.Errorf("certificate fingerprint %s not trusted", fpStr)
        }
        
        return nil
    },
}
```

**Task 4.2: Implement certificate pinning verification**
```go
// internal/p2p/p2p.go

func (p *P2PManager) verifyPeerCertificate(conn *tls.Conn) error {
    state := conn.ConnectionState()
    if len(state.PeerCertificates) == 0 {
        return fmt.Errorf("no peer certificate")
    }
    
    cert := state.PeerCertificates[0]
    fingerprint := transport.CertFingerprint(cert.Raw)
    
    // In pairing mode, accept any cert and show fingerprint to user
    if p.pairingMode {
        return nil // User will verify manually
    }
    
    // Otherwise, must be in peer store
    if !p.peerStore.IsTrusted(fingerprint) {
        return fmt.Errorf("untrusted peer: %s", fingerprint)
    }
    
    return nil
}
```

**Task 4.3: Add pairing mode flag**
```go
type P2PConfig struct {
    // ... other fields ...
    PairingMode bool // During pairing, accept any cert (user verifies manually)
}

// In connectToPeerTLS
if err := p.verifyPeerCertificate(conn); err != nil {
    if !p.pairingMode {
        conn.Close()
        return err
    }
    // In pairing mode, store fingerprint for manual verification
    p.pendingFingerprint = fingerprint
}
```

---

### Day 5: Race Condition Fix

**Issue:** Concurrent writer race in P2P send  
**File:** `internal/p2p/p2p.go:614-635`  
**Severity:** 🔴 CRITICAL

#### TODO List

**Task 5.1: Fix sendMessageTLS**
```go
// internal/p2p/p2p.go

func (p *P2PManager) sendMessageTLS(peerID string, msg SyncMessage) error {
    p.mu.RLock()
    peer, ok := p.tlsPeers[peerID]
    p.mu.RUnlock()

    if !ok {
        return fmt.Errorf("peer not found: %s", peerID)
    }

    data, err := json.Marshal(msg)
    if err != nil {
        return err
    }

    data = append(data, '\n')

    // Lock the peer for the entire write operation
    peer.mu.Lock()
    defer peer.mu.Unlock()

    if _, err := peer.writer.Write(data); err != nil {
        return err
    }

    return peer.writer.Flush()
}
```

**Task 5.2: Add per-peer mutex**
```go
type peerConnection struct {
    id          string
    name        string
    addr        string
    conn        *tls.Conn
    reader      *bufio.Reader
    writer      *bufio.Writer
    mu          sync.Mutex // Per-peer mutex
    connected   bool
    lastSeen    time.Time
    fingerprint string
}
```

**Task 5.3: Run race detector**
```bash
go test -race ./internal/p2p/...
# Should report: PASS with no race conditions
```

---

### Day 5-6: CSP Enablement

**Issue:** Content Security Policy disabled in Tauri  
**File:** `tauri.conf.json`  
**Severity:** 🔴 HIGH

#### TODO List

**Task 6.1: Enable CSP in tauri.conf.json**
```json
{
  "tauri": {
    "security": {
      "csp": "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; connect-src 'self' http://localhost:* https://localhost:*; img-src 'self' data:;"
    }
  }
}
```

**Task 6.2: Test UI still works**
```bash
cd src-tauri
cargo tauri dev
# Verify all UI components render correctly
```

---

## Phase 2: High-Priority Fixes (Week 2)

### Day 1: Silent Error Logging

**File:** `cmd/server/handlers/auth.go:141-142`, `pairing.go:403-404`

```go
// Add to auth.go
import "github.com/sirupsen/logrus"

var log = logrus.New()

// In Unlock handler
if err != nil {
    log.WithFields(logrus.Fields{
        "handler": "unlock",
        "client_ip": r.RemoteAddr,
        "error": err.Error(),
    }).Warn("Unlock attempt failed")
    api.Unauthorized(w, "wrong password")
    return
}
```

---

### Day 1-2: Secure Soft Deletion

**Issue:** Soft-deleted entries recoverable  
**File:** `internal/storage/sqlite.go:165`

```go
func (s *SQLite) DeleteEntry(id string) error {
    // 1. Get entry to delete
    entry, err := s.GetEntry(id)
    if err != nil {
        return err
    }
    
    // 2. Securely overwrite sensitive fields
    entry.EncryptedPassword = secureRandomString(len(entry.EncryptedPassword))
    entry.Notes = secureRandomString(len(entry.Notes))
    
    // 3. Save garbage data
    s.UpdateEntry(entry)
    
    // 4. Now mark as deleted
    _, err = s.db.Exec(
        "UPDATE entries SET deleted_at = ? WHERE id = ?",
        time.Now().UTC(), id,
    )
    return err
}
```

---

### Day 2-3: Atomic Re-Encryption

**Issue:** Non-atomic re-encryption during pairing  
**File:** `cmd/server/handlers/pairing.go:1006-1058`

```go
func reEncryptAtomically(storage *storage.SQLite, oldKey, newKey []byte) error {
    tx, err := storage.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()
    
    // 1. Create temporary table
    if _, err := tx.Exec("CREATE TEMP TABLE temp_entries AS SELECT * FROM entries"); err != nil {
        return err
    }
    
    // 2. Re-encrypt all entries in temp table
    entries, _ := tx.Query("SELECT * FROM temp_entries")
    for entries.Next() {
        // Decrypt with old key, encrypt with new key
        // ...
    }
    
    // 3. Validate all entries
    if !validateAllEntries(tx) {
        return fmt.Errorf("validation failed")
    }
    
    // 4. Atomically swap tables
    if _, err := tx.Exec("DELETE FROM entries"); err != nil {
        return err
    }
    if _, err := tx.Exec("INSERT INTO entries SELECT * FROM temp_entries"); err != nil {
        return err
    }
    
    // 5. Commit
    return tx.Commit()
}
```

---

### Day 3-4: Path Traversal Prevention

**Issue:** Peer-supplied vault name used unsanitized  
**File:** `cmd/server/handlers/pairing.go:318`

```go
func validateVaultName(name string) error {
    // Only allow alphanumeric, underscore, hyphen
    valid := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
    if !valid.MatchString(name) {
        return fmt.Errorf("invalid vault name: only alphanumeric, underscore, hyphen allowed")
    }
    
    // Check for path traversal attempts
    if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
        return fmt.Errorf("invalid vault name: path traversal detected")
    }
    
    return nil
}

// Use in pairing handler
if err := validateVaultName(vaultName); err != nil {
    api.BadRequest(w, err.Error())
    return
}
```

---

### Day 4: Authenticated Vault Switching

**File:** `cmd/server/main.go:118`

```go
// Add authentication middleware
func requireAuth(authManager *api.AuthManager) gin.HandlerFunc {
    return func(c *gin.Context) {
        token := c.GetHeader("Authorization")
        if token == "" || !authManager.Validate(token) {
            c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
            return
        }
        c.Next()
    }
}

// Apply to vault switch endpoint
router.POST("/api/vaults/use", requireAuth(authManager), vaultHandlers.SwitchVault)
```

---

### Day 5: Channel Close Panic Fix

**File:** `internal/p2p/p2p.go:741-768`

```go
type P2PManager struct {
    // ... other fields ...
    stopOnce sync.Once
}

func (p *P2PManager) Stop() {
    p.stopOnce.Do(func() {
        p.cancel()
        
        // Close connections first
        if p.listener != nil {
            p.listener.Close()
        }
        
        // Wait for goroutines to finish
        time.Sleep(100 * time.Millisecond)
        
        // Close channels
        close(p.messageChan)
        close(p.pairingRequestChan)
        // ... other channels ...
    })
}
```

---

## Phase 3: Frontend & Sync API (Week 3)

### Day 1-3: Implement Sync Endpoints

**Create:** `cmd/server/handlers/sync.go`

```go
package handlers

import (
    "net/http"
    "github.com/gin-gonic/gin"
    "github.com/bok1c4/pwman/internal/sync"
)

// GET /api/sync/status - Get current sync status
func (h *SyncHandlers) GetStatus(c *gin.Context) {
    vault, ok := h.state.GetVault()
    if !ok {
        c.JSON(400, gin.H{"error": "vault locked"})
        return
    }
    
    // Get clock for this device
    clock, _ := h.state.ClockManager.GetClock(vault.Config.DeviceID)
    
    // Count pending entries
    pendingCount := h.countPendingSync()
    
    c.JSON(200, gin.H{
        "device_id": vault.Config.DeviceID,
        "logical_clock": clock.Current(),
        "pending_changes": pendingCount,
        "last_sync": getLastSyncTime(),
    })
}

// POST /api/sync/pull - Pull changes from peer
func (h *SyncHandlers) Pull(c *gin.Context) {
    var req struct {
        SinceClock int64 `json:"since_clock"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": "invalid request"})
        return
    }
    
    // Get entries changed since requested clock
    entries := h.getEntriesSince(req.SinceClock)
    
    c.JSON(200, gin.H{
        "entries": entries,
        "max_clock": getMaxClock(),
    })
}

// POST /api/sync/push - Push changes to peer
func (h *SyncHandlers) Push(c *gin.Context) {
    var req struct {
        Entries []models.PasswordEntry `json:"entries"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": "invalid request"})
        return
    }
    
    // Merge incoming entries with local
    for _, entry := range req.Entries {
        // Witness remote clock
        clock, _ := h.state.ClockManager.GetClock(entry.OriginDevice)
        clock.Witness(entry.LogicalClock)
        
        // Merge entry
        existing, _ := h.storage.GetEntry(entry.ID)
        if existing != nil {
            merged := sync.MergeEntry(*existing, entry)
            h.storage.UpdateEntry(&merged)
        } else {
            h.storage.CreateEntry(&entry)
        }
    }
    
    c.JSON(200, gin.H{"success": true, "merged": len(req.Entries)})
}
```

**Register in main.go:**
```go
syncHandlers := handlers.NewSyncHandlers(state)
router.GET("/api/sync/status", syncHandlers.GetStatus)
router.POST("/api/sync/pull", syncHandlers.Pull)
router.POST("/api/sync/push", syncHandlers.Push)
```

---

### Day 3-4: Frontend Sync Integration

**Update Rust frontend:** `src-tauri/src/commands.rs`

```rust
#[tauri::command]
async fn sync_pull(state: State<'_, AppState>) -> Result<SyncResult, String> {
    let client = state.http_client.lock().await;
    
    let response = client
        .post("http://localhost:8080/api/sync/pull")
        .json(&json!({
            "since_clock": state.last_sync_clock
        }))
        .send()
        .await
        .map_err(|e| e.to_string())?;
    
    let result: SyncResult = response.json().await.map_err(|e| e.to_string())?;
    
    // Merge entries locally
    for entry in result.entries {
        merge_entry_local(&entry)?;
    }
    
    state.last_sync_clock = result.max_clock;
    Ok(result)
}

#[tauri::command]
async fn sync_push(state: State<'_, AppState>) -> Result<(), String> {
    let changes = get_pending_changes().await?;
    
    let client = state.http_client.lock().await;
    client
        .post("http://localhost:8080/api/sync/push")
        .json(&json!({
            "entries": changes
        }))
        .send()
        .await
        .map_err(|e| e.to_string())?;
    
    Ok(())
}
```

---

### Day 5: Frontend Cleanup

**Fix dead dependency:** `src-tauri/package.json`
```bash
npm uninstall react-router-dom
```

**Fix hardcoded password length:**
```rust
// Add to settings
const DEFAULT_PASSWORD_LENGTH: usize = 20;
const MIN_PASSWORD_LENGTH: usize = 8;
const MAX_PASSWORD_LENGTH: usize = 128;

#[tauri::command]
fn generate_password(length: Option<usize>) -> String {
    let len = length.unwrap_or(DEFAULT_PASSWORD_LENGTH)
        .clamp(MIN_PASSWORD_LENGTH, MAX_PASSWORD_LENGTH);
    
    // Generate password of specified length
}
```

**Fix go.mod version:**
```go
// go.mod
module github.com/bok1c4/pwman

go 1.23  // Fixed from go 1.25.6 (which doesn't exist)
```

---

## Phase 4: Testing & Validation (Week 4)

### Day 1-2: Security Tests

```go
// Test: Private key encryption
func TestPrivateKeyEncryption(t *testing.T) {
    // 1. Create identity
    id, _ := identity.GenerateIdentity()
    
    // 2. Save encrypted
    id.SaveEncrypted("/tmp/test.key", "password123")
    
    // 3. Verify file is encrypted
    data, _ := os.ReadFile("/tmp/test.key")
    assert.Contains(t, string(data), "ENCRYPTED PRIVATE KEY")
    assert.NotContains(t, string(data), id.SignPrivateKey.Seed())
    
    // 4. Load with correct password
    id2, err := identity.LoadIdentityEncrypted("/tmp/test.key", "password123")
    assert.NoError(t, err)
    assert.Equal(t, id.SignPublicKey, id2.SignPublicKey)
    
    // 5. Load with wrong password
    _, err = identity.LoadIdentityEncrypted("/tmp/test.key", "wrong")
    assert.Error(t, err)
}

// Test: Rate limiter
func TestRateLimiterIntegration(t *testing.T) {
    // Make 6 rapid requests
    for i := 0; i < 6; i++ {
        resp := makeUnlockRequest()
        if i < 5 {
            assert.Equal(t, 401, resp.StatusCode) // Wrong password
        } else {
            assert.Equal(t, 429, resp.StatusCode) // Rate limited
        }
    }
}

// Test: TLS certificate pinning
func TestTLSCertificatePinning(t *testing.T) {
    // 1. Create manager with pinned cert
    ps := transport.NewPeerStore("/tmp/test_peers.json")
    ps.Trust("abc123", "Test Device", "device-1")
    
    manager, _ := p2p.NewP2PManager(p2p.P2PConfig{
        PeerStore: ps,
    })
    
    // 2. Connect with wrong cert should fail
    err := manager.ConnectToPeer("localhost:1234")
    assert.Error(t, err) // Certificate not trusted
}
```

---

### Day 3: Integration Tests

```go
// Test: Full pairing flow
func TestFullPairingFlow(t *testing.T) {
    // 1. Device A creates vault
    // 2. Device A generates TOTP
    // 3. Device B enters TOTP
    // 4. Devices exchange certificates
    // 5. Device B trusts Device A's certificate
    // 6. Sync completes
}

// Test: Concurrent edits
func TestConcurrentEdits(t *testing.T) {
    // 1. Both devices sync same entry
    // 2. Device A edits offline
    // 3. Device B edits offline
    // 4. Both come online and sync
    // 5. Verify conflict resolution works
}

// Test: Network failure recovery
func TestNetworkFailureRecovery(t *testing.T) {
    // 1. Start sync
    // 2. Disconnect network mid-sync
    // 3. Reconnect
    // 4. Verify consistency
}
```

---

### Day 4: Load Testing

```go
// Test: 1000 entries, 5 devices
func TestSyncPerformance(t *testing.T) {
    // Create 1000 entries
    // Sync to 5 devices
    // Measure time
    // Verify all devices consistent
}
```

---

### Day 5: Final Validation

**Checklist:**
- [ ] All critical issues fixed
- [ ] All tests passing (including new ones)
- [ ] Race detector clean
- [ ] Security audit re-run
- [ ] Frontend working with new API
- [ ] Documentation updated

---

## Dead Code Removal

### Files to Remove

```bash
# Remove unused libp2p code (if not using legacy mode)
# Note: Keep for backward compatibility during transition
# rm internal/p2p/legacy.go  # Only if fully migrated to TLS

# Remove deprecated RSA code (after migration to Ed25519)
# rm internal/crypto/rsa_old.go  # After Phase 3 complete

# Remove placeholder files
rm -f internal/api/placeholder.go
rm -f cmd/server/handlers/unused.go
```

### Code Cleanup

```go
// Remove unused imports
// Remove commented-out debug code
// Remove println statements (replace with logging)
// Remove unused constants
```

---

## Updated Project Structure

```
pwman/
├── cmd/
│   ├── server/
│   │   ├── main.go
│   │   └── handlers/
│   │       ├── auth.go          # +encrypted key support
│   │       ├── pairing.go       # +path validation
│   │       ├── entry.go         # +soft delete
│   │       └── sync.go          # NEW: sync endpoints
│   └── pwman/
│       └── main.go
├── internal/
│   ├── api/
│   │   ├── auth.go
│   │   ├── ratelimit.go         # +wired middleware
│   │   └── secure.go            # NEW: secure memory
│   ├── crypto/
│   │   ├── aes.go
│   │   ├── hybrid.go            # +NaCl box
│   │   ├── kdf.go               # NEW: Argon2id
│   │   └── secure.go            # NEW: memory wiping
│   ├── identity/
│   │   ├── identity.go          # +encrypted storage
│   │   └── identity_test.go
│   ├── p2p/
│   │   ├── p2p.go               # +race fix, +cert verify
│   │   └── messages.go
│   ├── pairing/
│   │   ├── totp.go
│   │   └── totp_test.go
│   ├── state/
│   │   ├── server.go            # +rate limit fix
│   │   └── server_test.go
│   ├── storage/
│   │   ├── sqlite.go            # +secure delete
│   │   └── sqlite_test.go
│   ├── sync/
│   │   ├── clock.go             # NEW: Lamport clocks
│   │   └── clock_test.go
│   ├── transport/
│   │   ├── peerstore.go
│   │   └── tls.go
│   └── discovery/
│       ├── advertise.go
│       └── browse.go
├── pkg/
│   └── models/
│       └── models.go            # +new fields
├── src-tauri/
│   ├── src/
│   │   ├── main.rs
│   │   ├── commands.rs          # +sync commands
│   │   └── app_state.rs         # +sync state
│   └── Cargo.toml               # -dead deps
├── docs/
│   ├── ARCHITECTURE.md          # Updated
│   └── REFACTORING_SUMMARY.md   # Updated
└── go.mod                       # Fixed version
```

---

## Testing Strategy

### Test Coverage Requirements

| Component | Minimum Coverage |
|-----------|-----------------|
| Crypto | 95% |
| Identity | 95% |
| Storage | 90% |
| P2P | 85% |
| API | 85% |
| Overall | 90% |

### Critical Path Tests

```go
// Test: End-to-end happy path
func TestE2E_HappyPath(t *testing.T) {
    // 1. Create vault
    // 2. Add entry
    // 3. Pair second device
    // 4. Sync
    // 5. Edit on both
    // 6. Verify consistency
}

// Test: Security boundaries
func TestSecurity_Boundaries(t *testing.T) {
    // 1. Wrong password can't unlock
    // 2. Tampered key can't load
    // 3. Untrusted cert rejected
    // 4. Rate limit enforced
    // 5. Path traversal blocked
}
```

---

## Deployment Checklist

### Pre-Deployment
- [ ] All critical issues resolved
- [ ] All tests passing
- [ ] Security audit re-run and passed
- [ ] Performance benchmarks met
- [ ] Documentation complete
- [ ] Operational runbook ready

### Deployment
- [ ] Deploy to staging
- [ ] Run smoke tests
- [ ] Monitor for 24h
- [ ] Deploy to production
- [ ] Enable monitoring
- [ ] Announce to users

### Post-Deployment
- [ ] Monitor error rates
- [ ] Monitor sync success rates
- [ ] Monitor unlock success rates
- [ ] Gather user feedback
- [ ] Plan next iteration

---

## Success Criteria

The remediation is successful when:

✅ **Security Score:** 3/10 → 8/10 (target)  
✅ **Critical Issues:** 7/7 fixed  
✅ **High-Priority Issues:** 7/7 fixed  
✅ **Test Coverage:** 94% → 90%+  
✅ **Race Conditions:** 0 detected  
✅ **Frontend Sync:** Working end-to-end  
✅ **Production Readiness:** Ready for beta launch

---

## Timeline Summary

| Week | Focus | Key Deliverables |
|------|-------|-----------------|
| Week 1 | Critical Security Fixes | Key encryption, Rate limiting, TLS, Race fixes |
| Week 2 | High-Priority Fixes | Logging, Secure delete, Atomic ops, Validation |
| Week 3 | Sync API & Frontend | Sync endpoints, Frontend integration |
| Week 4 | Testing & Validation | Security tests, Integration tests, Load tests |

**Total Effort:** 4 weeks  
**Team Size:** 2-3 engineers  
**Risk Level:** 🔴 High (until critical issues fixed)

---

*Document created: 2026-03-14*  
*Based on: Production Readiness Review*  
*Next review: After Week 4 completion*
