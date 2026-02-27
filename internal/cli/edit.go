package cli

import (
	"crypto/rsa"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/crypto"
	"github.com/bok1c4/pwman/internal/device"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var editSite, editUsername, editPassword string

var editCmd = &cobra.Command{
	Use:   "edit <site>",
	Short: "Edit an existing password entry",
	Long:  `Edit an existing password entry. The password will be re-encrypted for all trusted devices.`,
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

		site := args[0]
		entry, err := mgr.GetStorage().GetEntryBySite(site)
		if err != nil {
			fmt.Printf("[ERROR] Failed to get entry for %s: %v\n", site, err)
			os.Exit(1)
		}

		username := editUsername
		password := editPassword

		if username == "" {
			username = entry.Username
		}
		if password == "" {
			fmt.Print("Enter new password: ")
			fmt.Scanln(&password)
		}

		trustedDevices, err := mgr.GetTrustedDevices()
		if err != nil {
			fmt.Printf("[ERROR] Failed to get trusted devices: %v\n", err)
			os.Exit(1)
		}

		getPublicKey := func(fingerprint string) (*rsa.PublicKey, error) {
			for _, d := range trustedDevices {
				if d.Fingerprint == fingerprint {
					return crypto.LoadPublicKey(d.PublicKey)
				}
			}
			return nil, fmt.Errorf("device not found")
		}

		encrypted, err := crypto.HybridEncrypt(password, trustedDevices, getPublicKey)
		if err != nil {
			fmt.Printf("[ERROR] Failed to encrypt password: %v\n", err)
			os.Exit(1)
		}

		entry.Username = username
		entry.EncryptedPassword = encrypted.EncryptedPassword
		entry.EncryptedAESKeys = encrypted.EncryptedAESKeys
		entry.Version++
		entry.UpdatedAt = time.Now()
		entry.UpdatedBy = mgr.GetDeviceID()

		if err := mgr.GetStorage().UpdateEntry(entry); err != nil {
			fmt.Printf("[ERROR] Failed to update entry: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[INFO] Password updated for %s\n", site)
	},
}

func init() {
	editCmd.Flags().StringVarP(&editUsername, "username", "u", "", "Username for the entry")
	editCmd.Flags().StringVarP(&editPassword, "password", "p", "", "Password (will prompt if not provided)")
	AddCommand(editCmd)
}
