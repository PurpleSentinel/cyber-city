package enrich

import (
	"fmt"
	"net/netip"
	"testing"
	"time"
)

func conn(clientPort uint16) Conn {
	return Conn{
		Client: netip.MustParseAddrPort(fmt.Sprintf("10.0.1.10:%d", clientPort)),
		Server: netip.MustParseAddrPort("203.0.113.10:443"),
	}
}

func TestRTTCleanHandshake(t *testing.T) {
	r := NewRTT(10*time.Second, 100, 20)
	t0 := time.Unix(0, 0)
	r.SYN(conn(1), t0)
	r.SYNACK(conn(1))
	ms, ok := r.ACK(conn(1), t0.Add(12*time.Millisecond))
	if !ok || ms != 12 {
		t.Fatalf("rtt = %v %v, want 12 ms", ms, ok)
	}
	if r.Pending() != 0 {
		t.Fatal("handshake still pending")
	}
	if med, ok := r.HostMedian(conn(1).Server.Addr()); !ok || med != 12 {
		t.Fatalf("server median %v %v", med, ok)
	}
	// A later ACK on the same connection is not a new sample.
	if _, ok := r.ACK(conn(1), t0.Add(50*time.Millisecond)); ok {
		t.Fatal("second ACK produced a sample")
	}
}

func TestRTTRetransmittedSYNGivesNoSample(t *testing.T) {
	r := NewRTT(10*time.Second, 100, 20)
	t0 := time.Unix(0, 0)
	r.SYN(conn(2), t0)
	r.SYN(conn(2), t0.Add(time.Second)) // retransmission
	r.SYNACK(conn(2))
	if _, ok := r.ACK(conn(2), t0.Add(time.Second+12*time.Millisecond)); ok {
		t.Fatal("retransmitted SYN produced a sample")
	}
}

func TestRTTNeedsSYNACKAndExpires(t *testing.T) {
	r := NewRTT(10*time.Second, 100, 20)
	t0 := time.Unix(0, 0)
	r.SYN(conn(3), t0)
	if _, ok := r.ACK(conn(3), t0.Add(time.Millisecond)); ok {
		t.Fatal("ACK without SYN-ACK produced a sample")
	}
	r.SYN(conn(4), t0)
	r.SYN(conn(5), t0.Add(11*time.Second)) // expires conn 4
	r.SYNACK(conn(4))
	if _, ok := r.ACK(conn(4), t0.Add(11*time.Second)); ok {
		t.Fatal("expired handshake produced a sample")
	}
}

func TestRTTPendingBoundAndMedianWindow(t *testing.T) {
	r := NewRTT(time.Minute, 2, 3)
	t0 := time.Unix(0, 0)
	r.SYN(conn(10), t0)
	r.SYN(conn(11), t0)
	r.SYN(conn(12), t0) // evicts conn 10
	if r.Pending() != 2 {
		t.Fatalf("pending = %d", r.Pending())
	}
	server := conn(0).Server.Addr()
	for i, d := range []time.Duration{100, 10, 20, 30} { // only the last 3 count
		c := conn(uint16(20 + i))
		r.SYN(c, t0)
		r.SYNACK(c)
		r.ACK(c, t0.Add(d*time.Millisecond))
	}
	if med, _ := r.HostMedian(server); med != 20 {
		t.Fatalf("median over last 3 = %v, want 20", med)
	}
	r.Forget(server)
	if _, ok := r.HostMedian(server); ok {
		t.Fatal("Forget kept samples")
	}
}
