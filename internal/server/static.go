package server

import (
	"io/fs"
	"net/http"
	"strings"
)

// contentSecurityPolicy allows only same-origin scripts, styles and
// connections, and forbids framing (docs/PROTOCOL.md "Security headers").
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// securityHeaders adds the headers every response carries.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		if p := r.URL.Path; p == "/" || p == "/index.html" || p == "/auth" || strings.HasPrefix(p, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// staticHandler serves the embedded frontend without directory listings.
func staticHandler(static fs.FS) http.Handler {
	files := http.FileServerFS(static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}
