// Package transport provides TLS certificate management and TOFU (Trust On First Use)
// certificate pinning for the P2P password manager.
package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PinnedPeer represents a trusted peer's certificate
type PinnedPeer struct {
	Fingerprint string    `json:"fingerprint"`
	DeviceName  string    `json:"device_name"`
	DeviceID    string    `json:"device_id"`
	PinnedAt    time.Time `json:"pinned_at"`
}

// PeerStore implements TOFU (Trust On First Use) certificate pinning
// Location: ~/.pwman/vaults/<name>/trusted_peers.json
type PeerStore struct {
	mu    sync.RWMutex
	peers map[string]PinnedPeer
	path  string
}

// NewPeerStore loads or creates a peer store from the given path
func NewPeerStore(path string) (*PeerStore, error) {
	ps := &PeerStore{
		peers: make(map[string]PinnedPeer),
		path:  path,
	}

	// Load existing peers if file exists
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &ps.peers)
	}

	return ps, nil
}

// IsTrusted checks if a certificate fingerprint is trusted
func (ps *PeerStore) IsTrusted(fp string) bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	_, ok := ps.peers[fp]
	return ok
}

// Trust adds a peer to the trusted list
func (ps *PeerStore) Trust(fp, deviceName, deviceID string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.peers[fp] = PinnedPeer{
		Fingerprint: fp,
		DeviceName:  deviceName,
		DeviceID:    deviceID,
		PinnedAt:    time.Now(),
	}

	return ps.save()
}

// Untrust removes a peer from the trusted list
func (ps *PeerStore) Untrust(fp string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	delete(ps.peers, fp)
	return ps.save()
}

// ListTrusted returns all trusted peers
func (ps *PeerStore) ListTrusted() []PinnedPeer {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make([]PinnedPeer, 0, len(ps.peers))
	for _, peer := range ps.peers {
		result = append(result, peer)
	}
	return result
}

// GetPeer returns a specific peer by fingerprint
func (ps *PeerStore) GetPeer(fp string) (PinnedPeer, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	peer, ok := ps.peers[fp]
	return peer, ok
}

func (ps *PeerStore) save() error {
	data, err := json.MarshalIndent(ps.peers, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(ps.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	return os.WriteFile(ps.path, data, 0600)
}

// CertFingerprint returns SHA-256 fingerprint of a certificate
func CertFingerprint(rawCert []byte) string {
	h := sha256.Sum256(rawCert)
	return hex.EncodeToString(h[:])
}
