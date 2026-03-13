package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"time"

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
	return &AuthHandlers{
		state:       s,
		authManager: am,
	}
}

func (h *AuthHandlers) Init(w http.ResponseWriter, r *http.Request) {
	type InitRequest struct {
		Name     string `json:"name"`
		Password string `json:"password"`
		Vault    string `json:"vault"`
	}

	var req InitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	deviceName, err := api.ValidateDeviceName(req.Name)
	if err != nil {
		api.BadRequest(w, err.Error())
		return
	}
	req.Name = deviceName

	if req.Password == "" || len(req.Password) < 8 {
		api.BadRequest(w, "password must be at least 8 characters")
		return
	}

	if err := api.ValidatePassword(req.Password); err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	vaultName := req.Vault
	if vaultName == "" {
		globalCfg, _ := config.LoadGlobalConfig()
		if len(globalCfg.Vaults) > 0 {
			vaultName = globalCfg.Vaults[0]
		} else {
			vaultName = "default"
		}
	}

	cfg, _ := config.LoadVaultConfig(vaultName)
	if cfg != nil && cfg.DeviceID != "" {
		api.BadRequest(w, "vault already initialized")
		return
	}

	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		api.InternalError(w, "failed to load config: "+err.Error())
		return
	}

	found := false
	for _, v := range globalCfg.Vaults {
		if v == vaultName {
			found = true
			break
		}
	}
	if !found {
		globalCfg.AddVault(vaultName)
		globalCfg.ActiveVault = vaultName
		if err := globalCfg.Save(); err != nil {
			api.InternalError(w, "failed to save config: "+err.Error())
			return
		}
	}

	if err := config.SetActiveVault(vaultName); err != nil {
		api.InternalError(w, "failed to set active vault: "+err.Error())
		return
	}

	vaultPath := config.VaultPath(vaultName)
	if err := os.MkdirAll(vaultPath, 0700); err != nil {
		api.InternalError(w, "failed to create vault: "+err.Error())
		return
	}

	keyPair, err := crypto.GenerateRSAKeyPair(4096)
	if err != nil {
		api.InternalError(w, "failed to generate keys: "+err.Error())
		return
	}

	salt, err := crypto.EncryptPrivateKeyAndSave(keyPair.PrivateKey, req.Password, config.PrivateKeyPathForVault(vaultName))
	if err != nil {
		api.InternalError(w, "failed to encrypt private key: "+err.Error())
		return
	}

	// Derive master key for TOTP generation
	masterKey, err := crypto.DeriveKey(req.Password, salt)
	if err != nil {
		api.InternalError(w, "failed to derive master key: "+err.Error())
		return
	}

	if err := crypto.SavePublicKey(keyPair.PublicKey, config.PublicKeyPathForVault(vaultName)); err != nil {
		api.InternalError(w, "failed to save public key: "+err.Error())
		return
	}

	deviceID := uuid.New().String()
	cfg = &config.Config{
		DeviceID:   deviceID,
		DeviceName: req.Name,
		Salt:       base64.StdEncoding.EncodeToString(salt),
	}

	cfgBytes, _ := json.Marshal(cfg)
	os.WriteFile(config.VaultConfigPath(vaultName), cfgBytes, 0600)

	db, err := storage.NewSQLite(config.DatabasePathForVault(vaultName))
	if err != nil {
		api.InternalError(w, "failed to create database: "+err.Error())
		return
	}

	device := models.Device{
		ID:          deviceID,
		Name:        req.Name,
		PublicKey:   "",
		Fingerprint: crypto.GetFingerprint(keyPair.PublicKey),
		Trusted:     true,
		CreatedAt:   time.Now(),
	}

	pubKeyPath := config.PublicKeyPathForVault(vaultName)
	if pubKeyBytes, err := os.ReadFile(pubKeyPath); err == nil {
		device.PublicKey = string(pubKeyBytes)
	}

	if err := db.UpsertDevice(&device); err != nil {
		api.InternalError(w, "failed to save device: "+err.Error())
		return
	}

	vault := &state.Vault{
		PrivateKey: keyPair,
		Storage:    db,
		Config:     cfg,
		VaultName:  vaultName,
		MasterKey:  masterKey,
	}

	h.state.SetVault(vault)
	h.authManager.SetVaultUnlocked(true)

	api.Success(w, map[string]string{"device_id": deviceID})
}

func (h *AuthHandlers) Unlock(w http.ResponseWriter, r *http.Request) {
	if h.state.IsUnlocked() {
		token := h.authManager.GenerateToken()
		api.Success(w, map[string]string{"token": token})
		return
	}

	type UnlockRequest struct {
		Password string `json:"password"`
	}

	var req UnlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	if req.Password == "" {
		api.BadRequest(w, "password required")
		return
	}

	activeVault, err := config.GetActiveVault()
	if err != nil || activeVault == "" {
		api.BadRequest(w, "no active vault")
		return
	}

	vaultConfig, err := config.LoadVaultConfig(activeVault)
	if err != nil || vaultConfig == nil || vaultConfig.DeviceID == "" {
		api.BadRequest(w, "vault not initialized")
		return
	}

	privateKeyPath := config.PrivateKeyPathForVault(activeVault)
	privateKey, err := crypto.LoadAndDecryptPrivateKey(req.Password, privateKeyPath)
	if err != nil {
		api.Unauthorized(w, "wrong password")
		return
	}

	// Load salt and derive master key for TOTP generation
	saltPath := privateKeyPath + ".salt"
	salt, err := os.ReadFile(saltPath)
	if err != nil {
		api.InternalError(w, "failed to read salt")
		return
	}
	masterKey, err := crypto.DeriveKey(req.Password, salt)
	if err != nil {
		api.InternalError(w, "failed to derive master key")
		return
	}

	db, err := storage.NewSQLite(config.DatabasePathForVault(activeVault))
	if err != nil {
		api.InternalError(w, "failed to open database")
		return
	}

	device, _ := db.GetDevice(vaultConfig.DeviceID)
	if device == nil {
		pubKeyPath := config.PublicKeyPathForVault(activeVault)
		pubKeyBytes, _ := os.ReadFile(pubKeyPath)
		pubKey, _ := crypto.LoadPublicKey(pubKeyPath)
		device = &models.Device{
			ID:          vaultConfig.DeviceID,
			Name:        vaultConfig.DeviceName,
			PublicKey:   string(pubKeyBytes),
			Fingerprint: crypto.GetFingerprint(pubKey),
			Trusted:     true,
			CreatedAt:   time.Now(),
		}
		db.UpsertDevice(device)
	} else if device.PublicKey == "" {
		pubKeyPath := config.PublicKeyPathForVault(activeVault)
		pubKeyBytes, _ := os.ReadFile(pubKeyPath)
		device.PublicKey = string(pubKeyBytes)
		db.UpsertDevice(device)
	}

	vault := &state.Vault{
		PrivateKey: &crypto.KeyPair{PrivateKey: privateKey, PublicKey: &privateKey.PublicKey},
		Storage:    db,
		Config:     vaultConfig,
		VaultName:  activeVault,
		MasterKey:  masterKey,
	}

	h.state.SetVault(vault)

	token := h.authManager.GenerateToken()
	h.authManager.SetVaultUnlocked(true)

	api.Success(w, map[string]string{"token": token})
}

func (h *AuthHandlers) Lock(w http.ResponseWriter, r *http.Request) {
	// Extract and invalidate the token
	token := r.Header.Get("Authorization")
	if token != "" {
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}
		h.authManager.InvalidateToken(token)
	}

	h.state.ClearVault()
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
