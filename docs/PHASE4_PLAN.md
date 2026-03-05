# Phase 4 Implementation Plan

**Status**: LOW Priority (Backlog)  
**Last Updated**: 2026-03-05

## Overview

Phase 4 contains architectural improvements and modernizations. These are **optional enhancements** that don't address immediate security vulnerabilities but improve code quality and maintainability.

## ⚠️ Important Notes

- **Phase 4 changes are LOW priority**
- **Some changes are BREAKING** (require all devices to update)
- **Large refactorings** should be done in separate feature branches
- **Test thoroughly** before deploying

## Phase 4.1: Refactor Monolithic Server

**Current State**: `cmd/server/main.go` is 2,777 lines

**Recommended Approach**:
1. Create feature branch: `git checkout -b refactor/modular-server`
2. Extract handlers incrementally (one module at a time)
3. Maintain backward compatibility
4. Add integration tests before refactoring

**Proposed Structure**:
```
cmd/server/
├── main.go              # Entry point (~100 lines)
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

**Migration Strategy**:
- [ ] Create handler packages
- [ ] Move one handler group at a time
- [ ] Run tests after each move
- [ ] Update imports
- [ ] Verify all endpoints work

**Risk**: MEDIUM - Could introduce bugs if not done carefully

---

## Phase 4.2: Migrate to OAEP Padding

**Current**: Using PKCS1v15 padding
**Target**: OAEP (Optimal Asymmetric Encryption Padding)

**Current Code**:
```go
ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, publicKey, plaintext)
```

**Target Code**:
```go
label := []byte("")
ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, plaintext, label)
```

### Why OAEP?

- **More secure**: Resists chosen-ciphertext attacks
- **Standard**: Recommended by RSA Laboratories
- **Forward-looking**: PKCS1v15 considered legacy

### Migration Path

**⚠️ BREAKING CHANGE** - All devices must update simultaneously

#### Version Strategy

1. **Version 1.x** (Current):
   - Use PKCS1v15
   - Mark as deprecated in code comments

2. **Version 2.0** (Future):
   - Dual support: Try OAEP first, fallback to PKCS1v15
   - Add version field to encrypted data
   - Gradual migration

3. **Version 3.0** (Future):
   - OAEP only
   - Require all devices to upgrade

#### Implementation Steps

```go
// internal/crypto/encryption_version.go
const (
    EncryptionPKCS1v15 = 1
    EncryptionOAEP     = 2
)

type EncryptedData struct {
    Version     int    `json:"version"`
    Ciphertext  []byte `json:"ciphertext"`
}

// Encrypt with version
func EncryptWithVersion(pubKey *rsa.PublicKey, plaintext []byte) (*EncryptedData, error) {
    // Use OAEP for new encryption
    label := []byte("")
    ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pubKey, plaintext, label)
    if err != nil {
        return nil, err
    }
    
    return &EncryptedData{
        Version:    EncryptionOAEP,
        Ciphertext: ciphertext,
    }, nil
}

// Decrypt with version detection
func DecryptWithVersion(privKey *rsa.PrivateKey, data *EncryptedData) ([]byte, error) {
    switch data.Version {
    case EncryptionOAEP:
        label := []byte("")
        return rsa.DecryptOAEP(sha256.New(), rand.Reader, privKey, data.Ciphertext, label)
    case EncryptionPKCS1v15:
        // Legacy support
        return rsa.DecryptPKCS1v15(rand.Reader, privKey, data.Ciphertext)
    default:
        return nil, fmt.Errorf("unknown encryption version: %d", data.Version)
    }
}
```

**Timeline**: Schedule for v2.0 release

**Risk**: HIGH - Breaking change, requires coordination

---

## Phase 4.3: Add Telemetry/Monitoring

**Status**: Safe to implement  
**Priority**: MEDIUM (within Phase 4)

### Health Check Endpoint

```go
// GET /api/health
func handleHealth(w http.ResponseWriter, r *http.Request) {
    health := map[string]interface{}{
        "status":    "ok",
        "timestamp": time.Now().Unix(),
        "version":   "1.0.0",
        "checks": map[string]bool{
            "vault_unlocked": vault != nil && vault.privateKey != nil,
            "p2p_running":    p2pManager != nil && p2pManager.IsRunning(),
        },
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(health)
}
```

### Metrics Collection (Optional)

```go
// GET /api/metrics (authenticated)
func handleMetrics(w http.ResponseWriter, r *http.Request) {
    metrics := map[string]interface{}{
        "entries_count":    0,
        "devices_count":    0,
        "p2p_connections":  0,
        "uptime_seconds":   time.Since(startTime).Seconds(),
    }
    
    if vault != nil && vault.storage != nil {
        entries, _ := vault.storage.ListEntries()
        devices, _ := vault.storage.ListDevices()
        metrics["entries_count"] = len(entries)
        metrics["devices_count"] = len(devices)
    }
    
    if p2pManager != nil {
        metrics["p2p_connections"] = len(p2pManager.GetConnectedPeers())
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(metrics)
}
```

**Implementation**: Safe, non-breaking

**Risk**: LOW - Optional feature

---

## Phase 4.4: Modern Crypto Migration (X25519/Ed25519)

**Current**: RSA-4096  
**Future**: X25519 (key exchange) + Ed25519 (signatures)

### Why Modern Crypto?

- **Performance**: 100x faster than RSA
- **Security**: Modern, well-audited
- **Key Size**: Smaller keys (32 bytes vs 512 bytes)
- **Trend**: Industry moving to elliptic curve crypto

### Migration Strategy

This is a **major version change** (v3.0 or later)

#### Phase 1: Add Support (v2.0)
- Add X25519/Ed25519 key generation
- Support both RSA and Ed25519
- Store key type in device metadata

#### Phase 2: Prefer Ed25519 (v2.5)
- New devices use Ed25519 by default
- Existing RSA devices continue working
- Gradual migration

#### Phase 3: RSA Deprecation (v3.0)
- Ed25519 only
- Migration tool for existing vaults
- All devices must update

### Implementation Sketch

```go
// internal/crypto/modern.go
import (
    "crypto/ed25519"
    "golang.org/x/crypto/curve25519"
)

type KeyType int

const (
    KeyTypeRSA KeyType = iota
    KeyTypeEd25519
)

type ModernKeyPair struct {
    Type       KeyType
    Ed25519    *ed25519.PrivateKey  // For signatures
    X25519     *[32]byte             // For key exchange (derived from Ed25519)
    RSA        *rsa.PrivateKey       // Legacy support
}

func GenerateModernKeyPair(keyType KeyType) (*ModernKeyPair, error) {
    switch keyType {
    case KeyTypeEd25519:
        pub, priv, err := ed25519.GenerateKey(rand.Reader)
        if err != nil {
            return nil, err
        }
        
        // Derive X25519 key for encryption
        var x25519Key [32]byte
        curve25519.ScalarBaseMult(&x25519Key, (*[32]byte)(&priv))
        
        return &ModernKeyPair{
            Type:    KeyTypeEd25519,
            Ed25519: &priv,
            X25519:  &x25519Key,
        }, nil
        
    case KeyTypeRSA:
        priv, err := rsa.GenerateKey(rand.Reader, 4096)
        if err != nil {
            return nil, err
        }
        return &ModernKeyPair{
            Type: KeyTypeRSA,
            RSA:  priv,
        }, nil
        
    default:
        return nil, fmt.Errorf("unsupported key type: %d", keyType)
    }
}
```

**Timeline**: Research phase, implement in v2.0+

**Risk**: HIGH - Major breaking change

---

## Summary & Recommendations

### Implement Now (Safe)
- ✅ **Phase 4.3**: Health check endpoint
- ✅ **Phase 4.3**: Metrics endpoint (authenticated)

### Schedule for v2.0
- 📅 **Phase 4.2**: OAEP padding migration
- 📅 **Phase 4.4**: Add Ed25519 support (dual mode)

### Schedule for v3.0+
- 📅 **Phase 4.1**: Server refactoring
- 📅 **Phase 4.4**: Ed25519-only mode

### Never (or much later)
- ❌ Breaking changes without migration path
- ❌ Forced simultaneous updates across all devices

---

## Testing Checklist

Before any Phase 4 changes:

- [ ] Create feature branch
- [ ] Write integration tests
- [ ] Test backward compatibility
- [ ] Document breaking changes
- [ ] Create migration guide
- [ ] Update version number
- [ ] Announce to users

---

## References

- [OAEP RFC 2447](https://tools.ietf.org/html/rfc2447)
- [Ed25519 Paper](https://ed25519.cr.yp.to/)
- [X25519 RFC 7748](https://tools.ietf.org/html/rfc7748)
- [Go Crypto Best Practices](https://github.com/gtank/cryptopasta)
