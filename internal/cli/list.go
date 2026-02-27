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

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all password entries",
	Long:  `List all password entries in the vault (metadata only, passwords are not shown).`,
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

		entries, err := mgr.GetStorage().ListEntries()
		if err != nil {
			fmt.Printf("[ERROR] Failed to list entries: %v\n", err)
			os.Exit(1)
		}

		if len(entries) == 0 {
			fmt.Println("[INFO] No entries found")
			return
		}

		fmt.Printf("%-30s %-30s\n", "SITE", "USERNAME")
		fmt.Println("---------------------------------------------------------------")
		for _, e := range entries {
			fmt.Printf("%-30s %-30s\n", e.Site, e.Username)
		}
	},
}

func init() {
	AddCommand(listCmd)
}
