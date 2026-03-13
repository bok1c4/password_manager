# P2P Vault Sync - Refactor Implementation Plan

## Executive Summary

Based on the comprehensive analysis document, this refactor plan transforms the current pwman P2P system into a security-hardened implementation. The plan is divided into 4 phases spanning approximately 11 days of development work.

**Key Architectural Changes:**
- Replace libp2p + Noise with standard TCP + mutual TLS 1.3
- Replace static pairing codes with TOTP (time-based codes)
- Replace RSA-4096 with Ed25519 for identity, X25519 for encryption
- Implement TOFU (Trust On First Use) certificate pinning
- Add Lamport logical clocks for conflict resolution
- Replace scrypt with Argon2id for key derivation

---

## Current State vs Target State

### Current Implementation
```
┌────────────────────────────────────────────┐
│ Discovery: libp2p internal mDNS            │
│ Transport: Noise protocol (auto-trust)     │
│ Identity: RSA-4096 keys                    │
│ Pairing: 9-char static random codes        │
│ Encryption: RSA-OAEP + AES-256-GCM         │
│ KDF: scrypt                                │
│ Sync: Wall-clock timestamps (LWW)          │
│ Rate Limiting: None                        │
└────────────────────────────────────────────┘
```

### Target Implementation
```
┌────────────────────────────────────────────┐
│ Discovery: Standard zeroconf mDNS (5353)   │
│ Transport: Mutual TLS 1.3 + cert pinning   │
│ Identity: Ed25519 keys                     │
│ Pairing: 6-digit TOTP (60s windows)        │
│ Encryption: NaCl box + AES-256-GCM         │
│ KDF: Argon2id                              │
│ Sync: Lamport logical clocks               │
│ Rate Limiting: 5 attempts → 30s lockout    │
└────────────────────────────────────────────┘
```

---

## Phase 1: Critical Security Fixes (1.5 days)

### Priority: SHIP IMMEDIATELY

These changes fix the most critical security vulnerabilities without requiring architectural changes. They can be deployed as hotfixes to the existing codebase.

### 1.1 Add Rate Limiting on Pairing Attempts

**Files to Modify:**
- `internal/state/server.go` - Add PairingAttempt tracking
- `cmd/server/handlers/pairing.go` - Use rate limiter

**Implementation:**

```go
// internal/state/server.go

type PairingAttempt struct {
    Count       int
    LastAttempt time.Time
    LockedUntil *time.Time
}

type ServerState struct {
    // ... existing fields ...
    pairingAttempts   map[string]*PairingAttempt // key = peer libp2p ID
    pairingAttemptsMu sync.Mutex
}

// RecordPairingAttempt returns false if peer is locked out
// Must be called BEFORE code validation
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
    
    // Lock after 5 failed attempts
    if att.Count >= 5 {
        until := time.Now().Add(30 * time.Second)
        att.LockedUntil = &until
        return false
    }
    
    return true
}
```

**In pairing handler:**
```go
// cmd/server/handlers/pairing.go:591
func (h *PairingHandlers) HandlePairingRequest(pm *p2p.P2PManager, msg p2p.ReceivedMessage) {
    // ADD THIS AS FIRST LINE
    if !h.state.RecordPairingAttempt(msg.FromPeer) {
        log.Printf("[Pairing] Rate limit exceeded for peer: %s", msg.FromPeer)
        return // drop silently - do not reveal lockout
    }
    
    // ... rest of validation
}
```

### 1.2 Implement TOTP-Based Pairing

**New File:** `internal/pairing/totp.go`

```go
package pairing

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/binary"
    "fmt"
    "time"
)

// GeneratePairingCode produces a 6-digit TOTP code seeded from vault master key
// The code changes every 60 seconds
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

// VerifyPairingCode accepts codes from window-1, window, window+1
// to handle up to 60 seconds of clock skew
func VerifyPairingCode(vaultMasterKey []byte, vaultID, candidate string) bool {
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

**Modify:** `cmd/server/handlers/pairing.go`

Replace the generatePairingCode() function and validation logic:

```go
// OLD: Random 9-char code
// func generatePairingCode() string { ... }

// NEW: TOTP-based
code := pairing.GeneratePairingCode(vaultMasterKey, vault.Config.DeviceID)

// In validation:
// OLD: code check
// if code.Used || time.Now().After(code.ExpiresAt) { ... }

// NEW: TOTP verification
if !pairing.VerifyPairingCode(vaultMasterKey, code.VaultID, pairingReq.Code) {
    response = p2p.PairingResponsePayload{Success: false, Error: "invalid_code"}
    return
}
```

### 1.3 UI Updates for TOTP

**Frontend Changes Required:**
- Replace XXX-XXX-XXX display with 6-digit code
- Add countdown timer showing seconds until next code
- Show "Generating..." during code generation

**Expected UI Flow:**
1. User clicks "Generate Pairing Code"
2. Screen shows: "Your code: 847291" with 45s countdown
3. Code regenerates automatically every 60 seconds
4. User tells Device B: "Enter code 847291"

### Phase 1 Deliverables

✅ Rate limiting prevents brute force attacks  
✅ TOTP codes prevent replay attacks  
✅ Backward compatible (can ship as hotfix)  
✅ ~10 hours development time  

---

## Phase 2: Transport Security Overhaul (3.5 days)

### Priority: HIGH - Prevents MITM attacks

This phase replaces libp2p with standard TCP/TLS and implements certificate pinning.

### 2.1 Create PeerStore for TOFU

**New File:** `internal/transport/peerstore.go`

```go
package transport

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "os"
    "sync"
    "time"
)

// PeerStore implements TOFU (Trust On First Use) certificate pinning
// Location: ~/.pwman/vaults/<name>/trusted_peers.json
type PeerStore struct {
    mu    sync.RWMutex
    peers map[string]PinnedPeer
    path  string
}

type PinnedPeer struct {
    Fingerprint string    `json:"fingerprint"`
    DeviceName  string    `json:"device_name"`
    DeviceID    string    `json:"device_id"`
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

func (ps *PeerStore) IsTrusted(fp string) bool {
    ps.mu.RLock()
    defer ps.mu.RUnlock()
    _, ok := ps.peers[fp]
    return ok
}

func (ps *PeerStore) Trust(fp, deviceName, deviceID string) error {
    ps.mu.Lock()
    defer ps.mu.Unlock()
    
    ps.peers[fp] = PinnedPeer{
        Fingerprint: fp,
        DeviceName:  deviceName,
        DeviceID:    deviceID,
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

func CertFingerprint(rawCert []byte) string {
    h := sha256.Sum256(rawCert)
    return hex.EncodeToString(h[:])
}
```

### 2.2 TLS Certificate Configuration

**New File:** `internal/transport/tls.go`

```go
package transport

import (
    "crypto/ecdsa"
    "crypto/elliptic"
    "crypto/rand"
    "crypto/tls"
    "crypto/x509"
    "crypto/x509/pkix"
    "encoding/pem"
    "fmt"
    "math/big"
    "net"
    "time"
)

// GenerateTLSCert creates a self-signed TLS certificate
func GenerateTLSCert() (tls.Certificate, error) {
    priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    if err != nil {
        return tls.Certificate{}, err
    }
    
    template := x509.Certificate{
        SerialNumber: big.NewInt(1),
        Subject: pkix.Name{
            CommonName: "pwman-device",
        },
        NotBefore:   time.Now().Add(-time.Hour),
        NotAfter:    time.Now().Add(365 * 24 * time.Hour),
        KeyUsage:    x509.KeyUsageDigitalSignature,
        ExtKeyUsage: []x509.ExtKeyUsage{
            x509.ExtKeyUsageClientAuth,
            x509.ExtKeyUsageServerAuth,
        },
        IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
    }
    
    certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, 
        &priv.PublicKey, priv)
    if err != nil {
        return tls.Certificate{}, err
    }
    
    certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
    
    privDER, err := x509.MarshalECPrivateKey(priv)
    if err != nil {
        return tls.Certificate{}, err
    }
    privPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})
    
    return tls.X509KeyPair(certPEM, privPEM)
}

// ServerTLSConfig for Device A (vault owner)
// pairingMode=true: accept any cert (during initial pairing)
// pairingMode=false: require pinned cert
func ServerTLSConfig(cert tls.Certificate, peers *PeerStore, pairingMode bool) *tls.Config {
    return &tls.Config{
        Certificates: []tls.Certificate{cert},
        ClientAuth:   tls.RequireAnyClientCert,
        MinVersion:   tls.VersionTLS13,
        VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
            if len(rawCerts) == 0 {
                return fmt.Errorf("no client certificate presented")
            }
            
            if pairingMode {
                // During pairing: accept any cert, will pin after TOTP verification
                return nil
            }
            
            fp := CertFingerprint(rawCerts[0])
            if !peers.IsTrusted(fp) {
                return fmt.Errorf("untrusted peer certificate: %s", fp)
            }
            return nil
        },
    }
}

// ClientTLSConfig for Device B (joiner)
func ClientTLSConfig(cert tls.Certificate, peers *PeerStore, pairingMode bool) *tls.Config {
    return &tls.Config{
        Certificates:       []tls.Certificate{cert},
        InsecureSkipVerify: true, // We verify manually below
        MinVersion:         tls.VersionTLS13,
        VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
            if len(rawCerts) == 0 {
                return fmt.Errorf("no server certificate presented")
            }
            
            if pairingMode {
                // During pairing: accept any cert, user verifies manually
                return nil
            }
            
            fp := CertFingerprint(rawCerts[0])
            if !peers.IsTrusted(fp) {
                return fmt.Errorf("untrusted server certificate: %s", fp)
            }
            return nil
        },
    }
}
```

### 2.3 Replace libp2p with Standard TCP/TLS

**Major Rewrite:** `internal/p2p/p2p.go`

Replace the entire libp2p-based implementation:

```go
package p2p

import (
    "bufio"
    "context"
    "crypto/tls"
    "encoding/json"
    "fmt"
    "net"
    "sync"
    "time"
    
    "github.com/bok1c4/pwman/internal/transport"
)

type P2PManager struct {
    listener    net.Listener
    tlsConfig   *tls.Config
    peerStore   *transport.PeerStore
    ctx         context.Context
    cancel      context.CancelFunc
    mu          sync.RWMutex
    peers       map[string]*PeerConnection
    
    // Channels for events
    pairingRequestChan  chan ReceivedMessage
    pairingResponseChan chan ReceivedMessage
    syncRequestChan     chan ReceivedMessage
    syncDataChan        chan ReceivedMessage
}

type PeerConnection struct {
    ID        string
    Name      string
    Addr      string
    Conn      *tls.Conn
    Reader    *bufio.Reader
    Writer    *bufio.Writer
    Connected bool
    LastSeen  time.Time
}

func NewP2PManager(cert tls.Certificate, peerStore *transport.PeerStore, pairingMode bool) (*P2PManager, error) {
    ctx, cancel := context.WithCancel(context.Background())
    
    tlsConfig := transport.ServerTLSConfig(cert, peerStore, pairingMode)
    
    return &P2PManager{
        ctx:                 ctx,
        cancel:              cancel,
        tlsConfig:           tlsConfig,
        peerStore:           peerStore,
        peers:               make(map[string]*PeerConnection),
        pairingRequestChan:  make(chan ReceivedMessage, 10),
        pairingResponseChan: make(chan ReceivedMessage, 10),
        syncRequestChan:     make(chan ReceivedMessage, 10),
        syncDataChan:        make(chan ReceivedMessage, 10),
    }, nil
}

func (p *P2PManager) Start(port int) error {
    ln, err := tls.Listen("tcp", fmt.Sprintf(":%d", port), p.tlsConfig)
    if err != nil {
        return fmt.Errorf("failed to start TLS listener: %w", err)
    }
    p.listener = ln
    
    go p.acceptLoop()
    return nil
}

func (p *P2PManager) acceptLoop() {
    for {
        conn, err := p.listener.Accept()
        if err != nil {
            select {
            case <-p.ctx.Done():
                return
            default:
                continue
            }
        }
        
        tlsConn := conn.(*tls.Conn)
        go p.handleConnection(tlsConn)
    }
}

func (p *P2PManager) handleConnection(conn *tls.Conn) {
    // Get peer fingerprint from TLS state
    state := conn.ConnectionState()
    if len(state.PeerCertificates) == 0 {
        conn.Close()
        return
    }
    
    peerFP := transport.CertFingerprint(state.PeerCertificates[0].Raw)
    
    peerConn := &PeerConnection{
        ID:        peerFP,
        Conn:      conn,
        Reader:    bufio.NewReader(conn),
        Writer:    bufio.NewWriter(conn),
        Connected: true,
        LastSeen:  time.Now(),
    }
    
    p.mu.Lock()
    p.peers[peerFP] = peerConn
    p.mu.Unlock()
    
    // Read loop
    for {
        line, err := peerConn.Reader.ReadBytes('\n')
        if err != nil {
            break
        }
        
        var msg SyncMessage
        if err := json.Unmarshal(line, &msg); err != nil {
            continue
        }
        
        p.routeMessage(ReceivedMessage{
            SyncMessage: msg,
            FromPeer:    peerFP,
        })
    }
    
    // Cleanup
    conn.Close()
    p.mu.Lock()
    delete(p.peers, peerFP)
    p.mu.Unlock()
}

func (p *P2PManager) ConnectToPeer(addr string, cert tls.Certificate) error {
    config := transport.ClientTLSConfig(cert, p.peerStore, true)
    conn, err := tls.Dial("tcp", addr, config)
    if err != nil {
        return err
    }
    
    // Extract fingerprint and create peer connection
    state := conn.ConnectionState()
    peerFP := transport.CertFingerprint(state.PeerCertificates[0].Raw)
    
    peerConn := &PeerConnection{
        ID:        peerFP,
        Conn:      conn,
        Reader:    bufio.NewReader(conn),
        Writer:    bufio.NewWriter(conn),
        Connected: true,
        LastSeen:  time.Now(),
    }
    
    p.mu.Lock()
    p.peers[peerFP] = peerConn
    p.mu.Unlock()
    
    return nil
}

func (p *P2PManager) SendMessage(peerID string, msg SyncMessage) error {
    p.mu.RLock()
    peer, ok := p.peers[peerID]
    p.mu.RUnlock()
    
    if !ok {
        return fmt.Errorf("peer not found: %s", peerID)
    }
    
    data, err := json.Marshal(msg)
    if err != nil {
        return err
    }
    
    data = append(data, '\n')
    
    _, err = peer.Writer.Write(data)
    if err != nil {
        return err
    }
    
    return peer.Writer.Flush()
}
```

### 2.4 Standard mDNS Discovery

**New Package:** `internal/discovery/`

```go
// internal/discovery/advertise.go
package discovery

import (
    "context"
    "fmt"
    "time"
    
    "github.com/grandcat/zeroconf"
)

const ServiceType = "_pwman._tcp"

type Advertiser struct {
    server *zeroconf.Server
}

// Announce broadcasts this device on the LAN
// vaultID is a public UUID - not the master password or vault name
// Only non-secret metadata in TXT records (visible to all LAN devices)
func Announce(vaultID, deviceName string, port int) (*Advertiser, error) {
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
    if err != nil {
        return nil, fmt.Errorf("mDNS announce: %w", err)
    }
    
    return &Advertiser{server: server}, nil
}

func (a *Advertiser) Shutdown() {
    if a.server != nil {
        a.server.Shutdown()
    }
}
```

```go
// internal/discovery/browse.go
package discovery

import (
    "context"
    "fmt"
    "net"
    "time"
    
    "github.com/grandcat/zeroconf"
)

type PeerInfo struct {
    VaultID    string
    DeviceName string
    Addr       net.IP
    Port       int
}

// Browse scans the LAN for a specific vaultID
func Browse(ctx context.Context, vaultID string, timeout time.Duration) (*PeerInfo, error) {
    resolver, err := zeroconf.NewResolver(nil)
    if err != nil {
        return nil, err
    }
    
    entries := make(chan *zeroconf.ServiceEntry, 8)
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    
    err = resolver.Browse(ctx, ServiceType, "local.", entries)
    if err != nil {
        return nil, err
    }
    
    for {
        select {
        case entry := <-entries:
            if entry == nil {
                return nil, fmt.Errorf("vault %q not found", vaultID)
            }
            
            // Check if this entry matches our vault
            for _, txt := range entry.Text {
                if txt == fmt.Sprintf("vault=%s", vaultID) {
                    var ip net.IP
                    if len(entry.AddrIPv4) > 0 {
                        ip = entry.AddrIPv4[0]
                    } else if len(entry.AddrIPv6) > 0 {
                        ip = entry.AddrIPv6[0]
                    }
                    
                    return &PeerInfo{
                        VaultID:    vaultID,
                        DeviceName: extractTXT(entry.Text, "device"),
                        Addr:       ip,
                        Port:       entry.Port,
                    }, nil
                }
            }
            
        case <-ctx.Done():
            return nil, fmt.Errorf("vault %q not found within %s", vaultID, timeout)
        }
    }
}

func extractTXT(texts []string, key string) string {
    prefix := key + "="
    for _, txt := range texts {
        if len(txt) > len(prefix) && txt[:len(prefix)] == prefix {
            return txt[len(prefix):]
        }
    }
    return ""
}
```

### 2.5 Manual Fingerprint Verification UI

**New API Endpoint:** `/api/pairing/verify-fingerprint`

```go
// cmd/server/handlers/pairing.go

type FingerprintVerifyRequest struct {
    DeviceID    string `json:"device_id"`
    Fingerprint string `json:"fingerprint"`
    Approved    bool   `json:"approved"`
}

func (h *PairingHandlers) VerifyFingerprint(w http.ResponseWriter, r *http.Request) {
    var req FingerprintVerifyRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        api.Error(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
        return
    }
    
    // Store user's decision
    h.state.SetFingerprintDecision(req.DeviceID, req.Fingerprint, req.Approved)
    
    api.Success(w, map[string]bool{"approved": req.Approved})
}
```

**Frontend Flow:**
1. Device B connects to Device A via TLS
2. Device B extracts Device A's certificate fingerprint
3. Device B shows: "Device A fingerprint: ab:cd:ef:12:34:56 - does this match what's shown on Device A?"
4. User compares and clicks Confirm/Cancel
5. Only after confirmation does Device B send TOTP code

### Phase 2 Deliverables

✅ Standard zeroconf mDNS (port 5353)  
✅ Mutual TLS 1.3 with certificate pinning  
✅ TOFU PeerStore persisted to disk  
✅ Manual fingerprint verification UI  
✅ Replaced libp2p with standard TCP  
✅ ~28 hours development time  

---

## Phase 3: Identity & Crypto Upgrade (3 days)

### Priority: MEDIUM - Modernizes cryptography

### 3.1 Ed25519 Identity Keys

**New File:** `internal/identity/identity.go`

```go
package identity

import (
    "crypto/ed25519"
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    "encoding/pem"
    "fmt"
    "os"
    
    "golang.org/x/crypto/curve25519"
)

// DeviceIdentity holds Ed25519 for signing/identity and X25519 for encryption
type DeviceIdentity struct {
    // Ed25519 - for device identity, TLS certificates, signing
    SignPublicKey  ed25519.PublicKey
    SignPrivateKey ed25519.PrivateKey
    
    // X25519 - for NaCl box encryption of vault AES keys
    BoxPublicKey  [32]byte
    BoxPrivateKey [32]byte
    
    // Fingerprint: hex(sha256(SignPublicKey)[:8]) - 16 hex chars, readable
    Fingerprint string
}

func GenerateIdentity() (*DeviceIdentity, error) {
    pub, priv, err := ed25519.GenerateKey(rand.Reader)
    if err != nil {
        return nil, fmt.Errorf("failed to generate Ed25519 key: %w", err)
    }
    
    // Derive X25519 key from Ed25519 private key seed
    var boxPriv, boxPub [32]byte
    copy(boxPriv[:], priv.Seed()[:32])
    curve25519.ScalarBaseMult(&boxPub, &boxPriv)
    
    // Generate fingerprint: first 8 bytes of SHA-256 hash = 16 hex chars
    h := sha256.Sum256(pub)
    fp := hex.EncodeToString(h[:8])
    
    return &DeviceIdentity{
        SignPublicKey:  pub,
        SignPrivateKey: priv,
        BoxPublicKey:   boxPub,
        BoxPrivateKey:  boxPriv,
        Fingerprint:    fp,
    }, nil
}

func (id *DeviceIdentity) Save(path string) error {
    // Save Ed25519 private key
    privPEM := pem.EncodeToMemory(&pem.Block{
        Type:  "ED25519 PRIVATE KEY",
        Bytes: id.SignPrivateKey.Seed(),
    })
    
    if err := os.WriteFile(path, privPEM, 0600); err != nil {
        return err
    }
    
    // Save Ed25519 public key
    pubPEM := pem.EncodeToMemory(&pem.Block{
        Type:  "ED25519 PUBLIC KEY",
        Bytes: id.SignPublicKey,
    })
    
    pubPath := path + ".pub"
    if err := os.WriteFile(pubPath, pubPEM, 0644); err != nil {
        return err
    }
    
    return nil
}

func LoadIdentity(path string) (*DeviceIdentity, error) {
    // Load private key
    privData, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    block, _ := pem.Decode(privData)
    if block == nil {
        return nil, fmt.Errorf("failed to decode private key PEM")
    }
    
    priv := ed25519.NewKeyFromSeed(block.Bytes)
    pub := priv.Public().(ed25519.PublicKey)
    
    // Derive X25519 keys
    var boxPriv, boxPub [32]byte
    copy(boxPriv[:], priv.Seed()[:32])
    curve25519.ScalarBaseMult(&boxPub, &boxPriv)
    
    // Generate fingerprint
    h := sha256.Sum256(pub)
    fp := hex.EncodeToString(h[:8])
    
    return &DeviceIdentity{
        SignPublicKey:  pub,
        SignPrivateKey: priv,
        BoxPublicKey:   boxPub,
        BoxPrivateKey:  boxPriv,
        Fingerprint:    fp,
    }, nil
}
```

### 3.2 NaCl Box Encryption

**Modify:** `internal/crypto/hybrid.go`

Replace RSA-OAEP with NaCl box (X25519 + XSalsa20 + Poly1305):

```go
package crypto

import (
    "encoding/base64"
    "fmt"
    
    "github.com/bok1c4/pwman/pkg/models"
    "golang.org/x/crypto/nacl/box"
)

// BoxEncryptedData holds encrypted password and per-device box keys
type BoxEncryptedData struct {
    EncryptedPassword string            // AES-GCM encrypted password
    BoxKeys          map[string]string // base64(box encrypted AES key) per device fingerprint
}

func HybridEncrypt(password string, devices []models.Device, 
    getBoxPublicKey func(string) (*[32]byte, error)) (*BoxEncryptedData, error) {
    
    // Generate random AES key
    aesKey, err := GenerateAESKey()
    if err != nil {
        return nil, fmt.Errorf("failed to generate AES key: %w", err)
    }
    
    // Encrypt password with AES-GCM
    encryptedPassword, err := AESEncrypt([]byte(password), aesKey)
    if err != nil {
        return nil, fmt.Errorf("failed to encrypt password: %w", err)
    }
    
    // Encrypt AES key for each trusted device using NaCl box
    boxKeys := make(map[string]string)
    for _, device := range devices {
        if !device.Trusted {
            continue
        }
        
        boxPubKey, err := getBoxPublicKey(device.Fingerprint)
        if err != nil {
            return nil, fmt.Errorf("failed to get box public key for device %s: %w", 
                device.ID, err)
        }
        
        // Generate ephemeral keypair for this encryption
        ephemeralPub, ephemeralPriv, err := box.GenerateKey(rand.Reader)
        if err != nil {
            return nil, err
        }
        
        // Encrypt AES key
        encryptedKey := box.Seal(nil, aesKey, &nonce, boxPubKey, ephemeralPriv)
        
        // Store: ephemeralPub + encryptedKey (base64 encoded)
        combined := append(ephemeralPub[:], encryptedKey...)
        boxKeys[device.Fingerprint] = base64.StdEncoding.EncodeToString(combined)
    }
    
    if len(boxKeys) == 0 {
        return nil, fmt.Errorf("no trusted devices to encrypt for")
    }
    
    return &BoxEncryptedData{
        EncryptedPassword: encryptedPassword,
        BoxKeys:          boxKeys,
    }, nil
}

func HybridDecrypt(entry *models.PasswordEntry, boxPrivKey *[32]byte) (string, error) {
    fingerprint := GetBoxFingerprint(boxPrivKey)
    
    boxKeyBase64, ok := entry.BoxKeys[fingerprint]
    if !ok {
        return "", fmt.Errorf("no box key for this device")
    }
    
    combined, err := base64.StdEncoding.DecodeString(boxKeyBase64)
    if err != nil {
        return "", fmt.Errorf("failed to decode box key: %w", err)
    }
    
    // Split ephemeral public key and encrypted AES key
    var ephemeralPub [32]byte
    copy(ephemeralPub[:], combined[:32])
    encryptedAESKey := combined[32:]
    
    // Decrypt AES key
    aesKey, ok := box.Open(nil, encryptedAESKey, &nonce, &ephemeralPub, boxPrivKey)
    if !ok {
        return "", fmt.Errorf("failed to decrypt AES key (box.Open failed)")
    }
    
    // Decrypt password
    password, err := AESDecrypt(entry.EncryptedPassword, aesKey)
    if err != nil {
        return "", fmt.Errorf("failed to decrypt password: %w", err)
    }
    
    return string(password), nil
}
```

### 3.3 Argon2id Key Derivation

**New File:** `internal/crypto/kdf.go`

```go
package crypto

import (
    "golang.org/x/crypto/argon2"
)

// Argon2idParams recommended for interactive use
var DefaultArgon2idParams = Argon2idParams{
    Time:    3,           // iterations
    Memory:  64 * 1024,   // 64 MB
    Threads: 4,
    KeyLen:  32,
}

type Argon2idParams struct {
    Time    uint32
    Memory  uint32
    Threads uint8
    KeyLen  uint32
}

func DeriveKeyArgon2id(password string, salt []byte, params Argon2idParams) []byte {
    return argon2.IDKey(
        []byte(password),
        salt,
        params.Time,
        params.Memory,
        params.Threads,
        params.KeyLen,
    )
}

// MigrateKDF checks if vault uses old scrypt and migrates to Argon2id
func MigrateKDF(storage Storage, password string) error {
    kdfType, _ := storage.GetMeta("kdf")
    
    if kdfType == "argon2id" {
        return nil // Already migrated
    }
    
    // Perform migration from scrypt to Argon2id
    // This happens transparently on unlock
    
    return nil
}
```

### 3.4 Database Migration

**Migration SQL:** `internal/storage/migrations/001_ed25519.sql`

```sql
-- Add columns for new identity system
ALTER TABLE devices ADD COLUMN sign_public_key TEXT;
ALTER TABLE devices ADD COLUMN box_public_key TEXT;
ALTER TABLE entries ADD COLUMN logical_clock INTEGER NOT NULL DEFAULT 0;
ALTER TABLE entries ADD COLUMN origin_device TEXT REFERENCES devices(id);

-- Create device_clocks table for Lamport timestamps
CREATE TABLE device_clocks (
    device_id TEXT PRIMARY KEY REFERENCES devices(id),
    clock INTEGER NOT NULL DEFAULT 0
);

-- Add vault metadata for KDF type
INSERT INTO vault_meta (key, value) VALUES ('kdf', 'scrypt')
    ON CONFLICT(key) DO UPDATE SET value = 'scrypt';
```

### Phase 3 Deliverables

✅ Ed25519 identity keys (signing)  
✅ X25519 keys (NaCl box encryption)  
✅ Argon2id key derivation  
✅ Database schema migration  
✅ 16-character hex fingerprints  
✅ ~24 hours development time  

---

## Phase 4: Sync Protocol Improvements (3 days)

### Priority: MEDIUM - Improves conflict resolution

### 4.1 Lamport Logical Clocks

**New File:** `internal/sync/clock.go`

```go
package sync

import (
    "sync"
    "time"
    
    "github.com/bok1c4/pwman/pkg/models"
)

// LamportClock is a monotonically increasing logical counter per device
type LamportClock struct {
    mu    sync.Mutex
    value int64
}

func NewLamportClock(initial int64) *LamportClock {
    return &LamportClock{value: initial}
}

// Tick increments and returns the new value
func (c *LamportClock) Tick() int64 {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.value++
    return c.value
}

// Witness updates clock based on remote value (takes max + 1)
func (c *LamportClock) Witness(remote int64) int64 {
    c.mu.Lock()
    defer c.mu.Unlock()
    if remote > c.value {
        c.value = remote
    }
    c.value++
    return c.value
}

// Current returns current value without incrementing
func (c *LamportClock) Current() int64 {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.value
}

// MergeEntry resolves conflicts between local and remote entries
// Priority: higher logical_clock wins
// Tie-break 1: later updated_at timestamp
// Tie-break 2: lexicographically higher origin_device (deterministic)
func MergeEntry(local, remote models.PasswordEntry) models.PasswordEntry {
    if remote.LogicalClock > local.LogicalClock {
        return remote
    }
    
    if remote.LogicalClock == local.LogicalClock {
        if remote.UpdatedAt.After(local.UpdatedAt) {
            return remote
        }
        
        if remote.UpdatedAt.Equal(local.UpdatedAt) {
            if remote.UpdatedBy > local.UpdatedBy {
                return remote
            }
        }
    }
    
    return local
}

// MergeEntries merges two sets of entries with conflict resolution
func MergeEntries(local, remote []models.PasswordEntry) []models.PasswordEntry {
    merged := make(map[string]models.PasswordEntry)
    
    // Add all local entries
    for _, e := range local {
        merged[e.ID] = e
    }
    
    // Merge remote entries
    for _, remoteEntry := range remote {
        if localEntry, exists := merged[remoteEntry.ID]; exists {
            merged[remoteEntry.ID] = MergeEntry(localEntry, remoteEntry)
        } else {
            merged[remoteEntry.ID] = remoteEntry
        }
    }
    
    // Convert map back to slice
    result := make([]models.PasswordEntry, 0, len(merged))
    for _, e := range merged {
        result = append(result, e)
    }
    
    return result
}
```

### 4.2 Bidirectional Sync Protocol

**New File:** `internal/sync/protocol.go`

```go
package sync

import (
    "encoding/json"
    "time"
)

// SyncRequest initiates a sync with another device
type SyncRequest struct {
    Type          string `json:"type"`
    DeviceID      string `json:"device_id"`
    VaultID       string `json:"vault_id"`
    SinceClock    int64  `json:"since_clock"` // Only send entries with clock > this
    RequestFull   bool   `json:"request_full"`
    Timestamp     int64  `json:"timestamp"`
}

// SyncPayload contains entries and device information
type SyncPayload struct {
    Type         string                `json:"type"`
    Entries      []SyncEntry           `json:"entries"`
    Devices      []SyncDevice          `json:"devices"`
    ClockUpdates map[string]int64      `json:"clock_updates"` // device_id -> max_clock
    Timestamp    int64                 `json:"timestamp"`
}

type SyncEntry struct {
    ID                string            `json:"id"`
    LogicalClock      int64             `json:"logical_clock"`
    Version           int64             `json:"version"`
    Site              string            `json:"site"`
    Username          string            `json:"username"`
    EncryptedPassword string            `json:"encrypted_password"`
    BoxKeys          map[string]string `json:"box_keys"`
    Notes             string            `json:"notes"`
    CreatedAt         int64             `json:"created_at"`
    UpdatedAt         int64             `json:"updated_at"`
    UpdatedBy         string            `json:"updated_by"`
    OriginDevice      string            `json:"origin_device"`
}

type SyncDevice struct {
    ID           string `json:"id"`
    Name         string `json:"name"`
    SignPubKey   string `json:"sign_pub_key"`
    BoxPubKey    string `json:"box_pub_key"`
    Fingerprint  string `json:"fingerprint"`
    Trusted      bool   `json:"trusted"`
    CreatedAt    int64  `json:"created_at"`
}

// CreateSyncRequest creates a sync request message
func CreateSyncRequest(deviceID, vaultID string, sinceClock int64, full bool) *SyncRequest {
    return &SyncRequest{
        Type:        "SYNC_REQUEST",
        DeviceID:    deviceID,
        VaultID:     vaultID,
        SinceClock:  sinceClock,
        RequestFull: full,
        Timestamp:   time.Now().UnixMilli(),
    }
}

// CreateSyncPayload creates a sync payload with entries
func CreateSyncPayload(entries []SyncEntry, devices []SyncDevice, 
    clockUpdates map[string]int64) *SyncPayload {
    return &SyncPayload{
        Type:         "SYNC_PAYLOAD",
        Entries:      entries,
        Devices:      devices,
        ClockUpdates: clockUpdates,
        Timestamp:    time.Now().UnixMilli(),
    }
}
```

### 4.3 Re-Sync Handler

**New Handler:** `cmd/server/handlers/sync.go`

```go
package handlers

import (
    "encoding/json"
    "net/http"
    "time"
    
    "github.com/bok1c4/pwman/internal/api"
    "github.com/bok1c4/pwman/internal/state"
    "github.com/bok1c4/pwman/internal/sync"
)

type SyncHandlers struct {
    state *state.ServerState
}

func NewSyncHandlers(s *state.ServerState) *SyncHandlers {
    return &SyncHandlers{state: s}
}

type ResyncRequest struct {
    DeviceID string `json:"device_id"` // Optional: specific device to sync with
    FullSync bool   `json:"full_sync"`
}

func (h *SyncHandlers) Resync(w http.ResponseWriter, r *http.Request) {
    storage, ok := h.state.GetVaultStorage()
    if !ok {
        api.Error(w, http.StatusBadRequest, "VAULT_LOCKED", "vault not unlocked")
        return
    }
    
    var req ResyncRequest
    json.NewDecoder(r.Body).Decode(&req)
    
    // Get all trusted devices
    devices, err := storage.ListDevices()
    if err != nil {
        api.Error(w, http.StatusInternalServerError, "DB_ERROR", "failed to list devices")
        return
    }
    
    var syncedDevices []string
    for _, device := range devices {
        if !device.Trusted {
            continue
        }
        
        // Skip self
        vault, _ := h.state.GetVault()
        if device.ID == vault.Config.DeviceID {
            continue
        }
        
        // If specific device requested, only sync with that one
        if req.DeviceID != "" && device.ID != req.DeviceID {
            continue
        }
        
        // Initiate sync with this device
        // ... sync logic here ...
        
        syncedDevices = append(syncedDevices, device.Name)
    }
    
    api.Success(w, map[string]interface{}{
        "synced_devices": syncedDevices,
        "timestamp": time.Now().Unix(),
    })
}
```

### Phase 4 Deliverables

✅ Lamport logical clocks for causal ordering  
✅ Bidirectional sync protocol  
✅ Delta sync (only changed entries)  
✅ Conflict resolution with tiebreakers  
✅ ~24 hours development time  

---

## Implementation Checklist

### Pre-Implementation
- [ ] Create feature branch: `feature/p2p-security-rewrite`
- [ ] Update go.mod dependencies (add zeroconf, argon2, ed25519)
- [ ] Set up database migration framework
- [ ] Create test environment with 2+ devices

### Phase 1: Critical Security (1.5 days)
- [ ] Implement rate limiting in ServerState
- [ ] Create internal/pairing/totp.go with TOTP functions
- [ ] Replace generatePairingCode() with TOTP
- [ ] Update validation logic
- [ ] Update UI to show 6-digit code + countdown
- [ ] Write unit tests
- [ ] Deploy as hotfix

### Phase 2: Transport Security (3.5 days)
- [ ] Create internal/transport/peerstore.go
- [ ] Create internal/transport/tls.go
- [ ] Implement zeroconf discovery package
- [ ] Rewrite internal/p2p/p2p.go for TCP/TLS
- [ ] Add fingerprint verification API endpoint
- [ ] Update frontend with fingerprint confirmation
- [ ] Write integration tests
- [ ] Remove libp2p dependencies

### Phase 3: Identity & Crypto (3 days)
- [ ] Create internal/identity/identity.go
- [ ] Update internal/crypto/hybrid.go for NaCl box
- [ ] Create internal/crypto/kdf.go with Argon2id
- [ ] Write database migrations
- [ ] Update device registration for new key format
- [ ] Add transparent KDF migration
- [ ] Write end-to-end tests

### Phase 4: Sync Protocol (3 days)
- [ ] Create internal/sync/clock.go
- [ ] Create internal/sync/protocol.go
- [ ] Implement bidirectional sync handler
- [ ] Add conflict resolution to entry updates
- [ ] Create re-sync UI with device list
- [ ] Write offline conflict resolution tests

### Post-Implementation
- [ ] Security audit of all changes
- [ ] Performance testing with large vaults
- [ ] Cross-platform testing (macOS, Linux, Windows)
- [ ] Update documentation
- [ ] Create migration guide for users

---

## File Structure After Refactor

```
~/.pwman/
├── config.json                     # unchanged
└── vaults/
    └── <vault_name>/
        ├── config.json             # add: kdf=argon2id
        ├── vault.db                # add: logical_clock, origin_device columns
        ├── identity.key            # RENAME: Ed25519 + X25519 keys (encrypted)
        ├── identity.key.pub        # RENAME: public keys
        ├── identity.key.salt       # unchanged
        └── trusted_peers.json      # NEW: TOFU PeerStore

internal/
├── identity/                       # NEW
│   └── identity.go                 # Ed25519/X25519 key generation
├── discovery/                      # NEW
│   ├── advertise.go                # zeroconf mDNS announce
│   └── browse.go                   # zeroconf mDNS discovery
├── pairing/                        # NEW
│   └── totp.go                     # TOTP code generation/verification
├── transport/                      # NEW
│   ├── peerstore.go                # TOFU certificate pinning
│   └── tls.go                      # TLS configuration
├── sync/                           # NEW
│   ├── clock.go                    # Lamport logical clocks
│   └── protocol.go                 # Sync message types
├── crypto/                         # MODIFIED
│   ├── crypto.go                   # AES-GCM encryption (keep)
│   ├── hybrid.go                   # REPLACE: NaCl box encryption
│   └── kdf.go                      # NEW: Argon2id key derivation
├── device/                         # MINOR CHANGES
│   └── manager.go                  # Update fingerprint format
├── storage/                        # MINOR CHANGES
│   ├── sqlite.go                   # Add logical_clock column
│   └── migrations/                 # NEW: database migrations
├── state/                          # MODIFIED
│   └── server.go                   # Add pairing attempt tracking
└── config/                         # MINOR CHANGES
    └── config.go                   # Add kdf field

cmd/server/handlers/
├── pairing.go                      # MAJOR REWRITE
├── p2p.go                          # MAJOR REWRITE
└── sync.go                         # NEW
```

---

## Summary

| Phase | Description | Effort | Risk |
|-------|-------------|--------|------|
| 1 | TOTP + Rate Limiting | 1.5 days | CRITICAL - Ship immediately |
| 2 | TLS + Cert Pinning | 3.5 days | HIGH - Prevents MITM |
| 3 | Ed25519 + Argon2id | 3 days | MEDIUM - Modernizes crypto |
| 4 | Lamport Clocks | 3 days | MEDIUM - Better conflict resolution |
| **TOTAL** | | **11 days** | |

**Key Decisions:**
1. Keep existing HTTP API and UI structure - only P2P layer changes
2. Database migrations handle transparent upgrades
3. KDF migration happens automatically on unlock
4. Phased rollout: Phase 1 can ship independently
5. Phases 2-4 should be developed together on feature branch

**Success Criteria:**
- Pairing codes change every 60 seconds (TOTP)
- Brute force limited to 5 attempts per peer
- Manual fingerprint verification during pairing
- Mutual TLS with certificate pinning for all sync
- 16-character hex fingerprints (readable)
- Ed25519 keys for identity
- Argon2id for password hashing
- Lamport clocks for conflict resolution
