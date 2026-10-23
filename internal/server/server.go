// Package server serves the embedded frontend and streams protocol messages
// to browsers over WebSocket.
package server

import (
	"io/fs"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// Hub sends each new client a hello snapshot, then every tick.
//
// Still prototype-grade: a slow client stalls the broadcast (T1.5).
type Hub struct {
	upgrader websocket.Upgrader
	snapshot func() *protocol.Hello

	mu      sync.Mutex
	clients map[*websocket.Conn]*uint64 // per-connection tick sequence
}

// NewHub returns a hub that builds hello messages with snapshot and accepts
// WebSocket upgrades only from allowedOrigins (AllowedOrigins); others get 403.
func NewHub(snapshot func() *protocol.Hello, allowedOrigins []string) *Hub {
	return &Hub{
		upgrader: websocket.Upgrader{CheckOrigin: originChecker(allowedOrigins)},
		snapshot: snapshot,
		clients:  make(map[*websocket.Conn]*uint64),
	}
}

// Tick broadcasts t, numbering it per connection (docs/PROTOCOL.md "tick").
// It never fails: unwritable clients are dropped.
func (h *Hub) Tick(t *protocol.Tick) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c, seq := range h.clients {
		*seq++
		msg := *t
		msg.Seq = *seq
		b, err := protocol.EncodeTick(&msg)
		if err != nil {
			return err
		}
		if err := c.WriteMessage(websocket.TextMessage, b); err != nil {
			delete(h.clients, c)
			_ = c.Close()
		}
	}
	return nil
}

// ServeWS upgrades the request, sends hello and registers the client. The
// hub lock is held from snapshot to registration so no tick is missed.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.mu.Lock()
	b, err := protocol.EncodeHello(h.snapshot())
	if err == nil {
		err = conn.WriteMessage(websocket.TextMessage, b)
	}
	if err != nil {
		h.mu.Unlock()
		slog.Debug("hello failed", "err", err)
		_ = conn.Close()
		return
	}
	h.clients[conn] = new(uint64)
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

// Handler routes the endpoints of docs/PROTOCOL.md: /auth exchanges the token
// for a session, /api/session and /ws require it, and everything else is the
// static frontend (which holds no data).
func Handler(h *Hub, static fs.FS, auth *Auth) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth", auth.HandleAuth)
	mux.HandleFunc("GET /api/session", auth.HandleSession)
	mux.Handle("/ws", auth.Require(http.HandlerFunc(h.ServeWS)))
	mux.Handle("/", staticHandler(static))
	return securityHeaders(mux)
}
