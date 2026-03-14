package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/bok1c4/pwman/internal/api"
	"github.com/bok1c4/pwman/internal/state"
	"github.com/bok1c4/pwman/internal/storage"
	"github.com/bok1c4/pwman/internal/sync"
	"github.com/bok1c4/pwman/pkg/models"
)

type SyncHandlers struct {
	state *state.ServerState
}

func NewSyncHandlers(s *state.ServerState) *SyncHandlers {
	return &SyncHandlers{state: s}
}

// GET /api/sync/status - Get current sync status
func (h *SyncHandlers) Status(w http.ResponseWriter, r *http.Request) {
	vault, ok := h.state.GetVault()
	if !ok {
		api.Error(w, http.StatusBadRequest, "VAULT_LOCKED", "vault not unlocked")
		return
	}

	// Get clock for this device
	var logicalClock int64
	if h.state.ClockManager != nil {
		clock, err := h.state.ClockManager.GetClock(vault.Config.DeviceID)
		if err == nil && clock != nil {
			logicalClock = clock.Current()
		}
	}

	// Count entries that need sync
	storage, ok := h.state.GetVaultStorage()
	if !ok {
		api.Error(w, http.StatusBadRequest, "VAULT_LOCKED", "vault not unlocked")
		return
	}

	entries, err := storage.ListEntries()
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Calculate pending changes (entries with logical_clock > 0)
	pendingCount := 0
	for _, entry := range entries {
		if entry.LogicalClock > 0 {
			pendingCount++
		}
	}

	api.Success(w, map[string]interface{}{
		"device_id":       vault.Config.DeviceID,
		"logical_clock":   logicalClock,
		"pending_changes": pendingCount,
		"last_sync":       getLastSyncTime(storage),
	})
}

// POST /api/sync/pull - Pull changes from peer
func (h *SyncHandlers) Pull(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SinceClock int64  `json:"since_clock"`
		DeviceID   string `json:"device_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	storage, ok := h.state.GetVaultStorage()
	if !ok {
		api.Error(w, http.StatusBadRequest, "VAULT_LOCKED", "vault not unlocked")
		return
	}

	// Get entries changed since requested clock
	entries, err := storage.ListEntries()
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Filter entries with logical_clock > since_clock
	var changedEntries []models.PasswordEntry
	for _, entry := range entries {
		if entry.LogicalClock > req.SinceClock {
			changedEntries = append(changedEntries, entry)
		}
	}

	// Find max clock
	maxClock := int64(0)
	for _, entry := range entries {
		if entry.LogicalClock > maxClock {
			maxClock = entry.LogicalClock
		}
	}

	api.Success(w, map[string]interface{}{
		"entries":   changedEntries,
		"max_clock": maxClock,
	})
}

// POST /api/sync/push - Push changes to peer
func (h *SyncHandlers) Push(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Entries  []models.PasswordEntry `json:"entries"`
		DeviceID string                 `json:"device_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	storage, ok := h.state.GetVaultStorage()
	if !ok {
		api.Error(w, http.StatusBadRequest, "VAULT_LOCKED", "vault not unlocked")
		return
	}

	merged := 0
	created := 0

	// Merge incoming entries
	for _, entry := range req.Entries {
		// Witness remote clock
		if h.state.ClockManager != nil {
			clock, err := h.state.ClockManager.GetClock(entry.OriginDevice)
			if err == nil && clock != nil {
				clock.Witness(entry.LogicalClock)
			}
		}

		// Check if entry exists
		existing, err := storage.GetEntry(entry.ID)
		if err == nil && existing != nil {
			// Merge using Lamport clocks
			mergedEntry := sync.MergeEntry(*existing, entry)
			if err := storage.UpdateEntry(&mergedEntry); err != nil {
				log.Printf("Failed to update entry %s: %v", entry.ID, err)
				continue
			}
			merged++
		} else {
			// Create new entry
			if err := storage.CreateEntry(&entry); err != nil {
				log.Printf("Failed to create entry %s: %v", entry.ID, err)
				continue
			}
			created++
		}
	}

	api.Success(w, map[string]interface{}{
		"success": true,
		"merged":  merged,
		"created": created,
	})
}

func getLastSyncTime(s *storage.SQLite) int64 {
	// This would ideally track last sync time
	// For now return 0
	return 0
}
