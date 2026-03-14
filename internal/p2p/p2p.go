package p2p

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"

	"github.com/bok1c4/pwman/internal/transport"
)

// PeerInfo is the public peer information exposed to handlers
type PeerInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Addr      string    `json:"addr"`
	Connected bool      `json:"connected"`
	LastSeen  time.Time `json:"last_seen"`
}

// SyncMessage represents a message sent between peers
type SyncMessage struct {
	Type      string `json:"type"`
	PeerID    string `json:"peer_id"`
	Payload   []byte `json:"payload"`
	Timestamp int64  `json:"timestamp"`
}

// ReceivedMessage represents a received message with peer info
type ReceivedMessage struct {
	SyncMessage
	FromPeer string // Actual peer ID from connection
}

// peerConnection is the internal TLS connection struct
type peerConnection struct {
	id          string
	name        string
	addr        string
	conn        *tls.Conn
	reader      *bufio.Reader
	writer      *bufio.Writer
	connected   bool
	lastSeen    time.Time
	fingerprint string
	mu          sync.Mutex
}

// P2PConfig configures the P2P manager
type P2PConfig struct {
	DeviceName string
	DeviceID   string
	ListenPort int
	EnableMDNS bool

	// TLS fields (optional - for Phase 2+ TLS mode)
	Cert        tls.Certificate
	PeerStore   *transport.PeerStore
	PairingMode bool
	UseTLS      bool // If true, use TLS instead of libp2p
}

type P2PManager struct {
	// Legacy libp2p fields
	host host.Host

	// TLS fields
	listener  net.Listener
	tlsConfig *tls.Config
	tlsMode   bool
	tlsPeers  map[string]*peerConnection // Internal TLS connections

	// TLS verification fields
	peerStore          *transport.PeerStore
	pairingMode        bool
	pendingFingerprint string

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
	peers  map[string]PeerInfo // Public peer info (for backward compat)

	stopOnce sync.Once

	deviceName string
	deviceID   string

	messageChan         chan ReceivedMessage
	pairingRequestChan  chan ReceivedMessage
	pairingResponseChan chan ReceivedMessage
	syncRequestChan     chan ReceivedMessage
	syncDataChan        chan ReceivedMessage
	readyForSyncChan    chan ReceivedMessage
	syncAckChan         chan ReceivedMessage

	discovery      mdns.Service
	connectedCh    chan PeerInfo
	disconnectedCh chan string
}

func NewP2PManager(cfg P2PConfig) (*P2PManager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	p := &P2PManager{
		ctx:                 ctx,
		cancel:              cancel,
		peers:               make(map[string]PeerInfo),
		tlsPeers:            make(map[string]*peerConnection),
		deviceName:          cfg.DeviceName,
		deviceID:            cfg.DeviceID,
		tlsMode:             cfg.UseTLS,
		peerStore:           cfg.PeerStore,
		pairingMode:         cfg.PairingMode,
		messageChan:         make(chan ReceivedMessage, 100),
		pairingRequestChan:  make(chan ReceivedMessage, 10),
		pairingResponseChan: make(chan ReceivedMessage, 10),
		syncRequestChan:     make(chan ReceivedMessage, 10),
		syncDataChan:        make(chan ReceivedMessage, 10),
		readyForSyncChan:    make(chan ReceivedMessage, 10),
		syncAckChan:         make(chan ReceivedMessage, 10),
		connectedCh:         make(chan PeerInfo, 10),
		disconnectedCh:      make(chan string, 10),
	}

	if cfg.DeviceID == "" {
		p.deviceID = uuid.New().String()
	}

	// Configure TLS if in TLS mode
	if cfg.UseTLS {
		p.tlsConfig = transport.ServerTLSConfig(cfg.Cert, cfg.PeerStore, cfg.PairingMode)
	}

	return p, nil
}

func (p *P2PManager) Start() error {
	if p.tlsMode {
		return p.startTLS()
	}
	return p.startLibp2p()
}

func (p *P2PManager) startTLS() error {
	// Start TLS listener on random port
	ln, err := tls.Listen("tcp", ":0", p.tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to start TLS listener: %w", err)
	}
	p.listener = ln

	fmt.Printf("[P2P] Started TLS listener on %s\n", ln.Addr())

	// Accept loop
	go p.acceptLoop()

	// Start message router
	go p.routeMessages()

	return nil
}

func (p *P2PManager) startLibp2p() error {
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", 0),
		),
		libp2p.Transport(libp2ptcp.NewTCPTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.NATPortMap(),
		libp2p.EnableRelay(),
	}

	host, err := libp2p.New(opts...)
	if err != nil {
		return fmt.Errorf("failed to create libp2p host: %w", err)
	}

	p.host = host

	if p.deviceName == "" {
		p.deviceName = fmt.Sprintf("Device-%s", p.deviceID[:8])
	}

	host.SetStreamHandler("/pwman/1.0.0", p.handleStream)

	host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(n network.Network, c network.Conn) {
			pid := c.RemotePeer()
			fmt.Printf("[P2P] Peer connected: %s\n", pid)
			p.mu.Lock()
			p.peers[pid.String()] = PeerInfo{
				ID:        pid.String(),
				Connected: true,
				LastSeen:  time.Now(),
			}
			p.mu.Unlock()

			p.connectedCh <- PeerInfo{
				ID:        pid.String(),
				Connected: true,
			}
		},
		DisconnectedF: func(n network.Network, c network.Conn) {
			pid := c.RemotePeer()
			fmt.Printf("[P2P] Peer disconnected: %s\n", pid)
			p.mu.Lock()
			if peer, ok := p.peers[pid.String()]; ok {
				peer.Connected = false
				p.peers[pid.String()] = peer
			}
			p.mu.Unlock()

			p.disconnectedCh <- pid.String()
		},
	})

	if p.enableMDNS() {
		if err := p.startMDNS(); err != nil {
			fmt.Printf("Warning: failed to start mDNS discovery: %v\n", err)
		}
	}

	fmt.Printf("[P2P] Started with ID: %s\n", p.host.ID())
	fmt.Printf("[P2P] Listening on: %s\n", p.host.Addrs())

	go p.routeMessages()

	return nil
}

func (p *P2PManager) acceptLoop() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			select {
			case <-p.ctx.Done():
				return
			default:
				fmt.Printf("[P2P] Accept error: %v\n", err)
				continue
			}
		}

		tlsConn := conn.(*tls.Conn)
		go p.handleConnection(tlsConn)
	}
}

func (p *P2PManager) handleConnection(conn *tls.Conn) {
	// Perform handshake if not already done
	if !conn.ConnectionState().HandshakeComplete {
		if err := conn.Handshake(); err != nil {
			fmt.Printf("[P2P] TLS handshake failed: %v\n", err)
			conn.Close()
			return
		}
	}

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		fmt.Printf("[P2P] No peer certificate presented\n")
		conn.Close()
		return
	}

	fingerprint := transport.CertFingerprint(state.PeerCertificates[0].Raw)

	p.mu.Lock()
	// Check if peer already exists (from ConnectToPeer)
	if existing, ok := p.tlsPeers[fingerprint]; ok {
		if existing.conn != nil {
			existing.conn.Close()
		}
	}

	peer := &peerConnection{
		id:          fingerprint,
		fingerprint: fingerprint,
		conn:        conn,
		reader:      bufio.NewReader(conn),
		writer:      bufio.NewWriter(conn),
		connected:   true,
		lastSeen:    time.Now(),
	}
	p.tlsPeers[fingerprint] = peer
	p.peers[fingerprint] = PeerInfo{
		ID:        fingerprint,
		Connected: true,
		LastSeen:  time.Now(),
	}
	p.mu.Unlock()

	p.connectedCh <- PeerInfo{
		ID:        fingerprint,
		Connected: true,
	}

	// Read loop
	for {
		line, err := peer.reader.ReadBytes('\n')
		if err != nil {
			fmt.Printf("[P2P] Connection closed: %v\n", err)
			break
		}

		var msg SyncMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			fmt.Printf("[P2P] Failed to parse message: %v\n", err)
			continue
		}

		p.handleMessage(msg, fingerprint)
	}

	// Cleanup
	conn.Close()
	p.mu.Lock()
	delete(p.tlsPeers, fingerprint)
	if peer, ok := p.peers[fingerprint]; ok {
		peer.Connected = false
		p.peers[fingerprint] = peer
	}
	p.mu.Unlock()

	p.disconnectedCh <- fingerprint
}

func (p *P2PManager) handleMessage(msg SyncMessage, fromPeer string) {
	receivedMsg := ReceivedMessage{
		SyncMessage: msg,
		FromPeer:    fromPeer,
	}

	switch msg.Type {
	case MsgTypeRequestSync:
		fmt.Printf("[P2P] Received sync request from: %s\n", fromPeer)
		p.messageChan <- receivedMsg
	case MsgTypeSyncData:
		fmt.Printf("[P2P] Received sync data from: %s\n", fromPeer)
		p.messageChan <- receivedMsg
	case MsgTypeHello:
		fmt.Printf("[P2P] Received HELLO from: %s\n", fromPeer)
		p.messageChan <- receivedMsg
	case MsgTypePairingRequest:
		fmt.Printf("[P2P] Received PAIRING_REQUEST from: %s\n", fromPeer)
		p.messageChan <- receivedMsg
	case MsgTypePairingResponse:
		fmt.Printf("[P2P] Received PAIRING_RESPONSE from: %s\n", fromPeer)
		p.messageChan <- receivedMsg
	case MsgTypeReadyForSync:
		fmt.Printf("[P2P] Received READY_FOR_SYNC from: %s\n", fromPeer)
		select {
		case p.readyForSyncChan <- receivedMsg:
		default:
			fmt.Printf("[P2P] readyForSyncChan is full, dropping message\n")
		}
	case MsgTypeSyncAck:
		fmt.Printf("[P2P] Received SYNC_ACK from: %s\n", fromPeer)
		select {
		case p.syncAckChan <- receivedMsg:
		default:
			fmt.Printf("[P2P] syncAckChan is full, dropping message\n")
		}
	default:
		fmt.Printf("[P2P] Unknown message type: %s\n", msg.Type)
	}
}

func (p *P2PManager) routeMessages() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case msg := <-p.messageChan:
			switch msg.Type {
			case MsgTypePairingRequest:
				p.pairingRequestChan <- msg
			case MsgTypePairingResponse:
				p.pairingResponseChan <- msg
			case MsgTypeRequestSync:
				p.syncRequestChan <- msg
			case MsgTypeSyncData:
				p.syncDataChan <- msg
			default:
				fmt.Printf("[P2P] Unknown message type: %s\n", msg.Type)
			}
		}
	}
}

func (p *P2PManager) handleStream(stream network.Stream) {
	fmt.Println("[P2P] New stream handler called")

	reader := bufio.NewReader(stream)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			break
		}

		if len(line) <= 1 {
			continue
		}

		var msg SyncMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		p.handleMessage(msg, stream.Conn().RemotePeer().String())
	}
}

func (p *P2PManager) enableMDNS() bool {
	return true
}

func (p *P2PManager) startMDNS() error {
	if p.host == nil {
		return fmt.Errorf("libp2p host not initialized")
	}
	mdns := mdns.NewMdnsService(p.host, "pwman", p)
	p.discovery = mdns

	if err := mdns.Start(); err != nil {
		return fmt.Errorf("failed to start mDNS: %w", err)
	}

	fmt.Println("[P2P] mDNS discovery started")
	return nil
}

func (p *P2PManager) ConnectToPeer(addr string) error {
	if p.tlsMode {
		return p.connectToPeerTLS(addr)
	}
	return p.connectToPeerLibp2p(addr)
}

func (p *P2PManager) connectToPeerTLS(addr string) error {
	// Parse address (format: "host:port")
	config := &tls.Config{
		MinVersion: tls.VersionTLS13,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("no certificate presented")
			}

			// Compute fingerprint
			fingerprint := transport.CertFingerprint(rawCerts[0])

			// Check if trusted
			if p.pairingMode {
				// In pairing mode, store for manual verification
				p.mu.Lock()
				p.pendingFingerprint = fingerprint
				p.mu.Unlock()
				return nil
			}

			if p.peerStore == nil {
				return fmt.Errorf("no peer store configured")
			}

			if !p.peerStore.IsTrusted(fingerprint) {
				return fmt.Errorf("certificate %s not trusted", fingerprint)
			}

			return nil
		},
	}

	conn, err := tls.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	// Perform handshake
	if err := conn.Handshake(); err != nil {
		conn.Close()
		return fmt.Errorf("TLS handshake failed: %w", err)
	}

	// Delegate to handleConnection for registration
	go p.handleConnection(conn)

	return nil
}

func (p *P2PManager) connectToPeerLibp2p(addr string) error {
	fmt.Printf("[P2P] ConnectToPeer called with: %s\n", addr)
	addrInfo, err := peer.AddrInfoFromString(addr)
	if err != nil {
		return fmt.Errorf("failed to parse address: %w", err)
	}

	fmt.Printf("[P2P] Dialing peer: %s at %v\n", addrInfo.ID, addrInfo.Addrs)

	if err := p.host.Connect(p.ctx, *addrInfo); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	fmt.Printf("[P2P] Successfully connected to: %s\n", addrInfo.ID)

	p.mu.Lock()
	p.peers[addrInfo.ID.String()] = PeerInfo{
		ID:        addrInfo.ID.String(),
		Connected: true,
		LastSeen:  time.Now(),
	}
	p.mu.Unlock()

	p.connectedCh <- PeerInfo{
		ID:        addrInfo.ID.String(),
		Connected: true,
		Addr:      addr,
	}

	fmt.Printf("[P2P] Connected to: %s\n", addrInfo.ID)
	return nil
}

func (p *P2PManager) DisconnectFromPeer(peerID string) error {
	if p.tlsMode {
		return p.disconnectFromPeerTLS(peerID)
	}
	return p.disconnectFromPeerLibp2p(peerID)
}

func (p *P2PManager) disconnectFromPeerTLS(peerID string) error {
	p.mu.Lock()
	peer, ok := p.tlsPeers[peerID]
	p.mu.Unlock()

	if !ok {
		return fmt.Errorf("peer not found: %s", peerID)
	}

	if peer.conn != nil {
		peer.conn.Close()
	}

	p.mu.Lock()
	delete(p.tlsPeers, peerID)
	if peer, ok := p.peers[peerID]; ok {
		peer.Connected = false
		p.peers[peerID] = peer
	}
	p.mu.Unlock()

	p.disconnectedCh <- peerID
	return nil
}

func (p *P2PManager) disconnectFromPeerLibp2p(peerID string) error {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return fmt.Errorf("failed to decode peer ID: %w", err)
	}

	if err := p.host.Network().ClosePeer(pid); err != nil {
		return fmt.Errorf("failed to disconnect: %w", err)
	}

	p.mu.Lock()
	if peer, ok := p.peers[peerID]; ok {
		peer.Connected = false
		p.peers[peerID] = peer
	}
	p.mu.Unlock()

	p.disconnectedCh <- peerID
	return nil
}

func (p *P2PManager) GetConnectedPeers() []PeerInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var connected []PeerInfo
	for _, peer := range p.peers {
		if peer.Connected {
			connected = append(connected, peer)
		}
	}
	return connected
}

func (p *P2PManager) GetAllPeers() []PeerInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	peers := make([]PeerInfo, 0, len(p.peers))
	for _, peer := range p.peers {
		peers = append(peers, peer)
	}
	return peers
}

func (p *P2PManager) GetPeerID() string {
	if p.tlsMode {
		return p.deviceID
	}
	if p.host == nil {
		return ""
	}
	return p.host.ID().String()
}

func (p *P2PManager) GetListenAddresses() []string {
	if p.tlsMode {
		if p.listener != nil {
			return []string{p.listener.Addr().String()}
		}
		return nil
	}

	if p.host == nil {
		return nil
	}

	var addrs []string
	for _, addr := range p.host.Addrs() {
		addrs = append(addrs, addr.String())
	}
	return addrs
}

func (p *P2PManager) SendMessage(peerID string, msg SyncMessage) error {
	if p.tlsMode {
		return p.sendMessageTLS(peerID, msg)
	}
	return p.sendMessageLibp2p(peerID, msg)
}

func (p *P2PManager) sendMessageTLS(peerID string, msg SyncMessage) error {
	p.mu.RLock()
	peer, ok := p.tlsPeers[peerID]
	p.mu.RUnlock()

	if !ok {
		return fmt.Errorf("peer not found: %s", peerID)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	data = append(data, '\n')

	// Lock peer for entire write operation
	peer.mu.Lock()
	defer peer.mu.Unlock()

	_, err = peer.writer.Write(data)
	if err != nil {
		return err
	}

	return peer.writer.Flush()
}

func (p *P2PManager) sendMessageLibp2p(peerID string, msg SyncMessage) error {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return fmt.Errorf("failed to decode peer ID: %w", err)
	}

	stream, err := p.host.NewStream(p.ctx, pid, "/pwman/1.0.0")
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	data = append(data, '\n')

	_, err = stream.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	fmt.Printf("[P2P] Sending message to %s: %s\n", peerID, msg.Type)
	return nil
}

func (p *P2PManager) BroadcastMessage(msg SyncMessage) {
	peers := p.GetConnectedPeers()
	for _, peer := range peers {
		go func(peerID string) {
			if err := p.SendMessage(peerID, msg); err != nil {
				fmt.Printf("[P2P] Failed to send to %s: %v\n", peerID, err)
			}
		}(peer.ID)
	}
}

func (p *P2PManager) HandlePeerFound(info peer.AddrInfo) {
	if p.host == nil {
		return
	}
	fmt.Printf("[P2P] Peer discovered via mDNS: %s with addresses: %v\n", info.ID, info.Addrs)

	if len(info.Addrs) == 0 {
		fmt.Printf("[P2P] WARNING: No addresses in peer info for %s\n", info.ID)
	}

	p.host.Peerstore().AddAddrs(info.ID, info.Addrs, 30*time.Minute)

	p.mu.Lock()
	p.peers[info.ID.String()] = PeerInfo{
		ID:        info.ID.String(),
		Connected: true,
		LastSeen:  time.Now(),
	}
	p.mu.Unlock()

	p.connectedCh <- PeerInfo{
		ID:        info.ID.String(),
		Connected: true,
	}

	fmt.Printf("[P2P] Peer discovered via mDNS: %s\n", info.ID)
}

func (p *P2PManager) ConnectedChan() <-chan PeerInfo {
	return p.connectedCh
}

func (p *P2PManager) DisconnectedChan() <-chan string {
	return p.disconnectedCh
}

func (p *P2PManager) MessageChan() <-chan ReceivedMessage {
	return p.messageChan
}

func (p *P2PManager) PairingRequestChan() <-chan ReceivedMessage {
	return p.pairingRequestChan
}

func (p *P2PManager) PairingResponseChan() <-chan ReceivedMessage {
	return p.pairingResponseChan
}

func (p *P2PManager) SyncRequestChan() <-chan ReceivedMessage {
	return p.syncRequestChan
}

func (p *P2PManager) SyncDataChan() <-chan ReceivedMessage {
	return p.syncDataChan
}

func (p *P2PManager) ReadyForSyncChan() <-chan ReceivedMessage {
	return p.readyForSyncChan
}

func (p *P2PManager) SyncAckChan() <-chan ReceivedMessage {
	return p.syncAckChan
}

func (p *P2PManager) Stop() {
	p.stopOnce.Do(func() {
		p.cancel()

		if p.discovery != nil {
			p.discovery.Close()
		}

		if p.host != nil {
			p.host.Close()
		}

		if p.listener != nil {
			p.listener.Close()
		}

		// Wait a moment for goroutines to finish
		time.Sleep(100 * time.Millisecond)

		// Close all channels
		close(p.messageChan)
		close(p.pairingRequestChan)
		close(p.pairingResponseChan)
		close(p.syncRequestChan)
		close(p.syncDataChan)
		close(p.readyForSyncChan)
		close(p.syncAckChan)
		close(p.connectedCh)
		close(p.disconnectedCh)

		fmt.Println("[P2P] Stopped")
	})
}

func (p *P2PManager) IsRunning() bool {
	if p.tlsMode {
		return p.listener != nil
	}
	return p.host != nil
}

func (p *P2PManager) SyncWithPeers(fullSync bool) error {
	peers := p.GetConnectedPeers()
	if len(peers) == 0 {
		return fmt.Errorf("no peers connected")
	}

	msg := SyncMessage{
		Type:      MsgTypeRequestSync,
		PeerID:    p.deviceID,
		Timestamp: time.Now().UnixMilli(),
	}

	for _, peer := range peers {
		if peer.Connected {
			fmt.Printf("[P2P] Sending sync request to: %s\n", peer.ID)
			if err := p.SendMessage(peer.ID, msg); err != nil {
				fmt.Printf("[P2P] Failed to send sync to %s: %v\n", peer.ID, err)
			}
		}
	}

	return nil
}

// GetPendingFingerprint returns the fingerprint of the peer waiting for manual verification during pairing mode
func (p *P2PManager) GetPendingFingerprint() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.pendingFingerprint
}

// TrustPendingPeer trusts the pending peer after user verification during pairing
func (p *P2PManager) TrustPendingPeer(deviceName, deviceID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.pendingFingerprint == "" {
		return fmt.Errorf("no pending fingerprint to trust")
	}

	if p.peerStore == nil {
		return fmt.Errorf("no peer store configured")
	}

	if err := p.peerStore.Trust(p.pendingFingerprint, deviceName, deviceID); err != nil {
		return fmt.Errorf("failed to trust peer: %w", err)
	}

	// Clear pending fingerprint after trusting
	p.pendingFingerprint = ""
	return nil
}
