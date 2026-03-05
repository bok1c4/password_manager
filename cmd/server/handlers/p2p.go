package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/bok1c4/pwman/internal/api"
	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/p2p"
	"github.com/bok1c4/pwman/internal/state"
	"github.com/bok1c4/pwman/pkg/models"
)

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

type ConnectRequest struct {
	Address string `json:"address"`
}

type DisconnectRequest struct {
	PeerID string `json:"peer_id"`
}

type ApprovalRequest struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	PublicKey  string `json:"public_key"`
	Status     string `json:"status"`
}

type ApproveRequest struct {
	DeviceID string `json:"device_id"`
}

type RejectRequest struct {
	DeviceID string `json:"device_id"`
	Reason   string `json:"reason"`
}

type SyncRequest struct {
	FullSync bool `json:"full_sync"`
}

type P2PHandlers struct {
	state           *state.ServerState
	authManager     *api.AuthManager
	pairingHandlers *PairingHandlers
}

func NewP2PHandlers(s *state.ServerState, am *api.AuthManager) *P2PHandlers {
	return &P2PHandlers{
		state:       s,
		authManager: am,
	}
}

func (h *P2PHandlers) SetPairingHandlers(ph *PairingHandlers) {
	h.pairingHandlers = ph
}

func (h *P2PHandlers) Status(w http.ResponseWriter, r *http.Request) {
	pending := h.state.ListPendingApprovals()
	log.Printf("[DEBUG] Current pending approvals: %d", len(pending))
	for _, p := range pending {
		log.Printf("[DEBUG] - %s: %s", p.DeviceID, p.DeviceName)
	}

	pm, running := h.state.GetP2PManager()
	if !running || pm == nil || !pm.IsRunning() {
		api.Success(w, P2PStatusResponse{Running: false})
		return
	}

	response := P2PStatusResponse{
		Running:   true,
		PeerID:    pm.GetPeerID(),
		Addresses: pm.GetListenAddresses(),
	}

	peers := pm.GetConnectedPeers()
	for _, p := range peers {
		response.Connected = append(response.Connected, P2PPeerInfo{
			ID:        p.ID,
			Name:      p.Name,
			Connected: p.Connected,
		})
	}

	allPeers := pm.GetAllPeers()
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

	api.Success(w, response)
}

func (h *P2PHandlers) Start(w http.ResponseWriter, r *http.Request) {
	log.Printf("[%s] [P2P] handleP2PStart called", time.Now().Format("15:04:05"))

	pm, running := h.state.GetP2PManager()
	if running && pm != nil && pm.IsRunning() {
		log.Println("[P2P] Already running")
		api.Success(w, "P2P already running")
		return
	}

	log.Println("[P2P] Creating new P2P manager")

	deviceName := "Device"
	deviceID := ""

	vault, ok := h.state.GetVault()
	if ok && vault != nil && vault.Config != nil {
		deviceName = vault.Config.DeviceName
		deviceID = vault.Config.DeviceID
	}

	activeVault := ""
	if ok && vault != nil {
		activeVault = vault.VaultName
	}

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
		api.Error(w, http.StatusInternalServerError, "P2P_ERROR", "failed to create P2P manager")
		return
	}

	if err := manager.Start(); err != nil {
		api.Error(w, http.StatusInternalServerError, "P2P_ERROR", "failed to start P2P: "+err.Error())
		return
	}

	h.state.SetP2PManager(manager)

	ctx, cancel := context.WithCancel(context.Background())
	h.state.SetP2PCancel(cancel)

	h.StartP2PEventLoop(ctx, manager)

	api.Success(w, "P2P started")
}

func (h *P2PHandlers) StartP2PEventLoop(ctx context.Context, manager *p2p.P2PManager) {
	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				log.Printf("[P2P] Event loop stopped")
				return
			case peer := <-manager.ConnectedChan():
				log.Printf("[P2P] ========== PEER CONNECTED: %s ==========", peer.ID)
				log.Printf("[P2P] Connected peers: %d", len(manager.GetConnectedPeers()))
				log.Printf("[Pairing] Peer %s connected, waiting for pairing request...", peer.ID)
			case peerID := <-manager.DisconnectedChan():
				log.Printf("[P2P] Peer disconnected: %s", peerID)
			case msg := <-manager.PairingRequestChan():
				log.Printf("[P2P] Auto: Pairing request from %s", msg.FromPeer)
				if h.pairingHandlers != nil {
					h.pairingHandlers.HandlePairingRequest(manager, msg)
				}
			case msg := <-manager.PairingResponseChan():
				log.Printf("[P2P] Auto: Pairing response from %s", msg.FromPeer)
				if h.pairingHandlers != nil {
					h.pairingHandlers.HandlePairingResponse(msg)
				}
			case msg := <-manager.SyncRequestChan():
				log.Printf("[P2P] Auto: Sync request from %s", msg.FromPeer)
				if h.pairingHandlers != nil {
					h.pairingHandlers.HandleSyncRequest(manager, msg.FromPeer)
				}
			}
		}
	}(ctx)
}

func (h *P2PHandlers) Stop(w http.ResponseWriter, r *http.Request) {
	h.state.StopP2P()
	api.Success(w, "P2P stopped")
}

func (h *P2PHandlers) Peers(w http.ResponseWriter, r *http.Request) {
	log.Println("[P2P] handleP2PPeers called")

	pm, running := h.state.GetP2PManager()
	if !running || pm == nil {
		log.Println("[P2P] p2pManager is nil in peers")
		api.Success(w, []P2PPeerInfo{})
		return
	}

	log.Printf("[P2P] p2pManager running: %v", pm.IsRunning())

	peers := pm.GetConnectedPeers()
	log.Printf("[P2P] Connected peers count: %d", len(peers))

	allPeers := pm.GetAllPeers()
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

	api.Success(w, result)
}

func (h *P2PHandlers) Connect(w http.ResponseWriter, r *http.Request) {
	log.Printf("[%s] [P2P] handleP2PConnect called", time.Now().Format("15:04:05"))

	if r.Method != "POST" {
		api.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	pm, running := h.state.GetP2PManager()
	log.Printf("[P2P] p2pManager is nil: %v", !running)
	if !running || pm == nil {
		api.Error(w, http.StatusBadRequest, "P2P_NOT_STARTED", "P2P not started - run pwman p2p start first")
		return
	}
	if !pm.IsRunning() {
		api.Error(w, http.StatusBadRequest, "P2P_NOT_RUNNING", "P2P not running")
		return
	}

	var req ConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	log.Printf("[%s] [P2P] Attempting to connect to: %s", time.Now().Format("15:04:05"), req.Address)

	err := pm.ConnectToPeer(req.Address)
	if err != nil {
		log.Printf("[P2P] Connect error: %v", err)
		api.Error(w, http.StatusInternalServerError, "CONNECT_FAILED", "failed to connect: "+err.Error())
		return
	}

	log.Printf("[P2P] Connect succeeded to: %s", req.Address)
	api.Success(w, "Connected to peer")
}

func (h *P2PHandlers) Disconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		api.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	pm, running := h.state.GetP2PManager()
	if !running || pm == nil || !pm.IsRunning() {
		api.Error(w, http.StatusBadRequest, "P2P_NOT_RUNNING", "P2P not running")
		return
	}

	var req DisconnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	err := pm.DisconnectFromPeer(req.PeerID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "DISCONNECT_FAILED", "failed to disconnect: "+err.Error())
		return
	}

	api.Success(w, "Disconnected from peer")
}

func (h *P2PHandlers) Approvals(w http.ResponseWriter, r *http.Request) {
	pending := h.state.ListPendingApprovals()

	result := []ApprovalRequest{}
	for _, p := range pending {
		result = append(result, ApprovalRequest{
			DeviceID:   p.DeviceID,
			DeviceName: p.DeviceName,
			PublicKey:  p.PublicKey,
			Status:     p.Status,
		})
	}

	api.Success(w, result)
}

func (h *P2PHandlers) Approve(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		api.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	var req ApproveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	approval, exists := h.state.GetPendingApproval(req.DeviceID)
	if !exists {
		api.Error(w, http.StatusNotFound, "NOT_FOUND", "device not found in pending approvals")
		return
	}

	storage, ok := h.state.GetVaultStorage()
	if !ok {
		api.Error(w, http.StatusBadRequest, "VAULT_LOCKED", "vault not unlocked")
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

	if err := storage.UpsertDevice(&device); err != nil {
		api.Error(w, http.StatusInternalServerError, "DB_ERROR", "failed to save device: "+err.Error())
		return
	}

	h.state.RemovePendingApproval(req.DeviceID)

	api.Success(w, "Device approved")
}

func (h *P2PHandlers) Reject(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		api.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	var req RejectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	_, exists := h.state.GetPendingApproval(req.DeviceID)
	if !exists {
		api.Error(w, http.StatusNotFound, "NOT_FOUND", "device not found in pending approvals")
		return
	}

	h.state.RemovePendingApproval(req.DeviceID)

	api.Success(w, "Device rejected")
}

func (h *P2PHandlers) Sync(w http.ResponseWriter, r *http.Request) {
	pm, running := h.state.GetP2PManager()
	if !running || pm == nil || !pm.IsRunning() {
		api.Error(w, http.StatusBadRequest, "P2P_NOT_RUNNING", "P2P not running")
		return
	}

	var req SyncRequest
	if r.Method == "POST" {
		json.NewDecoder(r.Body).Decode(&req)
	}

	storage, ok := h.state.GetVaultStorage()
	if !ok {
		api.Error(w, http.StatusBadRequest, "VAULT_LOCKED", "vault not unlocked")
		return
	}

	entries, err := storage.ListEntries()
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "DB_ERROR", "failed to get entries")
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

	err = pm.SyncWithPeers(req.FullSync)
	if err != nil {
		log.Printf("[Sync] Error: %v", err)
		api.Error(w, http.StatusInternalServerError, "SYNC_FAILED", "sync failed")
		return
	}

	api.Success(w, map[string]interface{}{
		"entries":   syncData,
		"synced":    true,
		"timestamp": time.Now().Unix(),
	})
}
