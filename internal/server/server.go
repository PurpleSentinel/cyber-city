// Package server serves the embedded frontend and streams windows to browsers
// over WebSocket.
package server

import (
	"io/fs"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Hub fans broadcast messages out to connected WebSocket clients.
//
// Prototype behaviour, kept unchanged by T0.2: any origin may connect (T1.3) and
// a slow client stalls the broadcast (T1.5).
type Hub struct {
	upgrader websocket.Upgrader
	mu       sync.Mutex
	clients  map[*websocket.Conn]bool
}

// NewHub returns a hub with no clients.
func NewHub() *Hub {
	return &Hub{
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		clients:  make(map[*websocket.Conn]bool),
	}
}

// Broadcast sends payload to every client, ignoring write errors.
func (h *Hub) Broadcast(payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		_ = c.WriteMessage(websocket.TextMessage, payload)
	}
}

// ServeWS upgrades the request and keeps the client registered until it disconnects.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		_ = conn.Close()
	}()
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// Handler routes /ws to the hub and everything else to the static frontend.
func Handler(h *Hub, static fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.ServeWS)
	mux.Handle("/", http.FileServerFS(static))
	return mux
}
