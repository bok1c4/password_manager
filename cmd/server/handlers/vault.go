package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/bok1c4/pwman/internal/api"
	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/crypto"
	"github.com/bok1c4/pwman/internal/state"
)

type VaultHandlers struct {
	state *state.ServerState
}

func NewVaultHandlers(s *state.ServerState) *VaultHandlers {
	return &VaultHandlers{state: s}
}

type VaultInfo struct {
	Name        string `json:"name"`
	Active      bool   `json:"active"`
	Initialized bool   `json:"initialized"`
}

func (h *VaultHandlers) List(w http.ResponseWriter, r *http.Request) {
	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		log.Printf("[VaultList] Error loading config: %v", err)
		api.Success(w, []VaultInfo{})
		return
	}

	activeVault, _ := config.GetActiveVault()
	result := make([]VaultInfo, 0, len(globalCfg.Vaults))

	for _, vaultName := range globalCfg.Vaults {
		vaultConfig, _ := config.LoadVaultConfig(vaultName)
		initialized := vaultConfig != nil && vaultConfig.DeviceID != ""

		result = append(result, VaultInfo{
			Name:        vaultName,
			Active:      vaultName == activeVault,
			Initialized: initialized,
		})
	}

	api.Success(w, result)
}

func (h *VaultHandlers) Use(w http.ResponseWriter, r *http.Request) {
	type UseRequest struct {
		Name string `json:"name"`
	}

	var req UseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	if req.Name == "" {
		api.BadRequest(w, "vault name required")
		return
	}

	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "CONFIG_ERROR", err.Error())
		return
	}

	found := false
	for _, v := range globalCfg.Vaults {
		if v == req.Name {
			found = true
			break
		}
	}

	if !found {
		api.NotFound(w, "vault not found")
		return
	}

	if err := config.SetActiveVault(req.Name); err != nil {
		api.Error(w, http.StatusInternalServerError, "CONFIG_ERROR", err.Error())
		return
	}

	h.state.ClearVault()

	api.Success(w, map[string]string{"message": "vault switched"})
}

func (h *VaultHandlers) Create(w http.ResponseWriter, r *http.Request) {
	type CreateRequest struct {
		Name string `json:"name"`
	}

	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	if req.Name == "" {
		api.BadRequest(w, "vault name required")
		return
	}

	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "CONFIG_ERROR", err.Error())
		return
	}

	for _, v := range globalCfg.Vaults {
		if v == req.Name {
			api.BadRequest(w, "vault already exists")
			return
		}
	}

	vaultPath := config.VaultPath(req.Name)
	if err := os.MkdirAll(vaultPath, 0700); err != nil {
		api.Error(w, http.StatusInternalServerError, "FS_ERROR", err.Error())
		return
	}

	globalCfg.AddVault(req.Name)
	if err := globalCfg.Save(); err != nil {
		api.Error(w, http.StatusInternalServerError, "CONFIG_ERROR", err.Error())
		return
	}

	api.Success(w, map[string]string{"message": "vault created"})
}

func (h *VaultHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	type DeleteRequest struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}

	var req DeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	if req.Name == "" {
		api.BadRequest(w, "vault name required")
		return
	}

	if req.Password == "" {
		api.BadRequest(w, "password required to delete vault")
		return
	}

	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "CONFIG_ERROR", err.Error())
		return
	}

	found := false
	for _, v := range globalCfg.Vaults {
		if v == req.Name {
			found = true
			break
		}
	}

	if !found {
		api.NotFound(w, "vault not found")
		return
	}

	// Verify password before allowing deletion
	vaultConfig, err := config.LoadVaultConfig(req.Name)
	if err != nil || vaultConfig == nil || vaultConfig.DeviceID == "" {
		api.BadRequest(w, "vault not initialized")
		return
	}

	privateKeyPath := config.PrivateKeyPathForVault(req.Name)
	_, err = crypto.LoadAndDecryptPrivateKey(req.Password, privateKeyPath)
	if err != nil {
		api.Unauthorized(w, "incorrect password")
		return
	}

	vaultPath := config.VaultPath(req.Name)
	if err := os.RemoveAll(vaultPath); err != nil {
		api.Error(w, http.StatusInternalServerError, "FS_ERROR", err.Error())
		return
	}

	globalCfg.RemoveVault(req.Name)
	if err := globalCfg.Save(); err != nil {
		api.Error(w, http.StatusInternalServerError, "CONFIG_ERROR", err.Error())
		return
	}

	if activeVault, _ := config.GetActiveVault(); activeVault == req.Name {
		config.SetActiveVault("")
		h.state.ClearVault()
	}

	api.Success(w, map[string]string{"message": "vault deleted"})
}

func (h *VaultHandlers) SyncStatus(w http.ResponseWriter, r *http.Request) {
	api.Success(w, map[string]interface{}{
		"mode":    "p2p",
		"running": false,
	})
}
