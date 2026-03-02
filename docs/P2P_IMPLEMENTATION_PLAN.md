# P2P Implementation Plan

## Problem
The server P2P endpoints in `cmd/server/main.go` are stubs that don't actually implement P2P functionality. The real P2P code exists in `internal/p2p/` but is not connected to the server.

---

## Implementation Tasks

### Task 1: Connect P2PManager to Server
**File:** `cmd/server/main.go`

Add a global P2PManager instance and initialize it:
- Import `internal/p2p` package
- Add `var p2pManager *p2p.P2PManager` global variable
- Create initialization function that loads device info from vault config

**Files to modify:**
- `cmd/server/main.go`

---

### Task 2: Implement handleP2PStart
**File:** `cmd/server/main.go`

```go
func handleP2PStart(w http.ResponseWriter, r *http.Request) {
    if p2pManager != nil && p2pManager.IsRunning() {
        jsonResponse(w, Response{Success: true, Data: "P2P already running"})
        return
    }
    
    // Get device info from vault config
    vaultConfig, _ := config.LoadVaultConfig(activeVault)
    
    cfg := p2p.P2PConfig{
        DeviceName: vaultConfig.DeviceName,
        DeviceID:   vaultConfig.DeviceID,
    }
    
    manager, err := p2p.NewP2PManager(cfg)
    if err != nil {
        jsonResponse(w, Response{Success: false, Error: "failed to create P2P manager"})
        return
    }
    
    if err := manager.Start(); err != nil {
        jsonResponse(w, Response{Success: false, Error: "failed to start P2P: " + err.Error()})
        return
    }
    
    p2pManager = manager
    jsonResponse(w, Response{Success: true, Data: "P2P started"})
}
```

---

### Task 3: Implement handleP2PStatus
**File:** `cmd/server/main.go`

```go
func handleP2PStatus(w http.ResponseWriter, r *http.Request) {
    if p2pManager == nil || !p2pManager.IsRunning() {
        jsonResponse(w, Response{Success: true, Data: P2PStatusResponse{Running: false}})
        return
    }
    
    response := P2PStatusResponse{
        Running:   true,
        PeerID:    p2pManager.GetPeerID(),
        Addresses: p2pManager.GetListenAddresses(),
    }
    
    // Add connected peers
    peers := p2pManager.GetConnectedPeers()
    for _, p := range peers {
        response.Connected = append(response.Connected, P2PPeerInfo{
            ID:        p.ID,
            Name:      p.Name,
            Connected: p.Connected,
        })
    }
    
    // Add discovered peers
    allPeers := p2pManager.GetAllPeers()
    for _, p := range allPeers {
        if !p.Connected {
            response.Discovered = append(response.Discovered, P2PPeerInfo{
                ID:   p.ID,
                Name: p.Name,
            })
        }
    }
    
    jsonResponse(w, Response{Success: true, Data: response})
}
```

---

### Task 4: Implement handleP2PConnect
**File:** `cmd/server/main.go`

```go
func handleP2PConnect(w http.ResponseWriter, r *http.Request) {
    if p2pManager == nil || !p2pManager.IsRunning() {
        jsonResponse(w, Response{Success: false, Error: "P2P not running"})
        return
    }
    
    var req ConnectRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        jsonResponse(w, Response{Success: false, Error: err.Error()})
        return
    }
    
    if err := p2pManager.ConnectToPeer(req.Address); err != nil {
        jsonResponse(w, Response{Success: false, Error: "failed to connect: " + err.Error()})
        return
    }
    
    jsonResponse(w, Response{Success: true, Data: "Connected to peer"})
}
```

---

### Task 5: Implement handleP2PStop
**File:** `cmd/server/main.go`

```go
func handleP2PStop(w http.ResponseWriter, r *http.Request) {
    if p2pManager == nil {
        jsonResponse(w, Response{Success: true, Data: "P2P not running"})
        return
    }
    
    p2pManager.Stop()
    p2pManager = nil
    
    jsonResponse(w, Response{Success: true, Data: "P2P stopped"})
}
```

---

### Task 6: Implement Device Approval Flow
**Files:** `cmd/server/main.go`, `internal/p2p/sync.go`

Need to:
1. Track pending device approvals in memory or database
2. Implement REQUEST_APPROVAL → APPROVE_DEVICE flow
3. Re-encrypt all passwords for new device upon approval

**New functions needed:**
- `handleP2PApprovals` - return pending device list
- `handleP2PApprove` - approve device and re-encrypt keys
- `handleP2PReject` - reject device

---

### Task 7: Implement Sync Handler
**Files:** `cmd/server/main.go`, `internal/p2p/sync.go`

Connect SyncHandler to server:
1. Create SyncHandler during P2P start
2. Handle SYNC_DATA messages
3. Implement conflict resolution

---

### Task 8: Handle P2P Events
**File:** `cmd/server/main.go`

Set up goroutines to handle:
- `p2pManager.ConnectedChan()` - new peer connected
- `p2pManager.DisconnectedChan()` - peer disconnected
- `p2pManager.MessageChan()` - incoming messages

---

## Implementation Order

1. **Task 1** - Connect P2PManager to server (prerequisite) ✅
2. **Task 2** - Implement P2P start ✅
3. **Task 3** - Implement P2P status ✅
4. **Task 4** - Implement P2P connect ✅
5. **Task 5** - Implement P2P stop ✅
6. **Task 6** - Device approval flow ✅
7. **Task 7** - Sync handler ✅
8. **Task 8** - Event handling ✅

---

## Implementation Notes

### Completed
- Added `internal/p2p` import to server
- Global `p2pManager` variable with mutex protection
- Real P2P startup using libp2p
- P2P status returns peer ID and addresses
- Connect/disconnect peers
- Device approval tracking (in-memory)
- Event handling goroutine for connected/disconnected peers

### Known Limitations
- Pending approvals are stored in-memory only (lost on server restart)
- Sync handler not fully connected (would need more work for actual data sync)
- Device approval doesn't re-encrypt passwords yet (would need full re-encryption flow)

### To Build
```bash
go build -o server ./cmd/server
go build -o pwman ./cmd/pwman
```

---

## Testing Checklist

- [ ] `./pwman p2p start` actually starts P2P
- [ ] `./pwman p2p status` shows peer ID and addresses
- [ ] `./pwman p2p connect <addr>` connects to peer
- [ ] Two devices on LAN discover each other via mDNS
- [ ] Device approval works
- [ ] Passwords sync between devices

---

## Dependencies

The implementation requires these packages (already in go.mod):
- `github.com/libp2p/go-libp2p`
- `github.com/libp2p/go-libp2p/core/host`
- `github.com/libp2p/go-libp2p/core/peer`
- `github.com/libp2p/go-libp2p/p2p/discovery/mdns`
- `github.com/libp2p/go-libp2p/p2p/security/noise`
- `github.com/libp2p/go-libp2p/p2p/transport/tcp`

---

## Notes

- The vault must be unlocked before P2P can start (need device info)
- P2PManager requires device name and ID from vault config
- All P2P operations should be thread-safe (use mutexes where needed)
- Consider adding graceful shutdown handling
