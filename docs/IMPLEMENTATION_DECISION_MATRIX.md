# Implementation Decision Matrix

## Quick Reference: What to Implement When

| Phase | Status | Can Start | Blockers | Priority |
|-------|--------|-----------|----------|----------|
| **Phase 1** | ✅ Ready | **NOW** | None | 🔴 CRITICAL |
| **Phase 2** | ⚠️ Design Review | After Phase 1 | Interface design | 🟡 HIGH |
| **Phase 3** | ⚠️ Design Review | After Phase 2 | Storage migration | 🟡 HIGH |
| **Phase 4** | ⚠️ Design Review | After Phase 3 | Protocol design | 🟢 MEDIUM |

---

## PHASE 1: Critical Security Fixes

### 🚀 START IMMEDIATELY

**Why Phase 1 is Ready:**
- Self-contained changes
- No dependencies on other phases
- Fixes critical vulnerabilities
- Backward compatible
- Can deploy as hotfix

**What to Build:**

```
1. Rate Limiting (internal/state/server.go)
   └── RecordPairingAttempt() method
   └── 5 attempts → 30s lockout
   └── Test: TestPairingAttempt_RateLimiting

2. TOTP Generation (internal/pairing/totp.go)
   └── deriveTOTPKey() - HKDF sub-key
   └── GeneratePairingCode() - 6-digit, 60s
   └── VerifyPairingCode() - window±1
   └── Test: TestTOTPGeneration, TestTOTPClockSkew

3. Handler Integration (cmd/server/handlers/pairing.go)
   └── Line 591: Add rate limiting check
   └── Replace generatePairingCode() with TOTP
   └── Replace code validation with VerifyPairingCode()
```

**Success Criteria:**
- [ ] Rate limiting blocks after 5 attempts (not 4!)
- [ ] TOTP codes change every 60 seconds
- [ ] Codes verify across ±1 window
- [ ] Unit tests > 90% coverage
- [ ] Manual pairing test successful

---

## PHASE 2-4: Design Requirements

### ⚠️ DO NOT START YET

**Why Phases 2-4 Are Blocked:**
- Interface changes affect 8+ handler methods
- Storage migration needs design decision
- Channel semantics need clarification
- Goroutine lifecycle needs review

**Required Design Decisions:**

### 1. P2PManager Interface (CRITICAL)

**Current handlers call:**
```go
p2p.Start()                      // No args
p2p.ConnectToPeer(addr)          // No cert arg
p2p.GetPeerID()                  // Returns string
p2p.GetListenAddresses()         // Returns []string
p2p.GetAllPeers()                // Returns []PeerInfo
p2p.DisconnectFromPeer(id)       // Returns error
p2p.SyncWithPeers(fullSync)      // Returns error
p2p.BroadcastMessage(msg)        // Returns void
```

**Design Options:**

**Option A: Backward Compatible (RECOMMENDED)**
```go
// Keep all existing signatures
// Add new methods alongside
func (p *P2PManager) Start() error                    // Old
func (p *P2PManager) StartOnPort(port int) error     // New

func (p *P2PManager) ConnectToPeer(addr string) error // Old, gets cert internally
func (p *P2PManager) ConnectToPeerWithCert(addr string, cert tls.Certificate) error // New
```

**Option B: Breaking Change (NOT RECOMMENDED)**
```go
// Change all signatures at once
// Requires updating all handlers simultaneously
// High risk, high coordination cost
```

**Decision:** Use Option A (Backward Compatible)

### 2. Storage Migration Strategy (CRITICAL)

**Problem:** `encrypted_keys` table hardcoded for RSA

**Options:**

**Option A: Add key_type Column (RECOMMENDED)**
```sql
ALTER TABLE encrypted_keys ADD COLUMN key_type TEXT DEFAULT 'rsa';
-- 'rsa' for legacy RSA-OAEP keys
-- 'box' for new NaCl box keys
```

**Option B: New Table**
```sql
CREATE TABLE box_keys (
    entry_id TEXT REFERENCES entries(id),
    device_fingerprint TEXT,
    box_key TEXT,
    PRIMARY KEY (entry_id, device_fingerprint)
);
```

**Option C: In-Place Migration (SIMPLEST)**
```sql
-- Overwrite encrypted_keys during migration
-- No schema change needed
-- All devices must upgrade together
```

**Decision:** Use Option A (Add key_type column) - allows gradual migration

### 3. Channel Closing Semantics

**Current:** Stop() closes channels, handlers detect closed channels
**Plan:** Stop() doesn't close channels, uses context cancellation

**Decision:** Keep current behavior - close channels in Stop()

### 4. ConnectToPeer Race Condition

**Problem:** Double peer registration

**Fix:**
```go
func (p *P2PManager) ConnectToPeerWithCert(addr string, cert tls.Certificate) error {
    // Only dial + handshake
    // Don't register peer here
    // Let handleConnection do all registration
    go p.handleConnection(conn)
    return nil
}
```

**Decision:** Delegate all registration to handleConnection

---

## DOCUMENT REFERENCE GUIDE

### For Phase 1 Implementation:
**Primary:** `docs/IMPLEMENTATION_ROADMAP_FINAL.md` Section "PHASE 1: CRITICAL SECURITY FIXES"

**Key Sections:**
- 1.1 Rate Limiting (CORRECTED)
- 1.2 TOTP Implementation (CORRECTED)
- 1.3 Phase 1 Files to Modify
- 1.4 Phase 1 Success Criteria

**Files to Read:**
- Current: `internal/state/server.go` (add rate limiting)
- Current: `cmd/server/handlers/pairing.go` (integration point)
- Create: `internal/pairing/totp.go`

### For Understanding Bugs Fixed:
**Read:** `docs/CRITICAL_BUG_FIXES.md`

**Key Issues:**
- X25519 key derivation (CRITICAL)
- TLS handshake (CRITICAL)
- Rate limiter off-by-one
- TOTP master key exposure
- Missing read loop
- Undefined nonce
- Missing imports

### For Phase 2-4 Design:
**Primary:** `docs/IMPLEMENTATION_ROADMAP_FINAL.md` Section "PHASE 2-4: INTERFACE DESIGN REQUIREMENTS"

**Key Sections:**
- 2.1 P2PManager API Compatibility
- 2.2 ConnectToPeer Race Condition Fix
- 2.3 Storage Layer Migration Strategy
- 2.4 state.Vault Struct Update
- 2.5 Channel Closing Fix
- 2.6 Storage Interface Fix

---

## IMPLEMENTATION TIMELINE

### Week 1: Phase 1 (CRITICAL)
```
Monday:    Create totp.go + tests
Tuesday:   Add rate limiting to server.go
Wednesday: Integrate in pairing handlers
Thursday:  Testing + bug fixes
Friday:    Deploy hotfix
```

### Week 2-3: Design Review
```
Monday:    Review this document
Tuesday:   P2PManager interface design
Wednesday: Storage migration design
Thursday:  Vault struct design
Friday:    Design approval meeting
```

### Week 4+: Phases 2-4
```
Start after design approval
Follow IMPLEMENTATION_ROADMAP_FINAL.md
```

---

## TESTING STRATEGY

### Phase 1 Tests

```bash
# Unit tests
go test ./internal/pairing -v -cover        # TOTP
go test ./internal/state -v -cover          # Rate limiting

# Integration
go test ./cmd/server -run TestPairing -v

# Manual
# 1. Generate pairing code (6 digits, 60s countdown)
# 2. Attempt pairing with wrong code 5 times
# 3. Verify 6th attempt is blocked for 30s
# 4. Wait 30s, verify can try again
# 5. Pair with correct code
```

### Phase 2-4 Tests

```bash
# P2P layer
go test ./internal/p2p -v -cover

# Transport
go test ./internal/transport -v -cover

# Discovery
go test ./internal/discovery -v -cover

# Integration
go test ./tests/integration -v
```

---

## RISK MITIGATION

| Risk | Phase | Mitigation |
|------|-------|------------|
| Rate limiter blocks early | 1 | Test exactly 5 attempts pass |
| TOTP clock skew | 1 | Test ±1 window acceptance |
| Handler breakage | 2-4 | Maintain backward-compatible API |
| Data loss | 3 | Backup before migration |
| Goroutine leak | 2 | Close channels in Stop() |

---

## DECISION LOG

| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-03-14 | Phase 1 ready | Self-contained, critical fixes |
| 2026-03-14 | Phases 2-4 need design | Interface mismatches identified |
| 2026-03-14 | Use backward-compatible API | Minimize handler changes |
| 2026-03-14 | Add key_type column | Support gradual migration |
| 2026-03-14 | Close channels in Stop() | Match current behavior |

---

## NEXT STEPS

### Immediate (Today):
1. ✅ Review this document
2. ✅ Read IMPLEMENTATION_ROADMAP_FINAL.md Phase 1 section
3. ✅ Create feature branch: `git checkout -b feature/phase1-security-fixes`
4. 🚀 **START IMPLEMENTING Phase 1**

### This Week:
1. Implement rate limiting
2. Implement TOTP
3. Write tests
4. Deploy hotfix

### Next Week:
1. Schedule design review for Phases 2-4
2. Review interface designs
3. Approve storage migration strategy
4. Plan Phase 2 implementation

---

## QUESTIONS?

**Before starting Phase 1:**
- Read IMPLEMENTATION_ROADMAP_FINAL.md Section 1
- Review CRITICAL_BUG_FIXES.md for context
- Ensure test environment ready

**Before starting Phase 2:**
- Schedule design review meeting
- Review IMPLEMENTATION_ROADMAP_FINAL.md Section 2
- Approve P2PManager interface design
- Approve storage migration strategy

---

**Document Version:** 1.0  
**Last Updated:** March 14, 2026  
**Status:** ✅ Phase 1 Ready for Implementation
