package cli

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/atotto/clipboard"
	"github.com/bok1c4/pwman/internal/config"
	"github.com/bok1c4/pwman/internal/crypto"
	"github.com/bok1c4/pwman/internal/device"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var getSite string
var getClipboard bool

var getCmd = &cobra.Command{
	Use:   "get <site>",
	Short: "Get a password entry",
	Long:  `Retrieve and decrypt a password entry for the specified site.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil || cfg == nil {
			fmt.Println("[ERROR] Vault not initialized. Run 'pwman init' first")
			os.Exit(1)
		}

		fmt.Print("Enter vault password: ")
		password, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			fmt.Println("[ERROR] Failed to read password")
			os.Exit(1)
		}

		mgr, _, err := device.LoadVault(string(password))
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

		pass, err := crypto.HybridDecrypt(entry, mgr.GetPrivateKey())
		if err != nil {
			fmt.Printf("[ERROR] Failed to decrypt password: %v\n", err)
			os.Exit(1)
		}

		if getClipboard {
			if err := clipboard.WriteAll(pass); err != nil {
				fmt.Printf("[ERROR] Failed to copy to clipboard: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("[INFO] Password for %s copied to clipboard (clears in 30 seconds)\n", site)

			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()

			go func(password string) {
				select {
				case <-ctx.Done():
					current, _ := clipboard.ReadAll()
					if current == password {
						clipboard.WriteAll("")
					}
				case <-time.After(30 * time.Second):
					current, _ := clipboard.ReadAll()
					if current == password {
						clipboard.WriteAll("")
					}
				}
			}(pass)
		} else {
			fmt.Printf("[INFO] Password for %s:\n%s\n", site, pass)
		}
	},
}

func init() {
	getCmd.Flags().StringVar(&getSite, "site", "", "Site name")
	getCmd.Flags().BoolVarP(&getClipboard, "clipboard", "c", false, "Copy password to clipboard (clears after 30 seconds)")
	AddCommand(getCmd)
}
