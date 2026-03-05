package handlers

import (
	"log"
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

type DeviceResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Trusted     bool   `json:"trusted"`
	CreatedAt   string `json:"created_at"`
	Fingerprint string `json:"fingerprint"`
}

func (h *DeviceHandlers) List(w http.ResponseWriter, r *http.Request) {
	storage, ok := h.state.GetVaultStorage()
	if !ok {
		api.Success(w, []DeviceResponse{})
		return
	}

	devices, err := storage.ListDevices()
	if err != nil {
		log.Printf("[handleGetDevices] Error listing devices: %v", err)
		api.Success(w, []DeviceResponse{})
		return
	}

	log.Printf("[handleGetDevices] Found %d devices", len(devices))

	result := make([]DeviceResponse, 0, len(devices))
	for _, d := range devices {
		result = append(result, DeviceResponse{
			ID:          d.ID,
			Name:        d.Name,
			Trusted:     d.Trusted,
			CreatedAt:   d.CreatedAt.Format("2006-01-02T15:04:05Z"),
			Fingerprint: d.Fingerprint,
		})
	}

	api.Success(w, result)
}
