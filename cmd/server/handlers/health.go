package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/bok1c4/pwman/internal/api"
	"github.com/bok1c4/pwman/internal/crypto"
	"github.com/bok1c4/pwman/internal/state"
)

type HealthHandlers struct {
	state *state.ServerState
}

func NewHealthHandlers(s *state.ServerState) *HealthHandlers {
	return &HealthHandlers{state: s}
}

func (h *HealthHandlers) Health(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
		"version":   "1.0.0",
		"uptime":    time.Since(h.state.GetStartTime()).Seconds(),
		"checks": map[string]bool{
			"vault_unlocked": h.state.IsUnlocked(),
		},
	}

	api.Success(w, health)
}

func (h *HealthHandlers) Metrics(w http.ResponseWriter, r *http.Request) {
	metrics := map[string]interface{}{
		"uptime_seconds":  time.Since(h.state.GetStartTime()).Seconds(),
		"entries_count":   0,
		"devices_count":   0,
		"p2p_connections": 0,
	}

	vault, ok := h.state.GetVault()
	if ok && vault != nil && vault.Storage != nil {
		entries, _ := vault.Storage.ListEntries()
		devices, _ := vault.Storage.ListDevices()
		metrics["entries_count"] = len(entries)
		metrics["devices_count"] = len(devices)
	}

	api.Success(w, metrics)
}

func (h *HealthHandlers) GeneratePassword(w http.ResponseWriter, r *http.Request) {
	type GenRequest struct {
		Length int `json:"length"`
	}

	var req GenRequest
	json.NewDecoder(r.Body).Decode(&req)

	if req.Length < 4 {
		req.Length = 16
	}

	password, err := crypto.GenerateStrongPassword(req.Length)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "GEN_FAILED", err.Error())
		return
	}

	api.Success(w, map[string]string{"password": password})
}

func (h *HealthHandlers) Ping(w http.ResponseWriter, r *http.Request) {
	api.Success(w, "pong")
}
