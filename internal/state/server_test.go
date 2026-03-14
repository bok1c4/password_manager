package state

import (
	"testing"
	"time"
)

func TestNewServerState(t *testing.T) {
	s := NewServerState()
	if s == nil {
		t.Fatal("NewServerState returned nil")
	}
	if s.pairingCodes == nil {
		t.Error("pairingCodes not initialized")
	}
	if s.pairingRequests == nil {
		t.Error("pairingRequests not initialized")
	}
	if s.pairingResponseCh == nil {
		t.Error("pairingResponseCh not initialized")
	}
	if s.pendingApprovals == nil {
		t.Error("pendingApprovals not initialized")
	}
	if s.GetStartTime().IsZero() {
		t.Error("startTime not set")
	}
}

func TestVaultState(t *testing.T) {
	s := NewServerState()

	_, ok := s.GetVault()
	if ok {
		t.Error("GetVault should return false when vault is nil")
	}

	if s.IsUnlocked() {
		t.Error("IsUnlocked should be false initially")
	}

	v := &Vault{VaultName: "test"}
	s.SetVault(v)

	vault, ok := s.GetVault()
	if !ok {
		t.Error("GetVault should return true after SetVault")
	}
	if vault.VaultName != "test" {
		t.Errorf("expected vault name 'test', got '%s'", vault.VaultName)
	}

	if !s.IsUnlocked() {
		t.Error("IsUnlocked should be true after SetVault")
	}

	s.ClearVault()

	_, ok = s.GetVault()
	if ok {
		t.Error("GetVault should return false after ClearVault")
	}

	if s.IsUnlocked() {
		t.Error("IsUnlocked should be false after ClearVault")
	}
}

func TestPairingCodes(t *testing.T) {
	s := NewServerState()

	code := "TEST-CODE"
	pc := PairingCode{
		Code:       code,
		DeviceName: "Test Device",
		ExpiresAt:  time.Now().Add(5 * time.Minute),
		Used:       false,
	}

	s.AddPairingCode(code, pc)

	retrieved, ok := s.GetPairingCode(code)
	if !ok {
		t.Error("GetPairingCode should return true")
	}
	if retrieved.DeviceName != "Test Device" {
		t.Errorf("expected device name 'Test Device', got '%s'", retrieved.DeviceName)
	}

	s.MarkPairingCodeUsed(code)

	retrieved, ok = s.GetPairingCode(code)
	if !ok {
		t.Error("GetPairingCode should still return true")
	}
	if !retrieved.Used {
		t.Error("pairing code should be marked as used")
	}

	pc.Used = false
	s.UpdatePairingCode(code, pc)

	retrieved, _ = s.GetPairingCode(code)
	if retrieved.Used {
		t.Error("pairing code should not be used after update")
	}
}

func TestPairingRequests(t *testing.T) {
	s := NewServerState()

	code := "REQ-CODE"
	pr := PairingRequest{
		Code:       code,
		DeviceID:   "device-123",
		DeviceName: "Test Device",
	}

	s.AddPairingRequest(code, pr)

	retrieved, ok := s.GetPairingRequest(code)
	if !ok {
		t.Error("GetPairingRequest should return true")
	}
	if retrieved.DeviceID != "device-123" {
		t.Errorf("expected device ID 'device-123', got '%s'", retrieved.DeviceID)
	}
}

func TestPendingApprovals(t *testing.T) {
	s := NewServerState()

	deviceID := "device-456"
	pa := PendingApproval{
		DeviceID:    deviceID,
		DeviceName:  "Pending Device",
		Status:      "pending",
		ConnectedAt: time.Now(),
	}

	s.AddPendingApproval(deviceID, pa)

	retrieved, ok := s.GetPendingApproval(deviceID)
	if !ok {
		t.Error("GetPendingApproval should return true")
	}
	if retrieved.DeviceName != "Pending Device" {
		t.Errorf("expected device name 'Pending Device', got '%s'", retrieved.DeviceName)
	}

	list := s.ListPendingApprovals()
	if len(list) != 1 {
		t.Errorf("expected 1 pending approval, got %d", len(list))
	}

	s.RemovePendingApproval(deviceID)

	_, ok = s.GetPendingApproval(deviceID)
	if ok {
		t.Error("GetPendingApproval should return false after removal")
	}

	list = s.ListPendingApprovals()
	if len(list) != 0 {
		t.Errorf("expected 0 pending approvals, got %d", len(list))
	}
}

func TestPairingState(t *testing.T) {
	s := NewServerState()

	if s.GetPairingState() != nil {
		t.Error("pairing state should be nil initially")
	}

	ps := &PairingState{
		GeneratorPeerID: "gen-peer",
		JoinerPeerID:    "join-peer",
		Code:            "STATE-CODE",
		Status:          "pending",
	}

	s.SetPairingState(ps)

	retrieved := s.GetPairingState()
	if retrieved == nil {
		t.Fatal("pairing state should not be nil")
	}
	if retrieved.Code != "STATE-CODE" {
		t.Errorf("expected code 'STATE-CODE', got '%s'", retrieved.Code)
	}
}

func TestPairingResponseChannel(t *testing.T) {
	s := NewServerState()

	ch := s.GetPairingResponseChannel()
	if ch == nil {
		t.Error("pairing response channel should not be nil")
	}

	s.SetPairingResponseChannel(nil)

	ch = s.GetPairingResponseChannel()
	if ch != nil {
		t.Error("pairing response channel should be nil after set to nil")
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := NewServerState()

	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			s.SetVault(&Vault{VaultName: "test"})
			s.GetVault()
			s.IsUnlocked()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			s.AddPairingCode("code", PairingCode{})
			s.GetPairingCode("code")
			s.MarkPairingCodeUsed("code")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			s.AddPendingApproval("device", PendingApproval{})
			s.GetPendingApproval("device")
			s.RemovePendingApproval("device")
		}
		done <- true
	}()

	for i := 0; i < 3; i++ {
		<-done
	}
}

func TestPairingAttempt_RateLimiting(t *testing.T) {
	// Override lockout duration for faster tests
	oldDuration := BaseLockoutDelay
	BaseLockoutDelay = 100 * time.Millisecond
	defer func() { BaseLockoutDelay = oldDuration }()

	s := NewServerState()
	peerID := "test-peer-123"

	// Should allow exactly 5 attempts
	for i := 0; i < 5; i++ {
		if !s.RecordPairingAttempt(peerID) {
			t.Errorf("Attempt %d should succeed", i+1)
		}
	}

	// 6th attempt should be blocked and trigger lockout
	if s.RecordPairingAttempt(peerID) {
		t.Error("6th attempt should be blocked")
	}

	// After lockout period, should allow more attempts
	// LockoutCount is now 1, so delay is 100ms * 2 = 200ms
	time.Sleep(250 * time.Millisecond)
	if !s.RecordPairingAttempt(peerID) {
		t.Error("Should reset after lockout")
	}

	// Test exponential backoff - fail 5 more times to trigger second lockout
	for i := 0; i < 4; i++ {
		if !s.RecordPairingAttempt(peerID) {
			t.Errorf("Attempt %d after first lockout should succeed", i+1)
		}
	}
	// 5th attempt triggers second lockout (LockoutCount = 2)
	if s.RecordPairingAttempt(peerID) {
		t.Error("Should be blocked on 5th consecutive failure after lockout")
	}

	// LockoutCount=2 means multiplier is 4, so delay is 100ms * 4 = 400ms
	time.Sleep(450 * time.Millisecond)
	if !s.RecordPairingAttempt(peerID) {
		t.Error("Should work after second lockout with exponential delay")
	}
}

func TestPairingAttempt_DifferentPeers(t *testing.T) {
	s := NewServerState()
	peer1 := "peer-1"
	peer2 := "peer-2"

	// Max out peer1
	for i := 0; i < 5; i++ {
		s.RecordPairingAttempt(peer1)
	}
	if s.RecordPairingAttempt(peer1) {
		t.Error("Peer1 should be blocked")
	}

	// Peer2 should still work
	if !s.RecordPairingAttempt(peer2) {
		t.Error("Peer2 should not be affected")
	}
}

func TestVault_GetMasterKey(t *testing.T) {
	// Vault without master key
	vault := &Vault{
		VaultName: "test",
	}

	_, err := vault.GetMasterKey()
	if err == nil {
		t.Error("Should return error when vault not unlocked")
	}

	// Vault with master key
	vault.MasterKey = []byte("test-master-key")
	key, err := vault.GetMasterKey()
	if err != nil {
		t.Errorf("Should not return error: %v", err)
	}
	if string(key) != "test-master-key" {
		t.Errorf("expected 'test-master-key', got '%s'", string(key))
	}
}
