package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// SessionCookie is the cookie set by /auth (docs/PROTOCOL.md, DECISIONS D5).
const SessionCookie = "cc_session"

// maxSessions bounds the session store; the oldest session is dropped first.
const maxSessions = 64

// Auth holds the per-run access token and the sessions created from it.
type Auth struct {
	token string

	mu       sync.Mutex
	sessions map[string]struct{}
	order    []string // creation order, for eviction
}

// NewAuth creates a random 32-byte token.
func NewAuth() (*Auth, error) {
	token, err := randomString()
	if err != nil {
		return nil, err
	}
	return &Auth{token: token, sessions: make(map[string]struct{})}, nil
}

// Token returns the access token embedded in the printed URL.
func (a *Auth) Token() string { return a.token }

// AuthURL is the ready-to-open URL for a server at base (BaseURL).
func (a *Auth) AuthURL(base string) string { return base + "auth?token=" + a.token }

func randomString() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HandleAuth exchanges a valid token for a session cookie and redirects to a
// safe local path.
func (a *Auth) HandleAuth(w http.ResponseWriter, r *http.Request) {
	given := r.URL.Query().Get("token")
	if subtle.ConstantTimeCompare([]byte(given), []byte(a.token)) != 1 {
		http.Error(w, "invalid or expired link: reopen the URL printed by cybercity", http.StatusUnauthorized)
		return
	}
	id, err := randomString()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	a.mu.Lock()
	a.sessions[id] = struct{}{}
	a.order = append(a.order, id)
	for len(a.order) > maxSessions {
		delete(a.sessions, a.order[0])
		a.order = a.order[1:]
	}
	a.mu.Unlock()

	// Secure only over TLS: the default server is plain HTTP on loopback, where
	// a Secure cookie would not be sent back (docs/DECISIONS.md D5).
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // HttpOnly+SameSite=Strict always; Secure when TLS
		Name:     SessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
	})
	http.Redirect(w, r, safeNext(r.URL.Query().Get("next")), http.StatusFound) //nolint:gosec // safeNext admits local paths only
}

// safeNext allows only local absolute paths, never another host
// ("//evil.example", "/\evil.example") or a scheme.
func safeNext(next string) string {
	if next == "" || len(next) > 512 || !strings.HasPrefix(next, "/") ||
		strings.Contains(next, "//") || strings.Contains(next, `\`) {
		return "/"
	}
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil {
		return "/"
	}
	return u.RequestURI()
}

// Valid reports whether r carries a live session cookie.
func (a *Auth) Valid(r *http.Request) bool {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.sessions[c.Value]
	return ok
}

// HandleSession answers 204 for a valid session and 401 otherwise.
func (a *Auth) HandleSession(w http.ResponseWriter, r *http.Request) {
	if !a.Valid(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Require rejects requests without a valid session.
func (a *Auth) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.Valid(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
