package discovery

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/grandcat/zeroconf"
)

// PeerInfo represents a discovered peer
type PeerInfo struct {
	VaultID    string
	DeviceName string
	Addr       net.IP
	Port       int
	Hostname   string
}

// Browse scans the LAN for a specific vaultID
func Browse(ctx context.Context, vaultID string, timeout time.Duration) (*PeerInfo, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry, 8)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err = resolver.Browse(ctx, ServiceType, Domain, entries)
	if err != nil {
		return nil, fmt.Errorf("failed to browse: %w", err)
	}

	for {
		select {
		case entry := <-entries:
			if entry == nil {
				return nil, fmt.Errorf("vault %q not found", vaultID)
			}

			// Check if this entry matches our vault
			for _, txt := range entry.Text {
				if txt == fmt.Sprintf("vault=%s", vaultID) {
					var ip net.IP
					if len(entry.AddrIPv4) > 0 {
						ip = entry.AddrIPv4[0]
					} else if len(entry.AddrIPv6) > 0 {
						ip = entry.AddrIPv6[0]
					}

					return &PeerInfo{
						VaultID:    vaultID,
						DeviceName: extractTXT(entry.Text, "device"),
						Addr:       ip,
						Port:       entry.Port,
						Hostname:   entry.HostName,
					}, nil
				}
			}

		case <-ctx.Done():
			return nil, fmt.Errorf("vault %q not found within %s", vaultID, timeout)
		}
	}
}

// BrowseAll scans for all available vaults (for discovery UI)
func BrowseAll(ctx context.Context, timeout time.Duration) ([]PeerInfo, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry, 8)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err = resolver.Browse(ctx, ServiceType, Domain, entries)
	if err != nil {
		return nil, fmt.Errorf("failed to browse: %w", err)
	}

	var results []PeerInfo

	for {
		select {
		case entry := <-entries:
			if entry == nil {
				return results, nil
			}

			var ip net.IP
			if len(entry.AddrIPv4) > 0 {
				ip = entry.AddrIPv4[0]
			} else if len(entry.AddrIPv6) > 0 {
				ip = entry.AddrIPv6[0]
			}

			results = append(results, PeerInfo{
				VaultID:    extractTXT(entry.Text, "vault"),
				DeviceName: extractTXT(entry.Text, "device"),
				Addr:       ip,
				Port:       entry.Port,
				Hostname:   entry.HostName,
			})

		case <-ctx.Done():
			return results, nil
		}
	}
}

func extractTXT(texts []string, key string) string {
	prefix := key + "="
	for _, txt := range texts {
		if len(txt) > len(prefix) && txt[:len(prefix)] == prefix {
			return txt[len(prefix):]
		}
	}
	return ""
}
