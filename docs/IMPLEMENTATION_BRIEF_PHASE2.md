# IMPLEMENTATION BRIEF - Phase 2

**Status:** 🔄 **IN PROGRESS**  
**Progress:** Transport package complete, P2P layer pending  
**Source:** docs/IMPLEMENTATION_ROADMAP_PRODUCTION_READY.md (v3.1)  
**Duration:** 3.5 days  
**Priority:** HIGH

---

## Goal

Replace libp2p with standard TCP/TLS and implement certificate pinning (TOFU - Trust On First Use).

---

## Overview

Phase 2 overhauls the transport layer:
- Replace libp2p with standard TCP + TLS 1.3
- Implement certificate pinning via TOFU (Trust On First Use)
- Add manual fingerprint verification during pairing
- Replace libp2p mDNS with zeroconf mDNS on port 5353
- Maintain backward compatibility with existing handlers

---

## Files to Create

```
internal/transport/peerstore.go       [NEW] - TOFU certificate pinning
internal/transport/peerstore_test.go  [NEW] - Unit tests
internal/transport/tls.go             [NEW] - TLS certificate generation
internal/discovery/advertise.go       [NEW] - mDNS advertisement
internal/discovery/browse.go          [NEW] - mDNS discovery
```

## Files to Modify

```
internal/p2p/p2p.go                   [MODIFY] - Rewrite for TCP/TLS
cmd/server/handlers/pairing.go        [MODIFY] - Add fingerprint verification
go.mod                                [MODIFY] - Add zeroconf, remove libp2p
```

---

## Implementation Steps

### Step 1: Create Transport Package (Day 1)

**1.1 PeerStore for TOFU**

```go
// internal/transport/peerstore.go
package transport

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "os"
    "sync"
    "time"
)

// PinnedPeer represents a trusted peer's certificate
type PinnedPeer struct {
    Fingerprint string    `json:"fingerprint"`
    DeviceName  string    `json:"device_name"`
    DeviceID    string    `json:"device_id"`
    PinnedAt    time.Time `json:"pinned_at"`
}

// PeerStore implements TOFU (Trust On First Use) certificate pinning
type PeerStore struct {
    mu    sync.RWMutex
    peers map[string]PinnedPeer
    path  string
}

// NewPeerStore loads or creates a peer store
func NewPeerStore(path string) (*PeerStore, error) {
    ps := &PeerStore{
        peers: make(map[string]PinnedPeer),
        path:  path,
    }
    
    // Load existing peers
    data, err := os.ReadFile(path)
    if err == nil {
        json.Unmarshal(data, &ps.peers)
    }
    
    return ps, nil
}

// IsTrusted checks if a certificate fingerprint is trusted
func (ps *PeerStore) IsTrusted(fp string) bool {
    ps.mu.RLock()
    defer ps.mu.RUnlock()
    _, ok := ps.peers[fp]
    return ok
}

// Trust adds a peer to the trusted list
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

// Untrust removes a peer from the trusted list
func (ps *PeerStore) Untrust(fp string) error {
    ps.mu.Lock()
    defer ps.mu.Unlock()
    
    delete(ps.peers, fp)
    return ps.save()
}

// ListTrusted returns all trusted peers
func (ps *PeerStore) ListTrusted() []PinnedPeer {
    ps.mu.RLock()
    defer ps.mu.RUnlock()
    
    result := make([]PinnedPeer, 0, len(ps.peers))
    for _, peer := range ps.peers {
        result = append(result, peer)
    }
    return result
}

func (ps *PeerStore) save() error {
    data, err := json.MarshalIndent(ps.peers, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(ps.path, data, 0600)
}

// CertFingerprint returns SHA-256 fingerprint of a certificate
func CertFingerprint(rawCert []byte) string {
    h := sha256.Sum256(rawCert)
    return hex.EncodeToString(h[:])
}
```

**1.2 TLS Configuration**

```go
// internal/transport/tls.go
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
        NotAfter:    time.Now().Add(365 * 24 * time.Hour), // 1 year
        KeyUsage:    x509.KeyUsageDigitalSignature,
        ExtKeyUsage: []x509.ExtKeyUsage{
            x509.ExtKeyUsageClientAuth,
            x509.ExtKeyUsageServerAuth,
        },
        IPAddresses: []net.IP{
            net.ParseIP("127.0.0.1"),
            net.ParseIP("::1"),
        },
        DNSNames: []string{
            "localhost",
            "pwman.local",
        },
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

// ServerTLSConfig returns TLS config for server
func ServerTLSConfig(cert tls.Certificate, peers *PeerStore, pairingMode bool) *tls.Config {
    return &tls.Config{
        Certificates:       []tls.Certificate{cert},
        ClientAuth:         tls.RequireAnyClientCert,
        MinVersion:         tls.VersionTLS13,
        InsecureSkipVerify: true, // We verify manually
        VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
            if len(rawCerts) == 0 {
                return fmt.Errorf("no client certificate presented")
            }
            
            if pairingMode {
                // During pairing: accept any cert
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

// ClientTLSConfig returns TLS config for client
func ClientTLSConfig(cert tls.Certificate, peers *PeerStore, pairingMode bool) *tls.Config {
    return &tls.Config{
        Certificates:       []tls.Certificate{cert},
        MinVersion:         tls.VersionTLS13,
        InsecureSkipVerify: true, // We verify manually
        VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
            if len(rawCerts) == 0 {
                return fmt.Errorf("no server certificate presented")
            }
            
            if pairingMode {
                // During pairing: accept any cert
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

### Step 2: Create Discovery Package (Day 1-2)

```go
// internal/discovery/advertise.go
package discovery

import (
    "context"
    "fmt"
    
    "github.com/grandcat/zeroconf"
)

const (
    ServiceType = "_pwman._tcp"
    Domain      = "local."
)

// Advertiser handles mDNS service advertisement
type Advertiser struct {
    server *zeroconf.Server
}

// Announce broadcasts this device on the LAN
func Announce(vaultID, deviceName string, port int) (*Advertiser, error) {
    txt := []string{
        fmt.Sprintf("vault=%s", vaultID),
        fmt.Sprintf("device=%s", deviceName),
        "version=2",
    }
    
    server, err := zeroconf.Register(
        deviceName,
        ServiceType,
        Domain,
        port,
        txt,
        nil,
    )
    if err != nil {
        return nil, fmt.Errorf("mDNS announce: %w", err)
    }
    
    return &Advertiser{server: server}, nil
}

// Shutdown stops the mDNS advertisement
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

// PeerInfo represents a discovered peer
type PeerInfo struct {
    VaultID    string
    DeviceName string
    Addr       net.IP
    Port       int
}

// Browse scans for a specific vault
func Browse(ctx context.Context, vaultID string, timeout time.Duration) (*PeerInfo, error) {
    resolver, err := zeroconf.NewResolver(nil)
    if err != nil {
        return nil, err
    }
    
    entries := make(chan *zeroconf.ServiceEntry, 8)
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    
    err = resolver.Browse(ctx, ServiceType, Domain, entries)
    if err != nil {
        return nil, err
    }
    
    for {
        select {
        case entry := <-entries:
            if entry == nil {
                return nil, fmt.Errorf("vault %q not found", vaultID)
            }
            
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

### Step 3: Update go.mod (Day 2)

```bash
go get github.com/grandcat/zeroconf
go mod tidy
```

### Step 4: Modify P2P Layer (Day 2-3)

The P2P layer needs to maintain backward compatibility while supporting TLS mode.

Key changes:
1. Add optional TLS fields to P2PConfig
2. Maintain existing method signatures
3. Support both legacy (libp2p) and new (TLS) modes during transition
4. Update internal peer management for TLS connections

### Step 5: Add Fingerprint Verification UI (Day 3)

Add endpoint for fingerprint verification during pairing:

```go
// cmd/server/handlers/pairing.go

type FingerprintVerifyRequest struct {
    Fingerprint string `json:"fingerprint"`
    Approved    bool   `json:"approved"`
}

func (h *PairingHandlers) VerifyFingerprint(w http.ResponseWriter, r *http.Request) {
    // Implementation for manual fingerprint verification
}
```

---

## Testing Strategy

### Unit Tests
- PeerStore persistence
- TLS certificate generation
- Certificate fingerprinting
- mDNS discovery

### Integration Tests
- Full pairing flow with TLS
- Certificate pinning verification
- mDNS discovery on LAN
- Rejection of untrusted certificates

### Manual Tests
1. Device A advertises via mDNS
2. Device B discovers Device A
3. Device B shows fingerprint
4. User approves fingerprint
5. Pairing completes with TOTP
6. Subsequent sync uses pinned cert

---

## Success Criteria

- [ ] TLS 1.3 mutual authentication working
- [ ] Certificate pinning active (TOFU)
- [ ] Manual fingerprint verification UI
- [ ] mDNS discovery on port 5353
- [ ] Backward compatible with existing handlers
- [ ] All integration tests pass

---

## Blockers

None - design approved in Phase 1.

---

**Next:** Start Step 1 (Transport Package)
