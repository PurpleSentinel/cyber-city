package detect

import (
	"fmt"
	"net/netip"
	"testing"
	"time"
)

func testFlood() *Flood {
	return NewFlood(FloodConfig{
		Window: 5 * time.Second, BaselineHalfLife: 60 * time.Second, MinHistory: 30 * time.Second,
		Multiplier: 10, MinPPS: 2000, MinSources: 50,
	})
}

// drive sends pps packets per second from n sources to victim between from and
// to (seconds after t0), closing a window every 100 ms, and returns candidates.
func drive(f *Flood, from, to, pps, n int) []Candidate {
	var out []Candidate
	for ms := from * 1000; ms < to*1000; ms += 100 {
		at := t0.Add(time.Duration(ms) * time.Millisecond)
		for i := range pps / 10 {
			src := netip.MustParseAddr(fmt.Sprintf("198.51.100.%d", 1+i%n))
			f.Packet(&Packet{TS: at, Src: src, Dst: victim, TCP: true, SYN: true, DstPort: 80})
		}
		out = append(out, f.Window(at.Add(100*time.Millisecond))...)
	}
	return out
}

func TestFloodFiresAboveBaseline(t *testing.T) {
	f := testFlood()
	if c := drive(f, 0, 40, 50, 5); len(c) != 0 {
		t.Fatalf("baseline fired: %+v", c[0])
	}
	c := drive(f, 40, 45, 3000, 60)
	if len(c) == 0 {
		t.Fatal("flood did not fire")
	}
	e := c[0]
	if *e.Dst != victim || e.Src != nil || e.Evidence["distinct_sources"] < 50 || e.Evidence["baseline_pps"] > 60 {
		t.Fatalf("flood candidate %+v", e)
	}
	if f.byDst[victim].baseline > 60 {
		t.Fatalf("baseline not frozen while active: %v", f.byDst[victim].baseline)
	}
}

func TestFloodNeedsHistory(t *testing.T) {
	f := testFlood()
	if c := drive(f, 0, 10, 3000, 60); len(c) != 0 {
		t.Fatalf("fired without history: %+v", c[0])
	}
}

func TestFloodNeedsSources(t *testing.T) {
	f := testFlood()
	drive(f, 0, 40, 50, 5)
	if c := drive(f, 40, 45, 3000, 10); len(c) != 0 {
		t.Fatalf("fired with 10 sources: %+v", c[0])
	}
}

func TestFloodBaselineDecaysWhenQuiet(t *testing.T) {
	f := testFlood()
	drive(f, 0, 10, 100, 5)
	before := f.byDst[victim].baseline
	f.Window(t0.Add(70 * time.Second))
	if after := f.byDst[victim].baseline; after > before*0.55 || after < before*0.4 {
		t.Fatalf("baseline %v → %v after one half-life of silence", before, after)
	}
}
