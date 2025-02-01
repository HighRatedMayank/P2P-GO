package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"P2P-GO/pkg/filetransfer"
	"P2P-GO/pkg/p2p-RTC"
)

// SignalMessage is our custom signaling message format.
// In our signaling package the server uses a similar structure.
type SignalMessage struct {
	Type    string          `json:"type"`           // e.g. "offer", "answer", "candidate", "peers"
	FromId  string          `json:"fromId"`         // Sender's ID
	ToId    string          `json:"toId,omitempty"` // Recipient's ID (if applicable)
	Payload json.RawMessage `json:"payload"`        // Payload (SDP offer/answer, ICE candidate, or peer list)
}

func main() {
	// Command-line flags:
	signalURL := flag.String("signal", "ws://localhost:8080/ws", "Signaling server URL")
	filePath := flag.String("file", "", "Path to file to send (if empty, acts as receiver)")
	remotePeerID := flag.String("peer", "", "Remote peer ID to connect to (for sending)")
	clientID := flag.String("id", "", "Your client ID (optional)")
	flag.Parse()

	// Connect to the signaling server via WebSocket.
	wsConn, _, err := websocket.DefaultDialer.Dial(*signalURL, nil)
	if err != nil {
		log.Fatalf("Failed to connect to signaling server: %v", err)
	}
	defer wsConn.Close()

	// If no client ID is provided, generate one.
	if *clientID == "" {
		*clientID = fmt.Sprintf("client-%d", time.Now().UnixNano())
	}
	log.Printf("Client ID: %s", *clientID)

	// Create a channel for outgoing signaling messages.
	sendChan := make(chan []byte, 10)

	// Start a goroutine to send messages from sendChan to the WebSocket.
	go func() {
		for msg := range sendChan {
			if err := wsConn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("Error writing message: %v", err)
			}
		}
	}()

	// Create a new WebRTC PeerConnection using our p2p-rtc package.
	peer, err := p2prtc.CreatePeerConncetion()
	if err != nil {
		log.Fatalf("Error creating peer connection: %v", err)
	}

	// Create a DataChannel for file transfer.
	if err = peer.CreateDataChannel(); err != nil {
		log.Fatalf("Error creating data channel: %v", err)
	}

	// Setup file transfer over the DataChannel.
	ft := filetransfer.NewFileTransfer(peer.DataChannel)
	ft.SetupDataChannelHandlers()

	// Use a mutex to help coordinate whether we’ve already sent or processed an offer.
	var mu sync.Mutex
	offerSent := false

	// Start a goroutine that reads messages from the signaling server.
	go func() {
		for {
			_, message, err := wsConn.ReadMessage()
			if err != nil {
				log.Printf("Error reading from signaling server: %v", err)
				return
			}

			var sigMsg SignalMessage
			if err := json.Unmarshal(message, &sigMsg); err != nil {
				log.Printf("Error unmarshaling signaling message: %v", err)
				continue
			}

			switch sigMsg.Type {
			case "offer":
				// When receiving an offer, assume we are the callee.
				log.Printf("Received offer from %s", sigMsg.FromId)
				mu.Lock()
				if !offerSent {
					var offer string
					if err := json.Unmarshal(sigMsg.Payload, &offer); err != nil {
						log.Printf("Error parsing offer payload: %v", err)
						mu.Unlock()
						continue
					}
					// Process the offer and create an answer.
					answer, err := peer.HandleOffer(offer)
					if err != nil {
						log.Printf("Error handling offer: %v", err)
						mu.Unlock()
						continue
					}
					// Build the answer signaling message.
					resp := SignalMessage{
						Type:   "answer",
						FromId: *clientID,
						ToId:   sigMsg.FromId,
						// Wrap answer as JSON string.
						Payload: json.RawMessage([]byte(`"` + answer + `"`)),
					}
					respBytes, _ := json.Marshal(resp)
					sendChan <- respBytes
					log.Printf("Sent answer to %s", sigMsg.FromId)
					offerSent = true
				}
				mu.Unlock()

			case "answer":
				// When receiving an answer, we are the caller.
				log.Printf("Received answer from %s", sigMsg.FromId)
				var answer string
				if err := json.Unmarshal(sigMsg.Payload, &answer); err != nil {
					log.Printf("Error parsing answer payload: %v", err)
					continue
				}
				if err := peer.HandleAnswer(answer); err != nil {
					log.Printf("Error handling answer: %v", err)
				}

			case "candidate":
				// Process incoming ICE candidate.
				log.Printf("Received ICE candidate from %s", sigMsg.FromId)
				var candidate string
				if err := json.Unmarshal(sigMsg.Payload, &candidate); err != nil {
					log.Printf("Error parsing candidate payload: %v", err)
					continue
				}
				if err := peer.AddIceCandidate(candidate); err != nil {
					log.Printf("Error adding ICE candidate: %v", err)
				}

			case "peers":
				// Peer list update (for informational purposes).
				log.Printf("Peer list update: %s", string(sigMsg.Payload))

			default:
				log.Printf("Unknown signaling message type: %s", sigMsg.Type)
			}
		}
	}()

	// Decide our role: if a file path is provided, act as the caller (sender).
	// Otherwise, act as a receiver.
	if *filePath != "" {
		// Ensure we have a remote peer ID.
		if *remotePeerID == "" {
			log.Fatal("For sending a file, you must specify a remote peer ID using the -peer flag")
		}
		// Caller role: create an SDP offer.
		offer, err := peer.CreateOffer()
		if err != nil {
			log.Fatalf("Error creating offer: %v", err)
		}

		// Build the offer signaling message.
		offerMsg := SignalMessage{
			Type:    "offer",
			FromId:  *clientID,
			ToId:    *remotePeerID,
			Payload: json.RawMessage([]byte(`"` + offer + `"`)),
		}
		offerBytes, _ := json.Marshal(offerMsg)
		sendChan <- offerBytes
		log.Printf("Sent offer to %s", *remotePeerID)

		mu.Lock()
		offerSent = true
		mu.Unlock()

		// Wait for ICE connection before starting the file transfer.
		if !peer.WaitForConnection(10 * time.Second) {
			log.Fatal("ICE connection not established within timeout")
		}

		// Send the file over the data channel.
		if err = ft.SendFile(*filePath); err != nil {
			log.Fatalf("Error sending file: %v", err)
		}
		log.Println("File sent successfully!")
	} else {
		// Callee role: wait for an incoming offer and file transfer.
		log.Println("Waiting to receive a file. Press Enter when ready to start receiving...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')

		// Wait for ICE connection to be established.
		if !peer.WaitForConnection(10 * time.Second) {
			log.Fatal("ICE connection not established within timeout")
		}

		// Block until the entire file is received.
		if err = ft.ReceiveFile(); err != nil {
			log.Fatalf("Error receiving file: %v", err)
		}
		log.Println("File received and saved successfully!")
	}

	// Allow some time for any pending signaling messages to be processed.
	time.Sleep(2 * time.Second)
	peer.Close()
	close(sendChan)
}

// Optional: A helper function to cancel the operation after a timeout (not strictly necessary)
func withTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}
