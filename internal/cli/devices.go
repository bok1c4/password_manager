package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"

	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/device"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var devicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "Manage devices",
	Long:  `Manage trusted devices for multi-device sync.`,
}

var devicesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all devices",
	Long:  `List all devices in the vault.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil || cfg == nil {
			fmt.Println("[ERROR] Vault not initialized. Run 'pwman init' first")
			os.Exit(1)
		}

		fmt.Print("Enter vault password: ")
		vaultPassword, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			fmt.Println("[ERROR] Failed to read password")
			os.Exit(1)
		}

		mgr, _, err := device.LoadVault(string(vaultPassword))
		if err != nil {
			fmt.Printf("[ERROR] Failed to unlock vault: %v\n", err)
			os.Exit(1)
		}

		devices, err := mgr.ListDevices()
		if err != nil {
			fmt.Printf("[ERROR] Failed to list devices: %v\n", err)
			os.Exit(1)
		}

		if len(devices) == 0 {
			fmt.Println("[INFO] No devices found")
			return
		}

		fmt.Printf("%-40s %-20s %s\n", "NAME", "FINGERPRINT", "TRUSTED")
		fmt.Println("---------------------------------------------------------------")
		for _, d := range devices {
			trusted := "no"
			if d.Trusted {
				trusted = "yes"
			}
			fmt.Printf("%-40s %-20s %s\n", d.Name, d.Fingerprint[:20], trusted)
		}
	},
}

var devicesExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export this device's public key",
	Long:  `Export this device's public key for sharing with other devices.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil || cfg == nil {
			fmt.Println("[ERROR] Vault not initialized. Run 'pwman init' first")
			os.Exit(1)
		}

		publicKey, err := os.ReadFile(config.PublicKeyPath())
		if err != nil {
			fmt.Printf("[ERROR] Failed to read public key: %v\n", err)
			os.Exit(1)
		}

		keyData := map[string]string{
			"name":        cfg.DeviceName,
			"device_id":   cfg.DeviceID,
			"fingerprint": "",
			"public_key":  string(publicKey),
		}

		data, err := json.MarshalIndent(keyData, "", "  ")
		if err != nil {
			fmt.Printf("[ERROR] Failed to marshal key data: %v\n", err)
			os.Exit(1)
		}

		fmt.Println(string(data))
	},
}

var devicesAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a new device",
	Long:  `Add a new device by providing its public key export. This will generate an approval code.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil || cfg == nil {
			fmt.Println("[ERROR] Vault not initialized. Run 'pwman init' first")
			os.Exit(1)
		}

		fmt.Print("Enter vault password: ")
		vaultPassword, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			fmt.Println("[ERROR] Failed to read password")
			os.Exit(1)
		}

		mgr, _, err := device.LoadVault(string(vaultPassword))
		if err != nil {
			fmt.Printf("[ERROR] Failed to unlock vault: %v\n", err)
			os.Exit(1)
		}

		name := args[0]

		fmt.Print("Paste the device's public key export (JSON): ")
		var keyData map[string]string
		if err := json.NewDecoder(os.Stdin).Decode(&keyData); err != nil {
			fmt.Printf("[ERROR] Failed to read device key: %v\n", err)
			os.Exit(1)
		}

		publicKey := []byte(keyData["public_key"])
		d, approvalCode, err := mgr.AddDeviceWithApproval(publicKey, name)
		if err != nil {
			fmt.Printf("[ERROR] Failed to add device: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[INFO] Device '%s' added (ID: %s)\n", d.Name, d.ID)
		fmt.Printf("[INFO] Approval code: %s\n", approvalCode)
		fmt.Println("[INFO] Share this code with the new device owner")
		fmt.Println("[INFO] After they approve, run 'pwman devices trust' or sync will push the updated vault")
	},
}

var devicesTrustCmd = &cobra.Command{
	Use:   "trust <device-id>",
	Short: "Trust a device",
	Long:  `Trust a device to enable password sharing. This will re-encrypt all passwords for the new device.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil || cfg == nil {
			fmt.Println("[ERROR] Vault not initialized. Run 'pwman init' first")
			os.Exit(1)
		}

		fmt.Print("Enter vault password: ")
		vaultPassword, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			fmt.Println("[ERROR] Failed to read password")
			os.Exit(1)
		}

		mgr, _, err := device.LoadVault(string(vaultPassword))
		if err != nil {
			fmt.Printf("[ERROR] Failed to unlock vault: %v\n", err)
			os.Exit(1)
		}

		deviceID := args[0]

		if err := mgr.TrustDevice(deviceID); err != nil {
			fmt.Printf("[ERROR] Failed to trust device: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[INFO] Device %s is now trusted\n", deviceID)
		fmt.Println("[INFO] Re-encrypting all passwords for new device...")

		if err := mgr.ReEncryptForNewDevice(deviceID); err != nil {
			fmt.Printf("[ERROR] Failed to re-encrypt passwords: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("[INFO] All passwords re-encrypted successfully")
		fmt.Println("[INFO] Run 'pwman sync push' to share with the new device")
	},
}

var devicesApproveCmd = &cobra.Command{
	Use:   "approve <code>",
	Short: "Approve this device using a code from an existing device",
	Long:  `Approve this device using a one-time code from an existing device. This will trust this device and re-encrypt all passwords.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil || cfg == nil {
			fmt.Println("[ERROR] Vault not initialized. Run 'pwman init' first")
			os.Exit(1)
		}

		fmt.Print("Enter vault password: ")
		vaultPassword, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			fmt.Println("[ERROR] Failed to read password")
			os.Exit(1)
		}

		mgr, _, err := device.LoadVault(string(vaultPassword))
		if err != nil {
			fmt.Printf("[ERROR] Failed to unlock vault: %v\n", err)
			os.Exit(1)
		}

		approvalCode := args[0]

		deviceID, err := mgr.ApproveDevice(approvalCode)
		if err != nil {
			fmt.Printf("[ERROR] Failed to approve device: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[INFO] Device approved (ID: %s)\n", deviceID)
		fmt.Println("[INFO] Re-encrypting all passwords for this device...")

		if err := mgr.ReEncryptForNewDevice(deviceID); err != nil {
			fmt.Printf("[WARN] Failed to re-encrypt: %v\n", err)
		} else {
			fmt.Println("[INFO] All passwords encrypted for this device")
		}
	},
}

func init() {
	devicesCmd.AddCommand(devicesListCmd)
	devicesCmd.AddCommand(devicesExportCmd)
	devicesCmd.AddCommand(devicesAddCmd)
	devicesCmd.AddCommand(devicesTrustCmd)
	devicesCmd.AddCommand(devicesApproveCmd)
	AddCommand(devicesCmd)
}
