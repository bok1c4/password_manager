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
