package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bok1c4/pwman/cmd/server/handlers"
	"github.com/bok1c4/pwman/internal/api"
	"github.com/bok1c4/pwman/internal/middleware"
	"github.com/bok1c4/pwman/internal/state"
)

func TestServerHealthEndpoint(t *testing.T) {
	serverState := state.NewServerState()
	healthHandlers := handlers.NewHealthHandlers(serverState)

	req := httptest.NewRequest("GET", "/api/health", nil)
	rr := httptest.NewRecorder()

	healthHandlers.Health(rr, req)

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

func TestServerMetricsEndpoint(t *testing.T) {
	serverState := state.NewServerState()
	healthHandlers := handlers.NewHealthHandlers(serverState)
	authMgr := api.NewAuthManager()
	token := authMgr.GenerateToken()

	req := httptest.NewRequest("GET", "/api/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	healthHandlers.Metrics(rr, req)

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

func TestServerPingEndpoint(t *testing.T) {
	handler := middleware.CORS(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, Response{Success: true, Data: "pong"})
	})

	req := httptest.NewRequest("GET", "/api/ping", nil)
	req.Header.Set("Origin", "http://localhost:1420")
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp Response
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success to be true")
	}
}

func TestCORSMiddleware(t *testing.T) {
	handler := middleware.CORS(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, Response{Success: true})
	})

	tests := []struct {
		name           string
		origin         string
		expectedStatus int
		shouldAllow    bool
	}{
		{
			name:           "allowed origin",
			origin:         "http://localhost:1420",
			expectedStatus: http.StatusOK,
			shouldAllow:    true,
		},
		{
			name:           "blocked origin",
			origin:         "http://evil.com",
			expectedStatus: http.StatusForbidden,
			shouldAllow:    false,
		},
		{
			name:           "no origin",
			origin:         "",
			expectedStatus: http.StatusOK,
			shouldAllow:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/test", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rr := httptest.NewRecorder()

			handler(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			corsHeader := rr.Header().Get("Access-Control-Allow-Origin")
			if tt.shouldAllow && tt.origin != "" && corsHeader != tt.origin {
				t.Errorf("expected CORS header '%s', got '%s'", tt.origin, corsHeader)
			}
		})
	}
}

func TestInputValidation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		hasError bool
	}{
		{
			name:     "valid input",
			input:    "valid site name",
			maxLen:   256,
			hasError: false,
		},
		{
			name:     "too long input",
			input:    strings.Repeat("a", 300),
			maxLen:   256,
			hasError: true,
		},
		{
			name:     "empty input",
			input:    "",
			maxLen:   256,
			hasError: false,
		},
		{
			name:     "input with control chars",
			input:    "test\x00name",
			maxLen:   256,
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := api.SanitizeString(tt.input, tt.maxLen)

			if tt.hasError && err == nil {
				t.Error("expected error, got nil")
			}

			if !tt.hasError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			if !tt.hasError && len(result) > tt.maxLen {
				t.Errorf("result length %d exceeds max %d", len(result), tt.maxLen)
			}

			if !tt.hasError && strings.Contains(result, "\x00") {
				t.Error("control characters should be removed")
			}
		})
	}
}
