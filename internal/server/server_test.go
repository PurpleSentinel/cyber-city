package server

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

type testServer struct {
	*httptest.Server
	hub  *Hub
	auth *Auth
}

func newTestServer(t *testing.T) *testServer { return newTestServerWith(t, DefaultHubOptions()) }

func newTestServerWith(t *testing.T, opts HubOptions) *testServer {
	t.Helper()
	auth, err := NewAuth()
	if err != nil {
		t.Fatal(err)
	}
	static := fstest.MapFS{"index.html": {Data: []byte("<title>CyberCity-3D</title>")}}
	srv := httptest.NewUnstartedServer(nil)
	// Allowed origins come from the real listener, as in cmd/cybercity.
	hub := NewHub(protocol.ExampleHello, AllowedOrigins(srv.Listener.Addr(), []string{"http://localhost:5173"}), opts)
	srv.Config.Handler = Handler(hub, static, auth)
	srv.Start()
	t.Cleanup(srv.Close)
	return &testServer{srv, hub, auth}
}

func TestWSOriginCheck(t *testing.T) {
	s := newTestServer(t)
	jar, _ := cookiejar.New(nil)
	get(t, noRedirect(jar), s.auth.AuthURL(s.URL+"/"))
	d := websocket.Dialer{Jar: jar}
	cases := []struct {
		origin string
		ok     bool
	}{
		{s.URL, true},                    // the listener's own origin
		{"http://localhost:5173", true},  // --allow-origin
		{"", true},                       // non-browser client; cookie still required
		{"http://evil.example", false},   // cross-site page
		{"http://localhost:5174", false}, // wrong port
		{"https://" + s.URL[7:], false},  // wrong scheme
	}
	for _, tc := range cases {
		h := http.Header{}
		if tc.origin != "" {
			h.Set("Origin", tc.origin)
		}
		conn, resp, err := d.Dial(wsURL(s.URL), h)
		if tc.ok {
			if err != nil {
				t.Errorf("origin %q rejected: %v", tc.origin, err)
				continue
			}
			_ = conn.Close()
			continue
		}
		if err == nil {
			_ = conn.Close()
			t.Errorf("origin %q accepted", tc.origin)
		} else if resp == nil || resp.StatusCode != http.StatusForbidden {
			t.Errorf("origin %q: want 403, got %v", tc.origin, resp)
		}
	}
}

func TestAllowedOriginsForWildcardIncludesLoopback(t *testing.T) {
	addr, _ := net.ResolveTCPAddr("tcp", "0.0.0.0:9000")
	origins := AllowedOrigins(addr, []string{"http://extra.example"})
	if origins[0] != "http://127.0.0.1:9000" || origins[len(origins)-1] != "http://extra.example" {
		t.Fatalf("origins %v", origins)
	}
}

// noRedirect returns a client that reports redirects instead of following them.
func noRedirect(jar http.CookieJar) *http.Client {
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func get(t *testing.T, c *http.Client, u string) *http.Response {
	t.Helper()
	resp, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp
}

func TestAuthRejectsBadTokens(t *testing.T) {
	s := newTestServer(t)
	for _, q := range []string{"", "?token=", "?token=wrong", "?token=" + s.auth.Token() + "x"} {
		resp := get(t, noRedirect(nil), s.URL+"/auth"+q)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("/auth%s: status %d, want 401", q, resp.StatusCode)
		}
		if len(resp.Cookies()) != 0 {
			t.Errorf("/auth%s set a cookie", q)
		}
	}
}

func TestAuthSetsStrictCookieAndRedirects(t *testing.T) {
	s := newTestServer(t)
	resp := get(t, noRedirect(nil), s.auth.AuthURL(s.URL+"/"))
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/" {
		t.Fatalf("status %d location %q, want 302 /", resp.StatusCode, resp.Header.Get("Location"))
	}
	raw := resp.Header.Get("Set-Cookie")
	for _, attr := range []string{SessionCookie + "=", "HttpOnly", "SameSite=Strict", "Path=/"} {
		if !strings.Contains(raw, attr) {
			t.Errorf("Set-Cookie %q missing %s", raw, attr)
		}
	}
}

func TestAuthNextIsLocalOnly(t *testing.T) {
	s := newTestServer(t)
	cases := map[string]string{
		"/?debug=1":             "/?debug=1",
		"//evil.example":        "/",
		"/\\evil.example":       "/",
		"https://evil.example/": "/",
		"evil":                  "/",
	}
	for next, want := range cases {
		resp := get(t, noRedirect(nil), s.auth.AuthURL(s.URL+"/")+"&next="+url.QueryEscape(next))
		if got := resp.Header.Get("Location"); got != want {
			t.Errorf("next=%q redirected to %q, want %q", next, got, want)
		}
	}
}

func TestSessionEndpointAndWSRequireCookie(t *testing.T) {
	s := newTestServer(t)
	if resp := get(t, noRedirect(nil), s.URL+"/api/session"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/api/session without cookie: %d", resp.StatusCode)
	}
	if _, resp, err := websocket.DefaultDialer.Dial(wsURL(s.URL), nil); err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/ws without cookie: err=%v resp=%v, want 401", err, resp)
	}

	jar, _ := cookiejar.New(nil)
	get(t, noRedirect(jar), s.auth.AuthURL(s.URL+"/"))
	if resp := get(t, noRedirect(jar), s.URL+"/api/session"); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("/api/session with cookie: %d", resp.StatusCode)
	}
}

func TestSessionsAreBounded(t *testing.T) {
	s := newTestServer(t)
	for range maxSessions + 10 {
		get(t, noRedirect(nil), s.auth.AuthURL(s.URL+"/"))
	}
	if n := len(s.auth.sessions); n != maxSessions {
		t.Fatalf("%d sessions stored, want %d", n, maxSessions)
	}
}

func wsURL(httpURL string) string { return "ws" + strings.TrimPrefix(httpURL, "http") + "/ws" }

// dialAuthed logs in and opens the WebSocket with the session cookie.
func dialAuthed(t *testing.T, s *testServer) *websocket.Conn {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	get(t, noRedirect(jar), s.auth.AuthURL(s.URL+"/"))
	d := websocket.Dialer{Jar: jar}
	conn, _, err := d.Dial(wsURL(s.URL), http.Header{"Origin": {s.URL}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestStaticHelloAndTicks(t *testing.T) {
	s := newTestServer(t)
	resp, err := http.Get(s.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "CyberCity-3D") {
		t.Fatalf("index not served: %q", body)
	}

	conn := dialAuthed(t, s)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var first struct{ Type string }
	if _, msg, err := conn.ReadMessage(); err != nil || json.Unmarshal(msg, &first) != nil || first.Type != "hello" {
		t.Fatalf("first message %q (%v), want hello", msg, err)
	}
	// The client is registered before hello is written, so ticks reach it.
	for want := uint64(1); want <= 2; want++ {
		if err := s.hub.Tick(protocol.ExampleTick()); err != nil {
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
