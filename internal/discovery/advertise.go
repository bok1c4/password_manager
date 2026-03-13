// Package discovery provides mDNS service advertisement and discovery
// using the standard zeroconf library on port 5353.
package discovery

import (
	"context"
	"fmt"

	"github.com/grandcat/zeroconf"
)

const (
	// ServiceType is the mDNS service type for pwman
	ServiceType = "_pwman._tcp"
	// Domain is the mDNS domain
	Domain = "local."
)

// Advertiser handles mDNS service advertisement
type Advertiser struct {
	server *zeroconf.Server
	ctx    context.Context
	cancel context.CancelFunc
}

// Announce broadcasts this device on the LAN
// vaultID is a public UUID - not the master password or vault name
// Only non-secret metadata in TXT records (visible to all LAN devices)
func Announce(vaultID, deviceName string, port int) (*Advertiser, error) {
	txt := []string{
		fmt.Sprintf("vault=%s", vaultID),
		fmt.Sprintf("device=%s", deviceName),
		"version=2", // Version 2 indicates new TLS protocol
	}

	server, err := zeroconf.Register(
		deviceName,
		ServiceType,
		Domain,
		port,
		txt,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("mDNS announce: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Advertiser{
		server: server,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Shutdown stops the mDNS advertisement
func (a *Advertiser) Shutdown() error {
	a.cancel()
	if a.server != nil {
		a.server.Shutdown()
	}
	return nil
}

// Wait blocks until the advertiser is shut down
func (a *Advertiser) Wait() {
	<-a.ctx.Done()
}
