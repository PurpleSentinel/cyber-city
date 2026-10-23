// Package server serves the embedded frontend and streams protocol messages
// to browsers over WebSocket.
package server

import (
	"io/fs"
	"net/http"
)

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
