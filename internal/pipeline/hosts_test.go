package pipeline

import (
	"bytes"
	"context"
	"io"
	"net/netip"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"

	"github.com/PurpleSentinel/cyber-city/internal/capture"

	"github.com/PurpleSentinel/cyber-city/internal/enrich"
	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// removedAt returns, for each removed host id, the tick timestamp (ms
// relative to the first tick) at which it was removed.
func removedAt(ts []protocol.Tick) map[string]int64 {
	out := map[string]int64{}
	if len(ts) == 0 {
		return out
	}
	base := ts[0].TS
	for _, tk := range ts {
		for _, id := range tk.RemovedHosts {
			out[id] = tk.TS - base
		}
	}
	return out
}

func TestIdleHostsAreRemoved(t *testing.T) {
	ts := ticks(t, runFixture(t, "idle", testOptions()))
	rm := removedAt(ts)
	// 10.0.1.30 is last active at ~10 s; idle timeout 120 s.
	if at, ok := rm["10.0.1.30"]; !ok || at < 129_000 || at > 131_000 {
		t.Fatalf("10.0.1.30 removed at %d ms (ok=%v), want ≈130 s", at, ok)
	}
	// Its HTTPS peer was last seen at ~2 s.
	if at, ok := rm["203.0.113.20"]; !ok || at < 121_000 || at > 123_000 {
		t.Fatalf("203.0.113.20 removed at %d ms (ok=%v), want ≈122 s", at, ok)
	}
	// The heartbeat host keeps talking and is never removed.
	if _, ok := rm["10.0.1.10"]; ok {
		t.Fatal("active host 10.0.1.10 was removed")
	}
	// A removed host is not also sent as a host update in the same tick.
	for _, tk := range ts {
		for _, h := range tk.Hosts {
			if slices.Contains(tk.RemovedHosts, h.ID) {
				t.Fatalf("host %s both updated and removed in tick %d", h.ID, tk.Seq)
			}
		}
	}
}

// lastHosts returns the most recent Host object seen for each id.
func lastHosts(ts []protocol.Tick) map[string]protocol.Host {
	out := map[string]protocol.Host{}
	for _, tk := range ts {
		for _, h := range tk.Hosts {
			out[h.ID] = h
		}
	}
	return out
}

func hasService(h protocol.Host, proto string, port int, confirmed bool) bool {
	for _, s := range h.Services {
		if s.Proto == proto && s.Port == port && s.Confirmed == confirmed {
			return true
		}
	}
	return false
}

// docs/TESTING.md fixture rows for services.
func TestServiceAssertions(t *testing.T) {
	cases := []struct {
		fixture, host, proto string
		port                 int
		category             string
	}{
		{"web_https", "203.0.113.10", "tcp", 443, protocol.CatHTTPS},
		{"dns", "10.0.1.1", "udp", 53, protocol.CatDNS},
		{"ssh", "10.0.9.5", "tcp", 22, protocol.CatSSH},
	}
	for _, tc := range cases {
		h, ok := lastHosts(ticks(t, runFixture(t, tc.fixture, testOptions())))[tc.host]
		if !ok || !hasService(h, tc.proto, tc.port, true) {
			t.Errorf("%s: %s should serve %s/%d confirmed; got %+v", tc.fixture, tc.host, tc.proto, tc.port, h.Services)
			continue
		}
		for _, s := range h.Services {
			if s.Port == tc.port && s.Category != tc.category {
				t.Errorf("%s: category %s, want %s", tc.fixture, s.Category, tc.category)
			}
		}
		// Clients do not become servers of their ephemeral ports.
		for id, other := range lastHosts(ticks(t, runFixture(t, tc.fixture, testOptions()))) {
			if id != tc.host && len(other.Services) > 0 {
				t.Errorf("%s: client %s has services %+v", tc.fixture, id, other.Services)
			}
		}
	}
}

// docs/TESTING.md web_https row: rtt_ms 12.0 ± 0.5 (6 ms + 6 ms handshake).
func TestHandshakeRTT(t *testing.T) {
	ts := ticks(t, runFixture(t, "web_https", testOptions()))
	near12 := func(v *float64) bool { return v != nil && *v >= 11.5 && *v <= 12.5 }
	var flowRTT bool
	for _, tk := range ts {
		for _, f := range tk.Flows {
			if f.Category == protocol.CatHTTPS && near12(f.RTTMS) {
				flowRTT = true
			}
		}
	}
	if !flowRTT {
		t.Fatal("no https flow carries rtt_ms ≈ 12")
	}
	hosts := lastHosts(ts)
	for _, id := range []string{"10.0.1.10", "203.0.113.10"} {
		if !near12(hosts[id].RTTMS) {
			t.Fatalf("host %s rtt_ms %v, want ≈12", id, hosts[id].RTTMS)
		}
	}
	// SYN probes answered by RST/SYN-ACK but never ACKed yield no samples.
	for id, h := range lastHosts(ticks(t, runFixture(t, "scan_syn", testOptions()))) {
		if h.RTTMS != nil {
			t.Fatalf("scan fixture host %s has rtt %v", id, *h.RTTMS)
		}
	}
}

// docs/TESTING.md retrans row: retrans_rate 0.05 ± 0.01; web_https 0.
func TestRetransmissionRate(t *testing.T) {
	ts := ticks(t, runFixture(t, "retrans", testOptions()))
	rate := lastHosts(ts)["203.0.113.10"].RetransRate
	if rate < 0.04 || rate > 0.06 {
		t.Fatalf("retrans_rate %.3f, want 0.05 ± 0.01", rate)
	}
	var flowRetrans int
	for _, tk := range ts {
		for _, f := range tk.Flows {
			flowRetrans += f.Retrans
		}
	}
	if flowRetrans == 0 {
		t.Fatal("no flow reported retransmissions")
	}
	for id, h := range lastHosts(ticks(t, runFixture(t, "web_https", testOptions()))) {
		if h.RetransRate != 0 {
			t.Fatalf("web_https host %s retrans_rate %v, want 0", id, h.RetransRate)
		}
	}
}

// docs/TESTING.md web_https and gateway rows; T2.9 --show-mac toggle.
func TestMACVendorAndGateway(t *testing.T) {
	web := lastHosts(ticks(t, runFixture(t, "web_https", testOptions())))
	if v := web["10.0.1.10"].Vendor; v == nil || !strings.Contains(*v, "Raspberry Pi") {
		t.Fatalf("10.0.1.10 vendor %v, want Raspberry Pi", v)
	}
	if web["10.0.1.10"].MAC != nil {
		t.Fatal("mac sent without --show-mac")
	}
	if h := web["203.0.113.10"]; h.MAC != nil || h.Vendor != nil {
		t.Fatalf("external host has mac/vendor: %+v", h)
	}

	o := testOptions()
	o.Server.ShowMAC = true
	withMAC := lastHosts(ticks(t, runFixture(t, "web_https", o)))
	pi, _ := enrich.FindPrefix("Raspberry Pi")
	wantMAC := append(pi, 0, 1, 10).String() // fixturegen: vendor prefix + last 3 address bytes
	if m := withMAC["10.0.1.10"].MAC; m == nil || *m != wantMAC {
		t.Fatalf("--show-mac: mac %v, want %s", derefOr(m), wantMAC)
	}

	gw := lastHosts(ticks(t, runFixture(t, "gateway", o)))
	if m := gw["10.0.1.1"].MAC; m == nil || *m != "02:00:00:00:00:01" {
		t.Fatalf("gateway 10.0.1.1 mac %v", m)
	}
	for id, h := range gw {
		if strings.HasPrefix(id, "203.0.113.") && (h.MAC != nil || h.Vendor != nil) {
			t.Fatalf("external host %s has mac/vendor", id)
		}
	}
}

// T2.10: demo.pcap yields every device class; spot-check known hosts.
func TestDeviceClassesInDemo(t *testing.T) {
	hosts := lastHosts(ticks(t, runFixture(t, "demo", testOptions())))
	seen := map[string]int{}
	for _, h := range hosts {
		seen[h.DeviceClass]++
	}
	for _, c := range []string{protocol.ClassNetwork, protocol.ClassIoT, protocol.ClassServer, protocol.ClassWorkstation, protocol.ClassUnknown} {
		if seen[c] == 0 {
			t.Errorf("no %s host in demo (classes %v)", c, seen)
		}
	}
	for id, want := range map[string]string{
		"10.0.1.1": protocol.ClassNetwork, // gateway MAC
		"10.0.9.5": protocol.ClassServer,  // SSH server with several clients
	} {
		if got := hosts[id].DeviceClass; got != want {
			t.Errorf("%s class %s, want %s", id, got, want)
		}
	}
}

// limitedSource stops a looping file after n frames.
type limitedSource struct {
	*capture.File
	n int
}

func (l *limitedSource) Next() ([]byte, gopacket.CaptureInfo, error) {
	if l.n == 0 {
		return nil, gopacket.CaptureInfo{}, io.EOF
	}
	l.n--
	return l.File.Next()
}

// A looped replay repeats the same connections and sequence numbers; they must
// not be counted as retransmissions (found by the e2e shading test).
func TestLoopedReplayIsNotRetransmission(t *testing.T) {
	f, err := capture.OpenFile(filepath.Join("..", "..", "testdata", "pcap", "web_https.pcap"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	p := New(testOptions())
	var buf bytes.Buffer
	sink, err := NewJSONLSink(&buf, p.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), &limitedSource{File: f, n: 234 * 2}, p, sink, 0); err != nil {
		t.Fatal(err)
	}
	_ = sink.Close()
	ts := ticks(t, buf.Bytes())
	for id, h := range lastHosts(ts) {
		if h.RetransRate != 0 {
			t.Fatalf("host %s retrans_rate %v after a looped replay, want 0", id, h.RetransRate)
		}
	}
	var rtts int
	for _, tk := range ts {
		for _, fl := range tk.Flows {
			if fl.RTTMS != nil {
				rtts++
			}
		}
	}
	if rtts < 2 {
		t.Fatalf("handshake RTT measured in %d ticks, want both passes", rtts)
	}
}

func TestScanProbesDoNotCreateServices(t *testing.T) {
	h := lastHosts(ticks(t, runFixture(t, "scan_syn", testOptions())))["10.0.9.5"]
	// Only the two open ports answered with SYN-ACK.
	if len(h.Services) != 2 || !hasService(h, "tcp", 22, true) || !hasService(h, "tcp", 80, true) {
		t.Fatalf("scan target services %+v, want only confirmed 22 and 80", h.Services)
	}
}

func TestHostResentOnlyWhenChanged(t *testing.T) {
	p := New(testOptions())
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p.start(base)
	a := netip.MustParseAddr("10.0.0.1")
	touch := func(at time.Duration) {
		p.mu.Lock()
		p.touch(a, base.Add(at), 0)
		p.mu.Unlock()
	}
	sent := func(end time.Duration) bool {
		return len(p.tick(base.Add(end)).Hosts) == 1
	}

	touch(0)
	if !sent(100 * time.Millisecond) {
		t.Fatal("new host not sent")
	}
	touch(time.Second)
	if sent(1100 * time.Millisecond) {
		t.Fatal("host re-sent after only 1 s of last_seen progress")
	}
	touch(5 * time.Second)
	if !sent(5100 * time.Millisecond) {
		t.Fatal("host not re-sent after last_seen advanced 5 s")
	}
}

func TestMaxHostsEvictsLeastRecentlySeen(t *testing.T) {
	o := testOptions()
	o.MaxHosts = 3
	o.IdleTimeout = 0
	p := New(o)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p.start(base)
	for i, ip := range []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"} {
		p.mu.Lock()
		p.touch(netip.MustParseAddr(ip), base.Add(time.Duration(i)*time.Second), 0)
		p.mu.Unlock()
	}
	// Refresh .1 so .2 becomes the least recently seen, then add .4.
	p.mu.Lock()
	p.touch(netip.MustParseAddr("10.0.0.1"), base.Add(5*time.Second), 0)
	p.touch(netip.MustParseAddr("10.0.0.4"), base.Add(6*time.Second), 0)
	p.mu.Unlock()

	if len(p.hosts) != 3 {
		t.Fatalf("%d hosts, want 3", len(p.hosts))
	}
	if !slices.Equal(p.removed, []string{"10.0.0.2"}) {
		t.Fatalf("evicted %v, want [10.0.0.2]", p.removed)
	}
	tick := p.tick(base.Add(7 * time.Second))
	if !slices.Equal(tick.RemovedHosts, []string{"10.0.0.2"}) {
		t.Fatalf("tick removed_hosts %v", tick.RemovedHosts)
	}
	for _, h := range p.Snapshot().Hosts {
		if h.ID == "10.0.0.2" {
			t.Fatal("evicted host still in snapshot")
		}
	}
}

func derefOr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
