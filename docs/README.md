# Documentation Overview

This folder contains comprehensive analysis and planning documents for refactoring the pwman P2P sync system.

## Documents Created

### 1. `analyzation.md` - System Analysis
**Size:** ~400 lines  
**Purpose:** Comprehensive analysis of the current system architecture

**Contains:**
- Architecture overview with system diagrams
- Detailed component breakdown (identity, discovery, pairing, encryption)
- Current implementation specifics (libp2p, RSA-4096, static codes)
- Database schema documentation
- Security analysis with gaps identified
- Key files reference

**Use this when:**
- You need to understand how the current system works
- You want to see the gap analysis between current and desired state
- You're onboarding new developers to the project

---

### 2. `improvements.md` - Comparison & Recommendations
**Size:** ~500 lines  
**Purpose:** Detailed comparison with the desired workflow and improvement recommendations

**Contains:**
- Side-by-side comparison matrix
- Critical issues (TOTP, rate limiting, cert pinning)
- High priority issues (Ed25519, fingerprint verification)
- Medium priority issues (mDNS, vector clocks, Argon2id)
- Specific code examples for fixes
- Migration strategies
- Common mistakes to avoid

**Use this when:**
- You want to understand what needs to change
- You need code examples for implementation
- You're evaluating security requirements

---

### 3. `refactor_plan.md` - Implementation Roadmap
**Size:** ~800 lines  
**Purpose:** Detailed, actionable implementation plan with code examples

**Contains:**
- 4-phase implementation roadmap (~11 days total)
- Phase 1: Critical security fixes (1.5 days) - Can ship immediately
- Phase 2: Transport security overhaul (3.5 days) - TLS + cert pinning
- Phase 3: Identity & crypto upgrade (3 days) - Ed25519 + Argon2id
- Phase 4: Sync protocol improvements (3 days) - Lamport clocks
- Complete code examples for every change
- File structure after refactor
- Implementation checklist
- Testing requirements

**Use this when:**
- You're ready to start coding
- You need the exact implementation details
- You want to track progress against milestones

---

## Key Changes Summary

### Current → Target

| Component | Current | Target | Priority |
|-----------|---------|--------|----------|
| **Pairing Code** | 9-char random static | 6-digit TOTP (60s) | CRITICAL |
| **Rate Limiting** | None | 5 attempts → 30s lockout | CRITICAL |
| **Transport** | libp2p + Noise | TCP + Mutual TLS 1.3 | HIGH |
| **Certificate Trust** | Auto-trust | TOFU + manual verify | HIGH |
| **Identity Keys** | RSA-4096 | Ed25519 | HIGH |
| **Encryption** | RSA-OAEP | NaCl box (X25519) | HIGH |
| **KDF** | scrypt | Argon2id | MEDIUM |
| **Discovery** | libp2p mDNS | zeroconf (port 5353) | MEDIUM |
| **Conflict Resolution** | Wall-clock | Lamport logical clocks | MEDIUM |
| **Fingerprint** | Full Base64 | 16-char hex | LOW |

---

## Quick Start for Developers

### If you want to understand the system:
1. Read `analyzation.md` for architecture overview
2. Check the "Key Files Reference" section at the end

### If you want to see what needs fixing:
1. Read `improvements.md` comparison section
2. Check the "Critical Issues" tables

### If you want to start coding:
1. Read `refactor_plan.md` Phase 1 section
2. Follow the implementation checklist
3. Use the code examples as templates

---

## Phase 1: Critical Security (Ship First!)

**Estimated time:** 1.5 days  
**Can ship independently:** Yes  
**Files to modify:** 3-4

1. **Add rate limiting** (`internal/state/server.go`)
2. **Implement TOTP** (`internal/pairing/totp.go` - NEW)
3. **Update pairing handler** (`cmd/server/handlers/pairing.go`)
4. **Update UI** (countdown timer for 6-digit code)

**Why ship first:** These are critical security vulnerabilities that can be exploited NOW. Everything else can wait.

---

## Dependencies to Add

### Phase 1
```bash
go get golang.org/x/crypto/hmac
go get golang.org/x/crypto/sha256
```

### Phase 2
```bash
go get github.com/grandcat/zeroconf
go mod tidy  # Remove libp2p dependencies
```

### Phase 3
```bash
go get golang.org/x/crypto/ed25519
go get golang.org/x/crypto/curve25519
go get golang.org/x/crypto/nacl/box
go get golang.org/x/crypto/argon2
```

---

## Testing Strategy

### Unit Tests
- TOTP generation/verification with clock skew
- Rate limiting edge cases (exactly 5 attempts)
- Lamport clock tick and witness
- PeerStore save/load

### Integration Tests
- Full pairing flow (Device A → Device B)
- Certificate pinning acceptance/rejection
- Bidirectional sync with conflicts
- Offline edits → online sync → merge

### Manual Tests
- Cross-platform pairing (macOS ↔ Linux)
- Large vault sync (1000+ entries)
- Network interruption during sync
- Clock skew > 60 seconds

---

## Migration Path

### For Existing Users
1. **Phase 1:** No migration needed - transparent upgrade
2. **Phase 2-4:** Database migrations handle automatically
   - On next unlock, KDF migrates scrypt → Argon2id
   - New columns added automatically
   - Old RSA keys remain for backward compatibility during transition

### For Developers
1. Create feature branch: `git checkout -b feature/p2p-rewrite`
2. Implement Phase 1, merge to main as hotfix
3. Implement Phases 2-4 together on branch
4. Test thoroughly before merging

---

## Questions & Support

### Common Questions

**Q: Do I need to rewrite the UI?**  
A: Minimal changes - only the pairing code display (9-char → 6-digit) and one fingerprint confirmation screen.

**Q: Will existing vaults break?**  
A: No - database migrations handle everything automatically.

**Q: Can I skip Phase 2 and just do Phase 1?**  
A: Yes! Phase 1 can ship independently. But don't skip Phase 2 for production - MITM attacks are real.

**Q: How long will users be locked out after 5 failed attempts?**  
A: 30 seconds (configurable in rate limiter).

**Q: What if clocks are more than 60 seconds off?**  
A: TOTP verification fails - user must sync system clocks. This is the correct security behavior.

---

## Success Metrics

After all phases complete:

✅ Pairing codes expire after 60 seconds (not 5 minutes)  
✅ Maximum 5 pairing attempts per peer per 30 seconds  
✅ All connections use mutual TLS with pinned certificates  
✅ Users manually verify fingerprints during first pairing  
✅ Ed25519 keys used for identity (not RSA-4096)  
✅ Argon2id used for password hashing  
✅ No libp2p dependencies in go.mod  
✅ All tests pass  
✅ Security audit passed  

---

## Contact & Updates

- **Version:** 1.0 (March 2026)
- **Status:** Planning complete, ready for implementation
- **Next step:** Start Phase 1 implementation

---

## Quick Reference: File Locations

### Current Key Files
```
cmd/server/handlers/pairing.go:47    # generatePairingCode() - REPLACE
cmd/server/handlers/pairing.go:591   # HandlePairingRequest - ADD rate limit
cmd/server/handlers/pairing.go:602   # Code validation - REPLACE with TOTP

internal/p2p/p2p.go:96               # P2P initialization - REWRITE
internal/p2p/p2p.go:210              # mDNS discovery - REPLACE
internal/crypto/hybrid.go:16         # Hybrid encrypt - REPLACE with NaCl box
internal/crypto/crypto.go:24         # RSA key generation - REPLACE with Ed25519
internal/state/server.go:55          # ServerState - ADD pairingAttempts
```

### New Files to Create
```
internal/pairing/totp.go             # TOTP generation/verification
internal/transport/peerstore.go      # Certificate pinning
internal/transport/tls.go            # TLS configuration
internal/identity/identity.go        # Ed25519/X25519 keys
internal/discovery/advertise.go      # zeroconf mDNS announce
internal/discovery/browse.go         # zeroconf mDNS discovery
internal/sync/clock.go               # Lamport logical clocks
internal/sync/protocol.go            # Sync message types
internal/crypto/kdf.go               # Argon2id key derivation
```

---

**End of Documentation Overview**
