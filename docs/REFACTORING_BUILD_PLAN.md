# Refactoring Build Plan - Complete the God File Extraction

**Status**: 60% Complete (Phase 3 in progress)  
**Target**: Reduce main.go from 2,786 lines to <200 lines  
**Priority**: HIGH - Critical for maintainability  

---

## What's Been Done ✅

### Phase 1: Infrastructure (100% Complete)
- ✅ `internal/api/response.go` - Response helpers (Success, Error, BadRequest, etc.)
- ✅ `internal/api/validation.go` - Input validation utilities
- ✅ `internal/api/ratelimit.go` - Rate limiting middleware
- ✅ `internal/middleware/cors.go` - CORS middleware
- ✅ `internal/middleware/auth.go` - Authentication middleware
- ✅ Tests for all infrastructure components

### Phase 2: State Management (100% Complete)
- ✅ `internal/state/server.go` - Centralized state management
- ✅ Thread-safe access with fine-grained locks
- ✅ All state types (Vault, PairingCode, PairingRequest, PendingApproval)
- ✅ Complete test coverage

### Phase 3: Handler Extraction (60% Complete)

**Completed Handlers:**
1. ✅ `cmd/server/handlers/auth.go` (6,944 bytes)
   - Init, Unlock, Lock, IsUnlocked, IsInitialized
   - Uses: state.ServerState, api.AuthManager
   - Has tests: auth_test.go

2. ✅ `cmd/server/handlers/entry.go` (11,029 bytes)
   - List, Add, Update, Delete, GetPassword
   - Uses: state.ServerState
   - Includes password encryption/decryption logic

3. ✅ `cmd/server/handlers/vault.go` (4,436 bytes)
   - List, Use, Create, Delete vaults
   - Uses: state.ServerState

4. ✅ `cmd/server/handlers/device.go` (1,239 bytes)
   - List devices
   - Uses: state.ServerState

5. ✅ `cmd/server/handlers/health.go` (1,898 bytes)
   - Health, Metrics, GeneratePassword
   - Has tests: health_test.go

---

## What's Remaining 🚧

### Task 1: Extract P2P Handlers to `cmd/server/handlers/p2p.go`

**Handlers to extract from main.go (lines 1177-2427):**

```go
// P2P Network Handlers
func handleP2PStatus(w http.ResponseWriter, r *http.Request)       // Line 1177
func handleP2PStart(w http.ResponseWriter, r *http.Request)        // Line 1228
func handleP2PStop(w http.ResponseWriter, r *http.Request)         // Line 1326
func handleP2PPeers(w http.ResponseWriter, r *http.Request)        // Line 1346
func handleP2PConnect(w http.ResponseWriter, r *http.Request)      // Line 1384
func handleP2PDisconnect(w http.ResponseWriter, r *http.Request)   // Line 1432
func handleP2PApprovals(w http.ResponseWriter, r *http.Request)    // Line 1471
func handleP2PApprove(w http.ResponseWriter, r *http.Request)      // Line 1492
func handleP2PReject(w http.ResponseWriter, r *http.Request)       // Line 1549
func handleP2PSync(w http.ResponseWriter, r *http.Request)         // Line 2367
```

**File Structure:**
```go
package handlers

import (
    "encoding/json"
    "net/http"
    
    "github.com/bok1c4/pwman/internal/api"
    "github.com/bok1c4/pwman/internal/state"
    "github.com/bok1c4/pwman/internal/p2p"
)

type P2PHandlers struct {
    state *state.ServerState
}

func NewP2PHandlers(s *state.ServerState) *P2PHandlers {
    return &P2PHandlers{state: s}
}

func (h *P2PHandlers) Status(w http.ResponseWriter, r *http.Request) {
    // Extract from handleP2PStatus (line 1177)
}

func (h *P2PHandlers) Start(w http.ResponseWriter, r *http.Request) {
    // Extract from handleP2PStart (line 1228)
    // Replace global state with h.state calls
}

// ... continue for all 10 P2P handlers
```

**Key Changes During Extraction:**
- Replace global `p2pManager`, `p2pCancel` with `h.state.GetP2PManager()`, `h.state.StopP2P()`
- Replace global `pendingApprovals` with `h.state.ListPendingApprovals()`, etc.
- Replace `vault` global with `h.state.GetVault()`
- Replace `authManager` global - need to pass in constructor

**Dependencies:**
- Need to add `authManager *api.AuthManager` to P2PHandlers struct
- Update NewP2PHandlers to accept authManager

---

### Task 2: Extract Pairing Handlers to `cmd/server/handlers/pairing.go`

**Handlers to extract from main.go (lines 1587-2701):**

```go
// Pairing Handlers (HTTP endpoints)
func handlePairingGenerate(w http.ResponseWriter, r *http.Request)  // Line 1587
func handlePairingJoin(w http.ResponseWriter, r *http.Request)      // Line 1706
func handlePairingStatus(w http.ResponseWriter, r *http.Request)    // Line 2145

// P2P Message Handlers (called by P2P layer)
func handlePairingRequest(pm *p2p.P2PManager, msg p2p.ReceivedMessage)     // Line 2428
func handlePairingResponse(msg p2p.ReceivedMessage)                       // Line 2546
func handleSyncRequest(pm *p2p.P2PManager, peerID string)                 // Line 2558
func handleJoinerPairingRequest(pm *p2p.P2PManager, msg p2p.ReceivedMessage) // Line 2656
func handleJoinerPairingResponse(msg p2p.ReceivedMessage)                 // Line 2687
func handleJoinerSyncData(msg p2p.ReceivedMessage)                        // Line 2701
```

**File Structure:**
```go
package handlers

import (
    "encoding/json"
    "net/http"
    
    "github.com/bok1c4/pwman/internal/api"
    "github.com/bok1c4/pwman/internal/state"
    "github.com/bok1c4/pwman/internal/p2p"
)

type PairingHandlers struct {
    state       *state.ServerState
    authManager *api.AuthManager
}

func NewPairingHandlers(s *state.ServerState, am *api.AuthManager) *PairingHandlers {
    return &PairingHandlers{
        state:       s,
        authManager: am,
    }
}

// HTTP Handlers
func (h *PairingHandlers) Generate(w http.ResponseWriter, r *http.Request) {
    // Extract from handlePairingGenerate (line 1587)
}

func (h *PairingHandlers) Join(w http.ResponseWriter, r *http.Request) {
    // Extract from handlePairingJoin (line 1706)
}

func (h *PairingHandlers) Status(w http.ResponseWriter, r *http.Request) {
    // Extract from handlePairingStatus (line 2145)
}

// P2P Message Handlers
func (h *PairingHandlers) HandlePairingRequest(pm *p2p.P2PManager, msg p2p.ReceivedMessage) {
    // Extract from handlePairingRequest (line 2428)
}

// ... continue for all pairing message handlers
```

**Key Challenges:**
- These handlers use global channels (`pairingResponseCh`)
- Replace with `h.state.GetPairingResponseChannel()`
- Need to update P2P message handler registration

---

### Task 3: Update main.go Route Registration (Phase 4)

**Current main.go route registration (lines ~100-250):**
```go
// Old way - handlers still in main.go
http.HandleFunc("/api/p2p/status", corsHandler(authMiddleware(handleP2PStatus)))
http.HandleFunc("/api/pairing/generate", corsHandler(authMiddleware(handlePairingGenerate)))
```

**New main.go route registration:**
```go
package main

import (
    "log"
    "net/http"
    "os"
    
    "github.com/bok1c4/pwman/internal/api"
    "github.com/bok1c4/pwman/internal/middleware"
    "github.com/bok1c4/pwman/internal/state"
    "github.com/bok1c4/pwman/cmd/server/handlers"
)

var (
    serverPort  = os.Getenv("PWMAN_PORT")
    authManager = api.NewAuthManager()
)

func main() {
    // Initialize server state
    serverState := state.NewServerState()
    
    // Initialize handlers
    authHandlers := handlers.NewAuthHandlers(serverState, authManager)
    entryHandlers := handlers.NewEntryHandlers(serverState)
    vaultHandlers := handlers.NewVaultHandlers(serverState)
    deviceHandlers := handlers.NewDeviceHandlers(serverState)
    healthHandlers := handlers.NewHealthHandlers(serverState)
    p2pHandlers := handlers.NewP2PHandlers(serverState, authManager)  // NEW
    pairingHandlers := handlers.NewPairingHandlers(serverState, authManager)  // NEW
    
    // Setup middleware chain
    auth := func(next http.HandlerFunc) http.HandlerFunc {
        return middleware.CORS(middleware.Auth(authManager, next))
    }
    
    // Auth routes
    http.HandleFunc("/api/init", auth(authHandlers.Init))
    http.HandleFunc("/api/unlock", auth(authHandlers.Unlock))
    http.HandleFunc("/api/lock", auth(authHandlers.Lock))
    http.HandleFunc("/api/is_unlocked", auth(authHandlers.IsUnlocked))
    http.HandleFunc("/api/is_initialized", auth(authHandlers.IsInitialized))
    
    // Entry routes
    http.HandleFunc("/api/entries", auth(entryHandlers.List))
    http.HandleFunc("/api/entries/add", auth(entryHandlers.Add))
    http.HandleFunc("/api/entries/update", auth(entryHandlers.Update))
    http.HandleFunc("/api/entries/delete", auth(entryHandlers.Delete))
    http.HandleFunc("/api/entries/get_password", auth(entryHandlers.GetPassword))
    
    // Vault routes
    http.HandleFunc("/api/vaults", auth(vaultHandlers.List))
    http.HandleFunc("/api/vaults/use", auth(vaultHandlers.Use))
    http.HandleFunc("/api/vaults/create", auth(vaultHandlers.Create))
    http.HandleFunc("/api/vaults/delete", auth(vaultHandlers.Delete))
    
    // Device routes
    http.HandleFunc("/api/devices", auth(deviceHandlers.List))
    
    // P2P routes (NEW)
    http.HandleFunc("/api/p2p/status", auth(p2pHandlers.Status))
    http.HandleFunc("/api/p2p/start", auth(p2pHandlers.Start))
    http.HandleFunc("/api/p2p/stop", auth(p2pHandlers.Stop))
    http.HandleFunc("/api/p2p/peers", auth(p2pHandlers.Peers))
    http.HandleFunc("/api/p2p/connect", auth(p2pHandlers.Connect))
    http.HandleFunc("/api/p2p/disconnect", auth(p2pHandlers.Disconnect))
    http.HandleFunc("/api/p2p/approvals", auth(p2pHandlers.Approvals))
    http.HandleFunc("/api/p2p/approve", auth(p2pHandlers.Approve))
    http.HandleFunc("/api/p2p/reject", auth(p2pHandlers.Reject))
    http.HandleFunc("/api/p2p/sync", auth(p2pHandlers.Sync))
    
    // Pairing routes (NEW)
    http.HandleFunc("/api/pairing/generate", auth(pairingHandlers.Generate))
    http.HandleFunc("/api/pairing/join", auth(pairingHandlers.Join))
    http.HandleFunc("/api/pairing/status", auth(pairingHandlers.Status))
    
    // Health routes (no auth required)
    http.HandleFunc("/api/health", middleware.CORS(healthHandlers.Health))
    http.HandleFunc("/api/metrics", middleware.CORS(healthHandlers.Metrics))
    http.HandleFunc("/api/generate", middleware.CORS(healthHandlers.GeneratePassword))
    
    // Find available port
    port, err := findAvailablePort()
    if err != nil {
        log.Fatalf("Failed to find port: %v", err)
    }
    
    log.Printf("Starting pwman API server on :%s", port)
    log.Fatal(http.ListenAndServe(":"+port, nil))
}

func findAvailablePort() (string, error) {
    // Keep existing logic (lines ~2700-2786)
    // This is one of the few helper functions that should remain
}
```

---

### Task 4: Remove Old Handler Functions from main.go

**After updating route registration, delete from main.go:**

1. **Lines 253-2144**: Auth handlers (handleInit, handleUnlock, etc.)
   - Already extracted to handlers/auth.go
   
2. **Lines 508-883**: Entry handlers (handleGetEntries, handleAddEntry, etc.)
   - Already extracted to handlers/entry.go

3. **Lines 885-922**: Device handlers (handleGetDevices)
   - Already extracted to handlers/device.go

4. **Lines 924-1001**: Health handlers (handleHealth, handleMetrics, etc.)
   - Already extracted to handlers/health.go

5. **Lines 1003-1175**: Vault handlers (handleVaults, handleVaultUse, etc.)
   - Already extracted to handlers/vault.go

6. **Lines 1177-2366**: P2P handlers (handleP2PStatus, etc.)
   - Will extract to handlers/p2p.go in Task 1

7. **Lines 2367-2700**: Pairing handlers (handlePairingGenerate, etc.)
   - Will extract to handlers/pairing.go in Task 2

**What should remain in main.go:**
- Package declaration and imports
- Global variables (serverPort, authManager)
- main() function
- findAvailablePort() helper
- Total: <200 lines

---

### Task 5: Update P2P Message Handler Registration

**Current approach in main.go:**
```go
// Message handlers are registered inline when starting P2P
pm.SetMessageHandler(p2p.MessageTypePairingRequest, func(msg p2p.ReceivedMessage) {
    handlePairingRequest(pm, msg)
})
```

**New approach:**
Need to create a way to register handlers from outside main.go. Options:

**Option A: Pass handlers to P2PManager.Start()**
```go
// In main.go
p2pHandlers := handlers.NewP2PHandlers(serverState, authManager)
pairingHandlers := handlers.NewPairingHandlers(serverState, authManager)

err := startP2PServer(ctx, pm, p2pHandlers, pairingHandlers)
```

**Option B: Create a RegisterHandlers method**
```go
// In handlers package
func (h *P2PHandlers) RegisterMessageHandlers(pm *p2p.P2PManager) {
    pm.SetMessageHandler(p2p.MessageTypePairingRequest, h.HandlePairingRequest)
    // ... register other handlers
}
```

**Recommended: Option B** - More explicit and testable

---

### Task 6: Testing & Verification

**After completing extraction:**

1. **Run existing tests:**
```bash
go test ./cmd/server/handlers/...
go test ./internal/...
go test -race ./...
```

2. **Add tests for new handlers:**
```bash
# Create test files
touch cmd/server/handlers/p2p_test.go
touch cmd/server/handlers/pairing_test.go
```

3. **Integration test:**
```bash
# Build and run server
go build -o pwman-server ./cmd/server
./pwman-server &

# Test all endpoints
curl http://localhost:PORT/api/health
curl http://localhost:PORT/api/p2p/status
curl http://localhost:PORT/api/pairing/status

# Kill server
pkill pwman-server
```

4. **Compare behavior:**
   - Before refactor: Test all endpoints, save responses
   - After refactor: Test all endpoints, compare responses
   - Ensure no regression

---

## Execution Order

**Recommended sequence:**

1. ✅ **Extract p2p.go handlers** (Task 1)
   - Create cmd/server/handlers/p2p.go
   - Copy handlers from main.go
   - Replace global state with h.state calls
   - Test compilation

2. ✅ **Extract pairing.go handlers** (Task 2)
   - Create cmd/server/handlers/pairing.go
   - Copy handlers from main.go
   - Replace global state with h.state calls
   - Handle message handler registration (Task 5)
   - Test compilation

3. ✅ **Update main.go routes** (Task 3)
   - Replace old route registrations with new handler-based ones
   - Update handler initialization
   - Test compilation

4. ✅ **Remove old handlers from main.go** (Task 4)
   - Delete lines 253-2700 from main.go
   - Verify line count <200

5. ✅ **Run tests** (Task 6)
   - Unit tests
   - Integration tests
   - Race condition tests

6. ✅ **Update documentation**
   - Update STATUS.md
   - Update ARCHITECTURE.md if needed

---

## Success Criteria

- [ ] main.go reduced to <200 lines
- [ ] All handlers extracted to cmd/server/handlers/
- [ ] All tests passing (`go test -race ./...`)
- [ ] No compilation errors
- [ ] No runtime regressions
- [ ] All endpoints functional
- [ ] Code follows project conventions (see CODER_AGENT.md)
- [ ] No global state in handlers (use ServerState)
- [ ] Proper error handling
- [ ] Security checklist passed

---

## Risk Mitigation

**Potential issues:**

1. **P2P message handlers need state**
   - Solution: Pass ServerState to handler constructors

2. **Circular dependencies**
   - Solution: Use dependency injection, avoid circular imports

3. **Race conditions**
   - Solution: Always use ServerState methods, never access state directly

4. **Breaking existing functionality**
   - Solution: Test thoroughly before and after, compare responses

5. **Missing imports**
   - Solution: Check all imports carefully, use `goimports`

---

## Estimated Effort

- **Task 1 (p2p.go)**: 2-3 hours
- **Task 2 (pairing.go)**: 2-3 hours  
- **Task 3 (routes)**: 1 hour
- **Task 4 (cleanup)**: 30 minutes
- **Task 5 (message handlers)**: 1 hour
- **Task 6 (testing)**: 2 hours

**Total**: 8-10 hours (1-2 days)

---

## Next Steps

1. Read this build plan
2. Start with Task 1 (extract p2p.go)
3. Follow execution order
4. Test after each task
5. Update STATUS.md when complete

**Ready to build? Let's complete this refactoring! 🚀**
