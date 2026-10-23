package pipeline

import (
	"net/netip"
	"testing"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

func rec(proto Proto, src string, sport uint16, dst string, dport uint16, flags uint8) *Record {
	return &Record{Proto: proto, Src: netip.MustParseAddr(src), SrcPort: sport, Dst: netip.MustParseAddr(dst), DstPort: dport, Flags: flags}
}

func dir(t *testing.T, tr *ServerTracker, r *Record) (string, string, int, Side) {
	t.Helper()
	d := tr.Direction(r)
	port := -1
	if d.ServicePort != nil {
		port = int(*d.ServicePort)
	}
	return d.Src.String(), d.Dst.String(), port, d.ServerSide
}

func TestServicePortRules(t *testing.T) {
	cases := []struct {
		name     string
		r        *Record
		src, dst string
		port     int
		side     Side
	}{
		{"rule 1 SYN", rec(ProtoTCP, "10.0.0.1", 50000, "10.0.0.2", 9999, FlagSYN), "10.0.0.1", "10.0.0.2", 9999, SideDst},
		{"rule 2 SYN-ACK", rec(ProtoTCP, "10.0.0.2", 9999, "10.0.0.1", 50000, FlagSYN|FlagACK), "10.0.0.1", "10.0.0.2", 9999, SideDst},
		{"rule 3 known port", rec(ProtoTCP, "10.0.0.1", 50000, "10.0.0.2", 443, FlagACK), "10.0.0.1", "10.0.0.2", 443, SideDst},
		{"rule 3 reply direction", rec(ProtoTCP, "10.0.0.2", 443, "10.0.0.1", 50000, FlagACK), "10.0.0.1", "10.0.0.2", 443, SideDst},
		{"rule 3 udp", rec(ProtoUDP, "10.0.0.1", 40000, "10.0.0.2", 53, 0), "10.0.0.1", "10.0.0.2", 53, SideDst},
		{"rule 4 low port", rec(ProtoTCP, "10.0.0.1", 50000, "10.0.0.2", 873, FlagACK), "10.0.0.1", "10.0.0.2", 873, SideDst},
		{"rule 4 both tabled? lower wins", rec(ProtoTCP, "10.0.0.1", 80, "10.0.0.2", 443, FlagACK), "10.0.0.2", "10.0.0.1", 80, SideDst},
		{"rule 5 both high", rec(ProtoUDP, "10.0.0.9", 50000, "10.0.0.2", 40000, 0), "10.0.0.2", "10.0.0.9", 40000, SideUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, dst, port, side := dir(t, NewServerTracker(100), tc.r)
			if src != tc.src || dst != tc.dst || port != tc.port || side != tc.side {
				t.Fatalf("got %s→%s:%d side=%d, want %s→%s:%d side=%d", src, dst, port, side, tc.src, tc.dst, tc.port, tc.side)
			}
		})
	}
}

func TestHandshakeOverridesPortGuess(t *testing.T) {
	tr := NewServerTracker(100)
	// Mid-stream packet first: rule 4 guesses the lower port (1000) is the server.
	if _, _, port, _ := dir(t, tr, rec(ProtoTCP, "10.0.0.1", 1000, "10.0.0.2", 2000, FlagACK)); port != 1000 {
		t.Fatalf("guess port = %d", port)
	}
	// A SYN to 2000 proves 2000 is the server.
	src, dst, port, _ := dir(t, tr, rec(ProtoTCP, "10.0.0.1", 1000, "10.0.0.2", 2000, FlagSYN))
	if src != "10.0.0.1" || dst != "10.0.0.2" || port != 2000 {
		t.Fatalf("after SYN: %s→%s:%d", src, dst, port)
	}
	// A later port-only packet keeps the handshake decision.
	if _, _, port, _ := dir(t, tr, rec(ProtoTCP, "10.0.0.2", 2000, "10.0.0.1", 1000, FlagACK)); port != 2000 {
		t.Fatalf("handshake decision lost: %d", port)
	}
}

func TestTrackerEviction(t *testing.T) {
	tr := NewServerTracker(2)
	tr.Direction(rec(ProtoTCP, "10.0.0.1", 50000, "10.0.0.2", 9001, FlagSYN))
	tr.Direction(rec(ProtoTCP, "10.0.0.1", 50001, "10.0.0.2", 9002, FlagSYN))
	tr.Direction(rec(ProtoTCP, "10.0.0.1", 50000, "10.0.0.2", 9001, FlagACK)) // refresh first
	tr.Direction(rec(ProtoTCP, "10.0.0.1", 50002, "10.0.0.2", 9003, FlagSYN)) // evicts second
	if tr.Len() != 2 {
		t.Fatalf("len = %d", tr.Len())
	}
	// The refreshed connection kept its handshake decision. (Checked first:
	// probing the evicted one re-inserts it and evicts the oldest entry.)
	if _, _, port, side := dir(t, tr, rec(ProtoTCP, "10.0.0.2", 9001, "10.0.0.1", 50000, FlagACK)); side != SideDst || port != 9001 {
		t.Fatalf("refreshed conn lost decision: port %d side %d", port, side)
	}
	// The evicted connection falls back to port rules: rule 5 (both high).
	if _, _, port, side := dir(t, tr, rec(ProtoTCP, "10.0.0.2", 9002, "10.0.0.1", 50001, FlagACK)); side != SideUnknown || port != 9002 {
		t.Fatalf("evicted conn kept its decision: port %d side %d", port, side)
	}
}

func TestICMPDirectionIsStable(t *testing.T) {
	tr := NewServerTracker(10)
	a, b, port, side := dir(t, tr, rec(ProtoICMPv6, "fe80::2", 0, "fe80::1", 0, 0))
	c, d, _, _ := dir(t, tr, rec(ProtoICMPv6, "fe80::1", 0, "fe80::2", 0, 0))
	if a != c || b != d || port != -1 || side != SideUnknown {
		t.Fatalf("unstable ICMP orientation: %s→%s vs %s→%s (port %d side %d)", a, b, c, d, port, side)
	}
	if tr.Len() != 0 {
		t.Fatal("ICMP should not be tracked")
	}
}

func TestCategoryTable(t *testing.T) {
	p := func(v uint16) *uint16 { return &v }
	cases := []struct {
		proto Proto
		port  *uint16
		want  string
	}{
		{ProtoTCP, p(443), protocol.CatHTTPS}, {ProtoTCP, p(8443), protocol.CatHTTPS}, {ProtoUDP, p(443), protocol.CatHTTPS},
		{ProtoTCP, p(80), protocol.CatWebCleartext}, {ProtoTCP, p(8000), protocol.CatWebCleartext}, {ProtoTCP, p(8080), protocol.CatWebCleartext},
		{ProtoUDP, p(53), protocol.CatDNS}, {ProtoTCP, p(53), protocol.CatDNS}, {ProtoTCP, p(853), protocol.CatDNS},
		{ProtoTCP, p(22), protocol.CatSSH},
		{ProtoTCP, p(21), protocol.CatLegacyCleartext}, {ProtoTCP, p(23), protocol.CatLegacyCleartext}, {ProtoTCP, p(25), protocol.CatLegacyCleartext},
		{ProtoTCP, p(110), protocol.CatLegacyCleartext}, {ProtoTCP, p(143), protocol.CatLegacyCleartext}, {ProtoTCP, p(512), protocol.CatLegacyCleartext},
		{ProtoTCP, p(513), protocol.CatLegacyCleartext}, {ProtoTCP, p(514), protocol.CatLegacyCleartext},
		{ProtoUDP, p(69), protocol.CatLegacyCleartext}, {ProtoUDP, p(161), protocol.CatLegacyCleartext},
		{ProtoUDP, p(67), protocol.CatDiscovery}, {ProtoUDP, p(68), protocol.CatDiscovery}, {ProtoUDP, p(137), protocol.CatDiscovery},
		{ProtoUDP, p(138), protocol.CatDiscovery}, {ProtoUDP, p(546), protocol.CatDiscovery}, {ProtoUDP, p(547), protocol.CatDiscovery},
		{ProtoUDP, p(1900), protocol.CatDiscovery}, {ProtoUDP, p(5353), protocol.CatDiscovery}, {ProtoUDP, p(5355), protocol.CatDiscovery},
		{ProtoICMP, nil, protocol.CatControl}, {ProtoICMPv6, nil, protocol.CatControl},
		{ProtoTCP, p(9999), protocol.CatOther}, {ProtoUDP, p(22), protocol.CatOther}, {ProtoOther, nil, protocol.CatOther},
	}
	for _, tc := range cases {
		if got := Category(tc.proto, tc.port); got != tc.want {
			port := -1
			if tc.port != nil {
				port = int(*tc.port)
			}
			t.Errorf("%s/%d: got %s, want %s", tc.proto, port, got, tc.want)
		}
	}
}

func mustAddr(s string) netip.Addr { return netip.MustParseAddr(s) }
