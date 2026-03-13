# P2P Vault Sync - Improvements & Recommendations

## Executive Summary

This document compares the current implementation with the desired workflow and provides specific recommendations for improving security, usability, and adherence to the original design goals.

---

## 1. Comparison Matrix

| Feature | Current Implementation | Desired Workflow | Priority |
|---------|----------------------|------------------|----------|
| **Identity Keys** | RSA-4096 | Ed25519 | High |
| **Discovery** | libp2p mDNS | Standard mDNS/UDP 5353 | Medium |
| **Pairing Code** | 9-char random | TOTP (time-based) | Critical |
| **Transport Security** | Noise protocol | Mutual TLS 1.3 + cert pinning | High |
| **Fingerprint Verification** | Automatic | Manual visual verification | High |
| **Rate Limiting** | None | 5 attempts then backoff | Critical |
| **Conflict Resolution** | Simple versioning | Vector clocks | Medium |
| **Key Derivation** | scrypt | Argon2id | Medium |

---

## 2. Critical Issues

### 2.1 ❌ TOTP-Based Pairing (CRITICAL)

**Current:** Uses static random 9-character code
```go
// cmd/server/handlers/pairing.go:47-55
func generatePairingCode() string {
    const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
    code := make([]byte, 9)
    // Random generation...
    return fmt.Sprintf("%s-%s-%s", string(code[0:3]), string(code[3:6]), string(code[6:9]))
}
```

**Problem:** Static codes are vulnerable to replay attacks if intercepted. No proof of physical proximity.

**Desired:** TOTP (Time-based One-Time Password)
```go
// Example implementation
func GeneratePairingCode(vaultMasterKey []byte, vaultID string) string {
    window := time.Now().Unix() / 60 // 60-second window
    mac := hmac.New(sha256.New, vaultMasterKey)
    mac.Write([]byte(vaultID))
    binary.Write(mac, binary.BigEndian, window)
    sum := mac.Sum(nil)
    // HOTP truncation to 6 digits
    offset := sum[len(sum)-1] & 0x0f
    code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
    return fmt.Sprintf("%06d", code%1_000_000)
}
```

**Impact:** HIGH - Anyone with the code can join even after it expires if they save it.

---

### 2.2 ❌ Rate Limiting (CRITICAL)

**Current:** No rate limiting on pairing attempts
```go
// cmd/server/handlers/pairing.go:591-620
func (h *PairingHandlers) HandlePairingRequest(pm *p2p.P2PManager, msg p2p.ReceivedMessage) {
    // ... validates code directly without tracking attempts
    code, exists := h.state.GetPairingCode(pairingReq.Code)
    // Brute force possible - 9 chars = 32^9 combinations
}
```

**Problem:** A 9-character code from a 32-character set has 32^9 ≈ 3.5 × 10^13 combinations. Without rate limiting, an attacker could brute force this over time.

**Desired:** Track failed attempts per peer/connection
```go
type PairingAttemptTracker struct {
    attempts  int
    lastAttempt time.Time
    lockedUntil time.Time
}

func (h *PairingHandlers) HandlePairingRequest(...) {
    tracker := h.state.GetAttemptTracker(fromPeer)
    if tracker.attempts >= 5 {
        return // Drop connection
    }
    
    if !validCode {
        tracker.attempts++
        if tracker.attempts >= 5 {
            time.Sleep(30 * time.Second) // Backoff
        }
    }
}
```

**Impact:** CRITICAL - Pairing codes can be brute forced.

---

### 2.3 ❌ Certificate Pinning & TOFU (HIGH)

**Current:** Uses libp2p Noise protocol with automatic trust
```go
// internal/p2p/p2p.go:96-107
opts := []libp2p.Option{
    libp2p.Security(noise.ID, noise.New),  // Automatic, no pinning
}
```

**Problem:** No mechanism to verify peer identity on first connection. An attacker on the LAN could intercept connections.

**Desired:** Mutual TLS with certificate pinning
```go
// Example: internal/transport/tls.go
type PeerStore struct {
    peers map[string]PinnedPeer // fingerprint → device
}

type PinnedPeer struct {
    Fingerprint string
    DeviceName  string
    PinnedAt    time.Time
}

func ServerTLSConfig(cert tls.Certificate, store *PeerStore) *tls.Config {
    return &tls.Config{
        Certificates: []tls.Certificate{cert},
        ClientAuth:   tls.RequireAnyClientCert,
        MinVersion:   tls.VersionTLS13,
        VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
            fp := certFingerprint(rawCerts[0])
            if !store.IsTrusted(fp) {
                return fmt.Errorf("untrusted peer: %s", fp)
            }
            return nil
        },
    }
}
```

**Impact:** HIGH - MITM attacks possible on initial pairing.

---

## 3. High Priority Issues

### 3.1 ❌ Ed25519 Identity Keys

**Current:** RSA-4096
```go
// internal/crypto/crypto.go:24-33
func GenerateRSAKeyPair(bits int) (*KeyPair, error) {
    privateKey, err := rsa.GenerateKey(rand.Reader, bits)
    // ...
}
```

**Problems:**
- RSA keys are large (significantly larger than Ed25519)
- Slower key generation and operations
- PKCS#1v15 padding has known issues (though the implementation uses it correctly)

**Desired:** Ed25519
```go
// Example implementation
func GenerateEd25519KeyPair() (*Ed25519KeyPair, error) {
    pub, priv, err := ed25519.GenerateKey(rand.Reader)
    return &Ed25519KeyPair{
        PublicKey:  pub,
        PrivateKey: priv,
        Fingerprint: fingerprint(pub),
    }, nil
}
```

**Migration Path:**
1. Generate Ed25519 keys alongside RSA keys during vault creation
2. Add fingerprint migration in database
3. Support both key types during transition period
4. Eventually deprecate RSA

---

### 3.2 ❌ Manual Fingerprint Verification

**Current:** Automatic trust establishment
```go
// cmd/server/handlers/pairing.go:611-620
// After TOTP verification, device is automatically trusted
if !VerifyPairingCode(...) {
    return error
}
// Device automatically trusted - no user verification
```

**Problem:** User never verifies they're connecting to the correct device. A malicious device could impersonate the generator.

**Desired:** Visual fingerprint comparison
```go
// UI flow:
// 1. Device A shows: "Fingerprint: ab:cd:ef:12:34:56"
// 2. Device B shows: "Verify Device A fingerprint: ab:cd:ef:12:34:56"
// 3. User confirms match on Device B
// 4. Device B pins Device A's certificate
```

**Implementation:**
```go
type PairingHandlers struct {
    // Add fingerprint verification channel
    fingerprintVerifyCh chan FingerprintVerifyRequest
}

type FingerprintVerifyRequest struct {
    DeviceID    string
    Fingerprint string
    Approved    chan bool
}

func (h *PairingHandlers) Join(w http.ResponseWriter, r *http.Request) {
    // After receiving response
    serverFP := certFingerprint(conn.ConnectionState().PeerCertificates[0].Raw)
    
    // Show user and wait for confirmation
    approved := make(chan bool, 1)
    h.fingerprintVerifyCh <- FingerprintVerifyRequest{
        DeviceID:    response.DeviceID,
        Fingerprint: serverFP,
        Approved:    approved,
    }
    
    if !<-approved {
        return error
    }
    
    // Now pin the certificate
    cfg.PeerStore.Trust(serverFP, peer.DeviceName)
}
```

---

### 3.3 ❌ Argon2id Key Derivation

**Current:** scrypt
```go
// internal/crypto/crypto.go:233-238
func DeriveKey(password string, salt []byte) ([]byte, error) {
    key, err := scrypt.Key([]byte(password), salt, N, R, P, SCRYPTKeyLen)
    // N=16384, R=8, P=1
}
```

**Problem:** scrypt is good but Argon2id is the modern recommended standard (won Password Hashing Competition).

**Desired:** Argon2id
```go
import "golang.org/x/crypto/argon2"

func DeriveKeyArgon2id(password string, salt []byte) []byte {
    return argon2.IDKey([]byte(password), salt, 
        3,       // time (iterations)
        64*1024, // memory (64 MB)
        4,       // threads
        32)      // key length
}
```

**Migration:**
- Add `key_derivation` field to config ("scrypt" or "argon2id")
- Use existing method for old vaults
- Use Argon2id for new vaults

---

## 4. Medium Priority Issues

### 4.1 ❌ Vector Clocks for Conflict Resolution

**Current:** Simple versioning with wall clock timestamps
```go
// pkg/models/models.go:15-27
type PasswordEntry struct {
    Version   int64     // Simple incrementing version
    UpdatedAt time.Time // Wall clock timestamp
    UpdatedBy string    // Device ID
}
```

**Problem:** Wall clocks can drift or be set backward. Last-write-wins based on timestamps can cause data loss.

**Desired:** Vector clocks for proper causal ordering
```go
type VectorClock map[string]int64 // deviceID → logical timestamp

type PasswordEntry struct {
    ID          string
    Version     VectorClock
    ModifiedAt  int64 // Monotonic logical clock
}

func MergeEntries(local, remote []Entry, localVC, remoteVC VectorClock) []Entry {
    // Compare vector clocks to determine causal relationship
    // Handle concurrent updates properly
}
```

---

### 4.2 ❌ Standard mDNS Implementation

**Current:** libp2p's built-in mDNS
```go
// internal/p2p/p2p.go:210-220
func (p *P2PManager) startMDNS() error {
    mdns := mdns.NewMdnsService(p.host, "pwman", p)
    return mdns.Start()
}
```

**Problem:** Tightly coupled to libp2p, less control over discovery protocol.

**Desired:** Standard mDNS with TXT records
```go
// internal/discovery/advertise.go
const ServiceType = "_pwman._tcp"

func Announce(vaultID string, deviceName string, port int) (*Advertiser, error) {
    txt := []string{
        fmt.Sprintf("vault=%s", vaultID),
        fmt.Sprintf("device=%s", deviceName),
        "version=1",
    }
    
    server, err := zeroconf.Register(
        deviceName,
        ServiceType,
        "local.",
        port,
        txt,
        nil,
    )
    return &Advertiser{server: server}, nil
}
```

**Benefits:**
- Better control over service metadata
- Standard port 5353 multicast
- Easier to debug with standard tools (avahi-browse, dns-sd)

---

## 5. Recommended Implementation Roadmap

### Phase 1: Security Hardening (Immediate)

1. **Add Rate Limiting**
   - Track failed pairing attempts per peer
   - Implement exponential backoff
   - Lock out after 5 failed attempts

2. **Implement TOTP Pairing**
   - Replace random codes with TOTP
   - Use vault master key as TOTP seed
   - 60-second validity windows with ±1 window tolerance

3. **Add Rate Limiting to HTTP API**
   - Use existing ratelimit.go infrastructure
   - Apply to pairing endpoints

### Phase 2: Transport Security (Next Sprint)

1. **Certificate Pinning**
   - Create PeerStore for TOFU model
   - Add manual fingerprint verification UI
   - Store pinned fingerprints persistently

2. **Migrate to Ed25519**
   - Add Ed25519 key generation
   - Support dual key types during migration
   - Update fingerprint calculation

3. **Argon2id Migration**
   - Implement Argon2id key derivation
   - Add backward compatibility for scrypt
   - Use for new vaults

### Phase 3: Protocol Improvements (Future)

1. **Vector Clocks**
   - Implement vector clock structure
   - Update conflict resolution logic
   - Add proper causal ordering

2. **Standard mDNS**
   - Implement zeroconf-based discovery
   - Add TXT record metadata
   - Maintain libp2p as fallback

3. **Delta Sync**
   - Implement delta encoding for sync
   - Reduce bandwidth for large vaults
   - Maintain full sync as fallback

---

## 6. Specific Code Changes Required

### 6.1 Add TOTP Pairing

**Files to modify:**
- `cmd/server/handlers/pairing.go:47-55` - Replace `generatePairingCode()`
- `cmd/server/handlers/pairing.go:602` - Replace code validation

**New file:** `internal/pairing/totp.go`
```go
package pairing

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/binary"
    "fmt"
    "time"
)

func GeneratePairingCode(vaultMasterKey []byte, vaultID string) string {
    window := time.Now().Unix() / 60
    mac := hmac.New(sha256.New, vaultMasterKey)
    mac.Write([]byte(vaultID))
    binary.Write(mac, binary.BigEndian, window)
    sum := mac.Sum(nil)
    offset := sum[len(sum)-1] & 0x0f
    code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
    return fmt.Sprintf("%06d", code%1_000_000)
}

func VerifyPairingCode(vaultMasterKey []byte, vaultID string, candidate string) bool {
    window := time.Now().Unix() / 60
    for _, w := range []int64{window - 1, window, window + 1} {
        mac := hmac.New(sha256.New, vaultMasterKey)
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
```

### 6.2 Add Rate Limiting

**Modify:** `internal/state/server.go`
```go
type PairingAttempt struct {
    Count       int
    LastAttempt time.Time
    LockedUntil *time.Time
}

type ServerState struct {
    // ... existing fields
    pairingAttempts   map[string]*PairingAttempt // peerID → attempts
    pairingAttemptsMu sync.Mutex
}

func (s *ServerState) RecordPairingAttempt(peerID string) bool {
    s.pairingAttemptsMu.Lock()
    defer s.pairingAttemptsMu.Unlock()
    
    attempt, exists := s.pairingAttempts[peerID]
    if !exists {
        s.pairingAttempts[peerID] = &PairingAttempt{Count: 1, LastAttempt: time.Now()}
        return true
    }
    
    if attempt.LockedUntil != nil && time.Now().Before(*attempt.LockedUntil) {
        return false // Still locked
    }
    
    attempt.Count++
    attempt.LastAttempt = time.Now()
    
    if attempt.Count >= 5 {
        lockUntil := time.Now().Add(30 * time.Second)
        attempt.LockedUntil = &lockUntil
        return false
    }
    
    return true
}
```

### 6.3 Add Certificate Pinning

**New file:** `internal/transport/peerstore.go`
```go
package transport

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "os"
    "sync"
)

type PeerStore struct {
    mu      sync.RWMutex
    peers   map[string]PinnedPeer
    path    string
}

type PinnedPeer struct {
    Fingerprint string    `json:"fingerprint"`
    DeviceName  string    `json:"device_name"`
    PinnedAt    time.Time `json:"pinned_at"`
}

func NewPeerStore(path string) (*PeerStore, error) {
    ps := &PeerStore{
        peers: make(map[string]PinnedPeer),
        path:  path,
    }
    
    data, err := os.ReadFile(path)
    if err == nil {
        json.Unmarshal(data, &ps.peers)
    }
    
    return ps, nil
}

func (ps *PeerStore) IsTrusted(fingerprint string) bool {
    ps.mu.RLock()
    defer ps.mu.RUnlock()
    _, ok := ps.peers[fingerprint]
    return ok
}

func (ps *PeerStore) Trust(fingerprint, deviceName string) error {
    ps.mu.Lock()
    defer ps.mu.Unlock()
    
    ps.peers[fingerprint] = PinnedPeer{
        Fingerprint: fingerprint,
        DeviceName:  deviceName,
        PinnedAt:    time.Now(),
    }
    
    return ps.save()
}

func (ps *PeerStore) save() error {
    data, err := json.MarshalIndent(ps.peers, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(ps.path, data, 0600)
}
```

---

## 7. Common Mistakes to Avoid (From Desired Workflow)

### 7.1 Already Avoided ✅

✅ Using HTTP instead of mutual TLS - Uses encrypted libp2p streams  
✅ Broadcasting vault master key - Only vaultID in mDNS  
✅ Sending vault data before verification - Data sent after pairing only  

### 7.2 Currently Vulnerable ⚠️

⚠️ **Not rate-limiting pairing attempts** - Codes can be brute forced  
⚠️ **No forward secrecy** - Noise provides some, but TLS 1.3 would be better  
⚠️ **Trusting mDNS data as authoritative** - Should verify via pairing code only  

### 7.3 Implementation Tips

```go
// DON'T: Trust mDNS metadata blindly
peerInfo := mdns.DiscoveredPeer()
// Trusting peerInfo.Name directly is unsafe

// DO: Use mDNS only as connection hint, verify via pairing
addr := peerInfo.Address // Just a hint
conn := connect(addr)
// Verify identity through pairing code exchange, not mDNS data

// DON'T: Send vault preview before TOTP verification
if validCode {
    sendVaultData() // Good
} else {
    sendError() // Good
}

// DON'T: Skip schema validation
var syncData SyncDataPayload
if err := json.Unmarshal(msg.Payload, &syncData); err != nil {
    return // Always validate
}
// Also validate field sizes and structure
```

---

## 8. Summary

### Current State
The system is **functional but not production-ready** from a security standpoint:
- Basic P2P sync works
- Encryption is sound (RSA+AES)
- Discovery via libp2p mDNS works
- Simple pairing with codes works

### Critical Gaps
1. **TOTP pairing** - Essential for security
2. **Rate limiting** - Prevents brute force
3. **Certificate pinning** - Prevents MITM
4. **Manual fingerprint verification** - User confirmation

### Recommended Priority
1. **Immediate:** Add rate limiting (5 lines of code)
2. **Next:** Implement TOTP (new file, ~50 lines)
3. **Soon:** Add certificate pinning (~100 lines)
4. **Future:** Ed25519, Argon2id, vector clocks

### Effort Estimate
- **Phase 1 (Security):** 1-2 days
- **Phase 2 (Transport):** 3-5 days
- **Phase 3 (Protocol):** 1-2 weeks

The desired workflow represents a more secure, peer-reviewed approach to P2P sync. Implementing these recommendations would bring the system in line with security best practices for password managers.
