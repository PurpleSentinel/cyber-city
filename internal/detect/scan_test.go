// SPDX-License-Identifier: MIT

package detect

import (
	"net/netip"
	"testing"
	"time"
)

func testScan() *Scan {
	return NewScan(ScanConfig{Window: 10 * time.Second, MinPorts: 50, MinHosts: 20, MinIncompleteRatio: 0.8, UDPMinPorts: 100})
}

var (
	scanner = netip.MustParseAddr("10.0.1.66")
	victim  = netip.MustParseAddr("10.0.9.5")
	t0      = time.Unix(1000, 0)
)

func TestScanBelowThreshold(t *testing.T) {
	s := testScan()
	for port := uint16(1); port < 50; port++ {
		s.Packet(&Packet{TS: t0, Src: scanner, Dst: victim, TCP: true, DstPort: port, SrcPort: 40000, SYN: true})
	}
	if c := s.Window(t0.Add(time.Second)); len(c) != 0 {
		t.Fatalf("49 ports fired: %+v", c)
	}
}

func TestScanCompletedHandshakesAreNotAScan(t *testing.T) {
	s := testScan()
	for port := uint16(1); port <= 60; port++ {
		s.Packet(&Packet{TS: t0, Src: scanner, Dst: victim, TCP: true, DstPort: port, SrcPort: 40000, SYN: true})
		if port%2 == 0 {
			s.Packet(&Packet{TS: t0, Src: victim, Dst: scanner, TCP: true, SrcPort: port, DstPort: 40000, SYN: true, ACK: true})
		}
	}
	if c := s.Window(t0.Add(time.Second)); len(c) != 0 {
		t.Fatalf("50%% complete fired: %+v", c)
	}
}

func TestScanFiresAndExpires(t *testing.T) {
	s := testScan()
	for port := uint16(1); port <= 50; port++ {
		s.Packet(&Packet{TS: t0, Src: scanner, Dst: victim, TCP: true, DstPort: port, SrcPort: 40000, SYN: true})
	}
	c := s.Window(t0.Add(time.Second))
	if len(c) != 1 || *c[0].Dst != victim || c[0].Evidence["distinct_ports"] != 50 || c[0].Evidence["incomplete_ratio"] != 1 {
		t.Fatalf("scan candidate %+v", c)
	}
	if c := s.Window(t0.Add(11 * time.Second)); len(c) != 0 || len(s.bySrc) != 0 {
		t.Fatalf("probes outside the window still count: %+v", c)
	}
}

func TestUDPScanIgnoresServiceReplies(t *testing.T) {
	s := testScan()
	dns := netip.MustParseAddr("10.0.0.53")
	for port := uint16(1); port <= 120; port++ {
		s.Packet(&Packet{TS: t0, Src: dns, Dst: victim, UDP: true, SrcPort: 53, DstPort: 30000 + port})
		s.Packet(&Packet{TS: t0, Src: scanner, Dst: victim, UDP: true, SrcPort: 50000, DstPort: port})
	}
	c := s.Window(t0.Add(time.Second))
	if len(c) != 1 || *c[0].Src != scanner {
		t.Fatalf("udp candidates %+v", c)
	}
}
