# Refactoring Plan: Password Manager

## Executive Summary

This document outlines a comprehensive refactoring plan for the password manager codebase. The codebase has evolved through "vibe coding" and needs structural cleanup, documentation, and proper separation of concerns.

---

## Current Status Analysis

### Codebase Metrics

| Metric | Current | Target |
|--------|---------|--------|
| `main.go` lines | 2167 | <400 |
| P2P files | 4 files | 2-3 focused files |
| CLI handlers | 25+ in main.go | Separate by domain |
| Test coverage | ~20% | >80% |

### What's Working ✅

1. **Vault Operations**: init, unlock, lock, status
2. **Password CRUD**: add, list, get, edit, delete
3. **P2P Discovery**: mDNS discovery works
4. **P2P Pairing**: Code-based pairing works (after recent fix)
5. **Basic Sync**: Data sync after pairing
6. **Encryption**: Hybrid AES-256-GCM + RSA working

### Dead Code 🚩

| File | Issue | Action |
|------|-------|--------|
| `internal/p2p/peer_manager.go` | Never instantiated | DELETE |
| `internal/p2p/sync.go` | Never used, duplicate of main.go logic | DELETE or INTEGRATE |
| `internal/device/manager.go` | Need to check usage | INVESTIGATE |

### Architecture Problems ❌

1. **God File**: `main.go` is 2167 lines with all HTTP handlers
2. **Duplicate P2P Logic**: Pairing code scattered across main.go (3+ places)
3. **Inconsistent Message Handling**: Multiple switch statements handling same messages
4. **Missing Error Types**: All errors are strings
5. **No Context Propagation**: Context not passed through call stacks
6. **Global State**: Package-level variables for vault, p2p, etc.

---

## Phase 1: Cleanup Dead Code

### 1.1 Remove Unused Files

```
DELETE:
- internal/p2p/peer_manager.go  (never used)
- internal/device/manager.go     (check first)
```

### 1.2 Remove Unused Imports

In `main.go`, check for unused imports after refactoring.

---

## Phase 2: Split main.go

### Current Structure (One File)
```
main.go (2167 lines)
├── Types (PairingCode, Vault, Entry, Response)
├── Helper functions (getVaultPath, jsonResponse, corsHandler)
├── Vault handlers (handleInit, handleUnlock, handleLock)
├── Entry handlers (handleAddEntry, handleGetEntries, handleDeleteEntry, handleUpdateEntry)
├── Password handlers (handleGetPassword, handleGeneratePassword)
├── Device handlers (handleGetDevices)
├── Vault management (handleVaults, handleVaultUse, handleVaultCreate, handleVaultDelete)
├── P2P status/control (handleP2PStart, handleP2PStop, handleP2PConnect, handleP2PDisconnect)
├── P2P approvals (handleP2PApprovals, handleP2PApprove, handleP2PReject)
├── Pairing (handlePairingGenerate, handlePairingJoin, handlePairingStatus)
├── Sync (handleP2PSync, handleGetSyncStatus)
└── Main (main())
```

### Target Structure
```
cmd/server/
├── main.go                 # Entry point, server setup (~100 lines)
├── handlers/
│   ├── vault.go            # init, unlock, lock, status
│   ├── entries.go          # add, list, get, edit, delete
│   ├── passwords.go        # get decrypted, generate
│   ├── devices.go          # device CRUD
│   ├── vaults.go           # vault management (multi-vault)
│   └── pairing.go          # pairing generate, join, status
├── p2p/
│   ├── server.go          # P2P HTTP handlers
│   ├── pairing.go         # Pairing logic (extract from main)
│   └── sync.go            # Sync logic
├── middleware/
│   └── cors.go            # CORS handling
└── types.go               # Response types
```

### Migration Steps

1. Create `cmd/server/handlers/` directory
2. Move handlers one by one (test after each)
3. Keep types in `types.go` or move to `pkg/models/`

---

## Phase 3: Refactor P2P Module

### Current Problems

1. **Duplicate Message Handling**: `handleMessage` appears in multiple places
2. **Pairing Logic Duplicated**: Same code in 3+ locations in main.go
3. **SyncHandler Not Used**: Has logic but never integrated

### Target P2P Structure

```
internal/p2p/
├── p2p.go                 # P2PManager (core)
├── messages.go            # Message types & creators
├── pairing.go             # PairingRequest, PairingResponse handling
└── discovery.go           # mDNS discovery wrapper
```

### Key Changes

1. **Consolidate Message Handling**: One `handleMessage` in P2PManager
2. **Extract Pairing Logic**: Move to `p2p/pairing.go`
3. **Use or Delete SyncHandler**: Either integrate or remove

---

## Phase 4: Add Proper Error Handling

### Current
```go
// Bad: Generic errors
if err != nil {
    return "failed: " + err.Error()
}
```

### Target
```go
// Good: Typed errors
type VaultError struct {
    Code    string
    Message string
    Err     error
}

func (e *VaultError) Error() string {
    return e.Message
}

// Error codes
const (
    ErrCodeVaultLocked     = "vault_locked"
    ErrCodeInvalidPassword = "invalid_password"
    ErrCodeVaultNotFound   = "vault_not_found"
)
```

---

## Phase 5: Add Tests

### Test Structure
```
internal/
├── crypto/
│   ├── crypto_test.go     # Existing
│   └── generator_test.go  # Existing
├── storage/
│   ├── sqlite_test.go     # Add more
│   └── interface_test.go
└── p2p/
    ├── p2p_test.go        # Add
    └── pairing_test.go    # Add
```

### Priority Tests
1. ✅ Crypto (existing tests)
2. ⬜ Storage CRUD operations
3. ⬜ P2P message serialization
4. ⬜ Pairing flow
5. ⬜ Vault unlock/lock

---

## Phase 6: Documentation

### Code Comments

Every exported function should have:
```go
// GetVault returns the vault for the given name.
// Returns nil if the vault doesn't exist or isn't loaded.
func GetVault(name string) *Vault
```

### README Updates

Update to reflect current state:
- What's working
- What's not working
- Known limitations

---

## Implementation Order

### Week 1: Cleanup
- [ ] Delete dead code files
- [ ] Run tests to verify nothing breaks

### Week 2: Structure
- [ ] Create handler directories
- [ ] Move first batch of handlers (vault operations)
- [ ] Test

### Week 3: P2P Refactor
- [ ] Consolidate message handling
- [ ] Extract pairing logic
- [ ] Remove SyncHandler or integrate it

### Week 4: Polish
- [ ] Add error types
- [ ] Add tests
- [ ] Update docs

---

## Success Criteria

| Metric | Before | After |
|--------|--------|-------|
| main.go lines | 2167 | <400 |
| Test coverage | ~20% | >60% |
| Exported functions documented | 0% | 100% |
| Dead code | ~15% | 0% |
| Duplicate code | Multiple | None |

---

## Risk Mitigation

1. **Small Changes**: Make one change at a time, test after each
2. **Branch Strategy**: Create feature branch for refactor
3. **Rollback Plan**: Keep backup of working state
4. **Incremental**: Don't try to refactor everything at once

---

## Open Questions

1. **Should we keep SyncHandler?** - Currently unused, but has good patterns
2. **Multi-vault support** - Currently partial, should we complete or remove?
3. **C++ import** - Still needed or legacy? (internal/imported/cpp.go)

