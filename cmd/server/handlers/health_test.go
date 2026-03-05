package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bok1c4/pwman/internal/state"
)

func TestHealthEndpoint(t *testing.T) {
	s := state.NewServerState()
	h := NewHealthHandlers(s)

	req := httptest.NewRequest("GET", "/api/health", nil)
	rr := httptest.NewRecorder()

	h.Health(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%v'", resp["status"])
	}
}

func TestMetricsEndpoint(t *testing.T) {
	s := state.NewServerState()
	h := NewHealthHandlers(s)

	req := httptest.NewRequest("GET", "/api/metrics", nil)
	rr := httptest.NewRecorder()

	h.Metrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if _, ok := resp["uptime_seconds"]; !ok {
		t.Error("expected uptime_seconds in response")
	}
}

func TestGeneratePassword(t *testing.T) {
	s := state.NewServerState()
	h := NewHealthHandlers(s)

	tests := []struct {
		name   string
		length int
	}{
		{"default length", 0},
		{"custom length", 20},
		{"minimum length", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := ""
			if tt.length > 0 {
				body = `{"length":` + string(rune(tt.length+'0')) + `}`
			}

			req := httptest.NewRequest("POST", "/api/generate", strings.NewReader(body))
			rr := httptest.NewRecorder()

			h.GeneratePassword(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", rr.Code)
			}

			var resp map[string]interface{}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to parse response: %v", err)
			}

			data, ok := resp["data"].(map[string]interface{})
			if !ok {
				t.Fatal("expected data object in response")
			}

			password, ok := data["password"].(string)
			if !ok {
				t.Fatal("expected password string in response")
			}

			if len(password) < 4 {
				t.Errorf("password too short: %d", len(password))
			}
		})
	}
}

func TestPingEndpoint(t *testing.T) {
	s := state.NewServerState()
	h := NewHealthHandlers(s)

	req := httptest.NewRequest("GET", "/api/ping", nil)
	rr := httptest.NewRecorder()

	h.Ping(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data, ok := resp["data"].(string)
	if !ok {
		t.Fatal("expected data string in response")
	}

	if data != "pong" {
		t.Errorf("expected 'pong', got '%s'", data)
	}
}
