package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

func TestNewRateLimiter(t *testing.T) {
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rps:      rate.Limit(10),
		burst:    20,
	}

	if rl == nil {
		t.Fatal("RateLimiter is nil")
	}

	if rl.rps != rate.Limit(10) {
		t.Errorf("rps = %v, want 10", rl.rps)
	}

	if rl.burst != 20 {
		t.Errorf("burst = %d, want 20", rl.burst)
	}

	if rl.limiters == nil {
		t.Error("limiters map should be initialized")
	}
}

func TestRateLimiterAllowsRequest(t *testing.T) {
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rps:      rate.Inf,
		burst:    100,
	}

	handler := rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRateLimiterBlocksExceeded(t *testing.T) {
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rps:      rate.Limit(1),
		burst:    1,
	}

	handler := rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("First request should succeed, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Second request should be rate limited, got %d", w.Code)
	}
}

func TestRateLimiterPerEndpoint(t *testing.T) {
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rps:      rate.Limit(1),
		burst:    1,
	}

	middleware := rl.Middleware

	handler1 := middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler2 := middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req1 := httptest.NewRequest("GET", "/api/entries", nil)
	req2 := httptest.NewRequest("GET", "/api/entries", nil)
	req3 := httptest.NewRequest("GET", "/api/entries", nil)

	w := httptest.NewRecorder()
	handler1(w, req1)
	if w.Code != http.StatusOK {
		t.Errorf("First request should succeed, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	handler2(w, req2)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Second request should be rate limited, got %d", w.Code)
	}

	req3.URL.Path = "/api/devices"
	w = httptest.NewRecorder()
	handler2(w, req3)
	if w.Code != http.StatusOK {
		t.Errorf("Different endpoint should have separate limit, got %d", w.Code)
	}
}

func TestRateLimiterDifferentIPs(t *testing.T) {
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rps:      rate.Limit(1),
		burst:    1,
	}

	handler := rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()
	handler(w, req1)

	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.2:1234"
	w = httptest.NewRecorder()
	handler(w, req2)

	if w.Code != http.StatusOK {
		t.Errorf("Different IPs should have separate limits, got %d", w.Code)
	}
}

func TestRateLimiterGetLimiterCreatesNew(t *testing.T) {
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rps:      rate.Limit(10),
		burst:    10,
	}

	limiter1 := rl.getLimiter("key1")
	limiter2 := rl.getLimiter("key2")
	limiter3 := rl.getLimiter("key1")

	if limiter1 == nil || limiter2 == nil {
		t.Fatal("Limiters should not be nil")
	}

	if limiter1 != limiter3 {
		t.Error("Same key should return same limiter")
	}

	rl.mu.Lock()
	count := len(rl.limiters)
	rl.mu.Unlock()

	if count != 2 {
		t.Errorf("Expected 2 limiters, got %d", count)
	}
}

func TestRateLimiterMiddlewareReturnsError(t *testing.T) {
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rps:      rate.Limit(0),
		burst:    0,
	}

	handler := rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	var response Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Success {
		t.Error("Response should indicate failure")
	}

	if response.Error != "rate limit exceeded" {
		t.Errorf("Error message = %q, want 'rate limit exceeded'", response.Error)
	}
}

func TestRateLimiterKeyFormat(t *testing.T) {
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rps:      rate.Inf,
		burst:    100,
	}

	handler := rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/entries", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	w := httptest.NewRecorder()

	handler(w, req)

	rl.mu.RLock()
	keys := make([]string, 0, len(rl.limiters))
	for k := range rl.limiters {
		keys = append(keys, k)
	}
	rl.mu.RUnlock()

	if len(keys) != 1 {
		t.Fatalf("Expected 1 key, got %d", len(keys))
	}

	expectedKey := "127.0.0.1:8080/api/entries"
	if keys[0] != expectedKey {
		t.Errorf("Key = %q, want %q", keys[0], expectedKey)
	}
}
