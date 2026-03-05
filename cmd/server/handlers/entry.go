package handlers

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/bok1c4/pwman/internal/api"
	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/crypto"
	"github.com/bok1c4/pwman/internal/state"
	"github.com/bok1c4/pwman/pkg/models"
	"github.com/google/uuid"
)

type EntryHandlers struct {
	state *state.ServerState
}

func NewEntryHandlers(s *state.ServerState) *EntryHandlers {
	return &EntryHandlers{state: s}
}

type EntryResponse struct {
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

func (h *EntryHandlers) List(w http.ResponseWriter, r *http.Request) {
	storage, ok := h.state.GetVaultStorage()
	if !ok {
		api.Success(w, []EntryResponse{})
		return
	}

	entries, err := storage.ListEntries()
	if err != nil {
		api.Success(w, []EntryResponse{})
		return
	}

	result := make([]EntryResponse, len(entries))
	for i, e := range entries {
		result[i] = EntryResponse{
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

	api.Success(w, result)
}

func (h *EntryHandlers) Add(w http.ResponseWriter, r *http.Request) {
	type AddRequest struct {
		Site     string `json:"site"`
		Username string `json:"username"`
		Password string `json:"password"`
		Notes    string `json:"notes"`
	}

	var req AddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	site, err := api.ValidateSite(req.Site)
	if err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	username, err := api.ValidateUsername(req.Username)
	if err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	if err := api.ValidatePassword(req.Password); err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	notes, err := api.ValidateNotes(req.Notes)
	if err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	req.Site = site
	req.Username = username
	req.Notes = notes

	vault, ok := h.state.GetVault()
	if !ok {
		api.Error(w, http.StatusForbidden, "VAULT_LOCKED", "vault not unlocked")
		return
	}

	devices, _ := vault.Storage.ListDevices()
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
			ID:          vault.Config.DeviceID,
			Fingerprint: crypto.GetFingerprint(vault.PrivateKey.PublicKey),
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
		api.Error(w, http.StatusInternalServerError, "ENCRYPTION_FAILED", "failed to encrypt: "+err.Error())
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
		UpdatedBy:         vault.Config.DeviceID,
	}

	if err := vault.Storage.CreateEntry(&entry); err != nil {
		api.Error(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	api.Success(w, map[string]string{"id": entry.ID})
}

func (h *EntryHandlers) Update(w http.ResponseWriter, r *http.Request) {
	type UpdateRequest struct {
		ID       string `json:"id"`
		Site     string `json:"site"`
		Username string `json:"username"`
		Password string `json:"password"`
		Notes    string `json:"notes"`
	}

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	site, err := api.ValidateSite(req.Site)
	if err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	username, err := api.ValidateUsername(req.Username)
	if err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	if err := api.ValidatePassword(req.Password); err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	notes, err := api.ValidateNotes(req.Notes)
	if err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	req.Site = site
	req.Username = username
	req.Notes = notes

	vault, ok := h.state.GetVault()
	if !ok {
		api.Error(w, http.StatusForbidden, "VAULT_LOCKED", "vault not unlocked")
		return
	}

	existingEntry, err := vault.Storage.GetEntry(req.ID)
	if err != nil {
		api.NotFound(w, "entry not found")
		return
	}

	devices, _ := vault.Storage.ListDevices()
	var trustedDevices []models.Device
	for _, d := range devices {
		if d.Trusted {
			trustedDevices = append(trustedDevices, d)
		}
	}

	if len(trustedDevices) == 0 {
		activeVault, _ := config.GetActiveVault()
		trustedDevices = append(trustedDevices, models.Device{
			ID:          vault.Config.DeviceID,
			Fingerprint: crypto.GetFingerprint(vault.PrivateKey.PublicKey),
			PublicKey:   config.PublicKeyPathForVault(activeVault),
		})
	}

	getPublicKey := func(fingerprint string) (*rsa.PublicKey, error) {
		for _, d := range trustedDevices {
			if d.Fingerprint == fingerprint {
				// Check if PublicKey is actual key content or file path
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
		api.Error(w, http.StatusInternalServerError, "ENCRYPTION_FAILED", "failed to encrypt: "+err.Error())
		return
	}

	existingEntry.Site = req.Site
	existingEntry.Username = req.Username
	existingEntry.EncryptedPassword = encrypted.EncryptedPassword
	existingEntry.EncryptedAESKeys = encrypted.EncryptedAESKeys
	existingEntry.Notes = req.Notes
	existingEntry.Version++
	existingEntry.UpdatedAt = time.Now()
	existingEntry.UpdatedBy = vault.Config.DeviceID

	if err := vault.Storage.UpdateEntry(existingEntry); err != nil {
		api.Error(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	api.Success(w, nil)
}

func (h *EntryHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	type DeleteRequest struct {
		ID string `json:"id"`
	}

	var req DeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	storage, ok := h.state.GetVaultStorage()
	if !ok {
		api.Error(w, http.StatusForbidden, "VAULT_LOCKED", "vault not unlocked")
		return
	}

	err := storage.DeleteEntry(req.ID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	api.Success(w, nil)
}

func (h *EntryHandlers) GetPassword(w http.ResponseWriter, r *http.Request) {
	vault, ok := h.state.GetVault()
	if !ok {
		api.Error(w, http.StatusForbidden, "VAULT_LOCKED", "vault not unlocked")
		return
	}

	var entryID string

	if site := r.URL.Query().Get("site"); site != "" {
		entries, _ := vault.Storage.ListEntries()
		for _, e := range entries {
			if e.Site == site {
				entryID = e.ID
				break
			}
		}
		if entryID == "" {
			api.NotFound(w, "entry not found for site: "+site)
			return
		}
	} else {
		type GetPasswordRequest struct {
			ID string `json:"id"`
		}

		var req GetPasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.BadRequest(w, err.Error())
			return
		}
		entryID = req.ID
	}

	entry, err := vault.Storage.GetEntry(entryID)
	if err != nil {
		api.NotFound(w, "entry not found")
		return
	}

	log.Printf("[GetPassword] Trying to decrypt entry %s with vault's private key (fingerprint available: %v)",
		entry.ID, vault.PrivateKey != nil)

	if entry.EncryptedAESKeys != nil {
		log.Printf("[GetPassword] Entry has %d encrypted keys: %v", len(entry.EncryptedAESKeys), func() []string {
			keys := []string{}
			for k := range entry.EncryptedAESKeys {
				keys = append(keys, k[:min(20, len(k))]+"...")
			}
			return keys
		}())
	}

	if vault.PrivateKey == nil || vault.PrivateKey.PublicKey == nil {
		log.Printf("[GetPassword] ERROR: vault.PrivateKey.PublicKey is nil! Cannot decrypt.")
		api.Error(w, http.StatusInternalServerError, "KEY_MISSING", "vault not properly initialized - public key missing")
		return
	}

	ourFingerprint := crypto.GetFingerprint(vault.PrivateKey.PublicKey)
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

	log.Printf("[GetPassword] Private key available: %v", vault.PrivateKey.PrivateKey != nil)
	log.Printf("[GetPassword] Public key in KeyPair: %v", vault.PrivateKey.PublicKey != nil)

	password, err := crypto.HybridDecrypt(entry, vault.PrivateKey.PrivateKey)
	if err != nil {
		log.Printf("[GetPassword] Decryption failed: %v", err)
		api.Error(w, http.StatusInternalServerError, "DECRYPTION_FAILED", "failed to decrypt password")
		return
	}

	api.Success(w, map[string]string{"password": password})
}
