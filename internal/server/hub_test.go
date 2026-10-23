// SPDX-License-Identifier: MIT

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// bigTick returns a tick of roughly n bytes so socket buffers fill quickly.
func bigTick(n int) *protocol.Tick {
	t := protocol.ExampleTick()
	t.RemovedHosts = []string{strings.Repeat("x", n)}
	return t
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSlowClientIsDroppedOthersContinue(t *testing.T) {
	s := newTestServerWith(t, HubOptions{MaxClients: 4, ClientQueue: 4, WriteTimeout: 300 * time.Millisecond, PingInterval: time.Minute})
	slow := dialAuthedWith(t, s, 4096) // never reads; tiny receive buffer fills fast
	fast := dialAuthed(t, s)
	waitFor(t, "both clients registered", func() bool { return s.hub.Clients() == 2 })

	received := make(chan uint64, 1000)
	go func() {
		for {
			_, msg, err := fast.ReadMessage()
			if err != nil {
				close(received)
				return
			}
			var tk struct {
				Type string
				Seq  uint64
			}
			if json.Unmarshal(msg, &tk) == nil && tk.Type == "tick" {
				received <- tk.Seq
			}
		}
	}()

	// Keep sending until socket buffers fill and the slow client is dropped;
	// how much that takes depends on kernel buffer sizes.
	ticks := uint64(0)
	for s.hub.Clients() == 2 && ticks < 2000 {
		if err := s.hub.Tick(bigTick(64 << 10)); err != nil {
			t.Fatal(err)
		}
		ticks++
		time.Sleep(time.Millisecond)
	}
	waitFor(t, "slow client dropped", func() bool { return s.hub.Clients() == 1 })

	// The fast client saw every tick in order.
	for want := uint64(1); want <= ticks; want++ {
		select {
		case got := <-received:
			if got != want {
				t.Fatalf("fast client got seq %d, want %d", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("fast client stalled at seq %d", want)
		}
	}

	// Draining the slow client ends in a close (1013) or a reset: never a hang.
	_ = slow.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		if _, _, err := slow.ReadMessage(); err != nil {
			var ce *websocket.CloseError
			if errors.As(err, &ce) && ce.Code != websocket.CloseTryAgainLater {
				t.Fatalf("slow client closed with %d, want 1013", ce.Code)
			}
			break
		}
	}
}

// dialAuthedWith logs in and dials with the given TCP receive buffer size.
func dialAuthedWith(t *testing.T, s *testServer, rcvbuf int) *websocket.Conn {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	get(t, noRedirect(jar), s.auth.AuthURL(s.URL+"/"))
	d := websocket.Dialer{Jar: jar, NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		c, err := (&net.Dialer{}).DialContext(ctx, network, addr)
		if err == nil {
			err = c.(*net.TCPConn).SetReadBuffer(rcvbuf)
		}
		return c, err
	}}
	conn, _, err := d.Dial(wsURL(s.URL), http.Header{"Origin": {s.URL}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestOversizedClientMessageClosesConnection(t *testing.T) {
	s := newTestServer(t)
	conn := dialAuthed(t, s)
	waitFor(t, "client registered", func() bool { return s.hub.Clients() == 1 })
	if err := conn.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("a", 1024))); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "client dropped", func() bool { return s.hub.Clients() == 0 })
}

func TestMaxClients(t *testing.T) {
	s := newTestServerWith(t, HubOptions{MaxClients: 4, ClientQueue: 8, WriteTimeout: time.Second, PingInterval: time.Minute})
	for range 4 {
		dialAuthed(t, s)
	}
	waitFor(t, "4 clients", func() bool { return s.hub.Clients() == 4 })

	jar, _ := cookiejar.New(nil)
	get(t, noRedirect(jar), s.auth.AuthURL(s.URL+"/"))
	d := websocket.Dialer{Jar: jar}
	_, resp, err := d.Dial(wsURL(s.URL), http.Header{"Origin": {s.URL}})
	if err == nil || resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("5th client: err=%v resp=%v, want 503", err, resp)
	}
}

func TestClientMissingPongsIsDropped(t *testing.T) {
	// A client that never reads never answers pings (gorilla replies to pings
	// while reading), so it is dropped after two missed intervals.
	s := newTestServerWith(t, HubOptions{MaxClients: 4, ClientQueue: 8, WriteTimeout: 100 * time.Millisecond, PingInterval: 100 * time.Millisecond})
	dialAuthed(t, s)
	waitFor(t, "client registered", func() bool { return s.hub.Clients() == 1 })
	waitFor(t, "silent client dropped", func() bool { return s.hub.Clients() == 0 })

	// A reading client answers pings and stays connected.
	live := dialAuthed(t, s)
	go func() {
		for {
			if _, _, err := live.ReadMessage(); err != nil {
				return
			}
		}
	}()
	time.Sleep(600 * time.Millisecond)
	if n := s.hub.Clients(); n != 1 {
		t.Fatalf("responsive client dropped (clients=%d)", n)
	}
}
