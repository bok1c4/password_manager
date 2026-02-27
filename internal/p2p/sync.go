package p2p

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type SyncHandler struct {
	manager          *P2PManager
	deviceID         string
	deviceName       string
	vaultID          string
	mu               sync.RWMutex
	lastSyncTime     int64
	currentVersion   int
	pendingApprovals map[string]ApprovalRequest
	approvalCh       chan ApprovalRequest

	getEntriesFunc func() ([]EntryData, error)
	getDevicesFunc func() ([]DeviceData, error)
	mergeDataFunc  func(SyncDataPayload) error
	onEntryUpdate  func(EntryData)
}

type ApprovalRequest struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	PublicKey  string `json:"public_key"`
	VaultID    string `json:"vault_id"`
	Timestamp  int64  `json:"timestamp"`
	Status     string `json:"status"`
}

type SyncCallbacks struct {
	GetEntries    func() ([]EntryData, error)
	GetDevices    func() ([]DeviceData, error)
	MergeData     func(SyncDataPayload) error
	OnEntryUpdate func(EntryData)
}

func NewSyncHandler(deviceID, deviceName, vaultID string, manager *P2PManager, callbacks SyncCallbacks) *SyncHandler {
	return &SyncHandler{
		manager:          manager,
		deviceID:         deviceID,
		deviceName:       deviceName,
		vaultID:          vaultID,
		pendingApprovals: make(map[string]ApprovalRequest),
		approvalCh:       make(chan ApprovalRequest, 10),
		lastSyncTime:     0,
		currentVersion:   1,
		getEntriesFunc:   callbacks.GetEntries,
		getDevicesFunc:   callbacks.GetDevices,
		mergeDataFunc:    callbacks.MergeData,
		onEntryUpdate:    callbacks.OnEntryUpdate,
	}
}

func (s *SyncHandler) Start(ctx context.Context) {
	go s.handleMessages(ctx)
	go s.keepAlive(ctx)
	go s.sendHello(ctx)
}

func (s *SyncHandler) sendHello(ctx context.Context) {
	time.Sleep(1 * time.Second)

	msg, err := CreateHelloMessage(s.deviceID, s.deviceName, s.vaultID, s.currentVersion)
	if err != nil {
		fmt.Printf("[Sync] Failed to create HELLO: %v\n", err)
		return
	}
	msg.PeerID = s.deviceID

	s.manager.BroadcastMessage(SyncMessage{
		Type:      msg.Type,
		Payload:   msg.Payload,
		Timestamp: msg.Timestamp,
	})
}

func (s *SyncHandler) handleMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-s.manager.MessageChan():
			s.processMessage(msg)
		}
	}
}

func (s *SyncHandler) processMessage(msg SyncMessage) {
	p2pMsg, err := MessageFromBytes(msg.Payload)
	if err != nil {
		fmt.Printf("[Sync] Failed to parse message: %v\n", err)
		return
	}

	fmt.Printf("[Sync] Received: %s from %s\n", p2pMsg.Type, msg.PeerID)

	switch p2pMsg.Type {
	case MsgTypeHello:
		s.handleHello(p2pMsg, msg.PeerID)
	case MsgTypeRequestSync:
		s.handleRequestSync(p2pMsg, msg.PeerID)
	case MsgTypeSyncData:
		s.handleSyncData(p2pMsg, msg.PeerID)
	case MsgTypeRequestApproval:
		s.handleRequestApproval(p2pMsg, msg.PeerID)
	case MsgTypeApproveDevice:
		s.handleApproveDevice(p2pMsg, msg.PeerID)
	case MsgTypeRejectDevice:
		s.handleRejectDevice(p2pMsg, msg.PeerID)
	case MsgTypePing:
		s.handlePing(msg.PeerID)
	case MsgTypePong:
		s.handlePong()
	}
}

func (s *SyncHandler) handleHello(msg *Message, peerID string) {
	var payload HelloPayload
	if err := msg.GetPayload(&payload); err != nil {
		fmt.Printf("[Sync] Failed to parse HELLO: %v\n", err)
		return
	}

	fmt.Printf("[Sync] Hello from %s (%s), vault: %s\n",
		payload.DeviceName, payload.DeviceID, payload.VaultID)

	if s.vaultID != payload.VaultID {
		fmt.Printf("[Sync] Vault mismatch: expected %s, got %s\n", s.vaultID, payload.VaultID)
		return
	}

	s.SendSyncRequest(peerID, true)
}

func (s *SyncHandler) handleRequestSync(msg *Message, peerID string) {
	var payload RequestSyncPayload
	if err := msg.GetPayload(&payload); err != nil {
		fmt.Printf("[Sync] Failed to parse REQUEST_SYNC: %v\n", err)
		return
	}

	fmt.Printf("[Sync] Sync requested by %s (full: %v)\n", peerID, payload.FullSync)

	s.sendSyncData(peerID)
}

func (s *SyncHandler) handleSyncData(msg *Message, peerID string) {
	var payload SyncDataPayload
	if err := msg.GetPayload(&payload); err != nil {
		fmt.Printf("[Sync] Failed to parse SYNC_DATA: %v\n", err)
		return
	}

	fmt.Printf("[Sync] Received %d entries, %d devices from %s\n",
		len(payload.Entries), len(payload.Devices), peerID)

	if s.mergeDataFunc != nil {
		if err := s.mergeDataFunc(payload); err != nil {
			fmt.Printf("[Sync] Failed to merge data: %v\n", err)
		}
	}

	for _, entry := range payload.Entries {
		if s.onEntryUpdate != nil {
			s.onEntryUpdate(entry)
		}
	}

	s.lastSyncTime = time.Now().UnixMilli()
}

func (s *SyncHandler) handleRequestApproval(msg *Message, peerID string) {
	var payload RequestApprovalPayload
	if err := msg.GetPayload(&payload); err != nil {
		fmt.Printf("[Sync] Failed to parse REQUEST_APPROVAL: %v\n", err)
		return
	}

	fmt.Printf("[Sync] Approval request from %s (%s)\n",
		payload.DeviceName, payload.DeviceID)

	approval := ApprovalRequest{
		DeviceID:   payload.DeviceID,
		DeviceName: payload.DeviceName,
		PublicKey:  payload.PublicKey,
		VaultID:    payload.VaultID,
		Timestamp:  time.Now().UnixMilli(),
		Status:     "pending",
	}

	s.pendingApprovals[payload.DeviceID] = approval
	s.approvalCh <- approval
}

func (s *SyncHandler) handleApproveDevice(msg *Message, peerID string) {
	var payload ApproveDevicePayload
	if err := msg.GetPayload(&payload); err != nil {
		fmt.Printf("[Sync] Failed to parse APPROVE_DEVICE: %v\n", err)
		return
	}

	fmt.Printf("[Sync] Device %s approved\n", payload.DeviceID)

	delete(s.pendingApprovals, payload.DeviceID)

	if s.mergeDataFunc != nil {
		s.mergeDataFunc(SyncDataPayload{
			Entries: payload.ReEncryptedEntries,
			Devices: []DeviceData{},
		})
	}

	s.lastSyncTime = time.Now().UnixMilli()
}

func (s *SyncHandler) handleRejectDevice(msg *Message, peerID string) {
	var payload RejectDevicePayload
	if err := msg.GetPayload(&payload); err != nil {
		fmt.Printf("[Sync] Failed to parse REJECT_DEVICE: %v\n", err)
		return
	}

	fmt.Printf("[Sync] Device %s rejected: %s\n", payload.DeviceID, payload.Reason)

	delete(s.pendingApprovals, payload.DeviceID)
}

func (s *SyncHandler) handlePing(peerID string) {
	msg, _ := CreatePongMessage()
	msg.PeerID = s.deviceID
	s.manager.SendMessage(peerID, SyncMessage{
		Type:      msg.Type,
		Payload:   msg.Payload,
		Timestamp: msg.Timestamp,
	})
}

func (s *SyncHandler) handlePong() {
}

func (s *SyncHandler) keepAlive(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.broadcastPing()
		}
	}
}

func (s *SyncHandler) broadcastPing() {
	msg, err := CreatePingMessage()
	if err != nil {
		return
	}
	msg.PeerID = s.deviceID

	s.manager.BroadcastMessage(SyncMessage{
		Type:      msg.Type,
		Payload:   msg.Payload,
		Timestamp: msg.Timestamp,
	})
}

func (s *SyncHandler) sendSyncData(peerID string) {
	var entryData []EntryData
	var deviceData []DeviceData

	if s.getEntriesFunc != nil {
		entries, err := s.getEntriesFunc()
		if err == nil {
			entryData = entries
		}
	}

	if s.getDevicesFunc != nil {
		devices, err := s.getDevicesFunc()
		if err == nil {
			deviceData = devices
		}
	}

	msg, err := CreateSyncDataMessage(entryData, deviceData)
	if err != nil {
		fmt.Printf("[Sync] Failed to create sync message: %v\n", err)
		return
	}
	msg.PeerID = s.deviceID

	s.manager.SendMessage(peerID, SyncMessage{
		Type:      msg.Type,
		Payload:   msg.Payload,
		Timestamp: msg.Timestamp,
	})
}

func (s *SyncHandler) SendSyncRequest(peerID string, fullSync bool) {
	msg, err := CreateRequestSyncMessage(s.lastSyncTime, fullSync)
	if err != nil {
		fmt.Printf("[Sync] Failed to create sync request: %v\n", err)
		return
	}
	msg.PeerID = s.deviceID

	s.manager.SendMessage(peerID, SyncMessage{
		Type:      msg.Type,
		Payload:   msg.Payload,
		Timestamp: msg.Timestamp,
	})
}

func (s *SyncHandler) BroadcastSync() {
	msg, err := CreateRequestSyncMessage(s.lastSyncTime, false)
	if err != nil {
		fmt.Printf("[Sync] Failed to create broadcast sync: %v\n", err)
		return
	}
	msg.PeerID = s.deviceID

	s.manager.BroadcastMessage(SyncMessage{
		Type:      msg.Type,
		Payload:   msg.Payload,
		Timestamp: msg.Timestamp,
	})
}

func (s *SyncHandler) RequestApproval(peerID string) error {
	msg, err := CreateRequestApprovalMessage(s.deviceID, s.deviceName, "", s.vaultID)
	if err != nil {
		return fmt.Errorf("failed to create approval request: %w", err)
	}
	msg.PeerID = s.deviceID

	return s.manager.SendMessage(peerID, SyncMessage{
		Type:      msg.Type,
		Payload:   msg.Payload,
		Timestamp: msg.Timestamp,
	})
}

func (s *SyncHandler) GetPendingApprovals() []ApprovalRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	approvals := make([]ApprovalRequest, 0, len(s.pendingApprovals))
	for _, a := range s.pendingApprovals {
		approvals = append(approvals, a)
	}
	return approvals
}

func (s *SyncHandler) ApproveDevice(deviceID string) error {
	approval, ok := s.pendingApprovals[deviceID]
	if !ok {
		return fmt.Errorf("no pending approval for device %s", deviceID)
	}

	var reEncryptedEntries []EntryData
	if s.getEntriesFunc != nil {
		entries, err := s.getEntriesFunc()
		if err == nil {
			reEncryptedEntries = entries
		}
	}

	encryptedKeys := make(map[string]string)
	encryptedKeys[approval.DeviceID] = "encrypted_key_for_new_device"

	msg, err := CreateApproveDeviceMessage(deviceID, encryptedKeys, reEncryptedEntries)
	if err != nil {
		return fmt.Errorf("failed to create approve message: %w", err)
	}
	msg.PeerID = s.deviceID

	peers := s.manager.GetConnectedPeers()
	for _, peer := range peers {
		s.manager.SendMessage(peer.ID, SyncMessage{
			Type:      msg.Type,
			Payload:   msg.Payload,
			Timestamp: msg.Timestamp,
		})
	}

	delete(s.pendingApprovals, deviceID)
	return nil
}

func (s *SyncHandler) RejectDevice(deviceID, reason string) error {
	_, ok := s.pendingApprovals[deviceID]
	if !ok {
		return fmt.Errorf("no pending approval for device %s", deviceID)
	}

	msg, err := CreateRejectDeviceMessage(deviceID, reason)
	if err != nil {
		return fmt.Errorf("failed to create reject message: %w", err)
	}
	msg.PeerID = s.deviceID

	peers := s.manager.GetConnectedPeers()
	for _, peer := range peers {
		s.manager.SendMessage(peer.ID, SyncMessage{
			Type:      msg.Type,
			Payload:   msg.Payload,
			Timestamp: msg.Timestamp,
		})
	}

	delete(s.pendingApprovals, deviceID)
	return nil
}

func (s *SyncHandler) ApprovalChan() <-chan ApprovalRequest {
	return s.approvalCh
}

func (s *SyncHandler) GetLastSyncTime() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSyncTime
}

func (s *SyncHandler) BroadcastEntryUpdate(entry EntryData) {
	msg, err := CreateSyncDataMessage([]EntryData{entry}, nil)
	if err != nil {
		fmt.Printf("[Sync] Failed to broadcast entry update: %v\n", err)
		return
	}
	msg.PeerID = s.deviceID

	s.manager.BroadcastMessage(SyncMessage{
		Type:      msg.Type,
		Payload:   msg.Payload,
		Timestamp: msg.Timestamp,
	})
}

func (s *SyncHandler) SetCallbacks(callbacks SyncCallbacks) {
	s.getEntriesFunc = callbacks.GetEntries
	s.getDevicesFunc = callbacks.GetDevices
	s.mergeDataFunc = callbacks.MergeData
	s.onEntryUpdate = callbacks.OnEntryUpdate
}
