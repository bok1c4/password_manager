package state

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/crypto"
	"github.com/bok1c4/pwman/internal/p2p"
	"github.com/bok1c4/pwman/internal/storage"
	syncpkg "github.com/bok1c4/pwman/internal/sync"
)

type Vault struct {
	PrivateKey *crypto.KeyPair
	Storage    *storage.SQLite
	Config     *config.Config
	VaultName  string
	MasterKey  []byte // Derived from password, stored during unlock for TOTP
}

// GetMasterKey returns the vault's master key for TOTP generation
func (v *Vault) GetMasterKey() ([]byte, error) {
	if len(v.MasterKey) == 0 {
		return nil, fmt.Errorf("vault not unlocked")
	}
	return v.MasterKey, nil
}

type PairingCode struct {
	Code        string
	VaultID     string
	VaultName   string
	DeviceID    string
	DeviceName  string
	PublicKey   string
	Fingerprint string
	ExpiresAt   time.Time
	Used        bool
}

type PairingRequest struct {
	Code       string
	DeviceID   string
	DeviceName string
}

type PendingApproval struct {
	DeviceID    string
	DeviceName  string
	PublicKey   string
	Fingerprint string
	Status      string
	ConnectedAt time.Time
}

type PairingState struct {
	GeneratorPeerID string
	JoinerPeerID    string
	Code            string
	Status          string
}

// PairingAttempt tracks failed pairing attempts for rate limiting
type PairingAttempt struct {
	ConsecutiveFailures int
	LockoutCount        int
	LastAttempt         time.Time
	LockedUntil         *time.Time
}

// BaseLockoutDelay is the base delay for exponential backoff
var BaseLockoutDelay = 30 * time.Second

// MaxBackoffMultiplier caps the exponential backoff at 10x (5 minutes)
const MaxBackoffMultiplier = 10

type ServerState struct {
	mu sync.RWMutex

	vault      *Vault
	isUnlocked bool
	vaultLock  sync.Mutex

	p2pManager *p2p.P2PManager
	p2pCancel  context.CancelFunc
	p2pLock    sync.Mutex

	pairingCodes      map[string]PairingCode
	pairingRequests   map[string]PairingRequest
	pairingResponseCh chan p2p.PairingResponsePayload
	pendingApprovals  map[string]PendingApproval
	pairingState      *PairingState
	pairingLock       sync.Mutex
	approvalsLock     sync.Mutex
	pairingStateLock  sync.Mutex

	// Rate limiting for pairing attempts
	pairingAttempts   map[string]*PairingAttempt
	pairingAttemptsMu sync.Mutex

	// Lamport clock manager for sync protocol
	ClockManager *syncpkg.ClockManager

	startTime time.Time
}

func NewServerState() *ServerState {
	return &ServerState{
		pairingCodes:      make(map[string]PairingCode),
		pairingRequests:   make(map[string]PairingRequest),
		pairingResponseCh: make(chan p2p.PairingResponsePayload, 10),
		pendingApprovals:  make(map[string]PendingApproval),
		pairingAttempts:   make(map[string]*PairingAttempt),
		startTime:         time.Now(),
	}
}

// RecordPairingAttempt tracks pairing attempts and enforces rate limiting
// Returns true if attempt is allowed, false if peer is locked out
func (s *ServerState) RecordPairingAttempt(peerID string) bool {
	s.pairingAttemptsMu.Lock()
	defer s.pairingAttemptsMu.Unlock()

	att, exists := s.pairingAttempts[peerID]
	if !exists {
		s.pairingAttempts[peerID] = &PairingAttempt{
			ConsecutiveFailures: 1,
			LastAttempt:         time.Now(),
		}
		return true
	}

	// Check if still locked
	if att.LockedUntil != nil && time.Now().Before(*att.LockedUntil) {
		return false
	}

	att.ConsecutiveFailures++
	att.LastAttempt = time.Now()

	// Lock after MORE than 5 attempts (allows exactly 5)
	if att.ConsecutiveFailures > 5 {
		// Increment lockout count (never reset)
		att.LockoutCount++

		// Calculate exponential backoff: baseDelay * min(2^lockoutCount, 10)
		multiplier := 1 << att.LockoutCount // 2^lockoutCount
		if multiplier > MaxBackoffMultiplier {
			multiplier = MaxBackoffMultiplier
		}
		delay := BaseLockoutDelay * time.Duration(multiplier)

		until := time.Now().Add(delay)
		att.LockedUntil = &until
		att.ConsecutiveFailures = 0 // Reset consecutive failures, keep lockout count
		return false
	}

	return true
}

func (s *ServerState) GetStartTime() time.Time {
	return s.startTime
}

func (s *ServerState) GetVault() (*Vault, bool) {
	s.vaultLock.Lock()
	defer s.vaultLock.Unlock()
	if s.vault == nil {
		return nil, false
	}
	return s.vault, true
}

func (s *ServerState) SetVault(v *Vault) {
	s.vaultLock.Lock()
	defer s.vaultLock.Unlock()
	s.vault = v
	s.isUnlocked = v != nil
}

func (s *ServerState) IsUnlocked() bool {
	s.vaultLock.Lock()
	defer s.vaultLock.Unlock()
	return s.isUnlocked
}

func (s *ServerState) CloseVault() error {
	s.vaultLock.Lock()
	defer s.vaultLock.Unlock()
	if s.vault != nil && s.vault.Storage != nil {
		err := s.vault.Storage.Close()
		s.vault = nil
		s.isUnlocked = false
		return err
	}
	return nil
}

func (s *ServerState) ClearVault() {
	s.vaultLock.Lock()
	defer s.vaultLock.Unlock()
	s.vault = nil
	s.isUnlocked = false
}

func (s *ServerState) GetP2PManager() (*p2p.P2PManager, bool) {
	s.p2pLock.Lock()
	defer s.p2pLock.Unlock()
	return s.p2pManager, s.p2pManager != nil
}

func (s *ServerState) SetP2PManager(m *p2p.P2PManager) {
	s.p2pLock.Lock()
	defer s.p2pLock.Unlock()
	s.p2pManager = m
}

func (s *ServerState) SetP2PCancel(cancel context.CancelFunc) {
	s.p2pLock.Lock()
	defer s.p2pLock.Unlock()
	s.p2pCancel = cancel
}

func (s *ServerState) StopP2P() {
	s.p2pLock.Lock()
	defer s.p2pLock.Unlock()
	if s.p2pCancel != nil {
		s.p2pCancel()
		s.p2pCancel = nil
	}
	if s.p2pManager != nil {
		s.p2pManager.Stop()
		s.p2pManager = nil
	}
}

func (s *ServerState) AddPairingCode(code string, pc PairingCode) {
	s.pairingLock.Lock()
	defer s.pairingLock.Unlock()
	s.pairingCodes[code] = pc
}

func (s *ServerState) GetPairingCode(code string) (PairingCode, bool) {
	s.pairingLock.Lock()
	defer s.pairingLock.Unlock()
	pc, ok := s.pairingCodes[code]
	return pc, ok
}

func (s *ServerState) UpdatePairingCode(code string, pc PairingCode) {
	s.pairingLock.Lock()
	defer s.pairingLock.Unlock()
	s.pairingCodes[code] = pc
}

func (s *ServerState) MarkPairingCodeUsed(code string) {
	s.pairingLock.Lock()
	defer s.pairingLock.Unlock()
	if pc, ok := s.pairingCodes[code]; ok {
		pc.Used = true
		s.pairingCodes[code] = pc
	}
}

func (s *ServerState) AddPairingRequest(code string, pr PairingRequest) {
	s.pairingLock.Lock()
	defer s.pairingLock.Unlock()
	s.pairingRequests[code] = pr
}

func (s *ServerState) GetPairingRequest(code string) (PairingRequest, bool) {
	s.pairingLock.Lock()
	defer s.pairingLock.Unlock()
	pr, ok := s.pairingRequests[code]
	return pr, ok
}

func (s *ServerState) GetPairingResponseChannel() chan p2p.PairingResponsePayload {
	return s.pairingResponseCh
}

func (s *ServerState) SetPairingResponseChannel(ch chan p2p.PairingResponsePayload) {
	s.pairingLock.Lock()
	defer s.pairingLock.Unlock()
	s.pairingResponseCh = ch
}

func (s *ServerState) AddPendingApproval(deviceID string, pa PendingApproval) {
	s.approvalsLock.Lock()
	defer s.approvalsLock.Unlock()
	s.pendingApprovals[deviceID] = pa
}

func (s *ServerState) GetPendingApproval(deviceID string) (PendingApproval, bool) {
	s.approvalsLock.Lock()
	defer s.approvalsLock.Unlock()
	pa, ok := s.pendingApprovals[deviceID]
	return pa, ok
}

func (s *ServerState) RemovePendingApproval(deviceID string) {
	s.approvalsLock.Lock()
	defer s.approvalsLock.Unlock()
	delete(s.pendingApprovals, deviceID)
}

func (s *ServerState) ListPendingApprovals() []PendingApproval {
	s.approvalsLock.Lock()
	defer s.approvalsLock.Unlock()

	result := make([]PendingApproval, 0, len(s.pendingApprovals))
	for _, pa := range s.pendingApprovals {
		result = append(result, pa)
	}
	return result
}

func (s *ServerState) GetPairingState() *PairingState {
	s.pairingStateLock.Lock()
	defer s.pairingStateLock.Unlock()
	return s.pairingState
}

func (s *ServerState) SetPairingState(ps *PairingState) {
	s.pairingStateLock.Lock()
	defer s.pairingStateLock.Unlock()
	s.pairingState = ps
}

func (s *ServerState) GetP2PManagerAndCancel() (*p2p.P2PManager, context.CancelFunc) {
	s.p2pLock.Lock()
	defer s.p2pLock.Unlock()
	return s.p2pManager, s.p2pCancel
}

func (s *ServerState) GetVaultStorage() (*storage.SQLite, bool) {
	s.vaultLock.Lock()
	defer s.vaultLock.Unlock()
	if s.vault == nil || s.vault.Storage == nil {
		return nil, false
	}
	return s.vault.Storage, true
}

func (s *ServerState) GetAllPairingCodes() []PairingCode {
	s.pairingLock.Lock()
	defer s.pairingLock.Unlock()

	result := make([]PairingCode, 0, len(s.pairingCodes))
	for _, c := range s.pairingCodes {
		result = append(result, c)
	}
	return result
}
