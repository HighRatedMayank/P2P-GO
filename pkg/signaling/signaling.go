package signaling

import (
    "encoding/json"
    "log"
    "sync"
    "github.com/gorilla/websocket"
)


type SignalingServer struct {
    Clients    map[string]*Client
    Register   chan *Client
    Unregister chan *Client
    mu         sync.RWMutex
}

type Client struct {
    Server   *SignalingServer //ref to signal server
    Conn     *websocket.Conn
    Send     chan []byte
    ID       string
}

func NewSignalingServer() *SignalingServer {
    return &SignalingServer{
        Clients:    make(map[string]*Client),
        Register:   make(chan *Client),
        Unregister: make(chan *Client),
    }
}

//registering and unregistering clients
func (s *SignalingServer) Run() {
    for {
        select {
        case client := <-s.Register:
            s.mu.Lock()
            s.Clients[client.ID] = client
            s.broadcastPeerList()
            s.mu.Unlock()

        case client := <-s.Unregister:
            s.mu.Lock()
            if _, ok := s.Clients[client.ID]; ok {
                delete(s.Clients, client.ID)
                close(client.Send)
                s.broadcastPeerList()
            }
            s.mu.Unlock()
        }
    }
}

//broadcasting the list of available clients to let the client 
//know which other peers are avilable for connection
func (s *SignalingServer) broadcastPeerList() {
    peers := make([]string, 0, len(s.Clients))
    for id := range s.Clients {
        peers = append(peers, id)
    }

    message := SignalMessage{
        Type:    "peers",
        Payload: peers,
    }

    messageBytes, _ := json.Marshal(message)
    for _, client := range s.Clients {
        select {
        case client.Send <- messageBytes:
        default:
            close(client.Send)
            delete(s.Clients, client.ID)
        }
    }
}

func NewClient(server *SignalingServer, conn *websocket.Conn, id string) *Client {
    return &Client{
        Server: server,
        Conn:   conn,
        Send:   make(chan []byte, 256),
        ID:     id,
    }
}

//continuosly listning for messages from websocket
func (c *Client) ReadPump() {
    defer func() {
        c.Server.Unregister <- c
        c.Conn.Close()
    }()

    //reding the messages from the websocket
    for {
        _, message, err := c.Conn.ReadMessage()
        if err != nil {
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                log.Printf("error: %v", err)
            }
            break
        }

        //unmarshaling the incoming json into signalmessage struct
        var signal SignalMessage
        if err := json.Unmarshal(message, &signal); err != nil {
            log.Printf("error unmarshaling message: %v", err)
            continue
        }

        //populating the FormId field with sender's clientId
        signal.FormId = c.ID

        //sends the message to specific client using the send channel
        //if the message has a toId
        if signal.ToId != "" {
            c.Server.mu.RLock()
            if recipient, ok := c.Server.Clients[signal.ToId]; ok {
                messageBytes, _ := json.Marshal(signal)
                recipient.Send <- messageBytes
            }
            c.Server.mu.RUnlock()
        }
    }
}

//listens on the Send channel for messages to 
//send to the client over the WebSocket
func (c *Client) WritePump() {
    defer func() {
        c.Conn.Close()
    }()

    for {
        select {
        case message, ok := <- c.Send:
            if !ok {
                c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }

            if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
                return
            }
        }
    }
}