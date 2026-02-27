package cli

import (
	"fmt"
	"os"
	"syscall"

	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/device"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <site>",
	Short: "Delete a password entry",
	Long:  `Delete a password entry from the vault (soft delete).`,
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

		if err := mgr.GetStorage().DeleteEntry(entry.ID); err != nil {
			fmt.Printf("[ERROR] Failed to delete entry: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[INFO] Password deleted for %s\n", site)
	},
}

func init() {
	AddCommand(deleteCmd)
}
