package p2p

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
)

type PeerInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Addr      string    `json:"addr"`
	Connected bool      `json:"connected"`
	LastSeen  time.Time `json:"last_seen"`
}

type SyncMessage struct {
	Type      string `json:"type"`
	PeerID    string `json:"peer_id"` // Sender's ID from message payload
	Payload   []byte `json:"payload"`
	Timestamp int64  `json:"timestamp"`
}

type ReceivedMessage struct {
	SyncMessage
	FromPeer string // Actual peer ID from connection
}

type P2PManager struct {
	host        host.Host
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
	peers       map[string]PeerInfo
	deviceName  string
	deviceID    string
	messageChan chan ReceivedMessage
	// Specialized channels for each message type
	pairingRequestChan  chan ReceivedMessage
	pairingResponseChan chan ReceivedMessage
	syncRequestChan     chan ReceivedMessage
	syncDataChan        chan ReceivedMessage
	readyForSyncChan    chan ReceivedMessage
	syncAckChan         chan ReceivedMessage
	discovery           mdns.Service
	connectedCh         chan PeerInfo
	disconnectedCh      chan string
}

type P2PConfig struct {
	DeviceName string
	DeviceID   string
	ListenPort int
	EnableMDNS bool
}

func NewP2PManager(cfg P2PConfig) (*P2PManager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	p := &P2PManager{
		ctx:                 ctx,
		cancel:              cancel,
		peers:               make(map[string]PeerInfo),
		deviceName:          cfg.DeviceName,
		deviceID:            cfg.DeviceID,
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

	return p, nil
}

func (p *P2PManager) Start() error {
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", 0),
		),
		libp2p.Transport(tcp.NewTCPTransport),
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

	// Start message router (dispatches messages to specialized channels)
	go p.routeMessages()

	return nil
}

// routeMessages dispatches incoming messages to specialized channels based on type
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

// Specialized channel accessors
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

func (p *P2PManager) enableMDNS() bool {
	return true
}

func (p *P2PManager) startMDNS() error {
	mdns := mdns.NewMdnsService(p.host, "pwman", p)
	p.discovery = mdns

	if err := mdns.Start(); err != nil {
		return fmt.Errorf("failed to start mDNS: %w", err)
	}

	fmt.Println("[P2P] mDNS discovery started")
	return nil
}

func (p *P2PManager) handleStream(stream network.Stream) {
	fmt.Println("[P2P] New stream handler called")

	reader := bufio.NewReader(stream)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			break
		}

		// Skip empty lines
		if len(line) <= 1 {
			continue
		}

		var msg SyncMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			var msgLen = len(line)
			if msgLen > 50 {
				msgLen = 50
			}
			fmt.Printf("[P2P] Failed to parse message: %v (data: %s)\n", err, string(line[:msgLen]))
			continue
		}

		fmt.Printf("[P2P] Received message: %s from %s\n", msg.Type, msg.PeerID)
		p.handleMessage(msg, stream.Conn().RemotePeer())
	}
}

func (p *P2PManager) handleMessage(msg SyncMessage, fromPeer peer.ID) {
	fmt.Printf("[P2P] handleMessage called: type=%s, fromPeer=%s, msg.PeerID=%s\n", msg.Type, fromPeer, msg.PeerID)

	receivedMsg := ReceivedMessage{
		SyncMessage: msg,
		FromPeer:    fromPeer.String(),
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
		fmt.Printf("[P2P] Received PAIRING_REQUEST from: %s (msg.PeerID=%s)\n", fromPeer, msg.PeerID)
		p.messageChan <- receivedMsg
	case MsgTypePairingResponse:
		fmt.Printf("[P2P] Received PAIRING_RESPONSE from: %s (msg.PeerID=%s)\n", fromPeer, msg.PeerID)
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

func (p *P2PManager) HandlePeerFound(info peer.AddrInfo) {
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

func (p *P2PManager) ConnectToPeer(addr string) error {
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
	fmt.Printf("[P2P] Disconnected from: %s\n", peerID)
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
	if p.host == nil {
		return ""
	}
	return p.host.ID().String()
}

func (p *P2PManager) GetListenAddresses() []string {
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

	// Add newline delimiter
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

func (p *P2PManager) OnPeerConnect(peerInfo peer.AddrInfo) {
	p.mu.Lock()
	p.peers[peerInfo.ID.String()] = PeerInfo{
		ID:        peerInfo.ID.String(),
		Connected: true,
		LastSeen:  time.Now(),
	}
	p.mu.Unlock()

	p.connectedCh <- PeerInfo{
		ID:        peerInfo.ID.String(),
		Connected: true,
	}

	fmt.Printf("[P2P] Peer connected: %s\n", peerInfo.ID)
}

func (p *P2PManager) OnPeerDisconnect(peerInfo peer.AddrInfo) {
	p.mu.Lock()
	if peer, ok := p.peers[peerInfo.ID.String()]; ok {
		peer.Connected = false
		p.peers[peerInfo.ID.String()] = peer
	}
	p.mu.Unlock()

	p.disconnectedCh <- peerInfo.ID.String()
	fmt.Printf("[P2P] Peer disconnected: %s\n", peerInfo.ID)
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

func (p *P2PManager) ReadyForSyncChan() <-chan ReceivedMessage {
	return p.readyForSyncChan
}

func (p *P2PManager) SyncAckChan() <-chan ReceivedMessage {
	return p.syncAckChan
}

func (p *P2PManager) Stop() {
	p.cancel()

	if p.discovery != nil {
		p.discovery.Close()
	}

	if p.host != nil {
		p.host.Close()
	}

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
}

func (p *P2PManager) IsRunning() bool {
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
