package client

import (
	"P2P-GO/pkg/filetransfer"
	p2prtc "P2P-GO/pkg/p2p-RTC"
	"P2P-GO/pkg/signaling"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/gorilla/websocket"
)

type P2PClient struct {
	mu              sync.Mutex
	signalingServer string
	wsConn          *websocket.Conn
	peerConnections map[string]*p2prtc.PeerConnection
	fileTransfers   map[string]*filetransfer.FileTransfer
	receiveDir      string
}

func NewP2PClient(signalingServer string) *P2PClient {
    return &P2PClient{
        signalingServer:  signalingServer,
        peerConnections:  make(map[string]*p2prtc.PeerConnection),
        fileTransfers:    make(map[string]*filetransfer.FileTransfer),
        receiveDir:       "./downloads", // Default receive directory
    }
}

func (c *P2PClient) SetReceiveDirectory(dir string) error {
    // Create directory if it doesn't exist
    err := os.MkdirAll(dir, 0755)
    if err != nil {
        return fmt.Errorf("failed to create receive directory: %v", err)
    }

    c.mu.Lock()
    defer c.mu.Unlock()
    c.receiveDir = dir
    return nil
}

func (c *P2PClient) GetAvailablePeers() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	peers := make([]string,0,len(c.peerConnections))
	for peerID := range c.peerConnections {
		peers = append(peers, peerID)
	}
	return peers
}

//connecting to signal server
func (c *P2PClient) Connect() error {
	conn, _ , err:= websocket.DefaultDialer.Dial(c.signalingServer,nil)
	if err != nil {
		return fmt.Errorf("failed to connect to signaling server: %v", err)
	}
	c.wsConn = conn

	go c.handleSignalMessages()
	return nil
}

func (c* P2PClient) handleSignalMessages() {
	for {
		var message signaling.SignalMessage
		err := c.wsConn.ReadJSON(&message)
		if err != nil {
			log.Printf("Signaling message error: %v", err)
            return	
		}

		c.processSignalingMessages(message)
	}
}

func (c *P2PClient) processSignalingMessages(message signaling.SignalMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch message.Type {
    case "peers":
        c.handlePeerList(message.Payload)
    case "offer":
        c.handleIncomingOffer(message)
    case "answer":
        c.handleIncomingAnswer(message)
    case "ice-candidate":
        c.handleICECandidate(message)
    }
}

func (c *P2PClient) SendFileToPeer(peerID, filePath string) error {
    c.mu.Lock()
    defer c.mu.Unlock()

    fileTransfer, exists := c.fileTransfers[peerID]
    if !exists {
        return fmt.Errorf("no peer connection for %s", peerID)
    }

    return fileTransfer.SendFile(filePath)
}

func (c *P2PClient) Close() {
    c.mu.Lock()
    defer c.mu.Unlock()

    if c.wsConn != nil {
        c.wsConn.Close()
    }

    for _, pc := range c.peerConnections {
        pc.Close()
    }
}

// Placeholder methods - need to implement these based on specific signaling and WebRTC logic
func (c *P2PClient) handlePeerList(peers interface{}) {}
func (c *P2PClient) handleIncomingOffer(message signaling.SignalMessage) {}
func (c *P2PClient) handleIncomingAnswer(message signaling.SignalMessage) {}
func (c *P2PClient) handleICECandidate(message signaling.SignalMessage) {}