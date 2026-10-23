// SPDX-License-Identifier: MIT

package server

import (
	"bytes"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListenWarnsForNonLoopback(t *testing.T) {
	cases := []struct {
		addr string
		warn bool
	}{
		{"127.0.0.1:0", false},
		{"[::1]:0", false},
		{"localhost:0", false},
		{"0.0.0.0:0", true},
		{"[::]:0", true},
	}
	for _, tc := range cases {
		t.Run(tc.addr, func(t *testing.T) {
			var logs bytes.Buffer
			ln, err := Listen(tc.addr, slog.New(slog.NewTextHandler(&logs, nil)))
			if err != nil {
				t.Skipf("cannot listen on %s here: %v", tc.addr, err)
			}
			defer func() { _ = ln.Close() }()
			if got := strings.Contains(logs.String(), "level=WARN"); got != tc.warn {
				t.Fatalf("warned=%v, want %v; logs: %s", got, tc.warn, logs.String())
			}
		})
	}
}

func TestListenPortZeroPicksFreePort(t *testing.T) {
	ln, err := Listen("127.0.0.1:0", slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	if strings.HasSuffix(ln.Addr().String(), ":0") {
		t.Fatalf("got %s, want a real port", ln.Addr())
	}
}

func TestBaseURL(t *testing.T) {
	for addr, want := range map[string]string{
		"127.0.0.1:8080": "http://127.0.0.1:8080/",
		"[::1]:8080":     "http://[::1]:8080/",
		"0.0.0.0:9000":   "http://127.0.0.1:9000/",
		"[::]:9000":      "http://127.0.0.1:9000/",
		"10.0.1.5:8080":  "http://10.0.1.5:8080/",
	} {
		tcp, err := net.ResolveTCPAddr("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		if got := BaseURL(tcp); got != want {
			t.Errorf("BaseURL(%s) = %s, want %s", addr, got, want)
		}
	}
}

func TestWriteURLFileIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "url")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteURLFile(path, "http://127.0.0.1:1/auth?token=x"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("mode %o, want 600", perm)
	}
	if b, _ := os.ReadFile(path); string(b) != "http://127.0.0.1:1/auth?token=x\n" {
		t.Fatalf("content %q", b)
	}
}
