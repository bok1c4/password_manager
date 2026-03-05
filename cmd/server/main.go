package main

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/crypto"
	"github.com/bok1c4/pwman/internal/p2p"
	"github.com/bok1c4/pwman/internal/storage"
	"github.com/bok1c4/pwman/pkg/models"
	"github.com/google/uuid"
)

var (
	serverPort = os.Getenv("PWMAN_PORT")
)

func init() {
	if serverPort == "" {
		serverPort = "18475"
	}
}

var (
	vault             *Vault
	vaultLock         sync.Mutex
	p2pManager        *p2p.P2PManager
	p2pLock           sync.Mutex
	pendingApprovals  = make(map[string]PendingApproval)
	approvalsLock     sync.Mutex
	pairingCodes      = make(map[string]PairingCode)
	pairingLock       sync.Mutex
	pairingResponseCh chan p2p.PairingResponsePayload
	pairingRequests   = make(map[string]PairingRequest)
	pairingState      *PairingState
	pairingStateLock  sync.Mutex
)

type PairingRequest struct {
	Code       string
	DeviceID   string
	DeviceName string
	ResponseCh chan p2p.PairingResponsePayload
	CreatedAt  time.Time
}

type PendingApproval struct {
	DeviceID    string
	DeviceName  string
	PublicKey   string
	Fingerprint string
	Status      string
	ConnectedAt time.Time
	PairingCode string
}

type PairingCode struct {
	Code        string
	VaultID     string
	VaultName   string // NEW: name of vault
	DeviceID    string
	DeviceName  string
	PublicKey   string
	Fingerprint string
	ExpiresAt   time.Time
	Used        bool
}

type PairingState struct {
	Phase      string
	Code       string
	PeerID     string
	DeviceID   string
	DeviceName string
	VaultName  string
	CreatedAt  time.Time
}

const (
	PairingPhaseIdle       = "idle"
	PairingPhaseGenerating = "generating"
	PairingPhaseWaiting    = "waiting"
	PairingPhaseConnected  = "connected"
	PairingPhaseSyncing    = "syncing"
	PairingPhaseComplete   = "complete"
	PairingPhaseFailed     = "failed"
)

type Vault struct {
	privateKey *crypto.KeyPair
	storage    *storage.SQLite
	cfg        *config.Config
	vaultName  string
}

type Entry struct {
	ID                string            `json:"id"`
	Site              string            `json:"site"`
	Username          string            `json:"username"`
	EncryptedPassword string            `json:"encrypted_password"`
	EncryptedAESKeys  map[string]string `json:"encrypted_aes_keys"`
	Notes             string            `json:"notes"`
	CreatedAt         string            `json:"created_at"`
	UpdatedAt         string            `json:"updated_at"`
	UpdatedBy         string            `json:"updated_by"`
}

func getVaultPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "."
	}
	return home + "/.pwman"
}

func getConfigPath() string     { return getVaultPath() + "/config.json" }
func getPrivateKeyPath() string { return getVaultPath() + "/private.key" }
func getPublicKeyPath() string  { return getVaultPath() + "/public.key" }
func getDatabasePath() string   { return getVaultPath() + "/vault.db" }

type Response struct {
	Success bool        `json:"success"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func jsonResponse(w http.ResponseWriter, v Response) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	json.NewEncoder(w).Encode(v)
}

func corsHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

func handleInit(w http.ResponseWriter, r *http.Request) {
	vaultLock.Lock()
	defer vaultLock.Unlock()

	type InitRequest struct {
		Name     string `json:"name"`
		Password string `json:"password"`
		Vault    string `json:"vault"`
	}
	var req InitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	if req.Password == "" || len(req.Password) < 8 {
		jsonResponse(w, Response{Success: false, Error: "password must be at least 8 characters"})
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
		jsonResponse(w, Response{Success: false, Error: "vault already initialized"})
		return
	}

	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		jsonResponse(w, Response{Success: false, Error: "failed to load config: " + err.Error()})
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
			jsonResponse(w, Response{Success: false, Error: "failed to save config: " + err.Error()})
			return
		}
	}

	if err := config.SetActiveVault(vaultName); err != nil {
		jsonResponse(w, Response{Success: false, Error: "failed to set active vault: " + err.Error()})
		return
	}

	vaultPath := config.VaultPath(vaultName)
	if err := os.MkdirAll(vaultPath, 0700); err != nil {
		jsonResponse(w, Response{Success: false, Error: "failed to create vault: " + err.Error()})
		return
	}

	keyPair, err := crypto.GenerateRSAKeyPair(2048)
	if err != nil {
		jsonResponse(w, Response{Success: false, Error: "failed to generate keys: " + err.Error()})
		return
	}

	salt, err := crypto.EncryptPrivateKeyAndSave(keyPair.PrivateKey, req.Password, config.PrivateKeyPathForVault(vaultName))
	if err != nil {
		jsonResponse(w, Response{Success: false, Error: "failed to encrypt private key: " + err.Error()})
		return
	}

	if err := crypto.SavePublicKey(keyPair.PublicKey, config.PublicKeyPathForVault(vaultName)); err != nil {
		jsonResponse(w, Response{Success: false, Error: "failed to save public key: " + err.Error()})
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
		jsonResponse(w, Response{Success: false, Error: "failed to create database: " + err.Error()})
		return
	}

	device := models.Device{
		ID:          deviceID,
		Name:        req.Name,
		PublicKey:   "", // Will be loaded from file below
		Fingerprint: crypto.GetFingerprint(keyPair.PublicKey),
		Trusted:     true,
		CreatedAt:   time.Now(),
	}

	// Read and store the actual public key content
	pubKeyPath := config.PublicKeyPathForVault(vaultName)
	if pubKeyBytes, err := os.ReadFile(pubKeyPath); err == nil {
		device.PublicKey = string(pubKeyBytes)
	}

	if err := db.UpsertDevice(&device); err != nil {
		jsonResponse(w, Response{Success: false, Error: "failed to save device: " + err.Error()})
		return
	}

	vault = &Vault{
		privateKey: keyPair,
		storage:    db,
		cfg:        cfg,
		vaultName:  vaultName,
	}

	jsonResponse(w, Response{Success: true, Data: map[string]string{"device_id": deviceID}})
}

func handleUnlock(w http.ResponseWriter, r *http.Request) {
	vaultLock.Lock()
	defer vaultLock.Unlock()

	if vault != nil && vault.privateKey != nil {
		jsonResponse(w, Response{Success: true})
		return
	}

	type UnlockRequest struct {
		Password string `json:"password"`
	}
	var req UnlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	if req.Password == "" {
		jsonResponse(w, Response{Success: false, Error: "password required"})
		return
	}

	activeVault, err := config.GetActiveVault()
	if err != nil || activeVault == "" {
		jsonResponse(w, Response{Success: false, Error: "no active vault"})
		return
	}

	vaultConfig, err := config.LoadVaultConfig(activeVault)
	if err != nil || vaultConfig == nil || vaultConfig.DeviceID == "" {
		jsonResponse(w, Response{Success: false, Error: "vault not initialized"})
		return
	}

	privateKeyPath := config.PrivateKeyPathForVault(activeVault)
	privateKey, err := crypto.LoadAndDecryptPrivateKey(req.Password, privateKeyPath)
	if err != nil {
		jsonResponse(w, Response{Success: false, Error: "wrong password"})
		return
	}

	db, err := storage.NewSQLite(config.DatabasePathForVault(activeVault))
	if err != nil {
		jsonResponse(w, Response{Success: false, Error: "failed to open database"})
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

	vault = &Vault{
		privateKey: &crypto.KeyPair{PrivateKey: privateKey, PublicKey: &privateKey.PublicKey},
		storage:    db,
		cfg:        vaultConfig,
		vaultName:  activeVault,
	}

	log.Printf("[handleUnlock] Vault unlocked, public key available: %v", vault.privateKey.PublicKey != nil)

	jsonResponse(w, Response{Success: true})
}

func handleLock(w http.ResponseWriter, r *http.Request) {
	vaultLock.Lock()
	defer vaultLock.Unlock()
	vault = nil
	jsonResponse(w, Response{Success: true})
}

func handleIsUnlocked(w http.ResponseWriter, r *http.Request) {
	vaultLock.Lock()
	defer vaultLock.Unlock()
	jsonResponse(w, Response{Success: true, Data: map[string]bool{"unlocked": vault != nil && vault.privateKey != nil}})
}

func handleIsInitialized(w http.ResponseWriter, r *http.Request) {
	activeVault, err := config.GetActiveVault()
	if err != nil || activeVault == "" {
		jsonResponse(w, Response{Success: true, Data: map[string]bool{"initialized": false}})
		return
	}

	vaultConfig, err := config.LoadVaultConfig(activeVault)
	initialized := vaultConfig != nil && vaultConfig.DeviceID != ""
	jsonResponse(w, Response{Success: true, Data: map[string]bool{"initialized": initialized}})
}

func handleGetEntries(w http.ResponseWriter, r *http.Request) {
	vaultLock.Lock()
	defer vaultLock.Unlock()

	if vault == nil || vault.storage == nil {
		jsonResponse(w, Response{Success: true, Data: []Entry{}})
		return
	}

	entries, err := vault.storage.ListEntries()
	if err != nil {
		jsonResponse(w, Response{Success: true, Data: []Entry{}})
		return
	}

	result := make([]Entry, len(entries))
	for i, e := range entries {
		result[i] = Entry{
			ID:                e.ID,
			Site:              e.Site,
			Username:          e.Username,
			EncryptedPassword: e.EncryptedPassword,
			EncryptedAESKeys:  e.EncryptedAESKeys,
			Notes:             e.Notes,
			CreatedAt:         e.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:         e.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedBy:         e.UpdatedBy,
		}
	}

	jsonResponse(w, Response{Success: true, Data: result})
}

func handleAddEntry(w http.ResponseWriter, r *http.Request) {
	vaultLock.Lock()
	defer vaultLock.Unlock()

	if vault == nil || vault.storage == nil {
		jsonResponse(w, Response{Success: false, Error: "vault not unlocked"})
		return
	}

	type AddRequest struct {
		Site     string `json:"site"`
		Username string `json:"username"`
		Password string `json:"password"`
		Notes    string `json:"notes"`
	}

	var req AddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	devices, _ := vault.storage.ListDevices()
	log.Printf("[AddEntry] Found %d devices in storage", len(devices))
	var trustedDevices []models.Device
	for _, d := range devices {
		log.Printf("[AddEntry] Device: id=%s, name=%s, trusted=%v, pubkey_len=%d",
			d.ID, d.Name, d.Trusted, len(d.PublicKey))
		if d.Trusted {
			trustedDevices = append(trustedDevices, d)
		}
	}

	if len(trustedDevices) == 0 {
		log.Printf("[AddEntry] No trusted devices found, using fallback device")
		activeVault, _ := config.GetActiveVault()
		trustedDevices = append(trustedDevices, models.Device{
			ID:          vault.cfg.DeviceID,
			Fingerprint: crypto.GetFingerprint(vault.privateKey.PublicKey),
			PublicKey:   config.PublicKeyPathForVault(activeVault),
		})
	}

	log.Printf("[AddEntry] Encrypting password for %d trusted devices", len(trustedDevices))
	getPublicKey := func(fingerprint string) (*rsa.PublicKey, error) {
		for _, d := range trustedDevices {
			if d.Fingerprint == fingerprint {
				log.Printf("[AddEntry] Loading public key for %s, pubkey starts with: %.30s",
					fingerprint[:min(20, len(fingerprint))], d.PublicKey)
				if strings.HasPrefix(d.PublicKey, "-----BEGIN") {
					return crypto.ParsePublicKey(d.PublicKey)
				}
				return crypto.LoadPublicKey(d.PublicKey)
			}
		}
		return nil, fmt.Errorf("device not found")
	}

	encrypted, err := crypto.HybridEncrypt(req.Password, trustedDevices, getPublicKey)
	if err != nil {
		log.Printf("[AddEntry] Encryption failed: %v", err)
		jsonResponse(w, Response{Success: false, Error: "failed to encrypt: " + err.Error()})
		return
	}

	log.Printf("[AddEntry] Encryption successful, encrypted password length: %d, keys count: %d",
		len(encrypted.EncryptedPassword), len(encrypted.EncryptedAESKeys))

	entry := models.PasswordEntry{
		ID:                uuid.New().String(),
		Version:           1,
		Site:              req.Site,
		Username:          req.Username,
		EncryptedPassword: encrypted.EncryptedPassword,
		EncryptedAESKeys:  encrypted.EncryptedAESKeys,
		Notes:             req.Notes,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		UpdatedBy:         vault.cfg.DeviceID,
	}

	if err := vault.storage.CreateEntry(&entry); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	jsonResponse(w, Response{Success: true, Data: map[string]string{"id": entry.ID}})
}

func handleDeleteEntry(w http.ResponseWriter, r *http.Request) {
	vaultLock.Lock()
	defer vaultLock.Unlock()

	if vault == nil || vault.storage == nil {
		jsonResponse(w, Response{Success: false, Error: "vault not unlocked"})
		return
	}

	type DeleteRequest struct {
		ID string `json:"id"`
	}

	var req DeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	err := vault.storage.DeleteEntry(req.ID)
	jsonResponse(w, Response{Success: err == nil, Error: errString(err)})
}

func handleUpdateEntry(w http.ResponseWriter, r *http.Request) {
	vaultLock.Lock()
	defer vaultLock.Unlock()

	if vault == nil || vault.storage == nil {
		jsonResponse(w, Response{Success: false, Error: "vault not unlocked"})
		return
	}

	type UpdateRequest struct {
		ID       string `json:"id"`
		Site     string `json:"site"`
		Username string `json:"username"`
		Password string `json:"password"`
		Notes    string `json:"notes"`
	}

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	existingEntry, err := vault.storage.GetEntry(req.ID)
	if err != nil {
		jsonResponse(w, Response{Success: false, Error: "entry not found"})
		return
	}

	devices, _ := vault.storage.ListDevices()
	var trustedDevices []models.Device
	for _, d := range devices {
		if d.Trusted {
			trustedDevices = append(trustedDevices, d)
		}
	}

	if len(trustedDevices) == 0 {
		activeVault, _ := config.GetActiveVault()
		trustedDevices = append(trustedDevices, models.Device{
			ID:          vault.cfg.DeviceID,
			Fingerprint: crypto.GetFingerprint(vault.privateKey.PublicKey),
			PublicKey:   config.PublicKeyPathForVault(activeVault),
		})
	}

	getPublicKey := func(fingerprint string) (*rsa.PublicKey, error) {
		for _, d := range trustedDevices {
			if d.Fingerprint == fingerprint {
				return crypto.LoadPublicKey(d.PublicKey)
			}
		}
		return nil, fmt.Errorf("device not found")
	}

	encrypted, err := crypto.HybridEncrypt(req.Password, trustedDevices, getPublicKey)
	if err != nil {
		jsonResponse(w, Response{Success: false, Error: "failed to encrypt: " + err.Error()})
		return
	}

	existingEntry.Site = req.Site
	existingEntry.Username = req.Username
	existingEntry.EncryptedPassword = encrypted.EncryptedPassword
	existingEntry.EncryptedAESKeys = encrypted.EncryptedAESKeys
	existingEntry.Notes = req.Notes
	existingEntry.Version++
	existingEntry.UpdatedAt = time.Now()
	existingEntry.UpdatedBy = vault.cfg.DeviceID

	if err := vault.storage.UpdateEntry(existingEntry); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	jsonResponse(w, Response{Success: true})
}

func handleGetPassword(w http.ResponseWriter, r *http.Request) {
	vaultLock.Lock()
	defer vaultLock.Unlock()

	if vault == nil || vault.storage == nil {
		jsonResponse(w, Response{Success: false, Error: "vault not unlocked"})
		return
	}

	// Support both JSON body and query params
	var entryID string

	// First try to get from query params
	if site := r.URL.Query().Get("site"); site != "" {
		entries, _ := vault.storage.ListEntries()
		for _, e := range entries {
			if e.Site == site {
				entryID = e.ID
				break
			}
		}
		if entryID == "" {
			jsonResponse(w, Response{Success: false, Error: "entry not found for site: " + site})
			return
		}
	} else {
		// Try JSON body
		type GetPasswordRequest struct {
			ID string `json:"id"`
		}

		var req GetPasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResponse(w, Response{Success: false, Error: err.Error()})
			return
		}
		entryID = req.ID
	}

	entry, err := vault.storage.GetEntry(entryID)
	if err != nil {
		jsonResponse(w, Response{Success: false, Error: "entry not found"})
		return
	}

	log.Printf("[GetPassword] Trying to decrypt entry %s with vault's private key (fingerprint available: %v)", entry.ID, vault.privateKey != nil)

	// Debug: Log what's in EncryptedAESKeys
	if entry.EncryptedAESKeys != nil {
		log.Printf("[GetPassword] Entry has %d encrypted keys: %v", len(entry.EncryptedAESKeys), func() []string {
			keys := []string{}
			for k := range entry.EncryptedAESKeys {
				keys = append(keys, k[:min(20, len(k))]+"...")
			}
			return keys
		}())
	}

	// Get our fingerprint to check if we have a key
	if vault.privateKey == nil || vault.privateKey.PublicKey == nil {
		log.Printf("[GetPassword] ERROR: vault.privateKey.PublicKey is nil! Cannot decrypt.")
		jsonResponse(w, Response{Success: false, Error: "vault not properly initialized - public key missing"})
		return
	}

	ourFingerprint := crypto.GetFingerprint(vault.privateKey.PublicKey)
	log.Printf("[GetPassword] Our device fingerprint: %s...", ourFingerprint[:min(20, len(ourFingerprint))])

	hasKey := false
	for fp := range entry.EncryptedAESKeys {
		log.Printf("[GetPassword] Checking key for: %s...", fp[:min(30, len(fp))])
		if fp == ourFingerprint {
			hasKey = true
			log.Printf("[GetPassword] MATCH! Our fingerprint matches")
			break
		}
	}
	log.Printf("[GetPassword] Have encrypted key for this device: %v", hasKey)

	// Debug: show what we're trying to decrypt with
	log.Printf("[GetPassword] Private key available: %v", vault.privateKey.PrivateKey != nil)
	log.Printf("[GetPassword] Public key in KeyPair: %v", vault.privateKey.PublicKey != nil)

	password, err := crypto.HybridDecrypt(entry, vault.privateKey.PrivateKey)
	if err != nil {
		log.Printf("[GetPassword] Decryption failed: %v", err)
		jsonResponse(w, Response{Success: false, Error: "failed to decrypt password"})
		return
	}

	jsonResponse(w, Response{Success: true, Data: map[string]string{"password": password}})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func handleGetDevices(w http.ResponseWriter, r *http.Request) {
	vaultLock.Lock()
	defer vaultLock.Unlock()

	if vault == nil || vault.storage == nil {
		jsonResponse(w, Response{Success: true, Data: []struct{}{}})
		return
	}

	devices, _ := vault.storage.ListDevices()
	type DeviceResp struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Fingerprint string `json:"fingerprint"`
		Trusted     bool   `json:"trusted"`
	}

	result := make([]DeviceResp, len(devices))
	for i, d := range devices {
		result[i] = DeviceResp{
			ID:          d.ID,
			Name:        d.Name,
			Fingerprint: d.Fingerprint,
			Trusted:     d.Trusted,
		}
	}

	jsonResponse(w, Response{Success: true, Data: result})
}

func handleGetSyncStatus(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, Response{Success: true, Data: map[string]interface{}{
		"mode":    "p2p",
		"running": false,
	}})
}

func handleGeneratePassword(w http.ResponseWriter, r *http.Request) {
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
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	jsonResponse(w, Response{Success: true, Data: map[string]string{"password": password}})
}

type VaultInfo struct {
	Name        string `json:"name"`
	Active      bool   `json:"active"`
	Initialized bool   `json:"initialized"`
}

func handleVaults(w http.ResponseWriter, r *http.Request) {
	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	vaults := []VaultInfo{}
	for _, v := range globalCfg.Vaults {
		vaultCfg, _ := config.LoadVaultConfig(v)
		initialized := vaultCfg != nil && vaultCfg.DeviceID != ""
		vaults = append(vaults, VaultInfo{
			Name:        v,
			Active:      v == globalCfg.ActiveVault,
			Initialized: initialized,
		})
	}

	jsonResponse(w, Response{Success: true, Data: vaults})
}

func handleVaultUse(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, Response{Success: false, Error: "method not allowed"})
		return
	}

	type UseRequest struct {
		Vault string `json:"vault"`
	}
	var req UseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	vaultLock.Lock()
	defer vaultLock.Unlock()

	// Close current vault storage if open
	if vault != nil && vault.storage != nil {
		vault.storage.Close()
		vault = nil
	}

	if err := config.SetActiveVault(req.Vault); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	jsonResponse(w, Response{Success: true})
}

func handleVaultCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, Response{Success: false, Error: "method not allowed"})
		return
	}

	type CreateRequest struct {
		Name string `json:"name"`
	}
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	for _, v := range globalCfg.Vaults {
		if v == req.Name {
			jsonResponse(w, Response{Success: false, Error: "vault already exists"})
			return
		}
	}

	if err := config.EnsureVaultDirForVault(req.Name); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	globalCfg.AddVault(req.Name)
	globalCfg.ActiveVault = req.Name
	if err := globalCfg.Save(); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	jsonResponse(w, Response{Success: true})
}

func handleVaultDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, Response{Success: false, Error: "method not allowed"})
		return
	}

	type DeleteRequest struct {
		Name          string `json:"name"`
		DeleteDataDir bool   `json:"delete_data_dir,omitempty"`
	}
	var req DeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	if req.Name == "" {
		jsonResponse(w, Response{Success: false, Error: "vault name required"})
		return
	}

	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
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
		jsonResponse(w, Response{Success: false, Error: "vault not found"})
		return
	}

	vaultLock.Lock()
	if vault != nil && vault.vaultName == req.Name {
		vault.storage.Close()
		vault = nil
	}
	vaultLock.Unlock()

	if req.DeleteDataDir {
		vaultPath := config.VaultPath(req.Name)
		if err := os.RemoveAll(vaultPath); err != nil {
			jsonResponse(w, Response{Success: false, Error: "failed to delete vault directory: " + err.Error()})
			return
		}
	}

	globalCfg.RemoveVault(req.Name)
	if err := globalCfg.Save(); err != nil {
		jsonResponse(w, Response{Success: false, Error: "failed to save config: " + err.Error()})
		return
	}

	jsonResponse(w, Response{Success: true})
}

type P2PPeerInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Addr      string `json:"addr"`
	Connected bool   `json:"connected"`
}

type P2PStatusResponse struct {
	Running    bool          `json:"running"`
	PeerID     string        `json:"peer_id"`
	Addresses  []string      `json:"addresses"`
	Connected  []P2PPeerInfo `json:"connected"`
	Discovered []P2PPeerInfo `json:"discovered"`
}

func handleP2PStatus(w http.ResponseWriter, r *http.Request) {
	approvalsLock.Lock()
	log.Printf("[DEBUG] Current pending approvals: %d", len(pendingApprovals))
	for id, p := range pendingApprovals {
		log.Printf("[DEBUG] - %s: %s", id, p.DeviceName)
	}
	approvalsLock.Unlock()

	p2pLock.Lock()
	defer p2pLock.Unlock()

	if p2pManager == nil || !p2pManager.IsRunning() {
		jsonResponse(w, Response{Success: true, Data: P2PStatusResponse{Running: false}})
		return
	}

	response := P2PStatusResponse{
		Running:   true,
		PeerID:    p2pManager.GetPeerID(),
		Addresses: p2pManager.GetListenAddresses(),
	}

	peers := p2pManager.GetConnectedPeers()
	for _, p := range peers {
		response.Connected = append(response.Connected, P2PPeerInfo{
			ID:        p.ID,
			Name:      p.Name,
			Connected: p.Connected,
		})
	}

	allPeers := p2pManager.GetAllPeers()
	for _, p := range allPeers {
		found := false
		for _, c := range response.Connected {
			if c.ID == p.ID {
				found = true
				break
			}
		}
		if !found {
			response.Discovered = append(response.Discovered, P2PPeerInfo{
				ID:   p.ID,
				Name: p.Name,
			})
		}
	}

	jsonResponse(w, Response{Success: true, Data: response})
}

func handleP2PStart(w http.ResponseWriter, r *http.Request) {
	log.Printf("[%s] [P2P] handleP2PStart called", time.Now().Format("15:04:05"))

	p2pLock.Lock()
	defer p2pLock.Unlock()

	if p2pManager != nil && p2pManager.IsRunning() {
		log.Println("[P2P] Already running")
		jsonResponse(w, Response{Success: true, Data: "P2P already running"})
		return
	}

	log.Println("[P2P] Creating new P2P manager")

	deviceName := "Device"
	deviceID := ""

	vaultLock.Lock()
	activeVault := ""
	if vault != nil && vault.cfg != nil {
		activeVault = vault.cfg.DeviceName
		deviceName = vault.cfg.DeviceName
		deviceID = vault.cfg.DeviceID
	}
	vaultLock.Unlock()

	if activeVault == "" {
		cfg, _ := config.GetActiveVault()
		activeVault = cfg
	}

	log.Printf("[P2P] Active vault: %s", activeVault)

	if activeVault != "" {
		vaultConfig, err := config.LoadVaultConfig(activeVault)
		if err == nil && vaultConfig != nil && vaultConfig.DeviceID != "" {
			deviceName = vaultConfig.DeviceName
			deviceID = vaultConfig.DeviceID
		}
	}

	cfg := p2p.P2PConfig{
		DeviceName: deviceName,
		DeviceID:   deviceID,
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

	go func() {
		for {
			select {
			case peer := <-p2pManager.ConnectedChan():
				log.Printf("[P2P] ========== PEER CONNECTED: %s ==========", peer.ID)
				log.Printf("[P2P] Connected peers: %d", len(p2pManager.GetConnectedPeers()))

				// Just log that a peer connected - generator waits for joiner to initiate
				log.Printf("[Pairing] Peer %s connected, waiting for pairing request...", peer.ID)
			case peerID := <-p2pManager.DisconnectedChan():
				log.Printf("[P2P] Peer disconnected: %s", peerID)
			case msg := <-p2pManager.PairingRequestChan():
				log.Printf("[P2P] Auto: Pairing request from %s", msg.FromPeer)
				handlePairingRequest(p2pManager, msg)
			case msg := <-p2pManager.PairingResponseChan():
				log.Printf("[P2P] Auto: Pairing response from %s", msg.FromPeer)
				handlePairingResponse(msg)
			case msg := <-p2pManager.SyncRequestChan():
				log.Printf("[P2P] Auto: Sync request from %s", msg.FromPeer)
				handleSyncRequest(p2pManager, msg.FromPeer)
			}
		}
	}()

	jsonResponse(w, Response{Success: true, Data: "P2P started"})
}

func handleP2PStop(w http.ResponseWriter, r *http.Request) {
	p2pLock.Lock()
	defer p2pLock.Unlock()

	if p2pManager == nil {
		jsonResponse(w, Response{Success: true, Data: "P2P not running"})
		return
	}

	p2pManager.Stop()
	p2pManager = nil

	jsonResponse(w, Response{Success: true, Data: "P2P stopped"})
}

func handleP2PPeers(w http.ResponseWriter, r *http.Request) {
	log.Println("[P2P] handleP2PPeers called")

	p2pLock.Lock()
	defer p2pLock.Unlock()

	if p2pManager == nil {
		log.Println("[P2P] p2pManager is nil in peers")
		jsonResponse(w, Response{Success: true, Data: []P2PPeerInfo{}})
		return
	}

	log.Printf("[P2P] p2pManager running: %v", p2pManager.IsRunning())

	peers := p2pManager.GetConnectedPeers()
	log.Printf("[P2P] Connected peers count: %d", len(peers))

	allPeers := p2pManager.GetAllPeers()
	log.Printf("[P2P] All peers count: %d", len(allPeers))

	result := make([]P2PPeerInfo, 0)
	for _, p := range allPeers {
		log.Printf("[P2P] Peer: %s connected: %v", p.ID, p.Connected)
		result = append(result, P2PPeerInfo{
			ID:        p.ID,
			Name:      p.Name,
			Addr:      p.Addr,
			Connected: p.Connected,
		})
	}

	jsonResponse(w, Response{Success: true, Data: result})
}

type ConnectRequest struct {
	Address string `json:"address"`
}

func handleP2PConnect(w http.ResponseWriter, r *http.Request) {
	log.Printf("[%s] [P2P] handleP2PConnect called", time.Now().Format("15:04:05"))

	if r.Method != "POST" {
		jsonResponse(w, Response{Success: false, Error: "method not allowed"})
		return
	}

	p2pLock.Lock()
	log.Printf("[P2P] p2pManager is nil: %v", p2pManager == nil)
	if p2pManager == nil {
		p2pLock.Unlock()
		jsonResponse(w, Response{Success: false, Error: "P2P not started - run pwman p2p start first"})
		return
	}
	if !p2pManager.IsRunning() {
		p2pLock.Unlock()
		jsonResponse(w, Response{Success: false, Error: "P2P not running"})
		return
	}
	p2pLock.Unlock()

	var req ConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("[%s] [P2P] Attempting to connect to: %s", time.Now().Format("15:04:05"), req.Address)

	p2pLock.Lock()
	err := p2pManager.ConnectToPeer(req.Address)
	p2pLock.Unlock()

	if err != nil {
		log.Printf("[P2P] Connect error: %v", err)
		jsonResponse(w, Response{Success: false, Error: "failed to connect: " + err.Error()})
		return
	}

	log.Printf("[P2P] Connect succeeded to: %s", req.Address)
	jsonResponse(w, Response{Success: true, Data: "Connected to peer"})
}

type DisconnectRequest struct {
	PeerID string `json:"peer_id"`
}

func handleP2PDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, Response{Success: false, Error: "method not allowed"})
		return
	}

	p2pLock.Lock()
	if p2pManager == nil || !p2pManager.IsRunning() {
		p2pLock.Unlock()
		jsonResponse(w, Response{Success: false, Error: "P2P not running"})
		return
	}
	p2pLock.Unlock()

	var req DisconnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	p2pLock.Lock()
	err := p2pManager.DisconnectFromPeer(req.PeerID)
	p2pLock.Unlock()

	if err != nil {
		jsonResponse(w, Response{Success: false, Error: "failed to disconnect: " + err.Error()})
		return
	}

	jsonResponse(w, Response{Success: true, Data: "Disconnected from peer"})
}

type ApprovalRequest struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	PublicKey  string `json:"public_key"`
	Status     string `json:"status"`
}

func handleP2PApprovals(w http.ResponseWriter, r *http.Request) {
	approvalsLock.Lock()
	defer approvalsLock.Unlock()

	pending := []ApprovalRequest{}
	for _, p := range pendingApprovals {
		pending = append(pending, ApprovalRequest{
			DeviceID:   p.DeviceID,
			DeviceName: p.DeviceName,
			PublicKey:  p.PublicKey,
			Status:     p.Status,
		})
	}

	jsonResponse(w, Response{Success: true, Data: pending})
}

type ApproveRequest struct {
	DeviceID string `json:"device_id"`
}

func handleP2PApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, Response{Success: false, Error: "method not allowed"})
		return
	}

	var req ApproveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	approvalsLock.Lock()
	approval, exists := pendingApprovals[req.DeviceID]
	approvalsLock.Unlock()

	if !exists {
		jsonResponse(w, Response{Success: false, Error: "device not found in pending approvals"})
		return
	}

	vaultLock.Lock()
	if vault == nil || vault.storage == nil {
		vaultLock.Unlock()
		jsonResponse(w, Response{Success: false, Error: "vault not unlocked"})
		return
	}

	device := models.Device{
		ID:          approval.DeviceID,
		Name:        approval.DeviceName,
		PublicKey:   approval.PublicKey,
		Fingerprint: approval.Fingerprint,
		Trusted:     true,
		CreatedAt:   time.Now(),
	}

	if err := vault.storage.UpsertDevice(&device); err != nil {
		vaultLock.Unlock()
		jsonResponse(w, Response{Success: false, Error: "failed to save device: " + err.Error()})
		return
	}

	approvalsLock.Lock()
	delete(pendingApprovals, req.DeviceID)
	approvalsLock.Unlock()

	vaultLock.Unlock()

	jsonResponse(w, Response{Success: true, Data: "Device approved"})
}

type RejectRequest struct {
	DeviceID string `json:"device_id"`
	Reason   string `json:"reason"`
}

func handleP2PReject(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, Response{Success: false, Error: "method not allowed"})
		return
	}

	var req RejectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	approvalsLock.Lock()
	_, exists := pendingApprovals[req.DeviceID]
	approvalsLock.Unlock()

	if !exists {
		jsonResponse(w, Response{Success: false, Error: "device not found in pending approvals"})
		return
	}

	approvalsLock.Lock()
	delete(pendingApprovals, req.DeviceID)
	approvalsLock.Unlock()

	jsonResponse(w, Response{Success: true, Data: "Device rejected"})
}

func generatePairingCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 9)
	for i := 0; i < 9; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		code[i] = chars[n.Int64()]
	}
	return fmt.Sprintf("%s-%s-%s", string(code[0:3]), string(code[3:6]), string(code[6:9]))
}

func handlePairingGenerate(w http.ResponseWriter, r *http.Request) {
	vaultLock.Lock()
	needsUnlock := vault == nil || vault.privateKey == nil
	vaultLock.Unlock()

	if needsUnlock {
		jsonResponse(w, Response{Success: false, Error: "vault_locked", Data: map[string]string{
			"message": "Unlock vault to generate pairing code",
		}})
		return
	}

	// Auto-start P2P if not running
	p2pLock.Lock()
	if p2pManager == nil || !p2pManager.IsRunning() {
		p2pLock.Unlock()
		// Start P2P automatically
		deviceName := ""
		deviceID := ""

		vaultLock.Lock()
		if vault != nil && vault.cfg != nil {
			deviceName = vault.cfg.DeviceName
			deviceID = vault.cfg.DeviceID
		}
		vaultLock.Unlock()

		if deviceName == "" {
			activeVault, _ := config.GetActiveVault()
			if activeVault != "" {
				vaultConfig, _ := config.LoadVaultConfig(activeVault)
				if vaultConfig != nil && vaultConfig.DeviceID != "" {
					deviceName = vaultConfig.DeviceName
					deviceID = vaultConfig.DeviceID
				}
			}
		}

		cfg := p2p.P2PConfig{
			DeviceName: deviceName,
			DeviceID:   deviceID,
		}

		log.Printf("[Pairing] Auto-starting P2P for pairing...")
		manager, err := p2p.NewP2PManager(cfg)
		if err != nil {
			log.Printf("[Pairing] Failed to auto-start P2P: %v", err)
		} else {
			if err := manager.Start(); err != nil {
				log.Printf("[Pairing] Failed to start P2P: %v", err)
			} else {
				p2pManager = manager
				go func() {
					for {
						select {
						case peer := <-p2pManager.ConnectedChan():
							log.Printf("[P2P] Auto: Received connected event: %s", peer.ID)
							approvalsLock.Lock()
							if _, exists := pendingApprovals[peer.ID]; !exists {
								pendingApprovals[peer.ID] = PendingApproval{
									DeviceID:    peer.ID,
									DeviceName:  peer.Name,
									Status:      "pending",
									ConnectedAt: time.Now(),
								}
							}
							approvalsLock.Unlock()
						case peerID := <-p2pManager.DisconnectedChan():
							log.Printf("[P2P] Auto: Peer disconnected: %s", peerID)
						case msg := <-p2pManager.PairingRequestChan():
							log.Printf("[P2P] Auto: Pairing request from %s", msg.FromPeer)
							handlePairingRequest(p2pManager, msg)
						case msg := <-p2pManager.PairingResponseChan():
							log.Printf("[P2P] Auto: Pairing response from %s", msg.FromPeer)
							handlePairingResponse(msg)
						case msg := <-p2pManager.SyncRequestChan():
							log.Printf("[P2P] Auto: Sync request from %s", msg.FromPeer)
							handleSyncRequest(p2pManager, msg.FromPeer)
						}
					}
				}()
			}
		}
	} else {
		p2pLock.Unlock()
	}

	vaultLock.Lock()
	deviceID := vault.cfg.DeviceID
	deviceName := vault.cfg.DeviceName
	vaultName := vault.vaultName
	publicKeyPath := config.PublicKeyPathForVault(vaultName)
	log.Printf("[Pairing] DeviceName: '%s', vaultName: '%s', publicKeyPath: '%s'", deviceName, vaultName, publicKeyPath)
	vaultLock.Unlock()

	log.Printf("[Pairing] Looking for public key at: %s", publicKeyPath)
	publicKeyBytes, err := os.ReadFile(publicKeyPath)
	if err != nil {
		log.Printf("[Pairing] Error reading public key: %v", err)
		jsonResponse(w, Response{Success: false, Error: "failed to read public key"})
		return
	}

	publicKey := string(publicKeyBytes)
	code := generatePairingCode()
	// Normalize code for lookup (remove dashes)
	normalizedCode := strings.ToUpper(strings.ReplaceAll(code, "-", ""))
	fingerprint := deviceID

	activeVault, _ := config.GetActiveVault()

	pairingLock.Lock()
	pairingCodes[normalizedCode] = PairingCode{
		Code:        code,
		VaultID:     activeVault,
		VaultName:   vaultName, // NEW
		DeviceID:    deviceID,
		DeviceName:  deviceName,
		PublicKey:   publicKey,
		Fingerprint: fingerprint,
		ExpiresAt:   time.Now().Add(5 * time.Minute),
		Used:        false,
	}
	pairingLock.Unlock()

	log.Printf("[Pairing] Generated code: %s for device: %s", code, deviceName)

	jsonResponse(w, Response{Success: true, Data: map[string]interface{}{
		"code":        code,
		"device_name": deviceName,
		"expires_in":  300,
	}})
}

type PairingJoinRequest struct {
	Code       string `json:"code"`
	DeviceName string `json:"device_name"`
	Password   string `json:"password"` // NEW: vault password for joining
}

func handlePairingJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, Response{Success: false, Error: "method not allowed"})
		return
	}

	// Auto-start P2P if not running (for the joining device)
	p2pLock.Lock()
	if p2pManager == nil || !p2pManager.IsRunning() {
		p2pLock.Unlock()

		// Get device info from vault config
		deviceName := ""
		deviceID := ""

		vaultLock.Lock()
		if vault != nil && vault.cfg != nil {
			deviceName = vault.cfg.DeviceName
			deviceID = vault.cfg.DeviceID
		}
		vaultLock.Unlock()

		if deviceName == "" {
			activeVault, _ := config.GetActiveVault()
			if activeVault != "" {
				vaultConfig, _ := config.LoadVaultConfig(activeVault)
				if vaultConfig != nil && vaultConfig.DeviceID != "" {
					deviceName = vaultConfig.DeviceName
					deviceID = vaultConfig.DeviceID
				}
			}
		}

		cfg := p2p.P2PConfig{
			DeviceName: deviceName,
			DeviceID:   deviceID,
		}

		log.Printf("[Pairing Join] Auto-starting P2P...")
		manager, err := p2p.NewP2PManager(cfg)
		if err != nil {
			log.Printf("[Pairing Join] Failed to auto-start P2P: %v", err)
		} else {
			if err := manager.Start(); err != nil {
				log.Printf("[Pairing Join] Failed to start P2P: %v", err)
			} else {
				p2pManager = manager
				go func() {
					for {
						select {
						case peer := <-p2pManager.ConnectedChan():
							log.Printf("[P2P] Join: Received connected event: %s", peer.ID)
							approvalsLock.Lock()
							if _, exists := pendingApprovals[peer.ID]; !exists {
								pendingApprovals[peer.ID] = PendingApproval{
									DeviceID:    peer.ID,
									DeviceName:  peer.Name,
									Status:      "pending",
									ConnectedAt: time.Now(),
								}
							}
							approvalsLock.Unlock()
						case peerID := <-p2pManager.DisconnectedChan():
							log.Printf("[P2P] Join: Peer disconnected: %s", peerID)
						case msg := <-p2pManager.PairingRequestChan():
							log.Printf("[P2P] Join: Pairing request from %s", msg.FromPeer)
							handleJoinerPairingRequest(p2pManager, msg)
						case msg := <-p2pManager.PairingResponseChan():
							log.Printf("[P2P] Join: Pairing response from %s", msg.FromPeer)
							handleJoinerPairingResponse(msg)
							// NOTE: Don't handle SyncDataChan here - it's handled after vault is created
						}
					}
				}()
			}
		}

		// Wait a moment for P2P to start
		time.Sleep(2 * time.Second)
	} else {
		p2pLock.Unlock()
	}

	var req PairingJoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	req.Code = strings.ToUpper(strings.ReplaceAll(req.Code, "-", ""))

	log.Printf("[Pairing Join] Looking for code: '%s' via P2P (password provided: %v)", req.Code, req.Password != "")

	// Get device info for the pairing request
	joiningDeviceID := ""
	joiningDeviceName := req.DeviceName
	joiningPassword := req.Password // NEW

	vaultLock.Lock()
	if vault != nil && vault.cfg != nil {
		joiningDeviceID = vault.cfg.DeviceID
	}
	vaultLock.Unlock()

	if joiningDeviceID == "" {
		activeVault, _ := config.GetActiveVault()
		if activeVault != "" {
			vaultConfig, _ := config.LoadVaultConfig(activeVault)
			if vaultConfig != nil {
				joiningDeviceID = vaultConfig.DeviceID
			}
		}
	}

	if joiningDeviceID == "" {
		joiningDeviceID = uuid.New().String()
	}

	// Store pairing request and wait for generating side to connect
	pairingLock.Lock()
	pairingRequests[req.Code] = PairingRequest{
		Code:       req.Code,
		DeviceID:   joiningDeviceID,
		DeviceName: joiningDeviceName,
		CreatedAt:  time.Now(),
	}
	pairingLock.Unlock()

	log.Printf("[Pairing Join] Stored request for code: %s, sending to discovered peers...", req.Code)

	// Set up response channel
	responseCh := make(chan p2p.PairingResponsePayload, 10)
	pairingResponseCh = responseCh

	// Send pairing request to all discovered peers immediately
	p2pLock.Lock()
	if p2pManager != nil && p2pManager.IsRunning() {
		peers := p2pManager.GetAllPeers() // Get all discovered peers (not just connected)
		log.Printf("[Pairing Join] Found %d discovered peers", len(peers))

		// Get joiner's public key to send in pairing request
		joinerPublicKey := ""
		vaultInitialized := false
		if vault != nil {
			vaultLock.Lock()
			if vault.cfg != nil {
				pubKeyPath := config.PublicKeyPathForVault(vault.vaultName)
				pubKeyBytes, err := os.ReadFile(pubKeyPath)
				if err == nil {
					joinerPublicKey = string(pubKeyBytes)
					vaultInitialized = true
				} else {
					log.Printf("[Pairing Join] Warning: No public key found at %s - vault may not be initialized", pubKeyPath)
				}
			}
			vaultLock.Unlock()
		}

		if !vaultInitialized {
			log.Printf("[Pairing Join] ERROR: Vault not initialized on joining device. Please initialize your vault first before pairing.")
		}

		for _, peer := range peers {
			go func(peerID string) {
				for attempt := 0; attempt < 10; attempt++ {
					// Create pairing request message
					msg, err := p2p.CreatePairingRequestMessage(req.Code, joiningDeviceID, joiningDeviceName, joinerPublicKey, joiningPassword)
					if err != nil {
						log.Printf("[Pairing Join] Failed to create message: %v", err)
						return
					}

					err = p2pManager.SendMessage(peerID, p2p.SyncMessage{
						Type:    msg.Type,
						Payload: msg.Payload,
					})
					if err != nil {
						log.Printf("[Pairing Join] Send to %s failed (attempt %d): %v", peerID, attempt+1, err)
						time.Sleep(1 * time.Second)
					} else {
						log.Printf("[Pairing Join] Sent pairing request to %s", peerID)
						return
					}
				}
			}(peer.ID)
		}
	}
	p2pLock.Unlock()

	// Wait for valid response
	select {
	case response := <-responseCh:
		pairingResponseCh = nil
		if !response.Success {
			jsonResponse(w, Response{Success: false, Error: response.Error})
			return
		}

		log.Printf("[Pairing Join] Received valid response from: %s", response.DeviceName)

		// Add to pending approvals (auto-approve since code was valid)
		approvalsLock.Lock()
		pendingApprovals[response.DeviceID] = PendingApproval{
			DeviceID:    response.DeviceID,
			DeviceName:  response.DeviceName,
			PublicKey:   response.PublicKey,
			Fingerprint: response.Fingerprint,
			Status:      "paired",
		}
		approvalsLock.Unlock()

		// NEW: Create vault if it doesn't exist (joining from scratch)
		vaultName := response.VaultName
		joinPassword := req.Password

		log.Printf("[Pairing Join] Vault name from generator: %s", vaultName)

		vaultLock.Lock()
		if vault == nil {
			// Need to create vault from scratch
			log.Printf("[Pairing Join] Creating vault '%s' from scratch...", vaultName)

			// Create vault directory FIRST
			vaultPath := config.VaultPath(vaultName)
			if err := os.MkdirAll(vaultPath, 0700); err != nil {
				log.Printf("[Pairing Join] Failed to create vault directory: %v", err)
				vaultLock.Unlock()
				jsonResponse(w, Response{Success: false, Error: "failed to create vault: " + err.Error()})
				return
			}

			// Add vault to global config BEFORE setting as active
			globalCfg, err := config.LoadGlobalConfig()
			if err != nil {
				globalCfg = &config.GlobalConfig{}
			}
			found := false
			for _, v := range globalCfg.Vaults {
				if v == vaultName {
					found = true
					break
				}
			}
			if !found {
				globalCfg.Vaults = append(globalCfg.Vaults, vaultName)
				if err := globalCfg.Save(); err != nil {
					log.Printf("[Pairing Join] Failed to save global config: %v", err)
				}
			}

			// Set as active vault (now that it's in config)
			if err := config.SetActiveVault(vaultName); err != nil {
				log.Printf("[Pairing Join] Failed to set active vault: %v", err)
			}

			// Generate new key pair for this device
			keyPair, err := crypto.GenerateRSAKeyPair(2048)
			if err != nil {
				log.Printf("[Pairing Join] Failed to generate keys: %v", err)
				vaultLock.Unlock()
				jsonResponse(w, Response{Success: false, Error: "failed to generate keys: " + err.Error()})
				return
			}

			// Encrypt and save private key with password
			salt, err := crypto.EncryptPrivateKeyAndSave(keyPair.PrivateKey, joinPassword, config.PrivateKeyPathForVault(vaultName))
			if err != nil {
				log.Printf("[Pairing Join] Failed to encrypt private key: %v", err)
				vaultLock.Unlock()
				jsonResponse(w, Response{Success: false, Error: "failed to encrypt private key: " + err.Error()})
				return
			}

			// Save public key
			if err := crypto.SavePublicKey(keyPair.PublicKey, config.PublicKeyPathForVault(vaultName)); err != nil {
				log.Printf("[Pairing Join] Failed to save public key: %v", err)
				vaultLock.Unlock()
				jsonResponse(w, Response{Success: false, Error: "failed to save public key: " + err.Error()})
				return
			}

			// Initialize SQLite database
			db, err := storage.NewSQLite(config.DatabasePathForVault(vaultName))
			if err != nil {
				log.Printf("[Pairing Join] Failed to initialize database: %v", err)
				vaultLock.Unlock()
				jsonResponse(w, Response{Success: false, Error: "failed to initialize database: " + err.Error()})
				return
			}

			// Create device entry for ourselves - store actual public key content
			deviceID := uuid.New().String()
			publicKeyBytes, _ := os.ReadFile(config.PublicKeyPathForVault(vaultName))
			selfDevice := models.Device{
				ID:          deviceID,
				Name:        joiningDeviceName,
				PublicKey:   string(publicKeyBytes), // Store actual key content, not path
				Fingerprint: crypto.GetFingerprint(keyPair.PublicKey),
				Trusted:     true,
				CreatedAt:   time.Now(),
			}
			db.UpsertDevice(&selfDevice)

			// Save device config
			cfg := &config.Config{
				DeviceID:   deviceID,
				DeviceName: joiningDeviceName,
				Salt:       base64.StdEncoding.EncodeToString(salt),
			}
			cfgBytes, _ := json.Marshal(cfg)
			os.WriteFile(config.VaultConfigPath(vaultName), cfgBytes, 0600)

			// Create vault struct
			vault = &Vault{
				privateKey: keyPair,
				storage:    db,
				cfg:        cfg,
				vaultName:  vaultName,
			}

			log.Printf("[Pairing Join] Created vault '%s' with device %s, storage=%v", vaultName, joiningDeviceName, vault.storage != nil)
		}
		vaultLock.Unlock()

		// Verify vault is ready
		vaultLock.Lock()
		log.Printf("[Pairing Join] After vault creation: vault=%v, storage=%v", vault != nil, vault != nil && vault.storage != nil)
		vaultLock.Unlock()

		// Find the generator's peer ID from connected peers
		p2pLock.Lock()
		peers := p2pManager.GetAllPeers()
		var generatorPeerID string
		for _, p := range peers {
			if p.ID == response.DeviceID || p.Name == response.DeviceName {
				generatorPeerID = p.ID
				break
			}
		}
		if generatorPeerID == "" && len(peers) > 0 {
			generatorPeerID = peers[len(peers)-1].ID
		}
		p2pLock.Unlock()

		// NEW: After creating vault, send pairing request WITH public key to trigger re-encryption
		if vault != nil && vault.privateKey != nil && generatorPeerID != "" {
			log.Printf("[Pairing Join] Sending updated pairing request with public key to %s...", generatorPeerID)

			// Get our newly created public key
			pubKeyPath := config.PublicKeyPathForVault(vaultName)
			if pubKeyBytes, err := os.ReadFile(pubKeyPath); err == nil {
				joinerPublicKey := string(pubKeyBytes)

				// Send new pairing request to generator with public key
				msg, err := p2p.CreatePairingRequestMessage(req.Code, vault.cfg.DeviceID, vault.cfg.DeviceName, joinerPublicKey, joinPassword)
				if err != nil {
					log.Printf("[Pairing Join] Failed to create updated request: %v", err)
				} else {
					err = p2pManager.SendMessage(generatorPeerID, p2p.SyncMessage{
						Type:    msg.Type,
						Payload: msg.Payload,
					})
					if err != nil {
						log.Printf("[Pairing Join] Failed to send updated request: %v", err)
					} else {
						log.Printf("[Pairing Join] Sent updated pairing request with public key to %s", generatorPeerID)
					}
				}
			}
		}

		// Request sync from generator (more reliable than waiting for push)
		log.Printf("[Pairing Join] Requesting vault sync from %s...", response.DeviceName)

		if generatorPeerID != "" {
			// Send request for full sync
			syncMsg, err := p2p.CreateRequestSyncMessage(0, true)
			if err != nil {
				log.Printf("[Pairing Join] Failed to create sync request: %v", err)
			} else {
				err = p2pManager.SendMessage(generatorPeerID, p2p.SyncMessage{
					Type:    syncMsg.Type,
					Payload: syncMsg.Payload,
				})
				if err != nil {
					log.Printf("[Pairing Join] Failed to send sync request: %v", err)
				} else {
					log.Printf("[Pairing Join] Sent sync request to %s", generatorPeerID)
				}
			}
		} else {
			log.Printf("[Pairing Join] WARNING: Could not find generator peer")
		}

		// Wait for sync data from generator (with timeout)
		log.Printf("[Pairing Join] Waiting for vault sync from %s...", response.DeviceName)

		vaultCreated := false
		syncTimeout := time.After(30 * time.Second)

		for {
			select {
			case msg := <-p2pManager.SyncDataChan():
				log.Printf("[Pairing Join] Received sync data from %s", msg.FromPeer)
				handleJoinerSyncData(msg)
				vaultCreated = true
				log.Printf("[Pairing Join] Vault sync complete!")
				break

			case <-syncTimeout:
				log.Printf("[Pairing Join] Timeout waiting for sync data")
				break
			}

			if vaultCreated {
				break
			}
		}

		jsonResponse(w, Response{Success: true, Data: map[string]interface{}{
			"message":         "Connected to vault",
			"device_name":     response.DeviceName,
			"device_approved": true,
			"vault_synced":    vaultCreated,
		}})
		return

	case <-time.After(15 * time.Second):
		pairingResponseCh = nil
		jsonResponse(w, Response{Success: false, Error: "no_response_from_vault"})
		return
	}
}

func handlePairingStatus(w http.ResponseWriter, r *http.Request) {
	pairingLock.Lock()
	var activeCode *PairingCode
	for _, c := range pairingCodes {
		if !c.Used && time.Now().Before(c.ExpiresAt) {
			activeCode = &c
			break
		}
	}
	pairingLock.Unlock()

	if activeCode == nil {
		jsonResponse(w, Response{Success: true, Data: map[string]interface{}{
			"active": false,
		}})
		return
	}

	jsonResponse(w, Response{Success: true, Data: map[string]interface{}{
		"active":      true,
		"device_name": activeCode.DeviceName,
		"expires_in":  int(time.Until(activeCode.ExpiresAt).Seconds()),
	}})
}

func reEncryptEntriesForDevice(peerID, deviceID, deviceName, publicKey, fingerprint string) {
	vaultLock.Lock()
	defer vaultLock.Unlock()

	if vault == nil || vault.storage == nil || vault.privateKey == nil {
		log.Printf("[Sync] Cannot re-encrypt: vault not unlocked")
		return
	}

	privateKey := vault.privateKey.PrivateKey
	db := vault.storage
	cfg := vault.cfg

	log.Printf("[Sync] Starting re-encryption for device: %s (%s)", deviceName, deviceID)

	entries, err := db.ListEntries()
	if err != nil {
		log.Printf("[Sync] Failed to list entries: %v", err)
		return
	}

	log.Printf("[Sync] Found %d entries to re-encrypt", len(entries))

	newDevice := models.Device{
		ID:          deviceID,
		Name:        deviceName,
		PublicKey:   publicKey,
		Fingerprint: fingerprint,
		Trusted:     true,
	}

	reEncryptedEntries := []p2p.EntryData{}

	for i := range entries {
		entry := &entries[i]
		// Decrypt with current device's private key
		password, err := crypto.HybridDecrypt(entry, privateKey)
		if err != nil {
			log.Printf("[Sync] Failed to decrypt entry %s: %v", entry.ID, err)
			continue
		}

		log.Printf("[Sync] Decrypted password for entry %s", entry.Site)

		// Get existing device for encryption (generator)
		generatorDevice, _ := db.GetDevice(cfg.DeviceID)

		// Re-encrypt for BOTH generator AND new device
		allDevices := []models.Device{*generatorDevice, newDevice}
		allGetPublicKey := func(fp string) (*rsa.PublicKey, error) {
			if fp == fingerprint {
				return crypto.ParsePublicKey(publicKey) // joiner
			}
			if fp == generatorDevice.Fingerprint {
				return crypto.ParsePublicKey(generatorDevice.PublicKey) // generator
			}
			devices, _ := db.ListDevices()
			for _, d := range devices {
				if d.Fingerprint == fp {
					return crypto.ParsePublicKey(d.PublicKey)
				}
			}
			return nil, fmt.Errorf("device not found")
		}

		encrypted, err := crypto.HybridEncrypt(password, allDevices, allGetPublicKey)
		if err != nil {
			log.Printf("[Sync] Failed to re-encrypt for both devices: %v", err)
			continue
		}

		log.Printf("[Sync] Re-encrypted for both devices")

		// Update entry with re-encrypted password for both devices
		// This ensures BOTH generator and joiner can decrypt
		entry.EncryptedPassword = encrypted.EncryptedPassword // Re-encrypted with NEW key
		entry.EncryptedAESKeys[fingerprint] = encrypted.EncryptedAESKeys[fingerprint]
		entry.EncryptedAESKeys[generatorDevice.Fingerprint] = encrypted.EncryptedAESKeys[generatorDevice.Fingerprint]
		entry.Version++
		entry.UpdatedAt = time.Now()
		entry.UpdatedBy = cfg.DeviceID

		log.Printf("[Sync] Updated both encrypted password and keys for both devices")

		log.Printf("[Sync] Re-encrypted entry: %s (%s) - key for fingerprint: %s...",
			entry.Site, entry.Username, fingerprint[:min(20, len(fingerprint))])

		if err := db.UpdateEntry(entry); err != nil {
			log.Printf("[Sync] Failed to update entry %s: %v", entry.ID, err)
			continue
		}

		// Add to sync data
		reEncryptedEntries = append(reEncryptedEntries, p2p.EntryData{
			ID:                entry.ID,
			Site:              entry.Site,
			Username:          entry.Username,
			EncryptedPassword: entry.EncryptedPassword,
			EncryptedAESKeys:  entry.EncryptedAESKeys,
			Notes:             entry.Notes,
			Version:           int(entry.Version),
			CreatedAt:         entry.CreatedAt.UnixMilli(),
			UpdatedAt:         entry.UpdatedAt.UnixMilli(),
		})
	}

	// Get all existing devices to send to joiner
	devices, _ := db.ListDevices()
	deviceList := []p2p.DeviceData{}

	// Add all existing trusted devices
	for _, d := range devices {
		deviceList = append(deviceList, p2p.DeviceData{
			ID:          d.ID,
			Name:        d.Name,
			PublicKey:   d.PublicKey,
			Fingerprint: d.Fingerprint,
			Trusted:     d.Trusted,
			CreatedAt:   d.CreatedAt.UnixMilli(),
		})
		log.Printf("[Sync] Will send device info: %s (trusted: %v)", d.Name, d.Trusted)
	}

	// Also add the new device we're re-encrypting for
	deviceList = append(deviceList, p2p.DeviceData{
		ID:          newDevice.ID,
		Name:        newDevice.Name,
		PublicKey:   newDevice.PublicKey,
		Fingerprint: newDevice.Fingerprint,
		Trusted:     newDevice.Trusted,
		CreatedAt:   newDevice.CreatedAt.UnixMilli(),
	})

	// Wait a moment for joiner to be ready to receive
	log.Printf("[Sync] Waiting for joiner to be ready...")
	time.Sleep(2 * time.Second)

	// Note: We don't send sync data here - the joiner will request sync via handleSyncRequest
	// This avoids sending duplicate sync messages
	log.Printf("[Sync] Re-encryption complete, waiting for joiner to request sync...")
}

func main() {
	http.HandleFunc("/api/init", corsHandler(handleInit))
	http.HandleFunc("/api/unlock", corsHandler(handleUnlock))
	http.HandleFunc("/api/lock", corsHandler(handleLock))
	http.HandleFunc("/api/is_unlocked", corsHandler(handleIsUnlocked))
	http.HandleFunc("/api/is_initialized", corsHandler(handleIsInitialized))
	http.HandleFunc("/api/entries", corsHandler(handleGetEntries))
	http.HandleFunc("/api/entries/add", corsHandler(handleAddEntry))
	http.HandleFunc("/api/entries/update", corsHandler(handleUpdateEntry))
	http.HandleFunc("/api/entries/delete", corsHandler(handleDeleteEntry))
	http.HandleFunc("/api/entries/get_password", corsHandler(handleGetPassword))
	http.HandleFunc("/api/devices", corsHandler(handleGetDevices))
	http.HandleFunc("/api/sync/status", corsHandler(handleGetSyncStatus))
	http.HandleFunc("/api/generate", corsHandler(handleGeneratePassword))
	http.HandleFunc("/api/vaults", corsHandler(handleVaults))
	http.HandleFunc("/api/vaults/use", corsHandler(handleVaultUse))
	http.HandleFunc("/api/vaults/create", corsHandler(handleVaultCreate))
	http.HandleFunc("/api/vaults/delete", corsHandler(handleVaultDelete))

	// Test endpoint
	http.HandleFunc("/api/ping", corsHandler(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, Response{Success: true, Data: "pong"})
	}))

	// P2P endpoints
	http.HandleFunc("/api/p2p/status", corsHandler(handleP2PStatus))
	http.HandleFunc("/api/p2p/start", corsHandler(handleP2PStart))
	http.HandleFunc("/api/p2p/stop", corsHandler(handleP2PStop))
	http.HandleFunc("/api/p2p/peers", corsHandler(handleP2PPeers))
	http.HandleFunc("/api/p2p/connect", corsHandler(handleP2PConnect))
	http.HandleFunc("/api/p2p/disconnect", corsHandler(handleP2PDisconnect))
	http.HandleFunc("/api/p2p/approvals", corsHandler(handleP2PApprovals))
	http.HandleFunc("/api/p2p/approve", corsHandler(handleP2PApprove))
	http.HandleFunc("/api/p2p/reject", corsHandler(handleP2PReject))
	http.HandleFunc("/api/p2p/sync", corsHandler(handleP2PSync))

	// Pairing endpoints
	http.HandleFunc("/api/pairing/generate", corsHandler(handlePairingGenerate))
	http.HandleFunc("/api/pairing/join", corsHandler(handlePairingJoin))
	http.HandleFunc("/api/pairing/status", corsHandler(handlePairingStatus))

	log.Printf("Starting pwman API server on :%s", serverPort)
	log.Fatal(http.ListenAndServe(":"+serverPort, nil))
}

type SyncRequest struct {
	FullSync bool `json:"full_sync"`
}

func handleP2PSync(w http.ResponseWriter, r *http.Request) {
	p2pLock.Lock()
	if p2pManager == nil || !p2pManager.IsRunning() {
		p2pLock.Unlock()
		jsonResponse(w, Response{Success: false, Error: "P2P not running"})
		return
	}
	p2pLock.Unlock()

	var req SyncRequest
	if r.Method == "POST" {
		json.NewDecoder(r.Body).Decode(&req)
	}

	vaultLock.Lock()
	if vault == nil || vault.storage == nil {
		vaultLock.Unlock()
		jsonResponse(w, Response{Success: false, Error: "vault not unlocked"})
		return
	}

	entries, err := vault.storage.ListEntries()
	vaultLock.Unlock()

	if err != nil {
		jsonResponse(w, Response{Success: false, Error: "failed to get entries"})
		return
	}

	syncData := make([]map[string]interface{}, len(entries))
	for i, e := range entries {
		syncData[i] = map[string]interface{}{
			"id":                 e.ID,
			"site":               e.Site,
			"username":           e.Username,
			"encrypted_password": e.EncryptedPassword,
			"encrypted_aes_keys": e.EncryptedAESKeys,
			"notes":              e.Notes,
			"version":            e.Version,
			"created_at":         e.CreatedAt.Unix(),
			"updated_at":         e.UpdatedAt.Unix(),
		}
	}

	p2pLock.Lock()
	err = p2pManager.SyncWithPeers(req.FullSync)
	p2pLock.Unlock()

	if err != nil {
		log.Printf("[Sync] Error: %v", err)
		jsonResponse(w, Response{Success: false, Error: "sync failed"})
		return
	}

	jsonResponse(w, Response{Success: true, Data: map[string]interface{}{
		"entries":   syncData,
		"synced":    true,
		"timestamp": time.Now().Unix(),
	}})
}

func handlePairingRequest(pm *p2p.P2PManager, msg p2p.ReceivedMessage) {
	var pairingReq p2p.PairingRequestPayload
	if err := json.Unmarshal(msg.Payload, &pairingReq); err != nil {
		log.Printf("[Pairing] Failed to parse request: %v", err)
		return
	}

	log.Printf("[Pairing] Received pairing request with code: %s from %s", pairingReq.Code, pairingReq.DeviceName)

	pairingLock.Lock()
	code, exists := pairingCodes[pairingReq.Code]
	pairingLock.Unlock()

	var response p2p.PairingResponsePayload
	if !exists {
		response = p2p.PairingResponsePayload{Success: false, Error: "invalid_code"}
	} else if time.Now().After(code.ExpiresAt) {
		response = p2p.PairingResponsePayload{Success: false, Error: "code_expired"}
	} else {
		// Check if this is a re-pairing attempt from the same device
		// Allow re-pairing if the device already exists (for updating public key)
		isRePairing := code.Used

		if !isRePairing {
			pairingLock.Lock()
			code.Used = true
			pairingCodes[pairingReq.Code] = code
			pairingLock.Unlock()
		}

		// Add joiner as trusted device using info from the REQUEST (not from stored code)
		// Compute fingerprint from public key
		joinerFingerprint := pairingReq.DeviceID
		if pairingReq.PublicKey != "" {
			if pubKey, err := crypto.ParsePublicKey(pairingReq.PublicKey); err == nil {
				joinerFingerprint = crypto.GetFingerprint(pubKey)
			}
		} else {
			log.Printf("[Pairing] WARNING: Joiner %s did not provide a public key. They must initialize their vault before pairing. Passwords will NOT be re-encrypted for this device.", pairingReq.DeviceName)
		}

		vaultLock.Lock()
		log.Printf("[Pairing] handlePairingRequest: vault=%v, storage=%v", vault != nil, vault != nil && vault.storage != nil)
		if vault != nil && vault.storage != nil {
			device := models.Device{
				ID:          pairingReq.DeviceID,
				Name:        pairingReq.DeviceName,
				PublicKey:   pairingReq.PublicKey,
				Fingerprint: joinerFingerprint,
				Trusted:     true,
				CreatedAt:   time.Now(),
			}
			vault.storage.UpsertDevice(&device)
			log.Printf("[Pairing] Added joiner %s as trusted device (fingerprint: %s)", pairingReq.DeviceName, joinerFingerprint)

			// FIX: Also update our own device with public key if it's empty
			if vault.cfg != nil && vault.privateKey != nil && vault.privateKey.PublicKey != nil {
				selfDevice, _ := vault.storage.GetDevice(vault.cfg.DeviceID)
				if selfDevice != nil && selfDevice.PublicKey == "" {
					pubKeyPath := config.PublicKeyPathForVault(vault.vaultName)
					if pubKeyBytes, err := os.ReadFile(pubKeyPath); err == nil {
						selfDevice.PublicKey = string(pubKeyBytes)
						selfDevice.Fingerprint = crypto.GetFingerprint(vault.privateKey.PublicKey)
						vault.storage.UpsertDevice(selfDevice)
						log.Printf("[Pairing] Updated own device with public key")
					}
				}
			}

			// Only re-encrypt if joiner provided a public key
			if pairingReq.PublicKey != "" {
				go reEncryptEntriesForDevice(
					msg.FromPeer,
					pairingReq.DeviceID,
					pairingReq.DeviceName,
					pairingReq.PublicKey,
					joinerFingerprint,
				)
			} else {
				log.Printf("[Pairing] Skipping re-encryption - joiner has no public key")
			}
		} else {
			log.Printf("[Pairing] WARNING: vault not available for adding trusted device")
		}
		vaultLock.Unlock()

		response = p2p.PairingResponsePayload{
			Success:     true,
			Code:        pairingReq.Code,
			VaultName:   code.VaultName,
			VaultID:     code.VaultID,
			DeviceID:    code.DeviceID,
			DeviceName:  code.DeviceName,
			PublicKey:   code.PublicKey,
			Fingerprint: code.Fingerprint,
		}
	}

	respMsg, err := p2p.CreatePairingResponseMessage(
		response.Success, response.Code, response.VaultName, response.VaultID,
		response.DeviceID, response.DeviceName, response.PublicKey, response.Fingerprint, response.Error)
	if err != nil {
		log.Printf("[Pairing] Failed to create response: %v", err)
		return
	}

	log.Printf("[Pairing] Sending response to peer: %s", msg.FromPeer)
	pm.SendMessage(msg.FromPeer, p2p.SyncMessage{Type: respMsg.Type, Payload: respMsg.Payload})
}

func handlePairingResponse(msg p2p.ReceivedMessage) {
	var pairingResp p2p.PairingResponsePayload
	if err := json.Unmarshal(msg.Payload, &pairingResp); err != nil {
		log.Printf("[Pairing] Failed to parse response: %v", err)
		return
	}

	if pairingResponseCh != nil {
		pairingResponseCh <- pairingResp
	}
}

func handleSyncRequest(pm *p2p.P2PManager, peerID string) {
	log.Printf("[Sync] Received sync request from %s", peerID)

	vaultLock.Lock()
	if vault == nil || vault.storage == nil || vault.privateKey == nil {
		vaultLock.Unlock()
		log.Printf("[Sync] Cannot respond: vault not available")
		return
	}

	// FIX: Also update our own device with public key before sending
	if vault.cfg != nil && vault.privateKey != nil && vault.privateKey.PublicKey != nil {
		selfDevice, _ := vault.storage.GetDevice(vault.cfg.DeviceID)
		if selfDevice != nil {
			pubKeyPath := config.PublicKeyPathForVault(vault.vaultName)
			if pubKeyBytes, err := os.ReadFile(pubKeyPath); err == nil {
				selfDevice.PublicKey = string(pubKeyBytes)
				selfDevice.Fingerprint = crypto.GetFingerprint(vault.privateKey.PublicKey)
				vault.storage.UpsertDevice(selfDevice)
				log.Printf("[Sync] Updated own device with public key before sync")
			}
		}
	}

	entries, _ := vault.storage.ListEntries()
	devices, _ := vault.storage.ListDevices()
	log.Printf("[Sync] Found %d entries, %d devices", len(entries), len(devices))

	// Deduplicate devices by ID
	seen := make(map[string]bool)
	deviceList := []p2p.DeviceData{}
	for _, d := range devices {
		if seen[d.ID] {
			log.Printf("[Sync] Skipping duplicate device: %s", d.Name)
			continue
		}
		seen[d.ID] = true

		publicKey := d.PublicKey
		// If stored public key looks like a file path, read from file
		if len(publicKey) > 0 && publicKey[0] == '/' {
			if pubKeyBytes, err := os.ReadFile(publicKey); err == nil {
				publicKey = string(pubKeyBytes)
			} else if pubKeyBytes, err := os.ReadFile(config.PublicKeyPathForVault(vault.vaultName)); err == nil {
				// Try default path
				publicKey = string(pubKeyBytes)
			}
		}
		deviceList = append(deviceList, p2p.DeviceData{
			ID:          d.ID,
			Name:        d.Name,
			PublicKey:   publicKey,
			Fingerprint: d.Fingerprint,
			Trusted:     d.Trusted,
			CreatedAt:   d.CreatedAt.UnixMilli(),
		})
	}

	entryData := make([]p2p.EntryData, len(entries))
	for i, e := range entries {
		log.Printf("[Sync] Sending entry: site=%s, username=%s, has %d encrypted keys",
			e.Site, e.Username, len(e.EncryptedAESKeys))
		for fp := range e.EncryptedAESKeys {
			log.Printf("[Sync]   - key for: %s...", fp[:min(20, len(fp))])
		}
		entryData[i] = p2p.EntryData{
			ID:                e.ID,
			Site:              e.Site,
			Username:          e.Username,
			EncryptedPassword: e.EncryptedPassword,
			EncryptedAESKeys:  e.EncryptedAESKeys,
			Notes:             e.Notes,
			Version:           int(e.Version),
			CreatedAt:         e.CreatedAt.UnixMilli(),
			UpdatedAt:         e.UpdatedAt.UnixMilli(),
		}
	}
	vaultLock.Unlock()

	syncMsg, err := p2p.CreateSyncDataMessage(entryData, deviceList)
	if err != nil {
		log.Printf("[Sync] Failed to create sync message: %v", err)
		return
	}

	log.Printf("[Sync] Sending sync data to %s", peerID)
	err = pm.SendMessage(peerID, p2p.SyncMessage{
		Type:    syncMsg.Type,
		Payload: syncMsg.Payload,
	})
	if err != nil {
		log.Printf("[Sync] Failed to send sync data: %v", err)
	} else {
		log.Printf("[Sync] Sent %d entries and %d devices to %s",
			len(entryData), len(deviceList), peerID)
	}
}

func handleJoinerPairingRequest(pm *p2p.P2PManager, msg p2p.ReceivedMessage) {
	var pairingReq p2p.PairingRequestPayload
	if err := json.Unmarshal(msg.Payload, &pairingReq); err != nil {
		log.Printf("[Pairing] Failed to parse request: %v", err)
		return
	}

	log.Printf("[Pairing] Received pairing request: code=%s from=%s", pairingReq.Code, pairingReq.DeviceName)

	pairingLock.Lock()
	req, exists := pairingRequests[pairingReq.Code]
	pairingLock.Unlock()

	var response p2p.PairingResponsePayload
	if !exists {
		response = p2p.PairingResponsePayload{Success: false, Error: "no_pending_request"}
	} else {
		response = p2p.PairingResponsePayload{
			Success:    true,
			Code:       pairingReq.Code,
			DeviceID:   req.DeviceID,
			DeviceName: req.DeviceName,
		}
		log.Printf("[Pairing] Validated code %s from joining device", pairingReq.Code)
	}

	log.Printf("[Pairing] Join: Sending response to peer: %s", msg.FromPeer)
	respMsg, _ := p2p.CreatePairingResponseMessage(response.Success, response.Code, "", response.VaultID, response.DeviceID, response.DeviceName, "", "", response.Error)
	pm.SendMessage(msg.FromPeer, p2p.SyncMessage{Type: respMsg.Type, Payload: respMsg.Payload})
}

func handleJoinerPairingResponse(msg p2p.ReceivedMessage) {
	var pairingResp p2p.PairingResponsePayload
	if err := json.Unmarshal(msg.Payload, &pairingResp); err != nil {
		log.Printf("[Pairing] Failed to parse response: %v", err)
		return
	}

	log.Printf("[Pairing] Received response: success=%v from %s", pairingResp.Success, pairingResp.DeviceName)

	if pairingResponseCh != nil {
		pairingResponseCh <- pairingResp
	}
}

func handleJoinerSyncData(msg p2p.ReceivedMessage) {
	var syncData p2p.SyncDataPayload
	if err := json.Unmarshal(msg.Payload, &syncData); err != nil {
		log.Printf("[Sync] Failed to parse sync data: %v", err)
		return
	}

	log.Printf("[Sync] Received %d entries from peer", len(syncData.Entries))

	vaultLock.Lock()
	defer vaultLock.Unlock()

	log.Printf("[Sync] Joiner: vault=%v, storage=%v, privateKey=%v",
		vault != nil,
		vault != nil && vault.storage != nil,
		vault != nil && vault.privateKey != nil)

	if vault == nil {
		log.Printf("[Sync] ERROR: vault is nil - need to create/initialize vault first")
		return
	}
	if vault.storage == nil {
		log.Printf("[Sync] ERROR: vault.storage is nil - vault not unlocked")
		return
	}

	for _, entryData := range syncData.Entries {
		entry := models.PasswordEntry{
			ID:                entryData.ID,
			Version:           int64(entryData.Version),
			Site:              entryData.Site,
			Username:          entryData.Username,
			EncryptedPassword: entryData.EncryptedPassword,
			EncryptedAESKeys:  entryData.EncryptedAESKeys,
			Notes:             entryData.Notes,
			CreatedAt:         time.UnixMilli(entryData.CreatedAt),
			UpdatedAt:         time.UnixMilli(entryData.UpdatedAt),
		}

		existing, _ := vault.storage.GetEntry(entry.ID)
		if existing != nil {
			vault.storage.UpdateEntry(&entry)
			log.Printf("[Sync] Updated entry: %s", entry.Site)
		} else {
			vault.storage.CreateEntry(&entry)
			log.Printf("[Sync] Created entry: %s", entry.Site)
		}
	}

	for _, deviceData := range syncData.Devices {
		// Check if device already exists to avoid duplicates
		existingDevice, _ := vault.storage.GetDevice(deviceData.ID)
		if existingDevice != nil {
			log.Printf("[Sync] Device already exists: %s", deviceData.Name)
			continue
		}

		device := models.Device{
			ID:          deviceData.ID,
			Name:        deviceData.Name,
			PublicKey:   deviceData.PublicKey,
			Fingerprint: deviceData.Fingerprint,
			Trusted:     deviceData.Trusted,
			CreatedAt:   time.UnixMilli(deviceData.CreatedAt),
		}
		vault.storage.UpsertDevice(&device)
		log.Printf("[Sync] Added device: %s", device.Name)
	}
}
