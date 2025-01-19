// File: P2P-GO/cmd/main.go

package main

import (
    "log"
    "net/http"

    "github.com/gorilla/websocket"
    "github.com/google/uuid"

    "github.com/AyushSriv06/P2P-GO/pkg"
)

var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
    CheckOrigin: func(r *http.Request) bool {
        return true 
    },
}

func main() {
    server := signaling.NewSignalingServer()
    go server.Run()

    http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
        
        conn, err := upgrader.Upgrade(w, r, nil)
        if err != nil {
            log.Printf("Error upgrading connection: %v", err)
            return
        }

        clientID := uuid.New().String()
        log.Printf("New client connected: %s", clientID)

        
        client := signaling.NewClient(server, conn, clientID)

        
        server.register <- client

        
        go client.writePump()
        go client.readPump()
    })

    
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("Server is running"))
    })

    serverAddr := ":8080"
    log.Printf("Starting P2P signaling server on %s", serverAddr)
    log.Printf("WebSocket endpoint available at ws://localhost%s/ws", serverAddr)
    
    if err := http.ListenAndServe(serverAddr, nil); err != nil {
        log.Fatalf("Failed to start server: %v", err)
    }
}