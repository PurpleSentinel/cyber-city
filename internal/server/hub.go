// SPDX-License-Identifier: MIT

package server

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// readLimit bounds incoming WebSocket messages; clients send nothing
// (docs/PROTOCOL.md "Transport").
const readLimit = 512

// HubOptions tune the WebSocket fan-out (docs/CONFIG.md server.*).
type HubOptions struct {
	MaxClients   int
	ClientQueue  int
	WriteTimeout time.Duration
	PingInterval time.Duration
}

// DefaultHubOptions match the documented defaults.
func DefaultHubOptions() HubOptions {
	return HubOptions{MaxClients: 4, ClientQueue: 32, WriteTimeout: 2 * time.Second, PingInterval: 15 * time.Second}
}

// Hub sends each new client a hello snapshot, then every tick. Each client
// has its own bounded queue and writer goroutine, so a slow client is dropped
// instead of stalling the others; no socket write happens under the hub lock.
type Hub struct {
	opts     HubOptions
	upgrader websocket.Upgrader
	snapshot func() *protocol.Hello

	mu      sync.Mutex
	clients map[*client]struct{}
}

type client struct {
	conn  *websocket.Conn
	send  chan []byte
	seq   uint64
	close sync.Once
}

// NewHub returns a hub that builds hello messages with snapshot and accepts
// WebSocket upgrades only from allowedOrigins (AllowedOrigins); others get 403.
func NewHub(snapshot func() *protocol.Hello, allowedOrigins []string, opts HubOptions) *Hub {
	return &Hub{
		opts:     opts,
		upgrader: websocket.Upgrader{CheckOrigin: originChecker(allowedOrigins)},
		snapshot: snapshot,
		clients:  make(map[*client]struct{}),
	}
}

// Clients returns the number of connected clients.
func (h *Hub) Clients() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// Tick queues t for every client, numbering it per connection
// (docs/PROTOCOL.md "tick"). Clients whose queue is full are closed with 1013.
func (h *Hub) Tick(t *protocol.Tick) error {
	var overflow []*client
	h.mu.Lock()
	for c := range h.clients {
		c.seq++
		msg := *t
		msg.Seq = c.seq
		b, err := protocol.EncodeTick(&msg)
		if err != nil {
			h.mu.Unlock()
			return err
		}
		select {
		case c.send <- b:
		default:
			overflow = append(overflow, c)
			delete(h.clients, c)
		}
	}
	h.mu.Unlock()

	for _, c := range overflow {
		slog.Warn("dropping slow client", "remote", c.conn.RemoteAddr().String())
		h.closeClient(c, websocket.CloseTryAgainLater, "client too slow")
	}
	return nil
}

// ServeWS upgrades the request and registers the client with hello as its
// first queued message. The hub lock covers snapshot and registration so no
// tick is missed or duplicated by sequence.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	if h.Clients() >= h.opts.MaxClients {
		http.Error(w, "too many clients", http.StatusServiceUnavailable)
		return
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(readLimit)
	c := &client{conn: conn, send: make(chan []byte, h.opts.ClientQueue)}

	h.mu.Lock()
	if len(h.clients) >= h.opts.MaxClients { // raced with another upgrade
		h.mu.Unlock()
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "too many clients"), time.Now().Add(time.Second))
		_ = conn.Close()
		return
	}
	hello, err := protocol.EncodeHello(h.snapshot())
	if err != nil {
		h.mu.Unlock()
		_ = conn.Close()
		return
	}
	c.send <- hello
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	go h.writer(c)
	h.reader(c)
}

// writer is the only goroutine that writes data frames to c.
func (h *Hub) writer(c *client) {
	ping := time.NewTicker(h.opts.PingInterval)
	defer ping.Stop()
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(h.opts.WriteTimeout))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				h.drop(c)
				return
			}
		case <-ping.C:
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(h.opts.WriteTimeout)); err != nil {
				h.drop(c)
				return
			}
		}
	}
}

// reader handles control frames and enforces liveness: a client that misses
// two pongs is dropped. Data messages are ignored; oversized ones end the
// connection (read limit).
func (h *Hub) reader(c *client) {
	deadline := func() time.Time { return time.Now().Add(2*h.opts.PingInterval + h.opts.WriteTimeout) }
	_ = c.conn.SetReadDeadline(deadline())
	c.conn.SetPongHandler(func(string) error { return c.conn.SetReadDeadline(deadline()) })
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			h.drop(c)
			return
		}
	}
}

// drop unregisters and closes c without a close code (I/O failure).
func (h *Hub) drop(c *client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	h.closeClient(c, 0, "")
}

// closeClient sends an optional close frame and releases c exactly once.
func (h *Hub) closeClient(c *client, code int, reason string) {
	c.close.Do(func() {
		if code != 0 {
			_ = c.conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(code, reason), time.Now().Add(time.Second))
		}
		close(c.send)
		_ = c.conn.Close()
	})
}
