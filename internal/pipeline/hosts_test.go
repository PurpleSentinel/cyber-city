package pipeline

import (
	"net/netip"
	"slices"
	"testing"
	"time"

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

func TestHostResentOnlyWhenChanged(t *testing.T) {
	p := New(testOptions())
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p.start(base)
	a := netip.MustParseAddr("10.0.0.1")
	touch := func(at time.Duration) {
		p.mu.Lock()
		p.touch(a, base.Add(at))
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
		p.touch(netip.MustParseAddr(ip), base.Add(time.Duration(i)*time.Second))
		p.mu.Unlock()
	}
	// Refresh .1 so .2 becomes the least recently seen, then add .4.
	p.mu.Lock()
	p.touch(netip.MustParseAddr("10.0.0.1"), base.Add(5*time.Second))
	p.touch(netip.MustParseAddr("10.0.0.4"), base.Add(6*time.Second))
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
