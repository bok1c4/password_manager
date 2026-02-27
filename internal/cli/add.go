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
	"github.com/bok1c4/pwman/pkg/models"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var addSite, addUsername, addPassword string
var addGenerate bool
var addLength int

var addCmd = &cobra.Command{
	Use:   "add <site>",
	Short: "Add a new password entry",
	Long:  `Add a new password to the vault. The password will be encrypted for all trusted devices.`,
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
		username := addUsername
		password := addPassword

		if addGenerate {
			pwd, err := crypto.GenerateStrongPassword(addLength)
			if err != nil {
				fmt.Printf("[ERROR] Failed to generate password: %v\n", err)
				os.Exit(1)
			}
			password = pwd
			fmt.Printf("[INFO] Generated password: %s\n", password)
		} else if password == "" {
			fmt.Print("Enter password: ")
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

		entry := &models.PasswordEntry{
			ID:                uuid.New().String(),
			Version:           1,
			Site:              site,
			Username:          username,
			EncryptedPassword: encrypted.EncryptedPassword,
			EncryptedAESKeys:  encrypted.EncryptedAESKeys,
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
			UpdatedBy:         mgr.GetDeviceID(),
		}

		if err := mgr.GetStorage().CreateEntry(entry); err != nil {
			fmt.Printf("[ERROR] Failed to save entry: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[INFO] Password added for %s\n", site)
	},
}

func init() {
	addCmd.Flags().StringVarP(&addUsername, "username", "u", "", "Username for the entry")
	addCmd.Flags().StringVarP(&addPassword, "password", "p", "", "Password (will prompt if not provided)")
	addCmd.Flags().BoolVarP(&addGenerate, "generate", "g", false, "Generate a secure password")
	addCmd.Flags().IntVarP(&addLength, "length", "l", 20, "Length of generated password")
	AddCommand(addCmd)
}
