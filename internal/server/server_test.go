package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

func TestHandlerServesStaticHelloAndTicks(t *testing.T) {
	static := fstest.MapFS{"index.html": {Data: []byte("<title>CyberCity-3D</title>")}}
	hub := NewHub(protocol.ExampleHello)
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
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	var first struct{ Type string }
	if _, msg, err := conn.ReadMessage(); err != nil || json.Unmarshal(msg, &first) != nil || first.Type != "hello" {
		t.Fatalf("first message %q (%v), want hello", msg, err)
	}

	// The client is registered before hello is written, so ticks reach it.
	for want := uint64(1); want <= 2; want++ {
		if err := hub.Tick(protocol.ExampleTick()); err != nil {
			t.Fatal(err)
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var tick protocol.Tick
		if err := json.Unmarshal(msg, &tick); err != nil || tick.Seq != want {
			t.Fatalf("tick seq %d (%v), want %d", tick.Seq, err, want)
		}
	}
}
