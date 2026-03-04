# Password Manager - Refactoring Plan

## Completed Refactoring

### Phase 1: Dead Code ✅
- Deleted `internal/p2p/peer_manager.go` (never used)
- Deleted `internal/p2p/sync.go` (never used, duplicate logic)

### Phase 2: P2P Message Routing ✅
- Added specialized channels to P2PManager:
  - `PairingRequestChan()` - for pairing request messages
  - `PairingResponseChan()` - for pairing response messages  
  - `SyncRequestChan()` - for sync request messages
  - `SyncDataChan()` - for sync data messages
- Added message router that dispatches messages to appropriate channels

### Phase 3: main.go Structure
**Status**: Not fully split (too complex for quick refactor)

The current main.go has ~2450 lines with these sections:
- Lines 1-100: Imports and global state
- Lines 100-250: Helper functions
- Lines 250-500: Vault handlers (init, unlock, lock)
- Lines 500-1000: Entry handlers (add, list, get, edit, delete)
- Lines 1000-1500: P2P handlers (start, stop, connect)
- Lines 1500-2000: Pairing handlers (generate, join)
- Lines 2000-2450: Sync handlers

**Recommendation**: Split incrementally when adding new features, not all at once.

---

## Current Architecture

```
P2P Message Flow (After Refactor):
                                        
    Incoming Message
          │
          ▼
   ┌──────────────┐
   │ routeMessages│  (in p2p.go)
   └──────────────┘
          │
          ▼
   ┌─────────────────────────────────────┐
   │  Specialized Channels               │
   │  - PairingRequestChan              │
   │  - PairingResponseChan             │
   │  - SyncRequestChan                 │
   │  - SyncDataChan                    │
   └─────────────────────────────────────┘
```

---

## What's Working Now

1. ✅ Vault operations (init, unlock, lock)
2. ✅ Password CRUD
3. ✅ P2P discovery (mDNS)
4. ✅ P2P pairing (code exchange)
5. ⚠️ P2P Sync (needs testing)

---

## Remaining Issues

1. **Message routing** - Still uses generic MessageChan() in some places
2. **Duplicate handlers** - Multiple handlers for same message types
3. **Testing** - Need to verify sync works

---

## Next Steps

1. Test vault sync end-to-end
2. If it works, mark as complete
3. If not, debug message routing

