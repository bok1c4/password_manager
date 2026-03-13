# P2P Security Refactor - Implementation Cheat Sheet

## Overview
```
┌─────────────────────────────────────────────────────────────┐
│                    REFACTOR SUMMARY                         │
├─────────────────────────────────────────────────────────────┤
│ FROM: libp2p + Noise + RSA + scrypt + static codes          │
│ TO:   TCP/TLS + Ed25519 + Argon2id + TOTP                   │
│ DURATION: 11 days | RISK: HIGH | PRIORITY: CRITICAL         │
└─────────────────────────────────────────────────────────────┘
```

## Phase Breakdown

### Phase 1: Critical Fixes 🚨 (1.5 days)
```
┌─────────────────────────────────────────────────┐
│ 1. Add rate limiting                            │
│    └── 5 attempts → 30s lockout                 │
│                                                 │
│ 2. Implement TOTP                               │
│    └── 6-digit codes, 60s windows               │
│                                                 │
│ FILES:                                          │
│   + internal/state/server.go (add rate limit)   │
│   + internal/pairing/totp.go (new)              │
│   ~ cmd/server/handlers/pairing.go (modify)     │
└─────────────────────────────────────────────────┘
```

**Key Code:**
```go
// Rate limiting
func (s *ServerState) RecordPairingAttempt(peerID string) bool {
    // Block after 5 attempts
}

// TOTP
code := GeneratePairingCode(masterKey, vaultID)
valid := VerifyPairingCode(masterKey, vaultID, userCode)
```

### Phase 2: Transport Security 🔒 (3.5 days)
```
┌─────────────────────────────────────────────────┐
│ 1. Create PeerStore (TOFU)                      │
│    └── trusted_peers.json                       │
│                                                 │
│ 2. TLS 1.3 + Certificate Pinning                │
│    └── Mutual TLS with manual verify            │
│                                                 │
│ 3. Standard mDNS (zeroconf)                     │
│    └── Replace libp2p mDNS                      │
│                                                 │
│ 4. Rewrite P2P layer                            │
│    └── TCP/TLS instead of libp2p                │
│                                                 │
│ FILES:                                          │
│   + internal/transport/peerstore.go (new)       │
│   + internal/transport/tls.go (new)             │
│   + internal/discovery/*.go (new package)       │
│   ~ internal/p2p/p2p.go (rewrite)               │
│   ~ go.mod (remove libp2p, add zeroconf)        │
└─────────────────────────────────────────────────┘
```

**Key Code:**
```go
// Certificate pinning
peerStore.Trust(fingerprint, deviceName, deviceID)
if !peerStore.IsTrusted(fp) { return error }

// TLS config
config := &tls.Config{
    MinVersion: tls.VersionTLS13,
    VerifyPeerCertificate: func(...) error {
        // Check pinned cert
    },
}
```

### Phase 3: Crypto Upgrade 🔐 (3 days)
```
┌─────────────────────────────────────────────────┐
│ 1. Ed25519 Identity Keys                        │
│    └── Replace RSA-4096                         │
│                                                 │
│ 2. X25519 + NaCl box                            │
│    └── Replace RSA-OAEP                         │
│                                                 │
│ 3. Argon2id KDF                                 │
│    └── Replace scrypt                           │
│                                                 │
│ 4. Database Migration                           │
│    └── Add sign_public_key, box_public_key      │
│    └── Add logical_clock column                 │
│                                                 │
│ FILES:                                          │
│   + internal/identity/identity.go (new)         │
│   ~ internal/crypto/hybrid.go (NaCl box)        │
│   + internal/crypto/kdf.go (Argon2id)           │
│   + internal/storage/migrations/001_*.sql       │
└─────────────────────────────────────────────────┘
```

**Key Code:**
```go
// Ed25519/X25519
identity, _ := GenerateIdentity()  // Ed25519 + X25519
identity.Save(path)
identity.Sign(message)

// NaCl box
encrypted := box.Seal(nil, aesKey, &nonce, peerPubKey, ephemeralPriv)
decrypted, ok := box.Open(nil, encrypted, &nonce, ephemeralPub, privKey)

// Argon2id
key := argon2.IDKey(password, salt, time, memory, threads, keyLen)
```

### Phase 4: Sync Protocol ⏱️ (3 days)
```
┌─────────────────────────────────────────────────┐
│ 1. Lamport Logical Clocks                       │
│    └── Causal ordering                          │
│                                                 │
│ 2. Bidirectional Sync                           │
│    └── Delta sync                               │
│                                                 │
│ 3. Conflict Resolution                          │
│    └── Higher clock wins                        │
│                                                 │
│ FILES:                                          │
│   + internal/sync/clock.go (new)                │
│   + internal/sync/protocol.go (new)             │
│   + cmd/server/handlers/sync.go (new)           │
│   ~ cmd/server/handlers/entry.go (use clocks)   │
└─────────────────────────────────────────────────┘
```

**Key Code:**
```go
// Lamport clock
clock := NewLamportClock(deviceID, 0)
clock.Tick()      // Before local update
clock.Witness(remoteClock)  // On sync

// Conflict resolution
if remote.LogicalClock > local.LogicalClock {
    return remote  // Remote wins
}
```

---

## File Structure Changes

### New Files (Create)
```
internal/pairing/totp.go
internal/transport/peerstore.go
internal/transport/tls.go
internal/discovery/advertise.go
internal/discovery/browse.go
internal/identity/identity.go
internal/crypto/kdf.go
internal/sync/clock.go
internal/sync/protocol.go
internal/storage/migrations/001_ed25519.sql
cmd/server/handlers/sync.go
```

### Modified Files (Update)
```
internal/state/server.go          + Rate limiting
internal/p2p/p2p.go              ~ Complete rewrite
internal/crypto/hybrid.go        ~ NaCl box encryption
cmd/server/handlers/pairing.go   ~ TOTP + rate limiting
cmd/server/handlers/p2p.go       ~ New P2P integration
cmd/server/handlers/entry.go     ~ Logical clocks
go.mod                           + zeroconf, - libp2p
```

---

## Database Schema Changes

### Current Schema
```sql
CREATE TABLE devices (
    id TEXT PRIMARY KEY,
    name TEXT,
    public_key TEXT,      -- RSA public key
    fingerprint TEXT,
    created_at TIMESTAMP,
    trusted BOOLEAN
);

CREATE TABLE entries (
    id TEXT PRIMARY KEY,
    version INTEGER,
    site TEXT,
    encrypted_password TEXT,
    notes TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    updated_by TEXT,
    deleted_at TIMESTAMP
);
```

### New Schema (Migration)
```sql
-- Add to devices
ALTER TABLE devices ADD COLUMN sign_public_key TEXT;  -- Ed25519
ALTER TABLE devices ADD COLUMN box_public_key TEXT;   -- X25519

-- Add to entries
ALTER TABLE entries ADD COLUMN logical_clock INTEGER DEFAULT 0;
ALTER TABLE entries ADD COLUMN origin_device TEXT;

-- New table
CREATE TABLE device_clocks (
    device_id TEXT PRIMARY KEY,
    clock INTEGER DEFAULT 0,
    updated_at TIMESTAMP
);

-- Meta
UPDATE vault_meta SET value = '2' WHERE key = 'schema_version';
```

---

## Dependencies

### Remove
```
github.com/libp2p/go-libp2p v0.47.0
```

### Add
```
github.com/grandcat/zeroconf v1.0.0  (mDNS)
golang.org/x/crypto                  (already present)
  ├── ed25519
  ├── curve25519
  ├── nacl/box
  └── argon2
```

### Commands
```bash
# Remove libp2p
go get -u github.com/libp2p/go-libp2p@none

# Add zeroconf
go get github.com/grandcat/zeroconf

# Update
go mod tidy
go mod verify
```

---

## Testing Quick Reference

### Unit Tests
```bash
# Rate limiting
go test ./internal/state -run TestPairingAttempt -v

# TOTP
go test ./internal/pairing -run TestTOTP -v

# Crypto
go test ./internal/crypto -run TestNaClBox -v

# Clocks
go test ./internal/sync -run TestLamportClock -v
```

### Integration Tests
```bash
# Full sync flow
go test ./tests/integration -run TestFullSyncFlow -v

# Conflict resolution
go test ./tests/integration -run TestConflictResolution -v
```

### Manual Tests
```bash
# Build
make build

# Start server
./bin/pwman-server

# Test Phase 1
curl http://localhost:8080/api/pairing/generate
curl -X POST http://localhost:8080/api/pairing/join \
  -d '{"code":"123456","device_name":"Test"}'
```

---

## Rollback Commands

### Phase 1 Rollback
```bash
git revert <phase-1-commit>
go build ./cmd/server
# No DB changes - safe to rollback
```

### Phase 2-4 Rollback
```bash
# Stop server
pkill pwman-server

# Restore DB from backup
cp -r ~/.pwman/backups/20260313_120000/myvault \
      ~/.pwman/vaults/

# Revert code
git checkout v1.0
go build ./cmd/server
```

---

## API Changes

### New Endpoints
```
POST /api/pairing/verify-fingerprint
  Request:  { device_id, fingerprint, approved }
  Response: { approved, message }

GET /api/sync/devices
  Response: { devices: [{ id, name, fingerprint, online }] }

POST /api/sync/resync
  Request:  { device_id?, full_sync? }
  Response: { synced_devices, failed_devices }
```

### Modified Endpoints
```
POST /api/pairing/generate
  - Code format: XXX-XXX-XXX → 6 digits
  - Response: { code, device_name, expires_in }

POST /api/pairing/join
  - Code validation: TOTP instead of static
```

---

## Security Checklist

### Phase 1 Complete When:
- [ ] Rate limiting blocks after 5 attempts
- [ ] TOTP codes change every 60 seconds
- [ ] Codes expire automatically
- [ ] Unit tests > 80% coverage

### Phase 2 Complete When:
- [ ] libp2p dependency removed
- [ ] TLS 1.3 mutual auth working
- [ ] Certificate pinning active
- [ ] Manual fingerprint verification UI
- [ ] mDNS discovery on port 5353

### Phase 3 Complete When:
- [ ] Ed25519 keys generated
- [ ] X25519 keys derived
- [ ] NaCl box encryption working
- [ ] Argon2id KDF implemented
- [ ] Database migration successful

### Phase 4 Complete When:
- [ ] Lamport clocks incrementing
- [ ] Clock witness on sync
- [ ] Conflict resolution working
- [ ] Delta sync implemented

---

## Performance Targets

| Operation | Target | Current (est.) |
|-----------|--------|----------------|
| Pairing | < 10s | ? |
| Sync (1000 entries) | < 5s | ? |
| Encrypt entry | < 100ms | ? |
| Decrypt entry | < 100ms | ? |
| Key derivation | < 200ms | ? |

**Measure with:**
```bash
go test -bench=. ./...
```

---

## Common Issues & Solutions

### Issue: mDNS not discovering devices
**Solution:** Check firewall rules for port 5353 (UDP)
```bash
# Test mDNS
avahi-browse -a  # Linux
dns-sd -B _pwman._tcp  # macOS
```

### Issue: TLS handshake fails
**Solution:** Check certificate generation and clock sync
```bash
# Check system time
ntpdate -q pool.ntp.org

# Verify certificate
openssl x509 -in cert.pem -text
```

### Issue: TOTP codes don't match
**Solution:** Check clock skew
```bash
# Check both devices have synced time
date +%s  # Should be within 60 seconds
```

### Issue: Database migration fails
**Solution:** Check schema version and backup
```bash
# Check version
sqlite3 vault.db "SELECT value FROM vault_meta WHERE key='schema_version'"

# Manual migration
sqlite3 vault.db < internal/storage/migrations/001_ed25519.sql
```

---

## Timeline Cheat Sheet

```
Week 1: Phase 1 (CRITICAL)
  Day 1: Rate limiting + TOTP
  Day 2: Testing + Deploy

Week 2: Phase 2 (Transport)
  Day 1-2: PeerStore + TLS
  Day 3-4: mDNS + P2P rewrite
  Day 5: Integration testing

Week 3: Phase 3 (Crypto)
  Day 1-2: Ed25519 + NaCl box
  Day 3: Argon2id + Migration
  Day 4-5: Testing

Week 4: Phase 4 (Sync)
  Day 1-2: Lamport clocks
  Day 3: Bidirectional sync
  Day 4-5: Conflict resolution

Week 5: QA & Security
  - Security audit
  - Penetration testing
  - Bug fixes

Week 6: Release
  - Beta rollout
  - Documentation
  - General availability
```

---

## Contact & Resources

**Documentation:**
- Full Roadmap: `docs/IMPLEMENTATION_ROADMAP.md`
- Quick Start: `docs/QUICK_START.md`
- Refactor Plan: `docs/refactor_plan.md`

**Key Files:**
- P2P: `internal/p2p/p2p.go`
- Pairing: `cmd/server/handlers/pairing.go`
- State: `internal/state/server.go`
- Crypto: `internal/crypto/hybrid.go`

**Commands:**
```bash
make build      # Build all
make test       # Run tests
make clean      # Clean build artifacts
```

---

**Last Updated:** March 13, 2026  
**Version:** 1.0  
**Status:** Ready for implementation
