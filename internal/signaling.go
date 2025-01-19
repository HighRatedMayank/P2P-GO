package signaling

import (
    "encoding/json"
    "log"
    "sync"
    "github.com/gorilla/websocket"
)


type SignalingServer struct {
    clients    map[string]*Client
    register   chan *Client
    unregister chan *Client
    mu         sync.RWMutex
}

type Client struct {
    server *SignalingServer
    conn   *websocket.Conn
    send   chan []byte
    ID     string
}

func NewSignalingServer() *SignalingServer {
    return &SignalingServer{
        clients:    make(map[string]*Client),
        register:   make(chan *Client),
        unregister: make(chan *Client),
    }
}

func (s *SignalingServer) Run() {
    for {
        select {
        case client := <-s.register:
            s.mu.Lock()
            s.clients[client.ID] = client
            s.broadcastPeerList()
            s.mu.Unlock()
        case client := <-s.unregister:
            s.mu.Lock()
            if _, ok := s.clients[client.ID]; ok {
                delete(s.clients, client.ID)
                close(client.send)
                s.broadcastPeerList()
            }
            s.mu.Unlock()
        }
    }
}

func (s *SignalingServer) broadcastPeerList() {
    peers := make([]string, 0, len(s.clients))
    for id := range s.clients {
        peers = append(peers, id)
    }
    message := SignalMessage{
        Type:    "peers",
        Payload: peers,
    }
    messageBytes, err := json.Marshal(message)
    if err != nil {
        log.Printf("error marshaling peer list: %v", err)
        return
    }
    
    for _, client := range s.clients {
        select {
        case client.send <- messageBytes:
        default:
            close(client.send)
            delete(s.clients, client.ID)
        }
    }
}

func NewClient(server *SignalingServer, conn *websocket.Conn, id string) *Client {
    return &Client{
        server: server,
        conn:   conn,
        send:   make(chan []byte, 256),
        ID:     id,
    }
}

func (c *Client) readPump() {
    defer func() {
        c.server.unregister <- c
        c.conn.Close()
    }()

    for {
        _, message, err := c.conn.ReadMessage()
        if err != nil {
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                log.Printf("error: %v", err)
            }
            break
        }

        var signal SignalMessage
        if err := json.Unmarshal(message, &signal); err != nil {
            log.Printf("error unmarshaling message: %v", err)
            continue
        }

        signal.FormId= c.ID
        if signal.ToId != "" {
            c.server.mu.RLock()
            if recipient, ok := c.server.clients[signal.ToId]; ok {
                messageBytes, err := json.Marshal(signal)
                if err != nil {
                    log.Printf("error marshaling signal: %v", err)
                    c.server.mu.RUnlock()
                    continue
                }
                recipient.send <- messageBytes
            }
            c.server.mu.RUnlock()
        }
    }
}

func (c *Client) writePump() {
    defer func() {
        c.conn.Close()
    }()

    for {
        select {
        case message, ok := <-c.send:
            if !ok {
                c.conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }
            if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
                return
            }
        }
    }
}