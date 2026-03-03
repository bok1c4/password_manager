package p2p

import (
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
	PeerID    string `json:"peer_id"`
	Payload   []byte `json:"payload"`
	Timestamp int64  `json:"timestamp"`
}

type P2PManager struct {
	host           host.Host
	ctx            context.Context
	cancel         context.CancelFunc
	mu             sync.RWMutex
	peers          map[string]PeerInfo
	deviceName     string
	deviceID       string
	messageChan    chan SyncMessage
	discovery      mdns.Service
	connectedCh    chan PeerInfo
	disconnectedCh chan string
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
		ctx:            ctx,
		cancel:         cancel,
		peers:          make(map[string]PeerInfo),
		deviceName:     cfg.DeviceName,
		deviceID:       cfg.DeviceID,
		messageChan:    make(chan SyncMessage, 100),
		connectedCh:    make(chan PeerInfo, 10),
		disconnectedCh: make(chan string, 10),
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

	return nil
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

	buf := make([]byte, 1024*1024)
	for {
		n, err := stream.Read(buf)
		if err != nil {
			break
		}

		var msg SyncMessage
		if err := json.Unmarshal(buf[:n], &msg); err != nil {
			fmt.Printf("[P2P] Failed to parse message: %v\n", err)
			continue
		}

		fmt.Printf("[P2P] Received message: %s from %s\n", msg.Type, msg.PeerID)
		p.handleMessage(msg, stream.Conn().RemotePeer())
	}
}

func (p *P2PManager) handleMessage(msg SyncMessage, fromPeer peer.ID) {
	switch msg.Type {
	case MsgTypeRequestSync:
		fmt.Printf("[P2P] Received sync request from: %s\n", fromPeer)
		p.messageChan <- msg
	case MsgTypeSyncData:
		fmt.Printf("[P2P] Received sync data from: %s\n", fromPeer)
		p.messageChan <- msg
	case MsgTypeHello:
		fmt.Printf("[P2P] Received HELLO from: %s\n", fromPeer)
		p.messageChan <- msg
	default:
		fmt.Printf("[P2P] Unknown message type: %s\n", msg.Type)
	}
}

func (p *P2PManager) HandlePeerFound(info peer.AddrInfo) {
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

func (p *P2PManager) MessageChan() <-chan SyncMessage {
	return p.messageChan
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
