package sync

import (
	"testing"
	"time"

	"github.com/bok1c4/pwman/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLamportClock_Tick(t *testing.T) {
	clock := NewLamportClock("device-1", 0)

	// Initial tick should return 1
	assert.Equal(t, int64(1), clock.Tick())

	// Subsequent ticks should increment
	assert.Equal(t, int64(2), clock.Tick())
	assert.Equal(t, int64(3), clock.Tick())
}

func TestLamportClock_Witness(t *testing.T) {
	clock := NewLamportClock("device-1", 0)

	// Set initial value
	clock.Set(5)

	// Witness a lower value - should just increment
	val := clock.Witness(3)
	assert.Equal(t, int64(6), val)
	assert.Equal(t, int64(6), clock.Current())

	// Witness a higher value - should take max + 1
	val = clock.Witness(10)
	assert.Equal(t, int64(11), val)
	assert.Equal(t, int64(11), clock.Current())
}

func TestLamportClock_Current(t *testing.T) {
	clock := NewLamportClock("device-1", 100)

	// Current should not change the value
	assert.Equal(t, int64(100), clock.Current())
	assert.Equal(t, int64(100), clock.Current())

	// After tick, current should reflect new value
	clock.Tick()
	assert.Equal(t, int64(101), clock.Current())
}

func TestLamportClock_ConcurrentAccess(t *testing.T) {
	clock := NewLamportClock("device-1", 0)

	// Concurrent ticks
	done := make(chan int64, 100)
	for i := 0; i < 100; i++ {
		go func() {
			done <- clock.Tick()
		}()
	}

	values := make(map[int64]bool)
	for i := 0; i < 100; i++ {
		values[<-done] = true
	}

	// All 100 values should be unique
	assert.Len(t, values, 100)

	// Current should be 100
	assert.Equal(t, int64(100), clock.Current())
}

func TestMergeEntry_RemoteWinsWithHigherClock(t *testing.T) {
	local := models.PasswordEntry{
		ID:           "entry-1",
		LogicalClock: 5,
		UpdatedAt:    time.Now(),
		OriginDevice: "device-a",
		Site:         "old-site",
	}

	remote := models.PasswordEntry{
		ID:           "entry-1",
		LogicalClock: 10,
		UpdatedAt:    time.Now(),
		OriginDevice: "device-b",
		Site:         "new-site",
	}

	result := MergeEntry(local, remote)
	assert.Equal(t, "new-site", result.Site)
	assert.Equal(t, int64(10), result.LogicalClock)
}

func TestMergeEntry_LocalWinsWithHigherClock(t *testing.T) {
	local := models.PasswordEntry{
		ID:           "entry-1",
		LogicalClock: 10,
		UpdatedAt:    time.Now(),
		OriginDevice: "device-a",
		Site:         "local-site",
	}

	remote := models.PasswordEntry{
		ID:           "entry-1",
		LogicalClock: 5,
		UpdatedAt:    time.Now(),
		OriginDevice: "device-b",
		Site:         "remote-site",
	}

	result := MergeEntry(local, remote)
	assert.Equal(t, "local-site", result.Site)
	assert.Equal(t, int64(10), result.LogicalClock)
}

func TestMergeEntry_TieBreakerTimestamp(t *testing.T) {
	now := time.Now()

	local := models.PasswordEntry{
		ID:           "entry-1",
		LogicalClock: 5,
		UpdatedAt:    now.Add(-1 * time.Hour), // Older
		OriginDevice: "device-a",
		Site:         "local-site",
	}

	remote := models.PasswordEntry{
		ID:           "entry-1",
		LogicalClock: 5,
		UpdatedAt:    now, // Newer
		OriginDevice: "device-b",
		Site:         "remote-site",
	}

	result := MergeEntry(local, remote)
	assert.Equal(t, "remote-site", result.Site)
}

func TestMergeEntry_TieBreakerOriginDevice(t *testing.T) {
	now := time.Now()

	local := models.PasswordEntry{
		ID:           "entry-1",
		LogicalClock: 5,
		UpdatedAt:    now,
		OriginDevice: "device-b", // Lexicographically higher
		Site:         "local-site",
	}

	remote := models.PasswordEntry{
		ID:           "entry-1",
		LogicalClock: 5,
		UpdatedAt:    now,
		OriginDevice: "device-a", // Lexicographically lower
		Site:         "remote-site",
	}

	result := MergeEntry(local, remote)
	assert.Equal(t, "local-site", result.Site)
}

func TestMergeEntries(t *testing.T) {
	now := time.Now()

	local := []models.PasswordEntry{
		{ID: "entry-1", LogicalClock: 5, UpdatedAt: now, Site: "local-site-1"},
		{ID: "entry-2", LogicalClock: 3, UpdatedAt: now, Site: "local-site-2"},
	}

	remote := []models.PasswordEntry{
		{ID: "entry-1", LogicalClock: 10, UpdatedAt: now, Site: "remote-site-1"}, // Higher clock
		{ID: "entry-3", LogicalClock: 1, UpdatedAt: now, Site: "remote-site-3"},  // New entry
	}

	result := MergeEntries(local, remote)

	// Should have 3 entries
	assert.Len(t, result, 3)

	// Find merged entries
	var entry1, entry2, entry3 *models.PasswordEntry
	for i := range result {
		switch result[i].ID {
		case "entry-1":
			entry1 = &result[i]
		case "entry-2":
			entry2 = &result[i]
		case "entry-3":
			entry3 = &result[i]
		}
	}

	// entry-1 should have remote's higher clock
	require.NotNil(t, entry1)
	assert.Equal(t, "remote-site-1", entry1.Site)
	assert.Equal(t, int64(10), entry1.LogicalClock)

	// entry-2 should be unchanged
	require.NotNil(t, entry2)
	assert.Equal(t, "local-site-2", entry2.Site)

	// entry-3 should be added
	require.NotNil(t, entry3)
	assert.Equal(t, "remote-site-3", entry3.Site)
}

func TestClockManager_GetClock(t *testing.T) {
	storage := &mockClockStorage{}
	cm := NewClockManager(storage)

	// Get new clock
	clock, err := cm.GetClock("device-1")
	require.NoError(t, err)
	assert.NotNil(t, clock)
	assert.Equal(t, int64(0), clock.Current())

	// Tick it a few times
	clock.Tick()
	clock.Tick()

	// Get same clock again - should return same instance
	clock2, err := cm.GetClock("device-1")
	require.NoError(t, err)
	assert.Equal(t, clock, clock2)
	assert.Equal(t, int64(2), clock2.Current())
}

func TestClockManager_SaveClock(t *testing.T) {
	storage := &mockClockStorage{
		data: make(map[string]int64),
	}
	cm := NewClockManager(storage)

	// Get and tick clock
	clock, _ := cm.GetClock("device-1")
	clock.Tick()
	clock.Tick()

	// Save
	err := cm.SaveClock("device-1")
	require.NoError(t, err)

	// Verify saved
	val, exists := storage.data["device-1"]
	assert.True(t, exists)
	assert.Equal(t, int64(2), val)
}

// Mock implementation of ClockStorage for testing
type mockClockStorage struct {
	data map[string]int64
}

func (m *mockClockStorage) GetDeviceClock(deviceID string) (int64, error) {
	if m.data == nil {
		return 0, nil
	}
	val, exists := m.data[deviceID]
	if !exists {
		return 0, nil
	}
	return val, nil
}

func (m *mockClockStorage) UpsertDeviceClock(deviceID string, clock int64) error {
	if m.data == nil {
		m.data = make(map[string]int64)
	}
	m.data[deviceID] = clock
	return nil
}
