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

// Benign fixtures must not look like scans.
func TestNoScanInBenignFixtures(t *testing.T) {
	for _, name := range []string{"web_https", "dns", "ssh", "retrans", "flood", "idle", "ipv6", "vlan", "gateway"} {
		if evs := eventsOf(ticks(t, runFixture(t, name, fixtureOptions(t, name))), protocol.EventPortScan); len(evs) != 0 {
			t.Errorf("%s: unexpected port_scan %+v", name, evs)
		}
	}
}
