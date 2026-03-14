# IMPLEMENTATION BRIEF - Phase 4

**Status:** 📋 READY TO START  
**Source:** docs/IMPLEMENTATION_ROADMAP_PRODUCTION_READY.md (v3.1)  
**Duration:** 3 days  
**Priority:** MEDIUM

---

## Goal

Implement Lamport logical clocks for causal ordering and improved conflict resolution in the sync protocol.

---

## Why This Matters

- **Wall-clock timestamps**: Vulnerable to clock skew, can cause conflicts
- **Lamport clocks**: Provide causal ordering (happens-before relationships)
- **Conflict resolution**: Higher logical clock wins (deterministic)
- **Offline sync**: Devices can sync after being offline without timestamp conflicts

---

## Files to Create

```
internal/sync/clock.go       [NEW] - Lamport clock implementation
internal/sync/clock_test.go  [NEW] - Unit tests
internal/sync/merge.go       [NEW] - Conflict resolution logic
```

## Files to Modify

```
cmd/server/handlers/sync.go   [NEW/MODIFY] - Add re-sync handler
cmd/server/handlers/entry.go  [MODIFY] - Use logical clocks on update
internal/state/server.go      [MODIFY] - Add ClockManager
```

---

## Implementation Steps

### Step 1: Create Sync Package with Lamport Clocks (Day 1)

**1.1 Lamport Clock Implementation**

```go
// internal/sync/clock.go
package sync

import (
    "sync"
    
    "github.com/bok1c4/pwman/pkg/models"
)

// LamportClock is a monotonically increasing logical counter per device
type LamportClock struct {
    mu     sync.Mutex
    value  int64
    deviceID string
}

func NewLamportClock(deviceID string, initial int64) *LamportClock {
    return &LamportClock{
        value:    initial,
        deviceID: deviceID,
    }
}

// Tick increments and returns the new value
// Call this before creating/updating an entry
func (c *LamportClock) Tick() int64 {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.value++
    return c.value
}

// Witness updates clock based on remote value (takes max + 1)
// Call this when receiving sync data from another device
func (c *LamportClock) Witness(remote int64) int64 {
    c.mu.Lock()
    defer c.mu.Unlock()
    if remote > c.value {
        c.value = remote
    }
    c.value++
    return c.value
}

// Current returns current value without incrementing
func (c *LamportClock) Current() int64 {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.value
}
```

**1.2 Conflict Resolution**

```go
// internal/sync/merge.go

// MergeEntry resolves conflicts between local and remote entries
// Priority: higher logical_clock wins
// Tie-break 1: later updated_at timestamp
// Tie-break 2: lexicographically higher origin_device (deterministic)
func MergeEntry(local, remote models.PasswordEntry) models.PasswordEntry {
    // Compare logical clocks
    if remote.LogicalClock > local.LogicalClock {
        return remote
    }
    
    if remote.LogicalClock == local.LogicalClock {
        // Tie-breaker 1: later updated_at timestamp
        if remote.UpdatedAt.After(local.UpdatedAt) {
            return remote
        }
        
        // Tie-breaker 2: lexicographically higher origin_device
        if remote.UpdatedAt.Equal(local.UpdatedAt) {
            if remote.OriginDevice > local.OriginDevice {
                return remote
            }
        }
    }
    
    return local
}

// MergeEntries merges two sets of entries with conflict resolution
func MergeEntries(local, remote []models.PasswordEntry) []models.PasswordEntry {
    merged := make(map[string]models.PasswordEntry)
    
    // Add all local entries
    for _, e := range local {
        merged[e.ID] = e
    }
    
    // Merge remote entries
    for _, remoteEntry := range remote {
        if localEntry, exists := merged[remoteEntry.ID]; exists {
            merged[remoteEntry.ID] = MergeEntry(localEntry, remoteEntry)
        } else {
            merged[remoteEntry.ID] = remoteEntry
        }
    }
    
    // Convert map back to slice
    result := make([]models.PasswordEntry, 0, len(merged))
    for _, e := range merged {
        result = append(result, e)
    }
    
    return result
}
```

### Step 2: Add ClockManager to State (Day 1-2)

```go
// internal/state/server.go

// ClockManager manages logical clocks for all devices
type ClockManager struct {
    mu      sync.RWMutex
    clocks  map[string]*sync.LamportClock
    storage ClockStorage
}

type ClockStorage interface {
    GetDeviceClock(deviceID string) (int64, error)
    UpsertDeviceClock(deviceID string, clock int64) error
}

func NewClockManager(storage ClockStorage) *ClockManager {
    return &ClockManager{
        clocks:  make(map[string]*sync.LamportClock),
        storage: storage,
    }
}

func (cm *ClockManager) GetClock(deviceID string) (*sync.LamportClock, error) {
    cm.mu.RLock()
    if clock, ok := cm.clocks[deviceID]; ok {
        cm.mu.RUnlock()
        return clock, nil
    }
    cm.mu.RUnlock()
    
    // Load from storage
    initial, err := cm.storage.GetDeviceClock(deviceID)
    if err != nil {
        initial = 0
    }
    
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    // Double-check after acquiring write lock
    if clock, ok := cm.clocks[deviceID]; ok {
        return clock, nil
    }
    
    clock := sync.NewLamportClock(deviceID, initial)
    cm.clocks[deviceID] = clock
    
    return clock, nil
}

func (cm *ClockManager) SaveClock(deviceID string) error {
    cm.mu.RLock()
    clock, ok := cm.clocks[deviceID]
    cm.mu.RUnlock()
    
    if !ok {
        return fmt.Errorf("clock not found for device: %s", deviceID)
    }
    
    return cm.storage.UpsertDeviceClock(deviceID, clock.Current())
}
```

### Step 3: Create Re-Sync Handler (Day 2)

```go
// cmd/server/handlers/sync.go

type SyncHandlers struct {
    state *state.ServerState
}

func NewSyncHandlers(s *state.ServerState) *SyncHandlers {
    return &SyncHandlers{state: s}
}

// Resync triggers sync with specific device or all devices
func (h *SyncHandlers) Resync(w http.ResponseWriter, r *http.Request) {
    storage, ok := h.state.GetVaultStorage()
    if !ok {
        api.Error(w, http.StatusBadRequest, "VAULT_LOCKED", "vault not unlocked")
        return
    }
    
    // Get all trusted devices
    devices, err := storage.ListDevices()
    if err != nil {
        api.Error(w, http.StatusInternalServerError, "DB_ERROR", "failed to list devices")
        return
    }
    
    // Initiate sync with each trusted device
    var syncedDevices []string
    for _, device := range devices {
        if !device.Trusted {
            continue
        }
        
        // Skip self
        vault, _ := h.state.GetVault()
        if device.ID == vault.Config.DeviceID {
            continue
        }
        
        // Initiate sync
        // ... sync logic here ...
        
        syncedDevices = append(syncedDevices, device.Name)
    }
    
    api.Success(w, map[string]interface{}{
        "synced_devices": syncedDevices,
        "timestamp": time.Now().Unix(),
    })
}
```

### Step 4: Update Entry Handlers (Day 3)

Modify entry creation/update to use logical clocks:

```go
// In entry creation:
clock, _ := h.state.ClockManager.GetClock(vault.Config.DeviceID)
entry := models.PasswordEntry{
    ID:           uuid.New().String(),
    Site:         req.Site,
    Username:     req.Username,
    // ... other fields ...
    LogicalClock: clock.Tick(),  // Use logical clock
    OriginDevice: vault.Config.DeviceID,
}

// In entry update:
existing.Site = req.Site
existing.Username = req.Username
// ... other fields ...
existing.LogicalClock = clock.Tick()  // Increment on update
existing.UpdatedAt = time.Now()
existing.UpdatedBy = vault.Config.DeviceID
```

### Step 5: Database Migration (Day 3)

```sql
-- Migration V4: Add device_clocks table for Lamport timestamps
CREATE TABLE IF NOT EXISTS device_clocks (
    device_id TEXT PRIMARY KEY REFERENCES devices(id),
    clock INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Note: logical_clock and origin_device columns added in Phase 3
UPDATE vault_meta SET value = '4' WHERE key = 'schema_version';
```

---

## Testing Strategy

### Unit Tests
- Lamport clock increment
- Lamport clock witness (max + 1)
- Conflict resolution (higher clock wins)
- Tie-breaker logic (timestamp, then origin_device)
- Entry merging

### Integration Tests
- Create entry with logical clock
- Update entry (clock increments)
- Sync between devices (clock witnesses)
- Conflict resolution during sync

### Manual Tests
1. Create entry on Device A (clock = 1)
2. Sync to Device B (B witnesses clock = 1)
3. Edit on Device B (B's clock = 2)
4. Edit on Device A (A's clock = 2)
5. Sync again (conflict resolved by tie-breaker)

---

## Success Criteria

- [ ] Lamport clocks increment correctly
- [ ] Clock witnesses remote values (max + 1)
- [ ] Conflict resolution uses logical clocks
- [ ] Tie-breakers work deterministically
- [ ] All sync operations update clocks
- [ ] All tests passing

---

## Blockers

None - Phase 3 complete.

---

**Ready to start:** Phase 4 implementation can begin immediately.
