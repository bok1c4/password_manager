# God File Refactoring Plan - cmd/server/main.go

**Current Size**: 2,786 lines  
**Target Size**: <200 lines (entry point only)  
**Priority**: MEDIUM (after security fixes)  
**Estimated Effort**: 2-3 days

---

## Why This Matters

### Current Problems

1. **Cognitive Overload**: 2,786 lines is impossible to understand in one sitting
2. **Merge Conflicts**: Everyone editing same file = constant conflicts
3. **Testing Nightmare**: Can't test handlers in isolation
4. **Security Risk**: Hard to audit, easy to miss vulnerabilities
5. **Onboarding**: New devs can't find where to make changes
6. **Code Duplication**: Same patterns repeated (auth checks, vault locking)

### Benefits of Refactoring

1. **Single Responsibility**: Each file does one thing well
2. **Testability**: Can test individual handlers without server
3. **Parallel Development**: Multiple devs can work on different endpoints
4. **Code Reuse**: Common patterns in middleware, utilities
5. **Maintainability**: 200-line files vs 2,786-line file
6. **Security Auditing**: Easy to review each domain separately

---

## Proposed Structure

```
cmd/server/
├── main.go                    # Entry point only (~150 lines)
├── routes.go                  # Route registration (~100 lines)
└── handlers/
    ├── auth.go               # Authentication handlers (init, unlock, lock)
    ├── vault.go              # Vault management handlers
    ├── entries.go            # Password CRUD handlers
    ├── devices.go            # Device management handlers
    ├── p2p.go                # P2P network handlers
    ├── pairing.go            # Device pairing handlers
    └── health.go             # Health/metrics endpoints

internal/
├── middleware/
│   ├── auth.go               # Authentication middleware
│   ├── cors.go               # CORS middleware
│   ├── logging.go            # Request logging
│   └── ratelimit.go          # Rate limiting middleware
├── state/
│   └── server.go             # Global server state management
└── api/
    └── response.go           # Common response helpers
```

---

## Migration Plan

### Phase 1: Extract Response Helpers & Middleware (Day 1)

**Files to Create**:

1. **internal/api/response.go**
```go
package api

import (
    "encoding/json"
    "net/http"
)

type Response struct {
    Success bool        `json:"success"`
    Error   string      `json:"error,omitempty"`
    Code    string      `json:"code,omitempty"`
    Data    interface{} `json:"data,omitempty"`
}

func JSONResponse(w http.ResponseWriter, status int, v Response) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(v)
}

func Success(w http.ResponseWriter, data interface{}) {
    JSONResponse(w, http.StatusOK, Response{Success: true, Data: data})
}

func Error(w http.ResponseWriter, status int, code, message string) {
    JSONResponse(w, status, Response{Success: false, Code: code, Error: message})
}
```

2. **internal/middleware/cors.go**
```go
package middleware

import "net/http"

var allowedOrigins = []string{
    "tauri://localhost",
    "https://tauri.localhost", 
    "http://localhost:1420",
}

func CORS(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        origin := r.Header.Get("Origin")
        
        // Check if origin is allowed
        allowed := false
        for _, o := range allowedOrigins {
            if origin == o {
                allowed = true
                w.Header().Set("Access-Control-Allow-Origin", origin)
                break
            }
        }
        
        if !allowed && origin != "" {
            http.Error(w, "Forbidden", http.StatusForbidden)
            return
        }
        
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        w.Header().Set("Vary", "Origin")
        
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        
        next(w, r)
    }
}
```

3. **internal/middleware/auth.go**
```go
package middleware

import (
    "net/http"
    "github.com/bok1c4/pwman/internal/api"
)

func Auth(authManager *api.AuthManager, next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Skip auth for specific endpoints
        if r.URL.Path == "/api/unlock" || r.URL.Path == "/api/init" {
            next(w, r)
            return
        }
        
        token := r.Header.Get("Authorization")
        if token == "" {
            api.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required")
            return
        }
        
        // Remove "Bearer " prefix
        if len(token) > 7 && token[:7] == "Bearer " {
            token = token[7:]
        }
        
        if !authManager.ValidateToken(token) {
            api.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "invalid or expired token")
            return
        }
        
        next(w, r)
    }
}
```

**Changes to main.go**:
- Remove `jsonResponse()` function
- Remove `corsHandler()` function
- Remove `requireAuth()` function
- Update imports

---

### Phase 2: Extract State Management (Day 1)

**Current Issue**: Global variables scattered throughout main.go

**Create internal/state/server.go**:
```go
package state

import (
    "context"
    "sync"
    
    "github.com/bok1c4/pwman/internal/config"
    "github.com/bok1c4/pwman/internal/crypto"
    "github.com/bok1c4/pwman/internal/p2p"
    "github.com/bok1c4/pwman/internal/storage"
)

// ServerState holds all server-wide state
type ServerState struct {
    mu sync.RWMutex
    
    // Vault state
    vault      *Vault
    isUnlocked bool
    
    // P2P state
    p2pManager *p2p.P2PManager
    p2pCancel  context.CancelFunc
    
    // Pairing state
    pairingCodes      map[string]PairingCode
    pairingRequests   map[string]PairingRequest
    pairingResponseCh chan p2p.PairingResponsePayload
    pendingApprovals  map[string]PendingApproval
    
    // Sub-locks for fine-grained concurrency
    vaultLock        sync.Mutex
    p2pLock          sync.Mutex
    pairingLock      sync.Mutex
    approvalsLock    sync.Mutex
}

type Vault struct {
    PrivateKey *crypto.KeyPair
    Storage    *storage.SQLite
    Config     *config.VaultConfig
    VaultName  string
}

type PairingCode struct {
    Code        string
    VaultID     string
    VaultName   string
    DeviceID    string
    DeviceName  string
    PublicKey   string
    Fingerprint string
    ExpiresAt   time.Time
    Used        bool
}

type PairingRequest struct {
    Code       string
    DeviceID   string
    DeviceName string
}

type PendingApproval struct {
    DeviceID    string
    DeviceName  string
    PublicKey   string
    Fingerprint string
    Status      string
    ConnectedAt time.Time
}

// NewServerState creates initialized state
func NewServerState() *ServerState {
    return &ServerState{
        pairingCodes:      make(map[string]PairingCode),
        pairingRequests:   make(map[string]PairingRequest),
        pairingResponseCh: make(chan p2p.PairingResponsePayload, 10),
        pendingApprovals:  make(map[string]PendingApproval),
    }
}

// Vault methods
func (s *ServerState) GetVault() (*Vault, bool) {
    s.vaultLock.Lock()
    defer s.vaultLock.Unlock()
    if s.vault == nil {
        return nil, false
    }
    return s.vault, true
}

func (s *ServerState) SetVault(v *Vault) {
    s.vaultLock.Lock()
    defer s.vaultLock.Unlock()
    s.vault = v
    s.isUnlocked = v != nil
}

func (s *ServerState) IsUnlocked() bool {
    s.vaultLock.Lock()
    defer s.vaultLock.Unlock()
    return s.isUnlocked
}

func (s *ServerState) CloseVault() error {
    s.vaultLock.Lock()
    defer s.vaultLock.Unlock()
    if s.vault != nil && s.vault.Storage != nil {
        err := s.vault.Storage.Close()
        s.vault = nil
        s.isUnlocked = false
        return err
    }
    return nil
}

// P2P methods
func (s *ServerState) GetP2PManager() (*p2p.P2PManager, bool) {
    s.p2pLock.Lock()
    defer s.p2pLock.Unlock()
    return s.p2pManager, s.p2pManager != nil
}

func (s *ServerState) SetP2PManager(m *p2p.P2PManager) {
    s.p2pLock.Lock()
    defer s.p2pLock.Unlock()
    s.p2pManager = m
}

func (s *ServerState) StopP2P() {
    s.p2pLock.Lock()
    defer s.p2pLock.Unlock()
    if s.p2pCancel != nil {
        s.p2pCancel()
    }
    if s.p2pManager != nil {
        s.p2pManager.Stop()
    }
    s.p2pManager = nil
}

// Pairing methods
func (s *ServerState) AddPairingCode(code string, pc PairingCode) {
    s.pairingLock.Lock()
    defer s.pairingLock.Unlock()
    s.pairingCodes[code] = pc
}

func (s *ServerState) GetPairingCode(code string) (PairingCode, bool) {
    s.pairingLock.Lock()
    defer s.pairingLock.Unlock()
    pc, ok := s.pairingCodes[code]
    return pc, ok
}

func (s *ServerState) MarkPairingCodeUsed(code string) {
    s.pairingLock.Lock()
    defer s.pairingLock.Unlock()
    if pc, ok := s.pairingCodes[code]; ok {
        pc.Used = true
        s.pairingCodes[code] = pc
    }
}

// Approval methods
func (s *ServerState) AddPendingApproval(deviceID string, pa PendingApproval) {
    s.approvalsLock.Lock()
    defer s.approvalsLock.Unlock()
    s.pendingApprovals[deviceID] = pa
}

func (s *ServerState) GetPendingApproval(deviceID string) (PendingApproval, bool) {
    s.approvalsLock.Lock()
    defer s.approvalsLock.Unlock()
    pa, ok := s.pendingApprovals[deviceID]
    return pa, ok
}

func (s *ServerState) RemovePendingApproval(deviceID string) {
    s.approvalsLock.Lock()
    defer s.approvalsLock.Unlock()
    delete(s.pendingApprovals, deviceID)
}

func (s *ServerState) ListPendingApprovals() []PendingApproval {
    s.approvalsLock.Lock()
    defer s.approvalsLock.Unlock()
    
    result := make([]PendingApproval, 0, len(s.pendingApprovals))
    for _, pa := range s.pendingApprovals {
        result = append(result, pa)
    }
    return result
}
```

**Changes to main.go**:
- Remove all global state variables
- Remove Vault, PairingCode, PairingRequest, PendingApproval types
- Use `state.NewServerState()` in main()

---

### Phase 3: Extract Handler Packages (Days 2-3)

Move handlers by domain:

#### 1. cmd/server/handlers/auth.go
```go
package handlers

import (
    "encoding/json"
    "net/http"
    
    "github.com/bok1c4/pwman/internal/api"
    "github.com/bok1c4/pwman/internal/config"
    "github.com/bok1c4/pwman/internal/crypto"
    "github.com/bok1c4/pwman/internal/state"
    "github.com/bok1c4/pwman/internal/storage"
    "github.com/bok1c4/pwman/pkg/models"
    "github.com/google/uuid"
)

type AuthHandlers struct {
    state       *state.ServerState
    authManager *api.AuthManager
}

func NewAuthHandlers(s *state.ServerState, am *api.AuthManager) *AuthHandlers {
    return &AuthHandlers{state: s, authManager: am}
}

func (h *AuthHandlers) Init(w http.ResponseWriter, r *http.Request) {
    // ... handleInit logic, but using h.state instead of global state
}

func (h *AuthHandlers) Unlock(w http.ResponseWriter, r *http.Request) {
    // ... handleUnlock logic
}

func (h *AuthHandlers) Lock(w http.ResponseWriter, r *http.Request) {
    h.state.CloseVault()
    h.authManager.SetVaultUnlocked(false)
    api.Success(w, nil)
}

func (h *AuthHandlers) IsUnlocked(w http.ResponseWriter, r *http.Request) {
    api.Success(w, map[string]bool{"unlocked": h.state.IsUnlocked()})
}

func (h *AuthHandlers) IsInitialized(w http.ResponseWriter, r *http.Request) {
    activeVault, _ := config.GetActiveVault()
    vaultConfig, _ := config.LoadVaultConfig(activeVault)
    initialized := vaultConfig != nil && vaultConfig.DeviceID != ""
    api.Success(w, map[string]bool{"initialized": initialized})
}
```

#### 2. cmd/server/handlers/entries.go
```go
package handlers

import (
    "encoding/json"
    "net/http"
    
    "github.com/bok1c4/pwman/internal/api"
    "github.com/bok1c4/pwman/internal/state"
    "github.com/bok1c4/pwman/pkg/models"
    "github.com/google/uuid"
)

type EntryHandlers struct {
    state *state.ServerState
}

func NewEntryHandlers(s *state.ServerState) *EntryHandlers {
    return &EntryHandlers{state: s}
}

func (h *EntryHandlers) List(w http.ResponseWriter, r *http.Request) {
    vault, ok := h.state.GetVault()
    if !ok {
        api.Success(w, []models.PasswordEntry{})
        return
    }
    
    entries, err := vault.Storage.ListEntries()
    if err != nil {
        api.Error(w, http.StatusInternalServerError, "DB_ERROR", "failed to list entries")
        return
    }
    
    api.Success(w, entries)
}

func (h *EntryHandlers) Add(w http.ResponseWriter, r *http.Request) {
    // ... handleAddEntry logic
}

func (h *EntryHandlers) Update(w http.ResponseWriter, r *http.Request) {
    // ... handleUpdateEntry logic
}

func (h *EntryHandlers) Delete(w http.ResponseWriter, r *http.Request) {
    // ... handleDeleteEntry logic
}

func (h *EntryHandlers) GetPassword(w http.ResponseWriter, r *http.Request) {
    // ... handleGetPassword logic
}
```

#### 3. cmd/server/handlers/vault.go
```go
package handlers

import (
    "encoding/json"
    "net/http"
    
    "github.com/bok1c4/pwman/internal/api"
    "github.com/bok1c4/pwman/internal/config"
    "github.com/bok1c4/pwman/internal/state"
)

type VaultHandlers struct {
    state *state.ServerState
}

func NewVaultHandlers(s *state.ServerState) *VaultHandlers {
    return &VaultHandlers{state: s}
}

func (h *VaultHandlers) List(w http.ResponseWriter, r *http.Request) {
    // ... handleVaults logic
}

func (h *VaultHandlers) Use(w http.ResponseWriter, r *http.Request) {
    // ... handleVaultUse logic
}

func (h *VaultHandlers) Create(w http.ResponseWriter, r *http.Request) {
    // ... handleVaultCreate logic
}

func (h *VaultHandlers) Delete(w http.ResponseWriter, r *http.Request) {
    // ... handleVaultDelete logic
}
```

#### 4. cmd/server/handlers/p2p.go
```go
package handlers

import (
    "net/http"
    
    "github.com/bok1c4/pwman/internal/api"
    "github.com/bok1c4/pwman/internal/state"
)

type P2PHandlers struct {
    state *state.ServerState
}

func NewP2PHandlers(s *state.ServerState) *P2PHandlers {
    return &P2PHandlers{state: s}
}

func (h *P2PHandlers) Status(w http.ResponseWriter, r *http.Request) {
    // ... handleP2PStatus logic
}

func (h *P2PHandlers) Start(w http.ResponseWriter, r *http.Request) {
    // ... handleP2PStart logic
}

func (h *P2PHandlers) Stop(w http.ResponseWriter, r *http.Request) {
    // ... handleP2PStop logic
}

func (h *P2PHandlers) Peers(w http.ResponseWriter, r *http.Request) {
    // ... handleP2PPeers logic
}

func (h *P2PHandlers) Connect(w http.ResponseWriter, r *http.Request) {
    // ... handleP2PConnect logic
}

func (h *P2PHandlers) Disconnect(w http.ResponseWriter, r *http.Request) {
    // ... handleP2PDisconnect logic
}

func (h *P2PHandlers) Approvals(w http.ResponseWriter, r *http.Request) {
    // ... handleP2PApprovals logic
}

func (h *P2PHandlers) Approve(w http.ResponseWriter, r *http.Request) {
    // ... handleP2PApprove logic
}

func (h *P2PHandlers) Reject(w http.ResponseWriter, r *http.Request) {
    // ... handleP2PReject logic
}

func (h *P2PHandlers) Sync(w http.ResponseWriter, r *http.Request) {
    // ... handleP2PSync logic
}
```

#### 5. cmd/server/handlers/pairing.go
```go
package handlers

import (
    "net/http"
    
    "github.com/bok1c4/pwman/internal/api"
    "github.com/bok1c4/pwman/internal/state"
)

type PairingHandlers struct {
    state *state.ServerState
}

func NewPairingHandlers(s *state.ServerState) *PairingHandlers {
    return &PairingHandlers{state: s}
}

func (h *PairingHandlers) Generate(w http.ResponseWriter, r *http.Request) {
    // ... handlePairingGenerate logic
}

func (h *PairingHandlers) Join(w http.ResponseWriter, r *http.Request) {
    // ... handlePairingJoin logic
}

func (h *PairingHandlers) Status(w http.ResponseWriter, r *http.Request) {
    // ... handlePairingStatus logic
}

// Also move P2P message handlers here
func (h *PairingHandlers) HandlePairingRequest(pm *p2p.P2PManager, msg p2p.ReceivedMessage) {
    // ... handlePairingRequest logic
}

func (h *PairingHandlers) HandleSyncRequest(pm *p2p.P2PManager, peerID string) {
    // ... handleSyncRequest logic
}
```

#### 6. cmd/server/handlers/devices.go
```go
package handlers

import (
    "net/http"
    
    "github.com/bok1c4/pwman/internal/api"
    "github.com/bok1c4/pwman/internal/state"
)

type DeviceHandlers struct {
    state *state.ServerState
}

func NewDeviceHandlers(s *state.ServerState) *DeviceHandlers {
    return &DeviceHandlers{state: s}
}

func (h *DeviceHandlers) List(w http.ResponseWriter, r *http.Request) {
    // ... handleGetDevices logic
}
```

#### 7. cmd/server/handlers/health.go
```go
package handlers

import (
    "net/http"
    "runtime"
    
    "github.com/bok1c4/pwman/internal/api"
    "github.com/bok1c4/pwman/internal/state"
)

type HealthHandlers struct {
    state *state.ServerState
}

func NewHealthHandlers(s *state.ServerState) *HealthHandlers {
    return &HealthHandlers{state: s}
}

func (h *HealthHandlers) Health(w http.ResponseWriter, r *http.Request) {
    api.Success(w, map[string]interface{}{
        "status": "healthy",
        "version": "1.0.0",
    })
}

func (h *HealthHandlers) Metrics(w http.ResponseWriter, r *http.Request) {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    
    api.Success(w, map[string]interface{}{
        "memory": map[string]interface{}{
            "alloc": m.Alloc,
            "total_alloc": m.TotalAlloc,
            "sys": m.Sys,
            "num_gc": m.NumGC,
        },
    })
}

func (h *HealthHandlers) GeneratePassword(w http.ResponseWriter, r *http.Request) {
    // ... handleGeneratePassword logic
}
```

---

### Phase 4: New main.go (Day 3)

**File: cmd/server/main.go** (Target: <150 lines)
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
    p2pHandlers := handlers.NewP2PHandlers(serverState)
    pairingHandlers := handlers.NewPairingHandlers(serverState)
    deviceHandlers := handlers.NewDeviceHandlers(serverState)
    healthHandlers := handlers.NewHealthHandlers(serverState)
    
    // Find available port
    port, err := findAvailablePort()
    if err != nil {
        log.Fatalf("Failed to find port: %v", err)
    }
    
    // Setup routes with middleware chain
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
    
    // P2P routes
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
    
    // Pairing routes
    http.HandleFunc("/api/pairing/generate", auth(pairingHandlers.Generate))
    http.HandleFunc("/api/pairing/join", auth(pairingHandlers.Join))
    http.HandleFunc("/api/pairing/status", auth(pairingHandlers.Status))
    
    // Health routes (no auth required)
    http.HandleFunc("/api/health", healthHandlers.Health)
    http.HandleFunc("/api/metrics", healthHandlers.Metrics)
    http.HandleFunc("/api/generate", healthHandlers.GeneratePassword)
    
    log.Printf("Starting pwman API server on :%s", port)
    log.Fatal(http.ListenAndServe(":"+port, nil))
}

func findAvailablePort() (string, error) {
    // ... existing logic
}
```

---

## Testing Strategy

### During Refactoring

1. **After each phase, run**:
```bash
go test -race ./...
go build ./cmd/server
./pwman-server &  # Test it actually runs
curl http://localhost:PORT/api/health
```

2. **Compare behavior**:
   - Before refactor: Test all endpoints
   - After refactor: Test all endpoints
   - Compare responses

3. **Add tests incrementally**:
```bash
# After Phase 1
go test ./internal/middleware/...

# After Phase 2
go test ./internal/state/...

# After Phase 3
go test ./cmd/server/handlers/...
```

### Test Isolation Benefits

After refactoring, you can test handlers in isolation:

```go
// cmd/server/handlers/auth_test.go
func TestAuthHandlers_Unlock(t *testing.T) {
    // Setup
    state := state.NewServerState()
    authManager := api.NewAuthManager()
    handlers := NewAuthHandlers(state, authManager)
    
    // Create request
    req := httptest.NewRequest("POST", "/api/unlock", strings.NewReader(`{"password":"test"}`))
    rr := httptest.NewRecorder()
    
    // Execute
    handlers.Unlock(rr, req)
    
    // Assert
    if rr.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", rr.Code)
    }
}
```

---

## Benefits Summary

### Before (2,786 lines)
- ❌ Impossible to navigate
- ❌ Merge conflicts daily
- ❌ Can't unit test handlers
- ❌ Security audit takes days
- ❌ Onboarding takes weeks

### After (~150 lines main + 1,500 lines organized)
- ✅ Clear structure
- ✅ Work on handlers in parallel
- ✅ Unit test each handler
- ✅ Audit one domain at a time
- ✅ New devs productive in hours

---

## Implementation Priority

**Recommended order**:
1. ✅ **Finish Phase 1 security fixes first** (CRITICAL)
2. **Then do this refactor** (MEDIUM - reduces merge conflicts)
3. **Then Phase 2-3 security fixes** (HIGH)

**Why after Phase 1?**
- Phase 1 touches many lines in main.go
- Refactoring will cause massive merge conflicts
- Better to stabilize security first, then refactor
- Tests can be written for refactored code

---

## Migration Checklist

### Phase 1: Infrastructure
- [ ] Create internal/api/response.go
- [ ] Create internal/middleware/cors.go
- [ ] Create internal/middleware/auth.go
- [ ] Update main.go to use new helpers
- [ ] Run tests

### Phase 2: State Management
- [ ] Create internal/state/server.go
- [ ] Move all global variables to ServerState
- [ ] Update main.go to initialize ServerState
- [ ] Run tests

### Phase 3: Handlers
- [ ] Create cmd/server/handlers/auth.go
- [ ] Create cmd/server/handlers/entries.go
- [ ] Create cmd/server/handlers/vault.go
- [ ] Create cmd/server/handlers/p2p.go
- [ ] Create cmd/server/handlers/pairing.go
- [ ] Create cmd/server/handlers/devices.go
- [ ] Create cmd/server/handlers/health.go
- [ ] Move all handler functions
- [ ] Run tests

### Phase 4: Cleanup
- [ ] Simplify main.go to <150 lines
- [ ] Remove old handler functions from main.go
- [ ] Add comprehensive tests for all handlers
- [ ] Update documentation
- [ ] Full integration test

---

**Ready to implement?** Start after Phase 1 security fixes are complete.
