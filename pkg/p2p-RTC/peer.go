package p2prtc

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/pion/webrtc/v3"
)

type PeerConnection struct {
	Conn         *webrtc.PeerConnection
	DataChannel  *webrtc.DataChannel
	IceConnected chan bool
	peerID       string
	SendCandidateFunc func([]byte)
}

type ICECandidate struct {
	Candidate string `json:"candidate"`
	SDPMid    string `json:"sdpMid"`
	SDPIndex  int    `json:"sdpIndex"`
}

//returns public stun srevers 
func NewPeerConnectionConfig() *webrtc.Configuration {
	return &webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
					"stun:stun1.l.google.com:19302",
				},
			},

			//will add turn server over here
			// {
			// }
		},
	}
}

func CreatePeerConncetion() (*PeerConnection, error) {
	config := NewPeerConnectionConfig()

	//new rtc peer connection
	peerConnection, err := webrtc.NewPeerConnection(*config)

	if err != nil {
		panic(err)
	}

	pc := &PeerConnection{
		Conn:         peerConnection,
		IceConnected: make(chan bool),
	}

	//ice connection state handling
	peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ICE Connection State: %s", state.String())

		switch state {
		case webrtc.ICEConnectionStateConnected:
			pc.IceConnected <- true

		case webrtc.ICEConnectionStateFailed:
			pc.IceConnected <- false
		}
	})

	return pc, nil
}

func (pc *PeerConnection) CreateDataChannel() error {
	dataChannel, err := pc.Conn.CreateDataChannel("file-transfer", nil)

	if err != nil {
		panic(err)
	}

	pc.DataChannel = dataChannel

	dataChannel.OnOpen(func() {
		log.Println("Data channel opened")
	})

	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		// Handle incoming messages
		log.Printf("Received message: %s", string(msg.Data))
	})

	return nil
}

func (pc *PeerConnection) CreateOffer() (string, error) {
	// Create offer
	offer, err := pc.Conn.CreateOffer(nil)
	if err != nil {
		return "", err
	}

	// Set local description
	err = pc.Conn.SetLocalDescription(offer)
	if err != nil {
		return "", err
	}

	// Convert offer to JSON for transmission
	offerJSON, err := json.Marshal(offer)
	if err != nil {
		return "", err
	}

	return string(offerJSON), nil
}

func (pc *PeerConnection) HandleOffer(offerStr string) (string, error) {
	var offer webrtc.SessionDescription
	err := json.Unmarshal([]byte(offerStr), &offer)
	if err != nil {
		return "", err
	}

	// Set remote description
	err = pc.Conn.SetRemoteDescription(offer)
	if err != nil {
		return "", err
	}

	// Create answer
	answer, err := pc.Conn.CreateAnswer(nil)
	if err != nil {
		return "", err
	}

	// Set local description
	err = pc.Conn.SetLocalDescription(answer)
	if err != nil {
		return "", err
	}

	// Convert answer to JSON
	answerJSON, err := json.Marshal(answer)
	if err != nil {
		return "", err
	}

	return string(answerJSON), nil
}

func (pc *PeerConnection) HandleAnswer(answerStr string) error {
	var answer webrtc.SessionDescription
	err := json.Unmarshal([]byte(answerStr), &answer)

	if err != nil {
		return err
	}

	return pc.Conn.SetRemoteDescription(answer)
}

//unmarshals a JSON string representing an ICE candidate and 
//adds it to the peer connection
func (pc *PeerConnection) AddIceCandidate(candidateStr string) error {
	var candidate webrtc.ICECandidateInit
	err := json.Unmarshal([]byte(candidateStr), &candidate)
	if err != nil {
		return err
	}

	return pc.Conn.AddICECandidate(candidate)
}

//waits for the ICE connection state to be signaled
func (pc *PeerConnection) WaitForConnection(timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case connected := <-pc.IceConnected:
		return connected
	case <-ctx.Done():
		return false
	}
}

func (pc *PeerConnection) Close() error {
	return pc.Conn.Close()
}