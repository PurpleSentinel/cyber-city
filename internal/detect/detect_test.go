// SPDX-License-Identifier: MIT

package detect

import (
	"fmt"
	"net/netip"
	"testing"
	"time"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// fixed reports one candidate per window.
type fixed struct{ c Candidate }

func (f *fixed) Packet(*Packet)               {}
func (f *fixed) Window(time.Time) []Candidate { return []Candidate{f.c} }

func plainID(a netip.Addr) string { return a.String() }

func TestManagerCooldownAndIDs(t *testing.T) {
	src := netip.MustParseAddr("10.0.1.66")
	d := &fixed{Candidate{Kind: protocol.EventPortScan, Key: "10.0.1.66", Src: &src, Severity: protocol.SeverityAlert, Duration: 6 * time.Second}}
	m := NewManager(map[string]time.Duration{protocol.EventPortScan: time.Minute}, d)
	t0 := time.Unix(1000, 0)

	first := m.Window(t0, plainID)
	if len(first) != 1 || first[0].ID != "e-1" || *first[0].Src != "10.0.1.66" || first[0].DurationMS != 6000 {
		t.Fatalf("first window: %+v", first)
	}
	if got := m.Window(t0.Add(30*time.Second), plainID); len(got) != 0 {
		t.Fatalf("within cooldown: %+v", got)
	}
	again := m.Window(t0.Add(61*time.Second), plainID)
	if len(again) != 1 || again[0].ID != "e-2" {
		t.Fatalf("after cooldown: %+v", again)
	}
}

func TestManagerCapsAndSortsTargets(t *testing.T) {
	var targets []netip.Addr
	for i := 254; i >= 1; i-- {
		targets = append(targets, netip.MustParseAddr(fmt.Sprintf("10.0.1.%d", i)))
	}
	m := NewManager(nil, &fixed{Candidate{Kind: protocol.EventPortScan, Key: "k", Targets: targets}})
	ev := m.Window(time.Unix(0, 0), plainID)[0]
	if len(ev.Targets) != protocol.MaxTargets {
		t.Fatalf("targets = %d, want %d", len(ev.Targets), protocol.MaxTargets)
	}
	if ev.Targets[0] != "10.0.1.1" || ev.Targets[1] != "10.0.1.10" {
		t.Fatalf("targets not sorted by id: %v", ev.Targets[:3])
	}
}

func TestHostIdleEvent(t *testing.T) {
	m := NewManager(nil)
	ev, ok := m.Emit(time.Unix(130, 0), HostIdle(netip.MustParseAddr("10.0.1.30"), 120*time.Second), plainID)
	if !ok || ev.Kind != protocol.EventHostIdle || ev.Severity != protocol.SeverityInfo || ev.Evidence["idle_s"] != 120 || *ev.Src != "10.0.1.30" {
		t.Fatalf("host idle event %+v", ev)
	}
}
