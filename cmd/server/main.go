package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/bok1c4/pwman/cmd/server/handlers"
	"github.com/bok1c4/pwman/internal/api"
	"github.com/bok1c4/pwman/internal/middleware"
	"github.com/bok1c4/pwman/internal/state"
)

var (
	serverPort      = os.Getenv("PWMAN_PORT")
	serverPortRange = os.Getenv("PWMAN_PORT_RANGE")
	authManager     = api.NewAuthManager()
)

func init() {
	if serverPort == "" {
		serverPort = "18475"
	}
}

func findAvailablePort() (string, error) {
	if serverPort != "" {
		ln, err := net.Listen("tcp", ":"+serverPort)
		if err != nil {
			return "", fmt.Errorf("port %s is not available: %w", serverPort, err)
		}
		ln.Close()
		return serverPort, nil
	}

	if serverPortRange != "" {
		parts := strings.Split(serverPortRange, "-")
		if len(parts) == 2 {
			start, err := strconv.Atoi(parts[0])
			if err != nil {
				return "", fmt.Errorf("invalid port range: %w", err)
			}
			end, err := strconv.Atoi(parts[1])
			if err != nil {
				return "", fmt.Errorf("invalid port range: %w", err)
			}
			for port := start; port <= end; port++ {
				ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
				if err == nil {
					ln.Close()
					return strconv.Itoa(port), nil
				}
			}
		}
	}

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return strconv.Itoa(port), nil
}

type Response struct {
	Success bool        `json:"success"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func jsonResponse(w http.ResponseWriter, v Response) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func main() {
	serverState := state.NewServerState()

	authHandlers := handlers.NewAuthHandlers(serverState, authManager)
	entryHandlers := handlers.NewEntryHandlers(serverState)
	vaultHandlers := handlers.NewVaultHandlers(serverState)
	deviceHandlers := handlers.NewDeviceHandlers(serverState)
	healthHandlers := handlers.NewHealthHandlers(serverState)
	p2pHandlers := handlers.NewP2PHandlers(serverState, authManager)
	pairingHandlers := handlers.NewPairingHandlers(serverState, authManager, p2pHandlers)

	p2pHandlers.SetPairingHandlers(pairingHandlers)

	cors := middleware.CORS
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return cors(middleware.Auth(authManager, next))
	}

	http.HandleFunc("/api/init", cors(authHandlers.Init))
	http.HandleFunc("/api/unlock", cors(authHandlers.Unlock))
	http.HandleFunc("/api/lock", auth(authHandlers.Lock))
	http.HandleFunc("/api/is_unlocked", cors(authHandlers.IsUnlocked))
	http.HandleFunc("/api/is_initialized", cors(authHandlers.IsInitialized))

	http.HandleFunc("/api/entries", auth(entryHandlers.List))
	http.HandleFunc("/api/entries/add", auth(entryHandlers.Add))
	http.HandleFunc("/api/entries/update", auth(entryHandlers.Update))
	http.HandleFunc("/api/entries/delete", auth(entryHandlers.Delete))
	http.HandleFunc("/api/entries/get_password", auth(entryHandlers.GetPassword))

	http.HandleFunc("/api/devices", auth(deviceHandlers.List))

	http.HandleFunc("/api/generate", auth(healthHandlers.GeneratePassword))

	// Public vault endpoints (no auth required - needed before login)
	http.HandleFunc("/api/vaults", cors(vaultHandlers.List))
	http.HandleFunc("/api/vaults/use", cors(vaultHandlers.Use)) // Public - needed to select vault before unlock
	// Authenticated vault operations
	http.HandleFunc("/api/vaults/create", auth(vaultHandlers.Create))
	// Public - requires password verification in handler instead of auth token
	http.HandleFunc("/api/vaults/delete", cors(vaultHandlers.Delete))

	http.HandleFunc("/api/ping", cors(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, Response{Success: true, Data: "pong"})
	}))

	http.HandleFunc("/api/health", cors(healthHandlers.Health))
	http.HandleFunc("/api/metrics", auth(healthHandlers.Metrics))

	http.HandleFunc("/api/p2p/status", auth(p2pHandlers.Status))
	http.HandleFunc("/api/p2p/start", auth(p2pHandlers.Start))
	http.HandleFunc("/api/p2p/stop", auth(p2pHandlers.Stop))
	http.HandleFunc("/api/p2p/peers", auth(p2pHandlers.Peers))
	http.HandleFunc("/api/p2p/connect", auth(p2pHandlers.Connect))
	http.HandleFunc("/api/p2p/disconnect", auth(p2pHandlers.Disconnect))
	http.HandleFunc("/api/p2p/approvals", auth(p2pHandlers.Approvals))
	http.HandleFunc("/api/p2p/approve", auth(p2pHandlers.Approve))
	http.HandleFunc("/api/p2p/reject", auth(p2pHandlers.Reject))
	http.HandleFunc("/api/p2p/sync", auth(p2pHandlers.Sync))

	http.HandleFunc("/api/pairing/generate", auth(pairingHandlers.Generate))
	http.HandleFunc("/api/pairing/join", auth(pairingHandlers.Join))
	http.HandleFunc("/api/pairing/status", auth(pairingHandlers.Status))

	port, err := findAvailablePort()
	if err != nil {
		log.Fatalf("Failed to find available port: %v", err)
	}

	log.Printf("Starting pwman API server on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
