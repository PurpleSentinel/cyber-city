package server

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// AllowedOrigins lists the browser origins that may open /ws
// (docs/PROTOCOL.md): the listener's own origin plus extra (--allow-origin).
// A wildcard bind also admits the machine's interface addresses on that port,
// so LAN viewers work without trusting the request's Host header.
func AllowedOrigins(listener net.Addr, extra []string) []string {
	origins := []string{strings.TrimSuffix(BaseURL(listener), "/")}
	ap, err := netip.ParseAddrPort(listener.String())
	if err == nil && ap.Addr().IsUnspecified() {
		if addrs, err := net.InterfaceAddrs(); err == nil {
			for _, a := range addrs {
				if p, err := netip.ParsePrefix(a.String()); err == nil {
					origins = append(origins, "http://"+netip.AddrPortFrom(p.Addr(), ap.Port()).String())
				}
			}
		}
	}
	return append(origins, extra...)
}

// originChecker returns a websocket CheckOrigin func. Requests without an
// Origin header come from non-browser clients, which cannot be driven by a
// malicious page; they still need the session cookie.
func originChecker(allowed []string) func(*http.Request) bool {
	set := make(map[string]bool, len(allowed))
	for _, o := range allowed {
		set[strings.ToLower(strings.TrimSuffix(o, "/"))] = true
	}
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "" || set[strings.ToLower(origin)]
	}
}
