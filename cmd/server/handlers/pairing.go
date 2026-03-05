package handlers

import (
	"context"
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
	"time"

	"github.com/bok1c4/pwman/internal/api"
	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/crypto"
	"github.com/bok1c4/pwman/internal/p2p"
	"github.com/bok1c4/pwman/internal/state"
	"github.com/bok1c4/pwman/internal/storage"
	"github.com/bok1c4/pwman/pkg/models"
	"github.com/google/uuid"
)

type PairingJoinRequest struct {
	Code       string `json:"code"`
	DeviceName string `json:"device_name"`
	Password   string `json:"password"`
}

type PairingHandlers struct {
	state       *state.ServerState
	authManager *api.AuthManager
	p2pHandlers *P2PHandlers
}

func NewPairingHandlers(s *state.ServerState, am *api.AuthManager, p2pHandlers *P2PHandlers) *PairingHandlers {
	return &PairingHandlers{
		state:       s,
		authManager: am,
		p2pHandlers: p2pHandlers,
	}
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

func (h *PairingHandlers) Generate(w http.ResponseWriter, r *http.Request) {
	vault, ok := h.state.GetVault()
	needsUnlock := !ok || vault == nil || vault.PrivateKey == nil

	if needsUnlock {
		api.Error(w, http.StatusBadRequest, "VAULT_LOCKED", "Unlock vault to generate pairing code")
		return
	}

	pm, running := h.state.GetP2PManager()
	if !running || pm == nil || !pm.IsRunning() {
		deviceName := ""
		deviceID := ""

		vault, ok := h.state.GetVault()
		if ok && vault != nil && vault.Config != nil {
			deviceName = vault.Config.DeviceName
			deviceID = vault.Config.DeviceID
		}

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
				h.state.SetP2PManager(manager)

				ctx, cancel := context.WithCancel(context.Background())
				h.state.SetP2PCancel(cancel)

				h.p2pHandlers.StartP2PEventLoop(ctx, manager)
			}
		}
	}

	vault, _ = h.state.GetVault()
	if vault == nil || vault.Config == nil {
		api.Error(w, http.StatusInternalServerError, "NO_VAULT", "no vault available")
		return
	}

	deviceID := vault.Config.DeviceID
	deviceName := vault.Config.DeviceName
	vaultName := vault.VaultName
	publicKeyPath := config.PublicKeyPathForVault(vaultName)
	log.Printf("[Pairing] DeviceName: '%s', vaultName: '%s', publicKeyPath: '%s'", deviceName, vaultName, publicKeyPath)

	log.Printf("[Pairing] Looking for public key at: %s", publicKeyPath)
	publicKeyBytes, err := os.ReadFile(publicKeyPath)
	if err != nil {
		log.Printf("[Pairing] Error reading public key: %v", err)
		api.Error(w, http.StatusInternalServerError, "KEY_ERROR", "failed to read public key")
		return
	}

	publicKey := string(publicKeyBytes)
	code := generatePairingCode()
	normalizedCode := strings.ToUpper(strings.ReplaceAll(code, "-", ""))
	fingerprint := deviceID

	activeVault, _ := config.GetActiveVault()

	h.state.AddPairingCode(normalizedCode, state.PairingCode{
		Code:        code,
		VaultID:     activeVault,
		VaultName:   vaultName,
		DeviceID:    deviceID,
		DeviceName:  deviceName,
		PublicKey:   publicKey,
		Fingerprint: fingerprint,
		ExpiresAt:   time.Now().Add(5 * time.Minute),
		Used:        false,
	})

	log.Printf("[Pairing] Generated code: %s for device: %s", code, deviceName)

	api.Success(w, map[string]interface{}{
		"code":        code,
		"device_name": deviceName,
		"expires_in":  300,
	})
}

func (h *PairingHandlers) Join(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		api.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	pm, running := h.state.GetP2PManager()
	if !running || pm == nil || !pm.IsRunning() {
		deviceName := ""
		deviceID := ""

		vault, ok := h.state.GetVault()
		if ok && vault != nil && vault.Config != nil {
			deviceName = vault.Config.DeviceName
			deviceID = vault.Config.DeviceID
		}

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
				h.state.SetP2PManager(manager)

				ctx, cancel := context.WithCancel(context.Background())
				h.state.SetP2PCancel(cancel)

				go func(ctx context.Context) {
					for {
						select {
						case <-ctx.Done():
							log.Printf("[P2P] Join event loop stopped")
							return
						case peer := <-manager.ConnectedChan():
							log.Printf("[P2P] Join: Received connected event: %s", peer.ID)
							h.state.AddPendingApproval(peer.ID, state.PendingApproval{
								DeviceID:    peer.ID,
								DeviceName:  peer.Name,
								Status:      "pending",
								ConnectedAt: time.Now(),
							})
						case peerID := <-manager.DisconnectedChan():
							log.Printf("[P2P] Join: Peer disconnected: %s", peerID)
						case msg := <-manager.PairingRequestChan():
							log.Printf("[P2P] Join: Pairing request from %s", msg.FromPeer)
							h.HandleJoinerPairingRequest(manager, msg)
						case msg := <-manager.PairingResponseChan():
							log.Printf("[P2P] Join: Pairing response from %s", msg.FromPeer)
							h.HandleJoinerPairingResponse(msg)
						}
					}
				}(ctx)
			}
		}

		time.Sleep(2 * time.Second)
	}

	var req PairingJoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	req.Code = strings.ToUpper(strings.ReplaceAll(req.Code, "-", ""))

	log.Printf("[Pairing Join] Looking for code: '%s' via P2P", req.Code)

	joiningDeviceID := ""
	joiningDeviceName := req.DeviceName

	vault, ok := h.state.GetVault()
	if ok && vault != nil && vault.Config != nil {
		joiningDeviceID = vault.Config.DeviceID
	}

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

	h.state.AddPairingRequest(req.Code, state.PairingRequest{
		Code:       req.Code,
		DeviceID:   joiningDeviceID,
		DeviceName: joiningDeviceName,
	})

	log.Printf("[Pairing Join] Stored request for code: %s, sending to discovered peers...", req.Code)

	responseCh := make(chan p2p.PairingResponsePayload, 10)
	h.state.SetPairingResponseChannel(responseCh)

	pm, _ = h.state.GetP2PManager()
	if pm != nil && pm.IsRunning() {
		peers := pm.GetAllPeers()
		log.Printf("[Pairing Join] Found %d discovered peers", len(peers))

		joinerPublicKey := ""
		vaultInitialized := false
		vault, ok := h.state.GetVault()
		if ok && vault != nil {
			if vault.Config != nil {
				pubKeyPath := config.PublicKeyPathForVault(vault.VaultName)
				pubKeyBytes, err := os.ReadFile(pubKeyPath)
				if err == nil {
					joinerPublicKey = string(pubKeyBytes)
					vaultInitialized = true
				} else {
					log.Printf("[Pairing Join] Warning: No public key found at %s - vault may not be initialized", pubKeyPath)
				}
			}
		}

		if !vaultInitialized {
			log.Printf("[Pairing Join] ERROR: Vault not initialized on joining device. Please initialize your vault first before pairing.")
		}

		for _, peer := range peers {
			go func(peerID string) {
				for attempt := 0; attempt < 10; attempt++ {
					msg, err := p2p.CreatePairingRequestMessage(req.Code, joiningDeviceID, joiningDeviceName, joinerPublicKey)
					if err != nil {
						log.Printf("[Pairing Join] Failed to create message: %v", err)
						return
					}

					err = pm.SendMessage(peerID, p2p.SyncMessage{
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

	select {
	case response := <-responseCh:
		h.state.SetPairingResponseChannel(nil)
		if !response.Success {
			api.Error(w, http.StatusBadRequest, "PAIRING_FAILED", response.Error)
			return
		}

		log.Printf("[Pairing Join] Received valid response from: %s", response.DeviceName)

		h.state.AddPendingApproval(response.DeviceID, state.PendingApproval{
			DeviceID:    response.DeviceID,
			DeviceName:  response.DeviceName,
			PublicKey:   response.PublicKey,
			Fingerprint: response.Fingerprint,
			Status:      "paired",
		})

		vaultName := response.VaultName
		joinPassword := req.Password

		log.Printf("[Pairing Join] Vault name from generator: %s", vaultName)

		vault, vaultExists := h.state.GetVault()
		if !vaultExists || vault == nil {
			log.Printf("[Pairing Join] Creating vault '%s' from scratch...", vaultName)

			vaultPath := config.VaultPath(vaultName)
			if err := os.MkdirAll(vaultPath, 0700); err != nil {
				log.Printf("[Pairing Join] Failed to create vault directory: %v", err)
				api.Error(w, http.StatusInternalServerError, "VAULT_ERROR", "failed to create vault: "+err.Error())
				return
			}

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

			if err := config.SetActiveVault(vaultName); err != nil {
				log.Printf("[Pairing Join] Failed to set active vault: %v", err)
			}

			keyPair, err := crypto.GenerateRSAKeyPair(4096)
			if err != nil {
				log.Printf("[Pairing Join] Failed to generate keys: %v", err)
				api.Error(w, http.StatusInternalServerError, "KEY_ERROR", "failed to generate keys: "+err.Error())
				return
			}

			salt, err := crypto.EncryptPrivateKeyAndSave(keyPair.PrivateKey, joinPassword, config.PrivateKeyPathForVault(vaultName))
			if err != nil {
				log.Printf("[Pairing Join] Failed to encrypt private key: %v", err)
				api.Error(w, http.StatusInternalServerError, "KEY_ERROR", "failed to encrypt private key: "+err.Error())
				return
			}

			if err := crypto.SavePublicKey(keyPair.PublicKey, config.PublicKeyPathForVault(vaultName)); err != nil {
				log.Printf("[Pairing Join] Failed to save public key: %v", err)
				api.Error(w, http.StatusInternalServerError, "KEY_ERROR", "failed to save public key: "+err.Error())
				return
			}

			db, err := storage.NewSQLite(config.DatabasePathForVault(vaultName))
			if err != nil {
				log.Printf("[Pairing Join] Failed to initialize database: %v", err)
				api.Error(w, http.StatusInternalServerError, "DB_ERROR", "failed to initialize database: "+err.Error())
				return
			}

			deviceID := uuid.New().String()
			publicKeyBytes, _ := os.ReadFile(config.PublicKeyPathForVault(vaultName))
			selfDevice := models.Device{
				ID:          deviceID,
				Name:        joiningDeviceName,
				PublicKey:   string(publicKeyBytes),
				Fingerprint: crypto.GetFingerprint(keyPair.PublicKey),
				Trusted:     true,
				CreatedAt:   time.Now(),
			}
			db.UpsertDevice(&selfDevice)

			cfg := &config.Config{
				DeviceID:   deviceID,
				DeviceName: joiningDeviceName,
				Salt:       base64.StdEncoding.EncodeToString(salt),
			}
			cfgBytes, _ := json.Marshal(cfg)
			os.WriteFile(config.VaultConfigPath(vaultName), cfgBytes, 0600)

			newVault := &state.Vault{
				PrivateKey: keyPair,
				Storage:    db,
				Config:     cfg,
				VaultName:  vaultName,
			}
			h.state.SetVault(newVault)

			log.Printf("[Pairing Join] Created vault '%s' with device %s, storage=%v", vaultName, joiningDeviceName, newVault.Storage != nil)
		}

		vault, _ = h.state.GetVault()
		log.Printf("[Pairing Join] After vault creation: vault=%v, storage=%v", vault != nil, vault != nil && vault.Storage != nil)

		pm, _ = h.state.GetP2PManager()
		var generatorPeerID string
		if pm != nil {
			peers := pm.GetAllPeers()
			for _, p := range peers {
				if p.ID == response.DeviceID || p.Name == response.DeviceName {
					generatorPeerID = p.ID
					break
				}
			}
			if generatorPeerID == "" && len(peers) > 0 {
				generatorPeerID = peers[len(peers)-1].ID
			}
		}

		vault, _ = h.state.GetVault()
		if vault != nil && vault.PrivateKey != nil && generatorPeerID != "" {
			log.Printf("[Pairing Join] Sending updated pairing request with public key to %s...", generatorPeerID)

			pubKeyPath := config.PublicKeyPathForVault(vaultName)
			if pubKeyBytes, err := os.ReadFile(pubKeyPath); err == nil {
				joinerPublicKey := string(pubKeyBytes)

				msg, err := p2p.CreatePairingRequestMessage(req.Code, vault.Config.DeviceID, vault.Config.DeviceName, joinerPublicKey)
				if err != nil {
					log.Printf("[Pairing Join] Failed to create updated request: %v", err)
				} else {
					err = pm.SendMessage(generatorPeerID, p2p.SyncMessage{
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

		log.Printf("[Pairing Join] Requesting vault sync from %s...", response.DeviceName)

		if generatorPeerID != "" {
			syncMsg, err := p2p.CreateRequestSyncMessage(0, true)
			if err != nil {
				log.Printf("[Pairing Join] Failed to create sync request: %v", err)
			} else {
				err = pm.SendMessage(generatorPeerID, p2p.SyncMessage{
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

		log.Printf("[Pairing Join] Waiting for vault sync from %s...", response.DeviceName)

		vaultCreated := false
		syncTimeout := time.After(30 * time.Second)

		for {
			select {
			case msg := <-pm.SyncDataChan():
				log.Printf("[Pairing Join] Received sync data from %s", msg.FromPeer)
				h.HandleJoinerSyncData(msg)
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

		api.Success(w, map[string]interface{}{
			"message":         "Connected to vault",
			"device_name":     response.DeviceName,
			"device_approved": true,
			"vault_synced":    vaultCreated,
		})
		return

	case <-time.After(15 * time.Second):
		h.state.SetPairingResponseChannel(nil)
		api.Error(w, http.StatusGatewayTimeout, "TIMEOUT", "no_response_from_vault")
		return
	}
}

func (h *PairingHandlers) Status(w http.ResponseWriter, r *http.Request) {
	codes := h.state.GetAllPairingCodes()
	var activeCode *state.PairingCode
	for _, c := range codes {
		if !c.Used && time.Now().Before(c.ExpiresAt) {
			activeCode = &c
			break
		}
	}

	if activeCode == nil {
		api.Success(w, map[string]interface{}{
			"active": false,
		})
		return
	}

	api.Success(w, map[string]interface{}{
		"active":      true,
		"device_name": activeCode.DeviceName,
		"expires_in":  int(time.Until(activeCode.ExpiresAt).Seconds()),
	})
}

func (h *PairingHandlers) HandlePairingRequest(pm *p2p.P2PManager, msg p2p.ReceivedMessage) {
	var pairingReq p2p.PairingRequestPayload
	if err := json.Unmarshal(msg.Payload, &pairingReq); err != nil {
		log.Printf("[Pairing] Failed to parse request: %v", err)
		return
	}

	log.Printf("[Pairing] Received pairing request with code: %s from %s", pairingReq.Code, pairingReq.DeviceName)

	var response p2p.PairingResponsePayload

	code, exists := h.state.GetPairingCode(pairingReq.Code)

	if !exists {
		response = p2p.PairingResponsePayload{Success: false, Error: "invalid_code"}
	} else if code.Used {
		response = p2p.PairingResponsePayload{Success: false, Error: "code_already_used"}
	} else if time.Now().After(code.ExpiresAt) {
		response = p2p.PairingResponsePayload{Success: false, Error: "code_expired"}
	} else {
		code.Used = true
		h.state.UpdatePairingCode(pairingReq.Code, code)

		joinerFingerprint := ""
		if pairingReq.PublicKey != "" {
			if pubKey, err := crypto.ParsePublicKey(pairingReq.PublicKey); err == nil {
				joinerFingerprint = crypto.GetFingerprint(pubKey)
			}
		}
		if joinerFingerprint == "" {
			joinerFingerprint = pairingReq.DeviceID
			log.Printf("[Pairing] WARNING: Joiner %s did not provide a public key", pairingReq.DeviceName)
		}

		storage, ok := h.state.GetVaultStorage()
		if ok {
			existingDevice, _ := storage.GetDevice(pairingReq.DeviceID)
			if existingDevice != nil {
				log.Printf("[Pairing] Updating existing device: %s", pairingReq.DeviceName)
				existingDevice.PublicKey = pairingReq.PublicKey
				existingDevice.Fingerprint = joinerFingerprint
				existingDevice.Trusted = true
				storage.UpsertDevice(existingDevice)
			} else {
				device := models.Device{
					ID:          pairingReq.DeviceID,
					Name:        pairingReq.DeviceName,
					PublicKey:   pairingReq.PublicKey,
					Fingerprint: joinerFingerprint,
					Trusted:     true,
					CreatedAt:   time.Now(),
				}
				storage.UpsertDevice(&device)
				log.Printf("[Pairing] Added joiner %s as trusted device (fingerprint: %s)", pairingReq.DeviceName, joinerFingerprint)
			}

			vault, _ := h.state.GetVault()
			if vault != nil && vault.Config != nil && vault.PrivateKey != nil && vault.PrivateKey.PublicKey != nil {
				selfDevice, _ := storage.GetDevice(vault.Config.DeviceID)
				if selfDevice != nil && selfDevice.PublicKey == "" {
					pubKeyPath := config.PublicKeyPathForVault(vault.VaultName)
					if pubKeyBytes, err := os.ReadFile(pubKeyPath); err == nil {
						selfDevice.PublicKey = string(pubKeyBytes)
						selfDevice.Fingerprint = crypto.GetFingerprint(vault.PrivateKey.PublicKey)
						storage.UpsertDevice(selfDevice)
						log.Printf("[Pairing] Updated own device with public key")
					}
				}
			}

			if pairingReq.PublicKey != "" {
				go h.reEncryptEntriesForDevice(
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

func (h *PairingHandlers) HandlePairingResponse(msg p2p.ReceivedMessage) {
	var pairingResp p2p.PairingResponsePayload
	if err := json.Unmarshal(msg.Payload, &pairingResp); err != nil {
		log.Printf("[Pairing] Failed to parse response: %v", err)
		return
	}

	ch := h.state.GetPairingResponseChannel()
	if ch != nil {
		ch <- pairingResp
	}
}

func (h *PairingHandlers) HandleSyncRequest(pm *p2p.P2PManager, peerID string) {
	log.Printf("[Sync] Received sync request from %s", peerID)

	vault, ok := h.state.GetVault()
	if !ok || vault == nil || vault.Storage == nil || vault.PrivateKey == nil {
		log.Printf("[Sync] Cannot respond: vault not available")
		return
	}

	if vault.Config != nil && vault.PrivateKey != nil && vault.PrivateKey.PublicKey != nil {
		selfDevice, _ := vault.Storage.GetDevice(vault.Config.DeviceID)
		if selfDevice != nil {
			pubKeyPath := config.PublicKeyPathForVault(vault.VaultName)
			if pubKeyBytes, err := os.ReadFile(pubKeyPath); err == nil {
				selfDevice.PublicKey = string(pubKeyBytes)
				selfDevice.Fingerprint = crypto.GetFingerprint(vault.PrivateKey.PublicKey)
				vault.Storage.UpsertDevice(selfDevice)
				log.Printf("[Sync] Updated own device with public key before sync")
			}
		}
	}

	entries, _ := vault.Storage.ListEntries()
	devices, _ := vault.Storage.ListDevices()
	log.Printf("[Sync] Found %d entries, %d devices", len(entries), len(devices))

	seen := make(map[string]bool)
	deviceList := []p2p.DeviceData{}
	for _, d := range devices {
		if seen[d.ID] {
			log.Printf("[Sync] Skipping duplicate device: %s", d.Name)
			continue
		}
		seen[d.ID] = true

		publicKey := d.PublicKey
		if len(publicKey) > 0 && publicKey[0] == '/' {
			if pubKeyBytes, err := os.ReadFile(publicKey); err == nil {
				publicKey = string(pubKeyBytes)
			} else if pubKeyBytes, err := os.ReadFile(config.PublicKeyPathForVault(vault.VaultName)); err == nil {
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

func (h *PairingHandlers) HandleJoinerPairingRequest(pm *p2p.P2PManager, msg p2p.ReceivedMessage) {
	var pairingReq p2p.PairingRequestPayload
	if err := json.Unmarshal(msg.Payload, &pairingReq); err != nil {
		log.Printf("[Pairing] Failed to parse request: %v", err)
		return
	}

	log.Printf("[Pairing] Received pairing request: code=%s from=%s", pairingReq.Code, pairingReq.DeviceName)

	req, exists := h.state.GetPairingRequest(pairingReq.Code)

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

func (h *PairingHandlers) HandleJoinerPairingResponse(msg p2p.ReceivedMessage) {
	var pairingResp p2p.PairingResponsePayload
	if err := json.Unmarshal(msg.Payload, &pairingResp); err != nil {
		log.Printf("[Pairing] Failed to parse response: %v", err)
		return
	}

	log.Printf("[Pairing] Received response: success=%v from %s", pairingResp.Success, pairingResp.DeviceName)

	ch := h.state.GetPairingResponseChannel()
	if ch != nil {
		ch <- pairingResp
	}
}

func (h *PairingHandlers) HandleJoinerSyncData(msg p2p.ReceivedMessage) {
	var syncData p2p.SyncDataPayload
	if err := json.Unmarshal(msg.Payload, &syncData); err != nil {
		log.Printf("[Sync] Failed to parse sync data: %v", err)
		return
	}

	log.Printf("[Sync] Received %d entries from peer", len(syncData.Entries))

	storage, ok := h.state.GetVaultStorage()
	if !ok {
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

		existing, _ := storage.GetEntry(entry.ID)
		if existing != nil {
			storage.UpdateEntry(&entry)
			log.Printf("[Sync] Updated entry: %s", entry.Site)
		} else {
			storage.CreateEntry(&entry)
			log.Printf("[Sync] Created entry: %s", entry.Site)
		}
	}

	for _, deviceData := range syncData.Devices {
		existingDevice, _ := storage.GetDevice(deviceData.ID)
		if existingDevice != nil {
			log.Printf("[Sync] Device already exists (by ID): %s", deviceData.Name)
			continue
		}

		allDevices, _ := storage.ListDevices()
		deviceExists := false
		for _, d := range allDevices {
			log.Printf("[Sync] Comparing fingerprint: %s (sync) vs %s (local)",
				deviceData.Fingerprint[:min(30, len(deviceData.Fingerprint))],
				d.Fingerprint[:min(30, len(d.Fingerprint))])
			if d.Fingerprint == deviceData.Fingerprint {
				log.Printf("[Sync] Device already exists (by fingerprint): %s (ID=%s)", deviceData.Name, d.ID)
				deviceExists = true
				break
			}
		}

		if !deviceExists {
			device := models.Device{
				ID:          deviceData.ID,
				Name:        deviceData.Name,
				PublicKey:   deviceData.PublicKey,
				Fingerprint: deviceData.Fingerprint,
				Trusted:     deviceData.Trusted,
				CreatedAt:   time.UnixMilli(deviceData.CreatedAt),
			}
			storage.UpsertDevice(&device)
			log.Printf("[Sync] Added device: %s (ID=%s)", device.Name, device.ID)
		}
	}

	log.Printf("[Sync] Sync complete: %d entries, %d devices", len(syncData.Entries), len(syncData.Devices))
}

func (h *PairingHandlers) reEncryptEntriesForDevice(peerID, deviceID, deviceName, publicKey, fingerprint string) {
	vault, ok := h.state.GetVault()
	if !ok || vault == nil || vault.Storage == nil || vault.PrivateKey == nil {
		log.Printf("[Sync] Cannot re-encrypt: vault not unlocked")
		return
	}

	privateKey := vault.PrivateKey.PrivateKey
	db := vault.Storage
	cfg := vault.Config

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
		password, err := crypto.HybridDecrypt(entry, privateKey)
		if err != nil {
			log.Printf("[Sync] Failed to decrypt entry %s: %v", entry.ID, err)
			continue
		}

		log.Printf("[Sync] Decrypted password for entry %s", entry.Site)

		generatorDevice, _ := db.GetDevice(cfg.DeviceID)

		allDevices := []models.Device{*generatorDevice, newDevice}
		allGetPublicKey := func(fp string) (*rsa.PublicKey, error) {
			if fp == fingerprint {
				return crypto.ParsePublicKey(publicKey)
			}
			if fp == generatorDevice.Fingerprint {
				return crypto.ParsePublicKey(generatorDevice.PublicKey)
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

		entry.EncryptedPassword = encrypted.EncryptedPassword
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

	devices, _ := db.ListDevices()
	deviceList := []p2p.DeviceData{}

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

	deviceList = append(deviceList, p2p.DeviceData{
		ID:          newDevice.ID,
		Name:        newDevice.Name,
		PublicKey:   newDevice.PublicKey,
		Fingerprint: newDevice.Fingerprint,
		Trusted:     newDevice.Trusted,
		CreatedAt:   newDevice.CreatedAt.UnixMilli(),
	})

	log.Printf("[Sync] Waiting for joiner to be ready...")
	time.Sleep(2 * time.Second)

	log.Printf("[Sync] Re-encryption complete, waiting for joiner to request sync...")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
