package enrich

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
	"testing"
)

func hw(s string) net.HardwareAddr {
	m, err := net.ParseMAC(s)
	if err != nil {
		panic(err)
	}
	return m
}

func TestMACsAttributeLocalHosts(t *testing.T) {
	m := NewMACs(5, 100)
	pi := netip.MustParseAddr("10.0.1.10")
	m.Observe(hw("b8:27:eb:00:01:0a"), pi, true, hw("02:00:00:00:00:01"), netip.MustParseAddr("203.0.113.10"), false)
	mac, vendor, ok := m.For(pi)
	if !ok || mac != "b8:27:eb:00:01:0a" || !strings.Contains(vendor, "Raspberry Pi") {
		t.Fatalf("got %q %q %v", mac, vendor, ok)
	}
	if _, _, ok := m.For(netip.MustParseAddr("203.0.113.10")); ok {
		t.Fatal("external host got a MAC")
	}
}

func TestMACsGatewayDetection(t *testing.T) {
	m := NewMACs(5, 100)
	gwMAC := hw("02:00:00:00:00:01")
	client := netip.MustParseAddr("10.0.1.10")
	router := netip.MustParseAddr("10.0.1.1")
	behind := netip.MustParseAddr("10.0.5.20") // another subnet reached via the router

	m.Observe(gwMAC, router, true, hw("b8:27:eb:00:01:0a"), client, true) // router answers DNS
	m.Observe(gwMAC, router, true, hw("b8:27:eb:00:01:0a"), client, true)
	m.Observe(gwMAC, behind, true, hw("b8:27:eb:00:01:0a"), client, true) // routed local host
	if m.IsGateway(router) {
		t.Fatal("gateway detected before the threshold")
	}
	var changed []netip.Addr
	for i := 1; i <= 5; i++ {
		changed = append(changed, m.Observe(gwMAC, netip.MustParseAddr(fmt.Sprintf("203.0.113.%d", i)), false, hw("b8:27:eb:00:01:0a"), client, true)...)
	}
	if len(changed) == 0 {
		t.Fatal("becoming a gateway should report affected hosts")
	}
	if !m.IsGateway(router) {
		t.Fatal("router not recognised as gateway")
	}
	if mac, _, ok := m.For(router); !ok || mac != gwMAC.String() {
		t.Fatalf("router MAC %q %v", mac, ok)
	}
	if _, _, ok := m.For(behind); ok {
		t.Fatal("host behind the router got the router's MAC")
	}
	if m.IsGateway(client) {
		t.Fatal("client marked as gateway")
	}
}
