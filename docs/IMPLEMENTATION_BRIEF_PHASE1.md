# IMPLEMENTATION BRIEF - Phase 1

**Status:** ✅ APPROVED - START IMPLEMENTATION  
**Source:** docs/IMPLEMENTATION_ROADMAP_PRODUCTION_READY.md (v3.1)  
**Duration:** 1.5 days  
**Priority:** CRITICAL

---

## Goal

Add rate limiting + TOTP pairing codes to the password manager.

---

## Files to Create

```
internal/pairing/totp.go       [NEW] - TOTP generation/verification
internal/pairing/totp_test.go  [NEW] - Unit tests
```

## Files to Modify

```
internal/state/server.go       [MODIFY] - Add MasterKey, rate limiting
cmd/server/handlers/auth.go    [MODIFY] - Store master key on unlock
cmd/server/handlers/pairing.go [MODIFY] - TOTP integration, rate limiting
```

## DO NOT TOUCH (Phase 2-4)

```
internal/p2p/
pkg/models/
internal/storage/
internal/crypto/ (except using existing DeriveKey)
```

---

## Implementation Details

### 1. Master Key Access

**Problem:** `crypto.LoadAndDecryptPrivateKey()` doesn't return the derived key.

**Solution:** In `auth.go Unlock()`, call `crypto.DeriveKey(password, salt)` separately:

```go
// cmd/server/handlers/auth.go - Unlock() function
func (h *AuthHandlers) Unlock(w http.ResponseWriter, r *http.Request) {
    // ... load salt from .salt file ...
    salt, err := os.ReadFile(config.SaltPathForVault(vaultName))
    
    // Derive master key
    masterKey := crypto.DeriveKey(password, salt)
    
    // Use master key to decrypt private key
    privateKey, err := crypto.LoadAndDecryptPrivateKey(privateKeyPath, password)
    
    // Store on Vault
    vault := &state.Vault{
        PrivateKey: privateKey,
        Storage:    db,
        Config:     cfg,
        VaultName:  vaultName,
        MasterKey:  masterKey,  // ← STORE THIS
    }
}
```

Same for `Init()` function when creating new vault.

### 2. PairingCode State Rekeying

**Problem:** Currently keyed by normalized code string. TOTP codes change every 60s.

**Current Code:**
```go
code, exists := h.state.GetPairingCode(pairingReq.Code)  // Keyed by code
```

**New Approach:**
- Key session metadata by vault ID (not code)
- Verify code computationally via `pairing.VerifyPairingCode()`

```go
// In HandlePairingRequest:
vault, _ := h.state.GetVault()
masterKey, _ := vault.GetMasterKey()

// Verify TOTP computationally (no state lookup by code)
if !pairing.VerifyPairingCode(masterKey, vault.Config.DeviceID, pairingReq.Code) {
    response := p2p.PairingResponsePayload{Success: false, Error: "invalid_code"}
    // ... send response ...
    return
}

// Code is valid - proceed with pairing
// Store pairing session by vault ID if needed
```

### 3. Rate Limit Test

**Problem:** Don't use `time.Sleep(30s)` in tests.

**Solution:** Make lockout duration configurable:

```go
// internal/state/server.go
var LockoutDuration = 30 * time.Second  // Can be overridden in tests

func (s *ServerState) RecordPairingAttempt(peerID string) bool {
    // ... 
    if att.Count > 5 {
        until := time.Now().Add(LockoutDuration)  // Use variable
        att.LockedUntil = &until
        att.Count = 0
        return false
    }
    return true
}
```

**Test:**
```go
func TestPairingAttempt_RateLimiting(t *testing.T) {
    // Override for faster tests
    oldDuration := state.LockoutDuration
    state.LockoutDuration = 100 * time.Millisecond
    defer func() { state.LockoutDuration = oldDuration }()
    
    s := NewServerState()
    peerID := "test-peer-123"
    
    // 5 attempts succeed
    for i := 0; i < 5; i++ {
        assert.True(t, s.RecordPairingAttempt(peerID))
    }
    
    // 6th blocked
    assert.False(t, s.RecordPairingAttempt(peerID))
    
    // Wait shorter time
    time.Sleep(150 * time.Millisecond)
    assert.True(t, s.RecordPairingAttempt(peerID))
}
```

---

## Implementation Steps

1. **Create `internal/pairing/totp.go`**
   - `deriveTOTPKey()` - HKDF derivation
   - `GeneratePairingCode()` - 6-digit TOTP
   - `VerifyPairingCode()` - Window ±1 verification

2. **Create `internal/pairing/totp_test.go`**
   - Test generation
   - Test verification
   - Test clock skew

3. **Modify `internal/state/server.go`**
   - Add `MasterKey []byte` to Vault struct
   - Add `GetMasterKey()` method
   - Add `PairingAttempt` struct
   - Add `RecordPairingAttempt()` method
   - Add `LockoutDuration` variable

4. **Modify `cmd/server/handlers/auth.go`**
   - In `Unlock()`: derive master key, store on Vault
   - In `Init()`: derive master key, store on Vault

5. **Modify `cmd/server/handlers/pairing.go`**
   - In `Generate()`: use TOTP instead of random
   - In `HandlePairingRequest()`: 
     - Add rate limit check as FIRST line
     - Replace code state lookup with TOTP verification

---

## Testing Checklist

- [ ] TOTP generates 6-digit codes
- [ ] TOTP codes change every 60s
- [ ] TOTP verifies within ±1 window
- [ ] Rate limiting allows exactly 5 attempts
- [ ] Rate limiting blocks 6th attempt
- [ ] Rate limiting resets after lockout
- [ ] Pairing with valid TOTP succeeds
- [ ] Pairing with invalid TOTP fails
- [ ] Pairing with expired TOTP fails

---

## Success Criteria

1. Rate limiting blocks brute force (5 attempts → 30s lockout)
2. TOTP codes prevent replay attacks (60s window)
3. All unit tests pass
4. Manual pairing test successful
5. No changes to Phase 2-4 files

---

## Rollback

If issues arise:
```bash
git revert <phase-1-commit>
# No database changes - safe rollback
```

---

**Start:** Create branch `feature/phase1-security-fixes` and begin.  
**Blockers:** None  
**Questions:** Reference IMPLEMENTATION_ROADMAP_PRODUCTION_READY.md v3.1

**Plan Status:** Battle-tested (7 security bugs caught, 7 interface mismatches fixed, 5 integration details resolved)

**GO!** 🚀
