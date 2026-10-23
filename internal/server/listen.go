package server

import (
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
)

// remoteWarning is logged when the server is reachable from other machines
// (docs/CONFIG.md --listen).
const remoteWarning = "listening on a non-loopback address: anyone who obtains the link can view your traffic metadata"

// Listen opens the HTTP listener. Port 0 picks a free port. A host that is
// not a loopback address (including wildcards such as 0.0.0.0) logs a warning.
func Listen(addr string, logger *slog.Logger) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("listen %q: %w", addr, err)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	if !isLoopbackHost(host) {
		logger.Warn(remoteWarning, "addr", ln.Addr().String())
	}
	return ln, nil
}

// BaseURL is the browser-openable URL of a listener. Wildcard binds are
// reachable on loopback, so they print 127.0.0.1 rather than 0.0.0.0 or [::].
func BaseURL(addr net.Addr) string {
	ap, err := netip.ParseAddrPort(addr.String())
	if err != nil {
		return "http://" + addr.String() + "/"
	}
	if ap.Addr().IsUnspecified() {
		ap = netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), ap.Port())
	}
	return "http://" + ap.String() + "/"
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	a, err := netip.ParseAddr(host)
	return err == nil && a.IsLoopback()
}

// WriteURLFile writes url to path with mode 0600, replacing any existing file
// (docs/CONFIG.md --url-file). The URL carries the access token, so the file
// must not be readable by other users.
func WriteURLFile(path, url string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // operator-supplied path
	if err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil { // tighten a pre-existing file
		_ = f.Close()
		return err
	}
	if _, err := fmt.Fprintln(f, url); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
