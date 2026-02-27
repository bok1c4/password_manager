package p2p

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type PeerManager struct {
	mu               sync.RWMutex
	peers            map[string]ManagedPeer
	configPath       string
	deviceID         string
	onPeerConnect    func(ManagedPeer)
	onPeerDisconnect func(string)
}

type ManagedPeer struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Addr          string            `json:"addr"`
	Fingerprint   string            `json:"fingerprint"`
	Trusted       bool              `json:"trusted"`
	LastConnected time.Time         `json:"last_connected"`
	AddedAt       time.Time         `json:"added_at"`
	Nickname      string            `json:"nickname,omitempty"`
	AutoConnect   bool              `json:"auto_connect"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type PeerManagerConfig struct {
	ConfigPath string
	DeviceID   string
}

func NewPeerManager(cfg PeerManagerConfig) *PeerManager {
	return &PeerManager{
		peers:      make(map[string]ManagedPeer),
		configPath: cfg.ConfigPath,
		deviceID:   cfg.DeviceID,
	}
}

func (pm *PeerManager) Load() error {
	if pm.configPath == "" {
		return nil
	}

	data, err := os.ReadFile(pm.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read peer config: %w", err)
	}

	var peers []ManagedPeer
	if err := json.Unmarshal(data, &peers); err != nil {
		return fmt.Errorf("failed to parse peer config: %w", err)
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, peer := range peers {
		pm.peers[peer.ID] = peer
	}

	return nil
}

func (pm *PeerManager) Save() error {
	if pm.configPath == "" {
		return nil
	}

	pm.mu.RLock()
	peers := make([]ManagedPeer, 0, len(pm.peers))
	for _, peer := range pm.peers {
		peers = append(peers, peer)
	}
	pm.mu.RUnlock()

	data, err := json.MarshalIndent(peers, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal peers: %w", err)
	}

	dir := filepath.Dir(pm.configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	if err := os.WriteFile(pm.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write peer config: %w", err)
	}

	return nil
}

func (pm *PeerManager) AddPeer(peer ManagedPeer) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if peer.AddedAt.IsZero() {
		peer.AddedAt = time.Now()
	}

	pm.peers[peer.ID] = peer

	if err := pm.Save(); err != nil {
		return err
	}

	return nil
}

func (pm *PeerManager) RemovePeer(peerID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	delete(pm.peers, peerID)

	if err := pm.Save(); err != nil {
		return err
	}

	return nil
}

func (pm *PeerManager) GetPeer(peerID string) (ManagedPeer, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	peer, ok := pm.peers[peerID]
	return peer, ok
}

func (pm *PeerManager) GetAllPeers() []ManagedPeer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	peers := make([]ManagedPeer, 0, len(pm.peers))
	for _, peer := range pm.peers {
		peers = append(peers, peer)
	}
	return peers
}

func (pm *PeerManager) GetTrustedPeers() []ManagedPeer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var peers []ManagedPeer
	for _, peer := range pm.peers {
		if peer.Trusted {
			peers = append(peers, peer)
		}
	}
	return peers
}

func (pm *PeerManager) GetUntrustedPeers() []ManagedPeer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var peers []ManagedPeer
	for _, peer := range pm.peers {
		if !peer.Trusted {
			peers = append(peers, peer)
		}
	}
	return peers
}

func (pm *PeerManager) TrustPeer(peerID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	peer, ok := pm.peers[peerID]
	if !ok {
		return fmt.Errorf("peer not found: %s", peerID)
	}

	peer.Trusted = true
	pm.peers[peerID] = peer

	return pm.Save()
}

func (pm *PeerManager) UntrustPeer(peerID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	peer, ok := pm.peers[peerID]
	if !ok {
		return fmt.Errorf("peer not found: %s", peerID)
	}

	peer.Trusted = false
	pm.peers[peerID] = peer

	return pm.Save()
}

func (pm *PeerManager) UpdatePeerAddress(peerID, addr string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	peer, ok := pm.peers[peerID]
	if !ok {
		return fmt.Errorf("peer not found: %s", peerID)
	}

	peer.Addr = addr
	peer.LastConnected = time.Now()
	pm.peers[peerID] = peer

	return nil
}

func (pm *PeerManager) RecordConnection(peerID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	peer, ok := pm.peers[peerID]
	if !ok {
		return fmt.Errorf("peer not found: %s", peerID)
	}

	peer.LastConnected = time.Now()
	pm.peers[peerID] = peer

	if pm.onPeerConnect != nil {
		go pm.onPeerConnect(peer)
	}

	return pm.Save()
}

func (pm *PeerManager) RecordDisconnection(peerID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if peer, ok := pm.peers[peerID]; ok {
		if pm.onPeerDisconnect != nil {
			go pm.onPeerDisconnect(peerID)
		}
		_ = peer
	}
}

func (pm *PeerManager) SetNickname(peerID, nickname string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	peer, ok := pm.peers[peerID]
	if !ok {
		return fmt.Errorf("peer not found: %s", peerID)
	}

	peer.Nickname = nickname
	pm.peers[peerID] = peer

	return pm.Save()
}

func (pm *PeerManager) SetPeerMetadata(peerID string, key, value string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	peer, ok := pm.peers[peerID]
	if !ok {
		return fmt.Errorf("peer not found: %s", peerID)
	}

	if peer.Metadata == nil {
		peer.Metadata = make(map[string]string)
	}
	peer.Metadata[key] = value
	pm.peers[peerID] = peer

	return pm.Save()
}

func (pm *PeerManager) OnPeerConnect(cb func(ManagedPeer)) {
	pm.onPeerConnect = cb
}

func (pm *PeerManager) OnPeerDisconnect(cb func(string)) {
	pm.onPeerDisconnect = cb
}

func (pm *PeerManager) FindPeerByFingerprint(fingerprint string) (ManagedPeer, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for _, peer := range pm.peers {
		if peer.Fingerprint == fingerprint {
			return peer, true
		}
	}
	return ManagedPeer{}, false
}

func (pm *PeerManager) FindPeerByName(name string) (ManagedPeer, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for _, peer := range pm.peers {
		if peer.Name == name || peer.Nickname == name {
			return peer, true
		}
	}
	return ManagedPeer{}, false
}
