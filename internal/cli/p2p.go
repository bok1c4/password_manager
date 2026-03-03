package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

const apiBase = "http://localhost:18475/api"

var p2pCmd = &cobra.Command{
	Use:   "p2p",
	Short: "P2P sync commands",
	Long:  `Manage P2P connections and sync with other devices.`,
}

var p2pStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start P2P server",
	Run:   p2pAction("start", nil),
}

var p2pStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop P2P server",
	Run:   p2pAction("stop", nil),
}

var p2pStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show P2P status",
	Run:   p2pAction("status", nil),
}

var p2pPeersCmd = &cobra.Command{
	Use:   "peers",
	Short: "List connected peers",
	Run:   p2pAction("peers", nil),
}

var p2pApprovalsCmd = &cobra.Command{
	Use:   "approvals",
	Short: "List pending approvals",
	Run:   p2pAction("approvals", nil),
}

var p2pConnectCmd = &cobra.Command{
	Use:   "connect <address>",
	Short: "Connect to a peer",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		body := map[string]string{"address": args[0]}
		p2pAction("connect", body)(cmd, args)
	},
}

var p2pDisconnectCmd = &cobra.Command{
	Use:   "disconnect <peer-id>",
	Short: "Disconnect from peer",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		body := map[string]string{"peer_id": args[0]}
		p2pAction("disconnect", body)(cmd, args)
	},
}

var p2pApproveCmd = &cobra.Command{
	Use:   "approve <device-id>",
	Short: "Approve a pending device",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		body := map[string]string{"device_id": args[0]}
		p2pAction("approve", body)(cmd, args)
	},
}

var p2pRejectCmd = &cobra.Command{
	Use:   "reject <device-id> [reason]",
	Short: "Reject a pending device",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		reason := "rejected"
		if len(args) > 1 {
			reason = args[1]
		}
		body := map[string]string{"device_id": args[0], "reason": reason}
		p2pAction("reject", body)(cmd, args)
	},
}

var p2pSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync with connected peers",
	Run:   p2pAction("sync", nil),
}

func p2pAction(action string, body map[string]string) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, args []string) {
		client := &http.Client{}
		url := apiBase + "/p2p/" + action

		var req *http.Request
		var err error

		if body != nil {
			jsonBody, _ := json.Marshal(body)
			req, err = http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
			if err != nil {
				fmt.Printf("[ERROR] Failed to create request: %v\n", err)
				os.Exit(1)
			}
			req.Header.Set("Content-Type", "application/json")
		} else {
			req, err = http.NewRequest("POST", url, nil)
			if err != nil {
				fmt.Printf("[ERROR] Failed to create request: %v\n", err)
				os.Exit(1)
			}
		}

		if action == "status" || action == "peers" || action == "approvals" {
			req.Method = "GET"
		}

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("[ERROR] Failed to connect to API server: %v\n", err)
			fmt.Println("Make sure the server is running: ./server")
			os.Exit(1)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		if resp.StatusCode != 200 {
			errMsg := "unknown error"
			if e, ok := result["error"].(string); ok {
				errMsg = e
			}
			fmt.Printf("[ERROR] %s\n", errMsg)
			os.Exit(1)
		}

		if action == "status" {
			if result["success"] == true {
				fmt.Println("[INFO] P2P server is running")
			} else {
				fmt.Println("[INFO] P2P server is stopped")
			}
			return
		}

		if action == "peers" {
			data, ok := result["data"].(map[string]interface{})
			if !ok {
				fmt.Println("[INFO] No peers connected")
				return
			}
			peers, ok := data["peers"].([]interface{})
			if !ok || len(peers) == 0 {
				fmt.Println("[INFO] No peers connected")
			} else {
				fmt.Println("[INFO] Connected peers:")
				for _, p := range peers {
					peer, _ := p.(map[string]interface{})
					fmt.Printf("  - %s (%v)\n", peer["name"], peer["peer_id"])
				}
			}
			return
		}

		if action == "approvals" {
			approvals, ok := result["data"].([]interface{})
			if !ok || len(approvals) == 0 {
				fmt.Println("[INFO] No pending approvals")
			} else {
				fmt.Println("[INFO] Pending approvals:")
				for _, a := range approvals {
					approval, _ := a.(map[string]interface{})
					fmt.Printf("  - %s (%s)\n", approval["name"], approval["device_id"])
				}
			}
			return
		}

		success, _ := result["success"].(bool)
		if success {
			data, _ := result["data"].(string)
			if data != "" {
				fmt.Printf("[INFO] %s\n", data)
			} else {
				fmt.Printf("[INFO] %s completed successfully\n", action)
			}
		} else {
			errMsg := "unknown error"
			if e, ok := result["error"].(string); ok {
				errMsg = e
			}
			fmt.Printf("[ERROR] %s\n", errMsg)
		}
	}
}

func init() {
	p2pCmd.AddCommand(p2pStartCmd)
	p2pCmd.AddCommand(p2pStopCmd)
	p2pCmd.AddCommand(p2pStatusCmd)
	p2pCmd.AddCommand(p2pPeersCmd)
	p2pCmd.AddCommand(p2pConnectCmd)
	p2pCmd.AddCommand(p2pDisconnectCmd)
	p2pCmd.AddCommand(p2pApprovalsCmd)
	p2pCmd.AddCommand(p2pApproveCmd)
	p2pCmd.AddCommand(p2pRejectCmd)
	p2pCmd.AddCommand(p2pSyncCmd)

	p2pCmd.AddCommand(pairingGenerateCmd)
	p2pCmd.AddCommand(pairingJoinCmd)
	p2pCmd.AddCommand(pairingStatusCmd)

	AddCommand(p2pCmd)
}

var pairingGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a pairing code to add a new device",
	Run: func(cmd *cobra.Command, args []string) {
		resp, err := http.Post(apiBase+"/pairing/generate", "application/json", nil)
		if err != nil {
			fmt.Printf("[ERROR] Failed to connect: %v\n", err)
			return
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		if resp.StatusCode != 200 {
			errMsg := "unknown error"
			if e, ok := result["error"].(string); ok {
				errMsg = e
			}
			if e, ok := result["data"].(map[string]interface{}); ok {
				if msg, ok := e["message"].(string); ok {
					errMsg = msg
				}
			}
			fmt.Printf("[ERROR] %s\n", errMsg)
			return
		}

		data, _ := result["data"].(map[string]interface{})
		code, _ := data["code"].(string)
		deviceName, _ := data["device_name"].(string)
		expiresIn, _ := data["expires_in"].(float64)

		fmt.Printf("[INFO] Pairing code: %s\n", code)
		fmt.Printf("[INFO] Device: %s\n", deviceName)
		fmt.Printf("[INFO] Expires in: %.0f seconds\n", expiresIn)
		fmt.Printf("\n[INFO] Share this code with your other device\n")
	},
}

var pairingJoinCmd = &cobra.Command{
	Use:   "join <code>",
	Short: "Join a vault using a pairing code",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		body := map[string]string{
			"code":        args[0],
			"device_name": "New Device",
		}
		jsonBody, _ := json.Marshal(body)
		resp, err := http.Post(apiBase+"/pairing/join", "application/json", bytes.NewBuffer(jsonBody))
		if err != nil {
			fmt.Printf("[ERROR] Failed to connect: %v\n", err)
			return
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		if resp.StatusCode != 200 {
			errMsg := "unknown error"
			if e, ok := result["error"].(string); ok {
				errMsg = e
			}
			fmt.Printf("[ERROR] %s\n", errMsg)
			return
		}

		data, _ := result["data"].(map[string]interface{})
		deviceName, _ := data["device_name"].(string)

		fmt.Printf("[INFO] Successfully joined vault from: %s\n", deviceName)
		fmt.Printf("[INFO] Syncing passwords...\n")
	},
}

var pairingStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check pairing status",
	Run: func(cmd *cobra.Command, args []string) {
		resp, err := http.Get(apiBase + "/pairing/status")
		if err != nil {
			fmt.Printf("[ERROR] Failed to connect: %v\n", err)
			return
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		data, _ := result["data"].(map[string]interface{})
		active, _ := data["active"].(bool)

		if !active {
			fmt.Printf("[INFO] No active pairing session\n")
			return
		}

		deviceName, _ := data["device_name"].(string)
		expiresIn, _ := data["expires_in"].(float64)

		fmt.Printf("[INFO] Active pairing session\n")
		fmt.Printf("[INFO] Device: %s\n", deviceName)
		fmt.Printf("[INFO] Expires in: %.0f seconds\n", expiresIn)
	},
}
