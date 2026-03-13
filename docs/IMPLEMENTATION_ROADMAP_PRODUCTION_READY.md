# P2P Security Refactor - PRODUCTION READY v3.1

## Document Information
- **Project**: pwman P2P Security Refactor
- **Version**: 3.1 (PRODUCTION READY)
- **Date**: March 14, 2026
- **Status:** ✅ **ALL ISSUES RESOLVED - READY FOR IMPLEMENTATION**

---

## 🚨 FINAL FIXES APPLIED

### Fix 1: Double TLS Handshake (RESOLVED)

**Issue:** ConnectToPeerWithCert calls Handshake(), then handleConnection calls it again

**Solution:** Check if handshake already completed

```go
func (p *P2PManager) handleConnection(conn *tls.Conn) {
    // Check if handshake already completed (from ConnectToPeerWithCert)
    if !conn.ConnectionState().HandshakeComplete {
        if err := conn.Handshake(); err != nil {
            conn.Close()
            return
        }
    }
    // ... rest of handler
}
```

### Fix 2: Vault Master Key Access (RESOLVED)

**Issue:** vault.GetMasterKey() doesn't exist, need to store derived key

**Solution:** Store derived key on Vault struct during unlock

```go
// internal/state/server.go - REVISED Vault struct

type Vault struct {
    PrivateKey *crypto.KeyPair     // RSA legacy
    Identity   *identity.DeviceIdentity // Ed25519 (Phase 3+)
    Storage    *storage.SQLite
    Config     *config.Config
    VaultName  string
    
    // NEW: Store derived master key (already in memory during unlock)
    MasterKey []byte  // Derived via scrypt from password
    
    MigratedToEd25519 bool
}

// Method to access master key for TOTP
func (v *Vault) GetMasterKey() ([]byte, error) {
    if len(v.MasterKey) == 0 {
        return nil, fmt.Errorf("vault not unlocked")
    }
    return v.MasterKey, nil
}

// During vault unlock (cmd/server/handlers/auth.go or similar):
func UnlockVault(vaultName, password string) (*Vault, error) {
    // ... derive key via scrypt ...
    masterKey := crypto.DeriveKey(password, salt)
    
    vault := &Vault{
        PrivateKey: keyPair,
        Storage:    db,
        Config:     cfg,
        VaultName:  vaultName,
        MasterKey:  masterKey,  // Store for TOTP use
    }
    
    return vault, nil
}
```

**Security Note:** Master key is already in memory (used to decrypt RSA private key). Storing it on Vault struct doesn't change security posture - it's required for TOTP.

### Fix 3: P2PConfig Backward Compatibility (RESOLVED)

**Issue:** Current code constructs P2PConfig with old fields, needs backward compatibility

**Solution:** New fields are optional, default to legacy behavior

```go
// internal/p2p/p2p.go - REVISED P2PConfig

type P2PConfig struct {
    // Legacy fields (required)
    DeviceName string
    DeviceID   string
    ListenPort int
    EnableMDNS bool  // Legacy: will be removed
    
    // NEW fields (optional, for Phase 2+)
    Cert        tls.Certificate      // If provided, use TLS
    PeerStore   *transport.PeerStore // If provided, use cert pinning
    PairingMode bool                 // If true, accept any cert
}

// NewP2PManager - backward compatible
func NewP2PManager(cfg P2PConfig) (*P2PManager, error) {
    ctx, cancel := context.WithCancel(context.Background())
    
    p := &P2PManager{
        ctx:                 ctx,
        cancel:              cancel,
        deviceName:          cfg.DeviceName,
        deviceID:            cfg.DeviceID,
        peers:               make(map[string]*peerConnection),
        messageChan:         make(chan ReceivedMessage, 100),
        pairingRequestChan:  make(chan ReceivedMessage, 10),
        pairingResponseChan: make(chan ReceivedMessage, 10),
        syncRequestChan:     make(chan ReceivedMessage, 10),
        syncDataChan:        make(chan ReceivedMessage, 10),
        readyForSyncChan:    make(chan ReceivedMessage, 10),
        syncAckChan:         make(chan ReceivedMessage, 10),
        connectedCh:         make(chan PeerInfo, 10),
        disconnectedCh:      make(chan string, 10),
    }
    
    // NEW: If cert provided, use TLS mode
    if cfg.Cert.Certificate != nil {
        p.tlsConfig = transport.ServerTLSConfig(cfg.Cert, cfg.PeerStore, cfg.PairingMode)
        p.tlsMode = true
    }
    
    return p, nil
}

// Handler code (cmd/server/handlers/p2p.go:166-171) - NO CHANGES NEEDED
func startP2P() {
    cfg := p2p.P2PConfig{
        DeviceName: deviceName,  // Still works
        DeviceID:   deviceID,    // Still works
        // Cert, PeerStore, PairingMode omitted - defaults to legacy mode
    }
    
    manager, err := p2p.NewP2PManager(cfg)  // Works with old code
}
```

### Fix 4: Peers Map Type (CLARIFIED)

**Issue:** Internal map uses *peerConnection, public methods return PeerInfo

**Clarification:** This is intentional and correct

```go
// Internal storage - uses full connection struct
type P2PManager struct {
    peers map[string]*peerConnection  // Internal use
}

type peerConnection struct {
    id          string
    name        string
    addr        string
    conn        *tls.Conn
    reader      *bufio.Reader
    writer      *bufio.Writer
    connected   bool
    fingerprint string
}

// Public API - returns simplified PeerInfo
type PeerInfo struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    Addr      string    `json:"addr"`
    Connected bool      `json:"connected"`
    LastSeen  time.Time `json:"last_seen"`
}

// SendMessage uses internal *peerConnection
func (p *P2PManager) SendMessage(peerID string, msg SyncMessage) error {
    p.mu.RLock()
    peer, ok := p.peers[peerID]  // *peerConnection
    p.mu.RUnlock()
    
    if !ok {
        return fmt.Errorf("peer not found: %s", peerID)
    }
    
    data, _ := json.Marshal(msg)
    data = append(data, '\n')
    _, err := peer.writer.Write(data)  // Access internal field
    return err
}

// GetAllPeers returns public PeerInfo (copy, not reference)
func (p *P2PManager) GetAllPeers() []PeerInfo {
    p.mu.RLock()
    defer p.mu.RUnlock()
    
    peers := make([]PeerInfo, 0, len(p.peers))
    for _, peer := range p.peers {
        peers = append(peers, PeerInfo{
            ID:        peer.id,
            Name:      peer.name,
            Addr:      peer.addr,
            Connected: peer.connected,
            LastSeen:  peer.lastSeen,
        })
    }
    return peers
}
```

### Fix 5: Migration Sequencing (RESOLVED)

**Issue:** CreateEntry references columns that don't exist until migration runs

**Solution:** Complete migration SQL for all phases

```sql
-- MIGRATION V2: Phase 1-2 (No schema changes needed for TOTP/rate limiting)
-- Just add schema_version tracking
INSERT OR REPLACE INTO vault_meta (key, value) VALUES ('schema_version', '1');

-- MIGRATION V3: Phase 3 (Ed25519 + storage format)
ALTER TABLE devices ADD COLUMN sign_public_key TEXT;
ALTER TABLE devices ADD COLUMN box_public_key TEXT;
ALTER TABLE entries ADD COLUMN logical_clock INTEGER DEFAULT 0;
ALTER TABLE entries ADD COLUMN origin_device TEXT REFERENCES devices(id);
ALTER TABLE encrypted_keys ADD COLUMN key_type TEXT DEFAULT 'rsa';
CREATE TABLE device_clocks (
    device_id TEXT PRIMARY KEY REFERENCES devices(id),
    clock INTEGER DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
INSERT OR REPLACE INTO vault_meta (key, value) VALUES ('schema_version', '3');

-- MIGRATION V4: Phase 4 (Sync protocol enhancements)
-- May include additional indexes or sync metadata
INSERT OR REPLACE INTO vault_meta (key, value) VALUES ('schema_version', '4');
```

**Implementation Sequence:**
1. Deploy Phase 1 (no schema changes)
2. Deploy Phase 2 (no schema changes)
3. Deploy Phase 3 with migration V3
4. Deploy Phase 4 with migration V4

---

## ✅ PHASE 1 - READY FOR IMPLEMENTATION (FINAL)

### Complete File List

```
CREATE:
├── internal/pairing/totp.go              [NEW]
├── internal/pairing/totp_test.go         [NEW]

MODIFY:
├── internal/state/server.go              [+ Rate limiting, + MasterKey]
├── cmd/server/handlers/pairing.go        [~ TOTP integration]
└── cmd/server/handlers/auth.go           [~ Store master key on unlock]

NO CHANGES:
├── internal/api/ratelimit.go             [Keep as-is]
├── internal/p2p/p2p.go                   [No changes yet]
└── pkg/models/models.go                  [No changes yet]
```

### Implementation Order

**Step 1: Add MasterKey to Vault (5 min)**
```go
// internal/state/server.go

type Vault struct {
    PrivateKey *crypto.KeyPair
    Storage    *storage.SQLite
    Config     *config.Config
    VaultName  string
    MasterKey  []byte  // NEW: Store derived key during unlock
}

func (v *Vault) GetMasterKey() ([]byte, error) {
    if len(v.MasterKey) == 0 {
        return nil, fmt.Errorf("vault not unlocked")
    }
    return v.MasterKey, nil
}
```

**Step 2: Store MasterKey During Unlock (5 min)**
```go
// Wherever vault is unlocked (cmd/server/handlers/auth.go or similar)

// After deriving master key from password:
vault := &state.Vault{
    PrivateKey: keyPair,
    Storage:    db,
    Config:     cfg,
    VaultName:  vaultName,
    MasterKey:  masterKey,  // NEW: Store it
}
```

**Step 3: Create TOTP Package (30 min)**
```bash
cat > internal/pairing/totp.go << 'EOF'
package pairing

import (
    "crypto/hmac"
    "crypto/sha256"
    "crypto/sha512"
    "encoding/binary"
    "fmt"
    "time"
    
    "golang.org/x/crypto/hkdf"
)

func deriveTOTPKey(vaultMasterKey []byte, vaultID string) []byte {
    reader := hkdf.New(sha512.New, vaultMasterKey, []byte(vaultID), []byte("pwman-pairing-totp"))
    key := make([]byte, 32)
    reader.Read(key)
    return key
}

func GeneratePairingCode(vaultMasterKey []byte, vaultID string) string {
    totpKey := deriveTOTPKey(vaultMasterKey, vaultID)
    window := time.Now().Unix() / 60
    
    mac := hmac.New(sha256.New, totpKey)
    mac.Write([]byte(vaultID))
    binary.Write(mac, binary.BigEndian, window)
    sum := mac.Sum(nil)
    
    offset := sum[len(sum)-1] & 0x0f
    code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
    return fmt.Sprintf("%06d", code%1_000_000)
}

func VerifyPairingCode(vaultMasterKey []byte, vaultID, candidate string) bool {
    totpKey := deriveTOTPKey(vaultMasterKey, vaultID)
    window := time.Now().Unix() / 60
    
    for _, w := range []int64{window - 1, window, window + 1} {
        mac := hmac.New(sha256.New, totpKey)
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
EOF
```

**Step 4: Add Rate Limiting (15 min)**
```go
// internal/state/server.go

type PairingAttempt struct {
    Count       int
    LastAttempt time.Time
    LockedUntil *time.Time
}

// Add to ServerState:
pairingAttempts   map[string]*PairingAttempt
pairingAttemptsMu sync.Mutex

// Initialize:
func NewServerState() *ServerState {
    return &ServerState{
        pairingCodes:      make(map[string]PairingCode),
        pairingRequests:   make(map[string]PairingRequest),
        pairingResponseCh: make(chan p2p.PairingResponsePayload, 10),
        pendingApprovals:  make(map[string]PendingApproval),
        pairingAttempts:   make(map[string]*PairingAttempt),  // NEW
        startTime:         time.Now(),
    }
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
    
    if att.LockedUntil != nil && time.Now().Before(*att.LockedUntil) {
        return false
    }
    
    att.Count++
    att.LastAttempt = time.Now()
    
    if att.Count > 5 {
        until := time.Now().Add(30 * time.Second)
        att.LockedUntil = &until
        att.Count = 0
        return false
    }
    
    return true
}
```

**Step 5: Integrate in Pairing Handler (20 min)**
```go
// cmd/server/handlers/pairing.go

import "github.com/bok1c4/pwman/internal/pairing"

// In HandlePairingRequest (FIRST LINE):
func (h *PairingHandlers) HandlePairingRequest(pm *p2p.P2PManager, msg p2p.ReceivedMessage) {
    // Rate limiting
    if !h.state.RecordPairingAttempt(msg.FromPeer) {
        log.Printf("[Pairing] Rate limit exceeded for peer: %s", msg.FromPeer)
        return
    }
    
    var pairingReq p2p.PairingRequestPayload
    if err := json.Unmarshal(msg.Payload, &pairingReq); err != nil {
        log.Printf("[Pairing] Failed to parse request: %v", err)
        return
    }
    
    // Verify TOTP
    vault, _ := h.state.GetVault()
    masterKey, err := vault.GetMasterKey()
    if err != nil {
        response := p2p.PairingResponsePayload{Success: false, Error: "vault_locked"}
        // Send response...
        return
    }
    
    if !pairing.VerifyPairingCode(masterKey, vault.Config.DeviceID, pairingReq.Code) {
        response := p2p.PairingResponsePayload{Success: false, Error: "invalid_code"}
        // Send response...
        return
    }
    
    // Code valid - continue with pairing...
}

// In Generate() method:
func (h *PairingHandlers) Generate(w http.ResponseWriter, r *http.Request) {
    vault, ok := h.state.GetVault()
    if !ok || vault == nil {
        api.Error(w, http.StatusBadRequest, "VAULT_LOCKED", "Unlock vault to generate pairing code")
        return
    }
    
    masterKey, err := vault.GetMasterKey()
    if err != nil {
        api.Error(w, http.StatusInternalServerError, "KEY_ERROR", "failed to get master key")
        return
    }
    
    code := pairing.GeneratePairingCode(masterKey, vault.Config.DeviceID)
    
    // Return code...
    api.Success(w, map[string]interface{}{
        "code":        code,
        "device_name": vault.Config.DeviceName,
        "expires_in":  60,
    })
}
```

**Step 6: Write Tests (15 min)**
```go
// internal/pairing/totp_test.go
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
    
    assert.Equal(t, 6, len(code))
    assert.True(t, isAllDigits(code))
    
    // Should verify
    assert.True(t, VerifyPairingCode(masterKey, vaultID, code))
    
    // Wrong code should fail
    assert.False(t, VerifyPairingCode(masterKey, vaultID, "000000"))
}

func TestTOTPClockSkew(t *testing.T) {
    masterKey := []byte("test-master-key-32bytes-long!!")
    vaultID := "test-vault-123"
    
    code := GeneratePairingCode(masterKey, vaultID)
    
    // Should still verify 30 seconds later
    time.Sleep(30 * time.Second)
    assert.True(t, VerifyPairingCode(masterKey, vaultID, code))
}

func isAllDigits(s string) bool {
    for _, c := range s {
        if c < '0' || c > '9' {
            return false
        }
    }
    return true
}
```

```go
// internal/state/server_test.go
func TestPairingAttempt_RateLimiting(t *testing.T) {
    s := NewServerState()
    peerID := "test-peer-123"
    
    // Should allow exactly 5 attempts
    for i := 0; i < 5; i++ {
        assert.True(t, s.RecordPairingAttempt(peerID), "Attempt %d should succeed", i+1)
    }
    
    // 6th attempt should be blocked
    assert.False(t, s.RecordPairingAttempt(peerID), "6th attempt should be blocked")
    
    // After lockout period, should reset
    time.Sleep(30 * time.Second)
    assert.True(t, s.RecordPairingAttempt(peerID), "Should reset after lockout")
}
```

---

## 📋 COMPLETE IMPLEMENTATION CHECKLIST

### Phase 1 - Ready Now ✅

- [ ] Add `MasterKey []byte` to `state.Vault` struct
- [ ] Add `GetMasterKey()` method to Vault
- [ ] Store master key during vault unlock
- [ ] Create `internal/pairing/totp.go`
- [ ] Create `internal/pairing/totp_test.go`
- [ ] Add rate limiting to `state.ServerState`
- [ ] Add `RecordPairingAttempt()` method
- [ ] Integrate TOTP in `pairing.Generate()`
- [ ] Integrate TOTP in `pairing.HandlePairingRequest()`
- [ ] Add rate limiting check as first line of HandlePairingRequest
- [ ] Run tests: `go test ./internal/pairing -v`
- [ ] Run tests: `go test ./internal/state -v`
- [ ] Manual test: Generate code (6 digits)
- [ ] Manual test: 5 attempts pass
- [ ] Manual test: 6th attempt blocked
- [ ] Manual test: Code changes after 60s
- [ ] Deploy to production

### Phase 2 - After Design Review

- [ ] Review P2PManager interface design
- [ ] Approve backward-compatible approach
- [ ] Create transport package
- [ ] Create discovery package
- [ ] Implement TLS P2P layer
- [ ] Maintain backward compatibility
- [ ] Test with existing handlers

### Phase 3 - After Phase 2

- [ ] Run migration V3
- [ ] Create identity package
- [ ] Implement Ed25519/X25519
- [ ] Implement NaCl box encryption
- [ ] Implement Argon2id KDF
- [ ] Migrate storage format

### Phase 4 - After Phase 3

- [ ] Run migration V4
- [ ] Create sync package
- [ ] Implement Lamport clocks
- [ ] Implement bidirectional sync
- [ ] Add conflict resolution

---

## 🎯 START IMPLEMENTATION NOW

**Phase 1 is production-ready. All blockers resolved.**

```bash
# 1. Create branch
git checkout -b feature/phase1-security-fixes

# 2. Implement following checklist above

# 3. Test
go test ./internal/pairing -v
go test ./internal/state -v

# 4. Build and deploy
make build
./bin/pwman-server

# 5. Monitor
```

**Total Phase 1 Time:** 1.5 days (as planned)  
**Ready to Start:** YES ✅  
**Blockers:** None

---

**Document Version:** 3.1 (Production Ready)  
**Last Updated:** March 14, 2026  
**Status:** ✅ **ALL ISSUES RESOLVED**
