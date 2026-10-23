package enrich

import (
	"net/netip"
	"testing"
	"time"
)

func TestServicesObserveAndConfirm(t *testing.T) {
	s := NewServices(16)
	h := netip.MustParseAddr("10.0.9.5")
	t0 := time.Unix(0, 0)
	if !s.Observe(h, "tcp", 22, "ssh", false, t0) {
		t.Fatal("new service should report a change")
	}
	if s.Observe(h, "tcp", 22, "ssh", false, t0.Add(time.Second)) {
		t.Fatal("repeat observation should not report a change")
	}
	if !s.Observe(h, "tcp", 22, "ssh", true, t0.Add(2*time.Second)) {
		t.Fatal("confirmation should report a change")
	}
	s.Observe(h, "udp", 53, "dns", false, t0)
	s.Observe(h, "tcp", 53, "dns", false, t0)
	got := s.For(h)
	if len(got) != 3 || got[0].Port != 22 || !got[0].Confirmed || got[1].Proto != "tcp" || got[2].Proto != "udp" {
		t.Fatalf("services %+v", got)
	}
}

func TestServicesCapEvictsLeastRecentlySeen(t *testing.T) {
	s := NewServices(2)
	h := netip.MustParseAddr("10.0.9.5")
	t0 := time.Unix(0, 0)
	s.Observe(h, "tcp", 80, "web_cleartext", false, t0)
	s.Observe(h, "tcp", 443, "https", false, t0.Add(time.Second))
	s.Observe(h, "tcp", 80, "web_cleartext", false, t0.Add(2*time.Second)) // refresh 80
	s.Observe(h, "tcp", 22, "ssh", false, t0.Add(3*time.Second))           // evicts 443
	got := s.For(h)
	if len(got) != 2 || got[0].Port != 22 || got[1].Port != 80 {
		t.Fatalf("services %+v, want 22 and 80", got)
	}
	s.Forget(h)
	if len(s.For(h)) != 0 {
		t.Fatal("Forget kept services")
	}
}
