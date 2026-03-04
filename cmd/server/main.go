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
	vault             *Vault
	vaultLock         sync.Mutex
	p2pManager        *p2p.P2PManager
	p2pLock           sync.Mutex
	pendingApprovals  = make(map[string]PendingApproval)
	approvalsLock     sync.Mutex
	pairingCodes      = make(map[string]PairingCode)
	pairingLock       sync.Mutex
	pairingResponseCh chan p2p.PairingResponsePayload
)

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
	DeviceID    string
	DeviceName  string
	PublicKey   string
	Fingerprint string
	ExpiresAt   time.Time
	Used        bool
}

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
		PublicKey:   config.PublicKeyPathForVault(vaultName),
		Fingerprint: crypto.GetFingerprint(keyPair.PublicKey),
		Trusted:     true,
		CreatedAt:   time.Now(),
	}

	if err := db.UpsertDevice(&device); err != nil {
		jsonResponse(w, Response{Success: false, Error: "failed to save device: " + err.Error()})
		return
	}

	vault = &Vault{
		privateKey: keyPair,
		storage:    db,
		cfg:        cfg,
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

	vault = &Vault{
		privateKey: &crypto.KeyPair{PrivateKey: privateKey},
		storage:    db,
		cfg:        vaultConfig,
		vaultName:  activeVault,
	}

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

	type GetPasswordRequest struct {
		ID string `json:"id"`
	}

	var req GetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, Response{Success: false, Error: err.Error()})
		return
	}

	entry, err := vault.storage.GetEntry(req.ID)
	if err != nil {
		jsonResponse(w, Response{Success: false, Error: "entry not found"})
		return
	}

	password, err := crypto.HybridDecrypt(entry, vault.privateKey.PrivateKey)
	if err != nil {
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
				log.Printf("[P2P] Received connected event: %s", peer.ID)
				approvalsLock.Lock()
				if _, exists := pendingApprovals[peer.ID]; !exists {
					pendingApprovals[peer.ID] = PendingApproval{
						DeviceID:    peer.ID,
						DeviceName:  peer.Name,
						Status:      "pending",
						ConnectedAt: time.Now(),
					}
					log.Printf("[P2P] Added to pending approvals: %s", peer.ID)
				} else {
					log.Printf("[P2P] Peer already in approvals: %s", peer.ID)
				}
				approvalsLock.Unlock()
			case peerID := <-p2pManager.DisconnectedChan():
				log.Printf("[P2P] Peer disconnected: %s", peerID)
			case msg := <-p2pManager.MessageChan():
				log.Printf("[P2P] Message received: %s from %s", msg.Type, msg.PeerID)

				// Handle pairing request
				if msg.Type == p2p.MsgTypePairingRequest {
					var pairingReq p2p.PairingRequestPayload
					if err := json.Unmarshal(msg.Payload, &pairingReq); err != nil {
						log.Printf("[Pairing] Failed to parse request: %v", err)
						continue
					}

					log.Printf("[Pairing] Received pairing request with code: %s from %s", pairingReq.Code, pairingReq.DeviceName)

					// Validate code
					pairingLock.Lock()
					code, exists := pairingCodes[pairingReq.Code]
					pairingLock.Unlock()

					var response p2p.PairingResponsePayload
					if !exists {
						response = p2p.PairingResponsePayload{Success: false, Error: "invalid_code"}
					} else if time.Now().After(code.ExpiresAt) {
						response = p2p.PairingResponsePayload{Success: false, Error: "code_expired"}
					} else if code.Used {
						response = p2p.PairingResponsePayload{Success: false, Error: "code_already_used"}
					} else {
						// Mark code as used
						pairingLock.Lock()
						code.Used = true
						pairingCodes[pairingReq.Code] = code
						pairingLock.Unlock()

						response = p2p.PairingResponsePayload{
							Success:     true,
							Code:        pairingReq.Code,
							VaultID:     code.VaultID,
							DeviceID:    code.DeviceID,
							DeviceName:  code.DeviceName,
							PublicKey:   code.PublicKey,
							Fingerprint: code.Fingerprint,
						}
						log.Printf("[Pairing] Validated code %s, responding to %s", pairingReq.Code, pairingReq.DeviceName)
					}

					// Send response back to the requesting peer
					respMsg, err := p2p.CreatePairingResponseMessage(
						response.Success,
						response.Code,
						response.VaultID,
						response.DeviceID,
						response.DeviceName,
						response.PublicKey,
						response.Fingerprint,
						response.Error,
					)
					if err != nil {
						log.Printf("[Pairing] Failed to create response: %v", err)
						continue
					}

					p2pManager.SendMessage(msg.PeerID, p2p.SyncMessage{
						Type:    respMsg.Type,
						Payload: respMsg.Payload,
					})
				}

				// Handle pairing response
				if msg.Type == p2p.MsgTypePairingResponse {
					var pairingResp p2p.PairingResponsePayload
					if err := json.Unmarshal(msg.Payload, &pairingResp); err != nil {
						log.Printf("[Pairing] Failed to parse response: %v", err)
						continue
					}

					log.Printf("[Pairing] Received response: success=%v from %s", pairingResp.Success, pairingResp.DeviceName)

					if pairingResponseCh != nil {
						pairingResponseCh <- pairingResp
					}
				}
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
						case msg := <-p2pManager.MessageChan():
							log.Printf("[P2P] Auto: Message received: %s from %s", msg.Type, msg.PeerID)

							// Handle pairing request
							if msg.Type == p2p.MsgTypePairingRequest {
								var pairingReq p2p.PairingRequestPayload
								if err := json.Unmarshal(msg.Payload, &pairingReq); err != nil {
									log.Printf("[Pairing] Failed to parse request: %v", err)
									continue
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
								} else if code.Used {
									response = p2p.PairingResponsePayload{Success: false, Error: "code_already_used"}
								} else {
									pairingLock.Lock()
									code.Used = true
									pairingCodes[pairingReq.Code] = code
									pairingLock.Unlock()

									response = p2p.PairingResponsePayload{
										Success:     true,
										Code:        pairingReq.Code,
										VaultID:     code.VaultID,
										DeviceID:    code.DeviceID,
										DeviceName:  code.DeviceName,
										PublicKey:   code.PublicKey,
										Fingerprint: code.Fingerprint,
									}
								}

								respMsg, err := p2p.CreatePairingResponseMessage(response.Success, response.Code, response.VaultID, response.DeviceID, response.DeviceName, response.PublicKey, response.Fingerprint, response.Error)
								if err != nil {
									log.Printf("[Pairing] Failed to create response: %v", err)
									continue
								}

								p2pManager.SendMessage(msg.PeerID, p2p.SyncMessage{Type: respMsg.Type, Payload: respMsg.Payload})
							}

							// Handle pairing response
							if msg.Type == p2p.MsgTypePairingResponse {
								var pairingResp p2p.PairingResponsePayload
								if err := json.Unmarshal(msg.Payload, &pairingResp); err != nil {
									log.Printf("[Pairing] Failed to parse response: %v", err)
									continue
								}

								if pairingResponseCh != nil {
									pairingResponseCh <- pairingResp
								}
							}
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
						case msg := <-p2pManager.MessageChan():
							log.Printf("[P2P] Join: Message received: %s from %s", msg.Type, msg.PeerID)

							// Handle pairing response
							if msg.Type == p2p.MsgTypePairingResponse {
								var pairingResp p2p.PairingResponsePayload
								if err := json.Unmarshal(msg.Payload, &pairingResp); err != nil {
									log.Printf("[Pairing] Failed to parse response: %v", err)
									continue
								}

								log.Printf("[Pairing] Received response: success=%v from %s", pairingResp.Success, pairingResp.DeviceName)

								if pairingResponseCh != nil {
									pairingResponseCh <- pairingResp
								}
							}
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

	log.Printf("[Pairing Join] Looking for code: '%s' via P2P", req.Code)

	// Get device info for the pairing request
	joiningDeviceID := ""
	joiningDeviceName := req.DeviceName

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

	// Send pairing request via P2P to all peers
	p2pLock.Lock()
	if p2pManager != nil && p2pManager.IsRunning() {
		// Create pairing request message
		msg, err := p2p.CreatePairingRequestMessage(req.Code, joiningDeviceID, joiningDeviceName)
		if err != nil {
			log.Printf("[Pairing Join] Failed to create message: %v", err)
		} else {
			// Broadcast to all peers
			p2pManager.BroadcastMessage(p2p.SyncMessage{
				Type:    msg.Type,
				Payload: msg.Payload,
			})
			log.Printf("[Pairing Join] Broadcast pairing request to all peers")
		}
	}
	p2pLock.Unlock()

	// Wait for response (with timeout)
	responseCh := make(chan p2p.PairingResponsePayload, 10)
	pairingResponseCh = responseCh

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

		jsonResponse(w, Response{Success: true, Data: map[string]interface{}{
			"message":         "Connected to vault",
			"device_name":     response.DeviceName,
			"device_approved": true,
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

	log.Println("Starting pwman API server on :18475")
	log.Fatal(http.ListenAndServe(":18475", nil))
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
