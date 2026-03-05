package p2p

import (
	"encoding/json"
	"testing"
)

func TestPairingRequestPayloadHasNoPassword(t *testing.T) {
	payload := PairingRequestPayload{
		Code:       "123456",
		DeviceID:   "device-123",
		DeviceName: "Test Device",
		PublicKey:  "-----BEGIN RSA PUBLIC KEY-----\ntest\n-----END RSA PUBLIC KEY-----",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if _, exists := raw["password"]; exists {
		t.Error("PairingRequestPayload should NOT contain password field")
	}

	if _, exists := raw["Password"]; exists {
		t.Error("PairingRequestPayload should NOT contain Password field")
	}

	if _, exists := raw["vault_password"]; exists {
		t.Error("PairingRequestPayload should NOT contain vault_password field")
	}
}

func TestCreatePairingRequestMessage(t *testing.T) {
	code := "123456"
	deviceID := "device-123"
	deviceName := "Test Device"
	publicKey := "-----BEGIN RSA PUBLIC KEY-----\ntest\n-----END RSA PUBLIC KEY-----"

	msg, err := CreatePairingRequestMessage(code, deviceID, deviceName, publicKey)
	if err != nil {
		t.Fatalf("CreatePairingRequestMessage failed: %v", err)
	}

	if msg.Type != MsgTypePairingRequest {
		t.Errorf("Message type = %s, want %s", msg.Type, MsgTypePairingRequest)
	}

	var payload PairingRequestPayload
	if err := msg.GetPayload(&payload); err != nil {
		t.Fatalf("Failed to get payload: %v", err)
	}

	if payload.Code != code {
		t.Errorf("Code = %s, want %s", payload.Code, code)
	}

	if payload.DeviceID != deviceID {
		t.Errorf("DeviceID = %s, want %s", payload.DeviceID, deviceID)
	}

	if payload.DeviceName != deviceName {
		t.Errorf("DeviceName = %s, want %s", payload.DeviceName, deviceName)
	}

	if payload.PublicKey != publicKey {
		t.Errorf("PublicKey = %s, want %s", payload.PublicKey, publicKey)
	}
}

func TestPairingRequestPayloadFields(t *testing.T) {
	tests := []struct {
		name     string
		payload  PairingRequestPayload
		expected string
	}{
		{
			name: "with code only",
			payload: PairingRequestPayload{
				Code: "ABC123",
			},
			expected: "ABC123",
		},
		{
			name: "with all fields",
			payload: PairingRequestPayload{
				Code:       "XYZ789",
				DeviceID:   "device-456",
				DeviceName: "My Device",
				PublicKey:  "key-data",
			},
			expected: "XYZ789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var decoded PairingRequestPayload
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if decoded.Code != tt.expected {
				t.Errorf("Code = %s, want %s", decoded.Code, tt.expected)
			}
		})
	}
}

func TestMessageSerialization(t *testing.T) {
	msg := &Message{
		Type:      MsgTypePairingRequest,
		PeerID:    "peer-123",
		Timestamp: 1234567890,
		Payload:   json.RawMessage(`{"code":"123","device_id":"dev1"}`),
	}

	data, err := msg.ToBytes()
	if err != nil {
		t.Fatalf("ToBytes failed: %v", err)
	}

	decoded, err := MessageFromBytes(data)
	if err != nil {
		t.Fatalf("MessageFromBytes failed: %v", err)
	}

	if decoded.Type != msg.Type {
		t.Errorf("Type = %s, want %s", decoded.Type, msg.Type)
	}

	if decoded.PeerID != msg.PeerID {
		t.Errorf("PeerID = %s, want %s", decoded.PeerID, msg.PeerID)
	}
}
