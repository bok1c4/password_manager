// Package transport provides TLS certificate management for the P2P password manager.
package transport

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

// GenerateTLSCert creates a self-signed TLS certificate for P2P communication
// The certificate is valid for 365 days and includes localhost and LAN addresses
func GenerateTLSCert() (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate private key: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "pwman-device",
		},
		NotBefore: time.Now().Add(-time.Hour), // Allow for clock skew
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
		},
		DNSNames: []string{
			"localhost",
			"pwman.local",
		},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template,
		&priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	privDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to marshal private key: %w", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})

	return tls.X509KeyPair(certPEM, privPEM)
}

// ServerTLSConfig returns TLS config for P2P server (Device A)
// pairingMode: if true, accepts any certificate (during initial pairing)
func ServerTLSConfig(cert tls.Certificate, peers *PeerStore, pairingMode bool) *tls.Config {
	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		ClientAuth:         tls.RequireAnyClientCert,
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true, // We verify manually in VerifyPeerCertificate
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("no client certificate presented")
			}

			if pairingMode {
				// During pairing: accept any cert, will pin after TOTP verification
				return nil
			}

			fp := CertFingerprint(rawCerts[0])
			if !peers.IsTrusted(fp) {
				return fmt.Errorf("untrusted peer certificate: %s", fp)
			}
			return nil
		},
	}
}

// ClientTLSConfig returns TLS config for P2P client (Device B)
// pairingMode: if true, accepts any certificate (during initial pairing)
func ClientTLSConfig(cert tls.Certificate, peers *PeerStore, pairingMode bool) *tls.Config {
	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true, // We verify manually in VerifyPeerCertificate
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("no server certificate presented")
			}

			if pairingMode {
				// During pairing: accept any cert, user verifies manually via fingerprint
				return nil
			}

			fp := CertFingerprint(rawCerts[0])
			if !peers.IsTrusted(fp) {
				return fmt.Errorf("untrusted server certificate: %s", fp)
			}
			return nil
		},
	}
}

// SaveTLSCert saves a TLS certificate and key to files
func SaveTLSCert(cert tls.Certificate, certPath, keyPath string) error {
	// Note: tls.Certificate doesn't expose the raw data directly
	// In practice, you'd generate and save the cert/key before loading
	// This function is here for API completeness
	return fmt.Errorf("SaveTLSCert not implemented - generate and save before loading")
}

// LoadTLSCert loads a TLS certificate and key from files
func LoadTLSCert(certPath, keyPath string) (tls.Certificate, error) {
	return tls.LoadX509KeyPair(certPath, keyPath)
}
