package pipeline

import (
	"testing"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

func eventsOf(ts []protocol.Tick, kind string) []protocol.Event {
	var out []protocol.Event
	for _, t := range ts {
		for _, e := range t.Events {
			if e.Kind == kind {
				out = append(out, e)
			}
		}
	}
	return out
}

func TestScanSYNFixture(t *testing.T) {
	evs := eventsOf(ticks(t, runFixture(t, "scan_syn", testOptions())), protocol.EventPortScan)
	if len(evs) != 1 {
		t.Fatalf("port_scan events = %d, want 1: %+v", len(evs), evs)
	}
	e := evs[0]
	if *e.Src != "10.0.1.66" || e.Dst == nil || *e.Dst != "10.0.9.5" || e.Evidence["distinct_ports"] < 50 || e.Severity != protocol.SeverityAlert {
		t.Fatalf("port_scan event %+v", e)
	}
}

func TestScanSweepFixture(t *testing.T) {
	evs := eventsOf(ticks(t, runFixture(t, "scan_sweep", testOptions())), protocol.EventPortScan)
	if len(evs) != 1 {
		t.Fatalf("port_scan events = %d, want 1: %+v", len(evs), evs)
	}
	e := evs[0]
	if *e.Src != "10.0.1.66" || e.Dst != nil || e.Evidence["distinct_hosts"] < 20 || len(e.Targets) < 20 || len(e.Targets) > protocol.MaxTargets {
		t.Fatalf("sweep event %+v (targets %d)", e, len(e.Targets))
	}
}

func TestFloodFixture(t *testing.T) {
	ts := ticks(t, runFixture(t, "flood", testOptions()))
	evs := eventsOf(ts, protocol.EventFlood)
	if len(evs) != 1 {
		t.Fatalf("flood events = %d, want 1: %+v", len(evs), evs)
	}
	e := evs[0]
	start := ts[0].TS + 40_000
	if e.Dst == nil || *e.Dst != "10.0.9.5" || e.Src != nil || e.Evidence["distinct_sources"] < 50 || e.TS < start || e.TS > start+5_000 {
		t.Fatalf("flood event %+v (flood starts at %d)", e, start)
	}
}

// Fixtures without a flood must not report one.
func TestNoFloodElsewhere(t *testing.T) {
	for _, name := range []string{"web_https", "dns", "ssh", "retrans", "scan_syn", "scan_sweep", "idle", "ipv6", "vlan", "gateway", "demo"} {
		if evs := eventsOf(ticks(t, runFixture(t, name, fixtureOptions(t, name))), protocol.EventFlood); len(evs) != 0 {
			t.Errorf("%s: unexpected flood %+v", name, evs)
		}
	}
}

// Benign fixtures must not look like scans.
func TestNoScanInBenignFixtures(t *testing.T) {
	for _, name := range []string{"web_https", "dns", "ssh", "retrans", "flood", "idle", "ipv6", "vlan", "gateway"} {
		if evs := eventsOf(ticks(t, runFixture(t, name, fixtureOptions(t, name))), protocol.EventPortScan); len(evs) != 0 {
			t.Errorf("%s: unexpected port_scan %+v", name, evs)
		}
	}
}
