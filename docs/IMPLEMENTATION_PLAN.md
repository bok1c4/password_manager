# Password Manager - Implementation Plan

## Requirements Restatement

Build a password manager with:
1. **P2P Sync** - Local network device discovery and password sync
2. **Pairing Flow** - Code-based device joining (generate code on device A, join on device B)
3. **Vault Management** - Multiple vaults, see which devices are connected to each vault
4. **Frontend UI** - Full UI for all CLI functionality (not Tauri invoke, use HTTP)
5. **Remote Sync** - Future: sync across different networks

## Current State Analysis

### What's Implemented:
- ✅ Server P2P start/stop/status
- ✅ P2P connection between devices (mDNS auto-discovery)
- ✅ Pairing code generation (`./pwman p2p generate`)
- ✅ Pairing join (`./pwman p2p join <code>`)
- ✅ CLI unlock with vault selection
- ✅ Frontend Settings component (but uses Tauri invoke - won't work with HTTP server)

### What's NOT Implemented:
- ❌ Password sync between devices
- ❌ Frontend pairing UI (generate/join)
- ❌ Vault devices list (which devices are connected to each vault)
- ❌ Proper HTTP API integration in frontend (currently uses Tauri invoke)
- ❌ Remote P2P (different networks)

### What to Remove:
- Git sync references in frontend (Settings.tsx, useVault.ts)
- Any sync-related Tauri commands that don't exist

---

## Implementation Phases

### Phase 1: Backend - Password Sync (P2P)

**Goal:** Implement actual password transfer between devices

**Tasks:**
1. Add `/api/p2p/sync` endpoint to server
2. Implement SyncHandler in `internal/p2p/sync.go`
3. Create sync protocol messages (REQUEST_SYNC, SYNC_DATA, etc.)
4. Re-encrypt passwords for new device on approval
5. Handle conflict resolution (last-write-wins for MVP)

**Files to modify:**
- `cmd/server/main.go` - Add sync endpoint
- `internal/p2p/sync.go` - Implement sync logic
- `internal/p2p/messages.go` - Add sync message types

---

### Phase 2: Frontend - HTTP API Integration

**Goal:** Replace Tauri invoke with HTTP calls to server

**Tasks:**
1. Create HTTP API service layer (use existing `apiBase: http://localhost:18475/api`)
2. Update `useVault.ts` to use fetch instead of invoke
3. Remove Tauri-specific code from frontend

**Files to modify:**
- `src/hooks/useVault.ts` - Replace invoke with fetch
- Create `src/lib/api.ts` - HTTP API client

---

### Phase 3: Frontend - Pairing UI

**Goal:** Add UI for device pairing flow

**Tasks:**
1. Add "Add Device" button in Settings
2. Show pairing code when generating
3. Add "Join Vault" input for entering code
4. Auto-refresh after pairing

**Files to modify:**
- `src/components/Settings.tsx` - Add pairing UI

---

### Phase 4: Vault Devices Management

**Goal:** Show which devices are connected to each vault

**Tasks:**
1. Add endpoint to list devices per vault
2. Show vault selector in Settings
3. Display devices per vault with connection status

**Files to modify:**
- `cmd/server/main.go` - Add vault devices endpoint
- `src/components/Settings.tsx` - Add vault devices UI

---

### Phase 5: Remove Git Sync

**Goal:** Clean up unused code

**Tasks:**
1. Remove git sync references from Settings.tsx
2. Remove initSync, syncPush, syncPull from useVault.ts
3. Remove any unused Tauri commands

**Files to modify:**
- `src/components/Settings.tsx`
- `src/hooks/useVault.ts`

---

### Phase 6: Remote P2P (Future)

**Goal:** Sync across different networks

**Tasks:**
1. Run relay server (simple WebSocket server)
2. Add relay connection in P2P manager
3. Implement NAT traversal via relay

---

## Dependencies

### Phase 1 (Sync):
- None - uses existing P2P infrastructure

### Phase 2 (HTTP Integration):
- None - pure frontend changes

### Phase 3-4 (UI):
- None

### Phase 5 (Cleanup):
- None

---

## Risks

1. **HIGH:** Sync conflict resolution - may lose data if both devices edit same password
   - MVP: Last-write-wins
   - Future: Show conflict UI to user

2. **MEDIUM:** P2P connection reliability - connections may drop
   - Add retry logic
   - Show connection status in UI

3. **LOW:** Frontend/HTTP integration - straightforward refactor

---

## Estimated Complexity: MEDIUM

| Phase | Backend | Frontend | Testing |
|-------|---------|----------|---------|
| Phase 1: Sync | 4-6 hrs | - | 2 hrs |
| Phase 2: HTTP | - | 3-4 hrs | 1 hr |
| Phase 3: Pairing UI | - | 2 hrs | 1 hr |
| Phase 4: Vault Devices | 2 hrs | 2 hrs | 1 hr |
| Phase 5: Cleanup | - | 1 hr | - |
| **Total** | **6-8 hrs** | **8-9 hrs** | **5 hrs** |

---

## Implementation Order

1. **Phase 1** - Implement password sync (backend)
2. **Phase 2** - Fix frontend HTTP integration
3. **Phase 3** - Add pairing UI
4. **Phase 4** - Vault devices management
5. **Phase 5** - Remove git sync code
6. **Phase 6** - Remote P2P (future)

---

## Notes

- CLI and server are the source of truth
- Frontend should mirror CLI functionality
- All P2P operations work on local network (same WiFi)
- Remote sync is future work after MVP is solid

---

**WAITING FOR CONFIRMATION**: Proceed with this plan?
