// SPDX-License-Identifier: MIT

//go:build live

// Live-capture smoke test (docs/TESTING.md "Layers"). Needs libpcap and
// CAP_NET_RAW; CI runs it as root on the loopback interface:
//
//	go test -c -tags live -o capture.test ./internal/capture && sudo ./capture.test -test.run Live
package capture_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/PurpleSentinel/cyber-city/internal/capture"
	"github.com/PurpleSentinel/cyber-city/internal/pipeline"
)

func TestLiveCaptureOnLoopback(t *testing.T) {
	live, err := capture.OpenLive("lo", 256, false)
	if err != nil {
		t.Fatalf("open lo: %v (needs root or CAP_NET_RAW)", err)
	}
	defer func() { _ = live.Close() }()
	if !live.Live() {
		t.Fatal("live source should report Live()")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
	defer srv.Close()
	port := netip.MustParseAddrPort(srv.Listener.Addr().String()).Port()

	// Generate traffic while capturing.
	go func() {
		for range 5 {
			if resp, err := http.Get(srv.URL); err == nil {
				_ = resp.Body.Close()
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	dec := pipeline.NewDecoder()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, ci, err := live.Next()
		if errors.Is(err, io.EOF) {
			t.Fatal("live capture ended")
		}
		if err != nil {
			t.Fatal(err)
		}
		rec, ok := dec.Decode(data, ci)
		if ok && rec.Proto == pipeline.ProtoTCP && (rec.DstPort == port || rec.SrcPort == port) {
			if rec.WireLen <= 0 || !rec.Src.IsLoopback() {
				t.Fatalf("unexpected record %+v", rec)
			}
			kd, id := live.Stats()
			t.Logf("captured %s:%d → %s:%d (%d bytes); drops kernel=%d if=%d", rec.Src, rec.SrcPort, rec.Dst, rec.DstPort, rec.WireLen, kd, id)
			return
		}
	}
	t.Fatal("no TCP packets for the test server captured on lo")
}
