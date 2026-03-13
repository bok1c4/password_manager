# CRITICAL BUG FIXES - Implementation Checklist

## Overview
This document lists all critical bugs and security issues identified in the original implementation plan that **MUST** be fixed before proceeding.

**Status:** 🚨 CRITICAL - Do NOT use v1.0 of the roadmap  
**Use:** `docs/IMPLEMENTATION_ROADMAP_CORRECTED.md` (v2.0)

---

## CRITICAL FIXES REQUIRED

### 1. X25519 Key Derivation (PHASE 3) 🔴 CRITICAL

**Bug:** Direct copy of Ed25519 seed to X25519 private key  
**Impact:** Shared secret material between key types - potential cross-protocol attacks  
**Fix:** Proper RFC 7748 §5 conversion with SHA-512 hash + clamping

```go
// ❌ WRONG - Direct copy (from v1.0)
var boxPriv, boxPub [32]byte
copy(boxPriv[:], priv.Seed()[:32])
curve25519.ScalarBaseMult(&boxPub, &boxPriv)

// ✅ CORRECT - RFC 7748 compliant
h := sha512.Sum512(priv.Seed())
h[0] &= 248    // Clear bits 0, 1, 2
h[31] &= 127   // Clear bit 7
h[31] |= 64    // Set bit 6
copy(boxPriv[:], h[:32])
curve25519.ScalarBaseMult(&boxPub, &boxPriv)
```

**Test:**
```go
func TestX25519KeyDerivation(t *testing.T) {
    identity, _ := GenerateIdentity()
    ed25519Seed := identity.SignPrivateKey.Seed()
    x25519Priv := identity.BoxPrivateKey[:]
    
    // Should NOT be direct copy
    assert.NotEqual(t, ed25519Seed[:32], x25519Priv)
    
    // Verify clamping
    assert.Equal(t, byte(0), x25519Priv[0]&7)
    assert.NotEqual(t, byte(0), x25519Priv[31]&64)
    assert.Equal(t, byte(0), x25519Priv[31]&128)
}
```

---

### 2. TLS Handshake (PHASE 2) 🔴 CRITICAL

**Bug:** Accessing `ConnectionState()` before handshake completes  
**Impact:** `PeerCertificates` will be empty, causing immediate connection failure  
**Fix:** Call `conn.Handshake()` before accessing connection state

```go
// ❌ WRONG (from refactor_plan.md)
func (p *P2PManager) handleConnection(conn *tls.Conn) {
    state := conn.ConnectionState()  // BUG: handshake not done!
    if len(state.PeerCertificates) == 0 {
        conn.Close()  // Always fires
        return
    }
}

// ✅ CORRECT
func (p *P2PManager) handleConnection(conn *tls.Conn) {
    if err := conn.Handshake(); err != nil {
        conn.Close()
        return
    }
    state := conn.ConnectionState()  // Now safe
    // ... rest of handler
}
```

---

### 3. Rate Limiter Off-By-One (PHASE 1) 🟡 HIGH

**Bug:** Only allows 4 attempts instead of 5  
**Impact:** User confusion, stricter than documented  
**Fix:** Change `>= 5` to `> 5`

```go
// ❌ WRONG - Blocks on 5th attempt
if att.Count >= 5 {  // When Count=5, this triggers
    lockout()
}

// ✅ CORRECT - Allows exactly 5 attempts
if att.Count > 5 {  // When Count=5, this doesn't trigger
    lockout()
}
```

**Test Correction:**
```go
// ❌ WRONG TEST (contradicts itself)
for i := 0; i < 5; i++ {  // 5 iterations
    assert.True(t, s.RecordPairingAttempt(peerID))  // 5th call returns false!
}

// ✅ CORRECT TEST
for i := 0; i < 5; i++ {  // First 5 succeed
    assert.True(t, s.RecordPairingAttempt(peerID))
}
assert.False(t, s.RecordPairingAttempt(peerID))  // 6th blocked
```

---

### 4. TOTP Master Key Exposure (PHASE 1) 🟡 HIGH

**Bug:** Using vault master key directly as TOTP HMAC key  
**Impact:** TOTP codes visible on screen could leak information about master key  
**Fix:** Derive dedicated sub-key using HKDF

```go
// ❌ WRONG - Uses master key directly
func GeneratePairingCode(vaultMasterKey []byte, vaultID string) string {
    mac := hmac.New(sha256.New, vaultMasterKey)  // DANGEROUS
    // ...
}

// ✅ CORRECT - Derives isolated sub-key
func deriveTOTPKey(vaultMasterKey []byte, vaultID string) []byte {
    reader := hkdf.New(sha512.New, vaultMasterKey, []byte(vaultID), []byte("pwman-pairing-totp"))
    key := make([]byte, 32)
    reader.Read(key)
    return key
}

func GeneratePairingCode(vaultMasterKey []byte, vaultID string) string {
    totpKey := deriveTOTPKey(vaultMasterKey, vaultID)
    mac := hmac.New(sha256.New, totpKey)  // Safe
    // ...
}
```

---

### 5. Missing Read Loop in ConnectToPeer (PHASE 2) 🟡 HIGH

**Bug:** `ConnectToPeer` stores connection but never reads from it  
**Impact:** Joiner never receives messages from host  
**Fix:** Start read loop goroutine

```go
// ❌ WRONG
func (p *P2PManager) ConnectToPeer(addr string, cert tls.Certificate) error {
    conn, _ := tls.Dial("tcp", addr, config)
    // Store connection...
    // BUG: Never starts read loop!
    return nil
}

// ✅ CORRECT
func (p *P2PManager) ConnectToPeer(addr string, cert tls.Certificate) error {
    conn, _ := tls.Dial("tcp", addr, config)
    // Store connection...
    go p.handleConnection(conn)  // Start read loop
    return nil
}
```

---

### 6. Undefined Nonce in NaCl Box (PHASE 3) 🟡 HIGH

**Bug:** Using undefined `nonce` variable  
**Impact:** Code won't compile / runtime panic  
**Fix:** Generate random nonce and serialize with ciphertext

```go
// ❌ WRONG (from refactor_plan.md)
encryptedKey := box.Seal(nil, aesKey, &nonce, boxPubKey, ephemeralPriv)  // nonce undefined!

// ✅ CORRECT
var nonce [24]byte
rand.Read(nonce[:])
encryptedKey := box.Seal(nil, aesKey, &nonce, boxPubKey, ephemeralPriv)

// Store: ephemeralPub + nonce + encryptedKey
combined := append(ephemeralPub[:], nonce[:]...)
combined = append(combined, encryptedKey...)
```

---

### 7. Missing Import (PHASE 3) 🟢 MEDIUM

**Bug:** Using `filepath.Dir()` without importing `path/filepath`  
**Impact:** Compilation error  
**Fix:** Add import

```go
// ✅ Add to imports
import "path/filepath"

func (id *DeviceIdentity) Save(path string) error {
    dir := filepath.Dir(path)  // Now works
    // ...
}
```

---

## IMPORTANT GAPS ADDRESSED

### Gap 1: Model Updates
**File:** `pkg/models/models.go` must be updated:

```go
type Device struct {
    // ... existing fields ...
    SignPublicKey string `json:"sign_public_key"` // NEW
    BoxPublicKey  string `json:"box_public_key"`  // NEW
}

type PasswordEntry struct {
    // ... existing fields ...
    BoxKeys        map[string]string `json:"box_keys"`       // NEW
    LogicalClock   int64             `json:"logical_clock"`  // NEW
    OriginDevice   string            `json:"origin_device"`  // NEW
}
```

### Gap 2: Route Registration
**File:** `cmd/server/main.go` must register new handlers:

```go
func setupRoutes(mux *http.ServeMux, state *state.ServerState, authMiddleware func(http.HandlerFunc) http.HandlerFunc) {
    // ... existing routes ...
    
    // NEW routes for Phase 2-4
    mux.HandleFunc("/api/pairing/verify-fingerprint", authMiddleware(pairingHandlers.VerifyFingerprint))
    mux.HandleFunc("/api/sync/devices", authMiddleware(syncHandlers.ListDevices))
    mux.HandleFunc("/api/sync/resync", authMiddleware(syncHandlers.Resync))
}
```

### Gap 3: Dependency Addition
**Command to run:**

```bash
# Add zeroconf for mDNS (Phase 2)
go get github.com/grandcat/zeroconf

# Remove libp2p
go get -u github.com/libp2p/go-libp2p@none

go mod tidy
```

### Gap 4: RSA to Ed25519 Migration Strategy
**Documented in v2.0:**
- Keep RSA keys during transition
- Generate Ed25519 identity alongside
- Re-encrypt entries on first unlock
- Mark migration complete in vault_meta

### Gap 5: TLS Certificate Expiration
**Documented in v2.0:**
- Certs expire after 365 days
- No automatic renewal (by design)
- Re-pairing required after expiration
- Forces periodic re-verification

### Gap 6: TOTP Requires Unlocked Vault
**UX Constraint:**
- Vault must be unlocked to generate TOTP
- Master key needed for derivation
- Show unlock prompt if vault locked

---

## WHICH DOCUMENT TO USE

### ❌ DO NOT USE:
- `docs/refactor_plan.md` (original analysis)
- `docs/IMPLEMENTATION_ROADMAP.md` v1.0 (contains bugs)

### ✅ USE:
- **`docs/IMPLEMENTATION_ROADMAP_CORRECTED.md` v2.0** (this version has all fixes)
- `docs/QUICK_START.md` (overview, but cross-reference with corrected roadmap)
- `docs/IMPLEMENTATION_CHEATSHEET.md` (reference, but verify against corrected code)

---

## PRE-IMPLEMENTATION CHECKLIST

Before writing any code:

- [ ] Read `IMPLEMENTATION_ROADMAP_CORRECTED.md` v2.0 completely
- [ ] Review this bug fix document
- [ ] Verify understanding of X25519 key derivation (CRITICAL)
- [ ] Set up test environment with 2 devices
- [ ] Create feature branch: `feature/p2p-security-rewrite`
- [ ] Backup existing vault data

---

## VERIFICATION COMMANDS

After implementing each phase:

```bash
# Phase 1
$ go test ./internal/pairing -v  # Test TOTP
go test ./internal/state -v      # Test rate limiting

# Phase 2
$ go test ./internal/transport -v  # Test TLS
go test ./internal/discovery -v    # Test mDNS

# Phase 3
$ go test ./internal/identity -v  # Test Ed25519/X25519
go test ./internal/crypto -v      # Test NaCl box

# Phase 4
$ go test ./internal/sync -v  # Test Lamport clocks

# All phases
$ go test ./... -v -cover  # Full test suite
```

---

## SECURITY AUDIT CHECKLIST

Before deploying:

- [ ] X25519 keys properly derived (not copied from Ed25519 seed)
- [ ] TOTP uses HKDF-derived sub-key (not master key)
- [ ] TLS handshake completes before accessing certificates
- [ ] Nonce is randomly generated for each NaCl box operation
- [ ] Rate limiting allows exactly 5 attempts
- [ ] Certificate pinning active (TOFU)
- [ ] All imports present (no undefined references)
- [ ] Model changes reflected in database schema
- [ ] All routes registered in main.go

---

**Last Updated:** March 14, 2026  
**Critical Fixes:** 7 bugs fixed  
**Gaps Addressed:** 6 gaps filled  
**Status:** ✅ CORRECTED VERSION READY FOR IMPLEMENTATION
