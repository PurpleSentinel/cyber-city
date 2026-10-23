package enrich

import (
	"math"
	"net/netip"
	"testing"
	"time"
)

var dir = Direction{From: netip.MustParseAddrPort("203.0.113.10:443"), To: netip.MustParseAddrPort("10.0.1.10:40000")}

func TestRetransDetection(t *testing.T) {
	r := NewRetrans(30*time.Second, 100)
	t0 := time.Unix(1000, 0)
	if r.Segment(dir, 1000, 100, t0) {
		t.Fatal("first segment flagged")
	}
	if r.Segment(dir, 1100, 100, t0) {
		t.Fatal("in-order segment flagged")
	}
	if !r.Segment(dir, 1100, 100, t0) {
		t.Fatal("exact resend not flagged")
	}
	if !r.Segment(dir, 1000, 50, t0) {
		t.Fatal("old data not flagged")
	}
	if r.Segment(dir, 1150, 100, t0) {
		t.Fatal("partially new data flagged") // ends at 1250 > 1200
	}
	if r.Segment(dir, 5000, 0, t0) {
		t.Fatal("pure ACK flagged")
	}
	if got := r.Rate(dir.From.Addr(), t0); math.Abs(got-0.4) > 1e-9 {
		t.Fatalf("rate = %v, want 2/5", got)
	}
}

func TestRetransSequenceWrap(t *testing.T) {
	r := NewRetrans(30*time.Second, 100)
	t0 := time.Unix(0, 0)
	r.Segment(dir, math.MaxUint32-50, 100, t0) // wraps to 49
	if r.Segment(dir, 49, 100, t0) {
		t.Fatal("segment after wrap flagged")
	}
	if !r.Segment(dir, math.MaxUint32-50, 100, t0) {
		t.Fatal("resend across wrap not flagged")
	}
}

func TestRetransRateWindow(t *testing.T) {
	r := NewRetrans(30*time.Second, 100)
	t0 := time.Unix(0, 0)
	r.Segment(dir, 0, 10, t0)
	r.Segment(dir, 0, 10, t0) // retrans at t0
	r.Segment(dir, 10, 10, t0.Add(40*time.Second))
	if got := r.Rate(dir.From.Addr(), t0.Add(40*time.Second)); got != 0 {
		t.Fatalf("rate %v: old buckets should have expired", got)
	}
	r.Forget(dir.From.Addr())
	if r.Rate(dir.From.Addr(), t0) != 0 {
		t.Fatal("Forget kept state")
	}
}
