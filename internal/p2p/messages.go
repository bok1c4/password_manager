package p2p

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MsgTypeHello           = "HELLO"
	MsgTypeRequestSync     = "REQUEST_SYNC"
	MsgTypeSyncData        = "SYNC_DATA"
	MsgTypeRequestApproval = "REQUEST_APPROVAL"
	MsgTypeApproveDevice   = "APPROVE_DEVICE"
	MsgTypeRejectDevice    = "REJECT_DEVICE"
	MsgTypePing            = "PING"
	MsgTypePong            = "PONG"
	MsgTypeEntryUpdate     = "ENTRY_UPDATE"
	MsgTypeEntryDelete     = "ENTRY_DELETE"
	MsgTypePairingRequest  = "PAIRING_REQUEST"
	MsgTypePairingResponse = "PAIRING_RESPONSE"
)

type Message struct {
	Type      string          `json:"type"`
	PeerID    string          `json:"peer_id"`
	Timestamp int64           `json:"timestamp"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type HelloPayload struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	VaultID    string `json:"vault_id"`
	Version    int    `json:"version"`
}

type RequestSyncPayload struct {
	LastSyncTime int64 `json:"last_sync_time"`
	FullSync     bool  `json:"full_sync"`
}

type SyncDataPayload struct {
	Entries   []EntryData  `json:"entries"`
	Devices   []DeviceData `json:"devices"`
	Version   int          `json:"version"`
	Timestamp int64        `json:"timestamp"`
}

type EntryData struct {
	ID                string            `json:"id"`
	Site              string            `json:"site"`
	Username          string            `json:"username"`
	EncryptedPassword string            `json:"encrypted_password"`
	EncryptedAESKeys  map[string]string `json:"encrypted_aes_keys"`
	Notes             string            `json:"notes"`
	Version           int               `json:"version"`
	CreatedAt         int64             `json:"created_at"`
	UpdatedAt         int64             `json:"updated_at"`
	DeletedAt         *int64            `json:"deleted_at,omitempty"`
}

type DeviceData struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	Trusted     bool   `json:"trusted"`
	CreatedAt   int64  `json:"created_at"`
}

type RequestApprovalPayload struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	PublicKey  string `json:"public_key"`
	VaultID    string `json:"vault_id"`
}

type ApproveDevicePayload struct {
	DeviceID           string            `json:"device_id"`
	EncryptedKeys      map[string]string `json:"encrypted_keys"`
	ReEncryptedEntries []EntryData       `json:"re_encrypted_entries"`
}

type RejectDevicePayload struct {
	DeviceID string `json:"device_id"`
	Reason   string `json:"reason"`
}

func NewMessage(msgType string, payload interface{}) (*Message, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	return &Message{
		Type:      msgType,
		Timestamp: time.Now().UnixMilli(),
		Payload:   payloadBytes,
	}, nil
}

func (m *Message) ToBytes() ([]byte, error) {
	return json.Marshal(m)
}

func MessageFromBytes(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}
	return &msg, nil
}

func (m *Message) GetPayload(v interface{}) error {
	return json.Unmarshal(m.Payload, v)
}

func CreateHelloMessage(deviceID, deviceName, vaultID string, version int) (*Message, error) {
	payload := HelloPayload{
		DeviceID:   deviceID,
		DeviceName: deviceName,
		VaultID:    vaultID,
		Version:    version,
	}
	return NewMessage(MsgTypeHello, payload)
}

func CreateRequestSyncMessage(lastSyncTime int64, fullSync bool) (*Message, error) {
	payload := RequestSyncPayload{
		LastSyncTime: lastSyncTime,
		FullSync:     fullSync,
	}
	return NewMessage(MsgTypeRequestSync, payload)
}

func CreateSyncDataMessage(entries []EntryData, devices []DeviceData) (*Message, error) {
	payload := SyncDataPayload{
		Entries:   entries,
		Devices:   devices,
		Version:   1,
		Timestamp: time.Now().UnixMilli(),
	}
	return NewMessage(MsgTypeSyncData, payload)
}

func CreateRequestApprovalMessage(deviceID, deviceName, publicKey, vaultID string) (*Message, error) {
	payload := RequestApprovalPayload{
		DeviceID:   deviceID,
		DeviceName: deviceName,
		PublicKey:  publicKey,
		VaultID:    vaultID,
	}
	return NewMessage(MsgTypeRequestApproval, payload)
}

func CreateApproveDeviceMessage(deviceID string, encryptedKeys map[string]string, reEncryptedEntries []EntryData) (*Message, error) {
	payload := ApproveDevicePayload{
		DeviceID:           deviceID,
		EncryptedKeys:      encryptedKeys,
		ReEncryptedEntries: reEncryptedEntries,
	}
	return NewMessage(MsgTypeApproveDevice, payload)
}

func CreateRejectDeviceMessage(deviceID, reason string) (*Message, error) {
	payload := RejectDevicePayload{
		DeviceID: deviceID,
		Reason:   reason,
	}
	return NewMessage(MsgTypeRejectDevice, payload)
}

func CreatePairingRequestMessage(code, deviceID, deviceName, password string) (*Message, error) {
	payload := PairingRequestPayload{
		Code:       code,
		DeviceID:   deviceID,
		DeviceName: deviceName,
		Password:   password,
	}
	return NewMessage(MsgTypePairingRequest, payload)
}

func CreatePairingResponseMessage(success bool, code, vaultName, vaultID, deviceID, deviceName, publicKey, fingerprint, errMsg string) (*Message, error) {
	payload := PairingResponsePayload{
		Success:     success,
		Code:        code,
		VaultName:   vaultName, // NEW
		VaultID:     vaultID,
		DeviceID:    deviceID,
		DeviceName:  deviceName,
		PublicKey:   publicKey,
		Fingerprint: fingerprint,
		Error:       errMsg,
	}
	return NewMessage(MsgTypePairingResponse, payload)
}

func CreatePingMessage() (*Message, error) {
	return NewMessage(MsgTypePing, nil)
}

func CreatePongMessage() (*Message, error) {
	return NewMessage(MsgTypePong, nil)
}

func GenerateMessageID() string {
	return uuid.New().String()
}

type PairingRequestPayload struct {
	Code       string `json:"code"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	Password   string `json:"password,omitempty"` // Vault password for verification
}

type PairingResponsePayload struct {
	Success     bool   `json:"success"`
	Code        string `json:"code,omitempty"`
	VaultName   string `json:"vault_name,omitempty"` // NEW: name of vault being joined
	VaultID     string `json:"vault_id,omitempty"`
	DeviceID    string `json:"device_id,omitempty"`
	DeviceName  string `json:"device_name,omitempty"`
	PublicKey   string `json:"public_key,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Error       string `json:"error,omitempty"`
}
