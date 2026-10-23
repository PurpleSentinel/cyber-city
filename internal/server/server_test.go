package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"
)

func TestHandlerServesStaticAndBroadcasts(t *testing.T) {
	static := fstest.MapFS{"index.html": {Data: []byte("<title>CyberCity-3D</title>")}}
	hub := NewHub()
	srv := httptest.NewServer(Handler(hub, static))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "CyberCity-3D") {
		t.Fatalf("index not served: %q", body)
	}

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	// The client registers asynchronously after the upgrade; retry until it receives.
	deadline := time.Now().Add(2 * time.Second)
	for {
		hub.Broadcast([]byte(`[]`))
		_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		if _, msg, err := conn.ReadMessage(); err == nil {
			if string(msg) != "[]" {
				t.Fatalf("got %q", msg)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no broadcast received")
		}
	}
}
