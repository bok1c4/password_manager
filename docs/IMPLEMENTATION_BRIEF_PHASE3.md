# IMPLEMENTATION BRIEF - Phase 3

**Status:** ✅ COMPLETED
**Completed:** March 14, 2026  
**Source:** docs/IMPLEMENTATION_ROADMAP_PRODUCTION_READY.md (v3.1)  
**Duration:** 3 days  
**Priority:** MEDIUM

---

## Goal

Modernize cryptography with Ed25519/X25519 keys, NaCl box encryption, and Argon2id KDF.

---

## Why This Matters

- **RSA-4096**: Large keys (512 bytes), slower operations
- **Ed25519**: Compact keys (32 bytes), faster signatures, modern standard
- **RSA-OAEP**: Complex padding, side-channel risks
- **NaCl box**: Simple authenticated encryption (X25519 + XSalsa20 + Poly1305)
- **scrypt**: Memory-hard but older
- **Argon2id**: Modern memory-hard KDF, winner of Password Hashing Competition

---

## Files to Create

```
internal/identity/identity.go       [NEW] - Ed25519/X25519 key generation
internal/identity/identity_test.go  [NEW] - Unit tests
internal/crypto/kdf.go              [NEW] - Argon2id implementation
```

## Files to Modify

```
internal/crypto/hybrid.go           [MODIFY] - Replace RSA with NaCl box
internal/storage/sqlite.go          [MODIFY] - Add migration support
cmd/server/handlers/auth.go         [MODIFY] - Use Argon2id for new vaults
```

## Database Migration

```sql
-- Add columns for Ed25519/X25519
ALTER TABLE devices ADD COLUMN sign_public_key TEXT;
ALTER TABLE devices ADD COLUMN box_public_key TEXT;
ALTER TABLE entries ADD COLUMN logical_clock INTEGER DEFAULT 0;
ALTER TABLE entries ADD COLUMN origin_device TEXT REFERENCES devices(id);
ALTER TABLE encrypted_keys ADD COLUMN key_type TEXT DEFAULT 'rsa';

-- Create device_clocks table
CREATE TABLE device_clocks (
    device_id TEXT PRIMARY KEY REFERENCES devices(id),
    clock INTEGER DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Update schema version
UPDATE vault_meta SET value = '3' WHERE key = 'schema_version';
```

---

## Implementation Steps

### Step 1: Create Identity Package (Day 1)

**1.1 Ed25519/X25519 Key Generation**

```go
// internal/identity/identity.go
package identity

import (
    "crypto/ed25519"
    "crypto/rand"
    "crypto/sha256"
    "crypto/sha512"
    "encoding/hex"
    "encoding/pem"
    "fmt"
    "os"
    "path/filepath"
    
    "golang.org/x/crypto/curve25519"
)

type DeviceIdentity struct {
    SignPublicKey  ed25519.PublicKey
    SignPrivateKey ed25519.PrivateKey
    BoxPublicKey  [32]byte
    BoxPrivateKey [32]byte
    Fingerprint   string
}

func GenerateIdentity() (*DeviceIdentity, error) {
    // Generate Ed25519 keypair
    pub, priv, err := ed25519.GenerateKey(rand.Reader)
    if err != nil {
        return nil, fmt.Errorf("failed to generate Ed25519 key: %w", err)
    }
    
    // Derive X25519 key from Ed25519 per RFC 7748
    h := sha512.Sum512(priv.Seed())
    h[0] &= 248
    h[31] &= 127
    h[31] |= 64
    
    var boxPriv, boxPub [32]byte
    copy(boxPriv[:], h[:32])
    curve25519.ScalarBaseMult(&boxPub, &boxPriv)
    
    // Generate fingerprint
    fpHash := sha256.Sum256(pub)
    fp := hex.EncodeToString(fpHash[:8])
    
    return &DeviceIdentity{
        SignPublicKey:  pub,
        SignPrivateKey: priv,
        BoxPublicKey:   boxPub,
        BoxPrivateKey:  boxPriv,
        Fingerprint:    fp,
    }, nil
}
```

**Key Points:**
- RFC 7748 compliant X25519 derivation (not direct copy)
- SHA-512 hash with clamping (bits 0-2 cleared, bit 6 set, bit 7 cleared)
- 16-character hex fingerprint from Ed25519 public key

### Step 2: Create Argon2id Package (Day 1)

```go
// internal/crypto/kdf.go
package crypto

import (
    "golang.org/x/crypto/argon2"
)

var DefaultArgon2idParams = Argon2idParams{
    Time:    3,         // 3 iterations
    Memory:  64 * 1024, // 64 MB
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
```

### Step 3: Modify Hybrid Encryption (Day 2)

Replace RSA-OAEP with NaCl box:

```go
// internal/crypto/hybrid.go

import "golang.org/x/crypto/nacl/box"

func HybridEncrypt(password string, devices []models.Device,
    getBoxPublicKey func(string) (*[32]byte, error)) (*BoxEncryptedData, error) {
    
    // Generate random AES key
    aesKey, err := GenerateAESKey()
    if err != nil {
        return nil, err
    }
    
    // Encrypt password with AES-GCM
    encryptedPassword, err := AESEncrypt([]byte(password), aesKey)
    if err != nil {
        return nil, err
    }
    
    // Encrypt AES key for each device using NaCl box
    boxKeys := make(map[string]string)
    for _, device := range devices {
        if !device.Trusted {
            continue
        }
        
        boxPubKey, err := getBoxPublicKey(device.Fingerprint)
        if err != nil {
            return nil, err
        }
        
        // Generate ephemeral keypair
        ephemeralPub, ephemeralPriv, err := box.GenerateKey(rand.Reader)
        if err != nil {
            return nil, err
        }
        
        // Generate random nonce
        var nonce [24]byte
        if _, err := rand.Read(nonce[:]); err != nil {
            return nil, err
        }
        
        // Encrypt AES key
        encryptedKey := box.Seal(nil, aesKey, &nonce, boxPubKey, ephemeralPriv)
        
        // Combine: ephemeralPub + nonce + encryptedKey
        combined := make([]byte, 0, 32+24+len(encryptedKey))
        combined = append(combined, ephemeralPub[:]...)
        combined = append(combined, nonce[:]...)
        combined = append(combined, encryptedKey...)
        
        boxKeys[device.Fingerprint] = base64.StdEncoding.EncodeToString(combined)
    }
    
    return &BoxEncryptedData{
        EncryptedPassword: encryptedPassword,
        BoxKeys:          boxKeys,
    }, nil
}
```

### Step 4: Database Migration (Day 2-3)

1. Create migration SQL file
2. Run migration on startup
3. Support both RSA and NaCl box during transition
4. Update models to include new fields

---

## Testing Strategy

### Unit Tests
- Ed25519 key generation and signing
- X25519 derivation (RFC 7748 compliance)
- NaCl box encryption/decryption
- Argon2id key derivation

### Integration Tests
- Create vault with Ed25519 keys
- Encrypt/decrypt password entries
- Re-encrypt for multiple devices
- Migration from RSA to Ed25519

### Security Tests
- Verify X25519 clamping
- Test nonce uniqueness
- Verify Argon2id parameters

---

## Success Criteria

- [ ] Ed25519 keys generated correctly
- [ ] X25519 derivation RFC 7748 compliant
- [ ] NaCl box encryption working
- [ ] Argon2id KDF implemented
- [ ] Database migration successful
- [ ] Backward compatibility maintained during transition
- [ ] All tests passing

---

## Migration Strategy

### Phase 3 Deployment
1. Deploy code with dual support (RSA + Ed25519)
2. New vaults use Ed25519
3. Existing vaults continue using RSA
4. Re-encrypt entries when adding new devices

### Phase 4 (Future)
- Eventually deprecate RSA-only devices
- Force migration on unlock

---

## Blockers

None - design complete from Phase 1-2.

---

**Ready to start:** Phase 3 implementation can begin immediately.
