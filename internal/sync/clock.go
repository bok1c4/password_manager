// Package sync provides Lamport logical clocks and conflict resolution
// for distributed synchronization of password entries.
package sync

import (
	"sync"

	"github.com/bok1c4/pwman/pkg/models"
)

// LamportClock is a monotonically increasing logical counter per device
type LamportClock struct {
	mu       sync.Mutex
	value    int64
	deviceID string
}

// NewLamportClock creates a new Lamport clock with the given initial value
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

// Set updates the clock value (use with caution - usually only during init)
func (c *LamportClock) Set(value int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value = value
}

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
		// This is stable and deterministic
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

// ClockManager manages logical clocks for all devices
type ClockManager struct {
	mu      sync.RWMutex
	clocks  map[string]*LamportClock
	storage ClockStorage
}

// ClockStorage interface for persisting clock values
type ClockStorage interface {
	GetDeviceClock(deviceID string) (int64, error)
	UpsertDeviceClock(deviceID string, clock int64) error
}

// NewClockManager creates a new clock manager
func NewClockManager(storage ClockStorage) *ClockManager {
	return &ClockManager{
		clocks:  make(map[string]*LamportClock),
		storage: storage,
	}
}

// GetClock returns the Lamport clock for a device, loading from storage if needed
func (cm *ClockManager) GetClock(deviceID string) (*LamportClock, error) {
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

	clock := NewLamportClock(deviceID, initial)
	cm.clocks[deviceID] = clock

	return clock, nil
}

// SaveClock persists the current clock value to storage
func (cm *ClockManager) SaveClock(deviceID string) error {
	cm.mu.RLock()
	clock, ok := cm.clocks[deviceID]
	cm.mu.RUnlock()

	if !ok {
		return nil // Clock doesn't exist, nothing to save
	}

	return cm.storage.UpsertDeviceClock(deviceID, clock.Current())
}

// SaveAllClocks persists all clock values to storage
func (cm *ClockManager) SaveAllClocks() error {
	cm.mu.RLock()
	clocks := make(map[string]*LamportClock)
	for k, v := range cm.clocks {
		clocks[k] = v
	}
	cm.mu.RUnlock()

	for deviceID, clock := range clocks {
		if err := cm.storage.UpsertDeviceClock(deviceID, clock.Current()); err != nil {
			return err
		}
	}

	return nil
}
