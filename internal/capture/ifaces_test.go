// SPDX-License-Identifier: MIT

package capture

import (
	"bytes"
	"strings"
	"testing"
)

const sampleRoute = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
wlp2s0	00000000	0101A8C0	0003	0	0	600	00000000	0	0	0
enp3s0	00000000	0100000A	0003	0	0	100	00000000	0	0	0
enp3s0	0000000A	00000000	0001	0	0	100	00FFFFFF	0	0	0
docker0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
blk0	00000000	00000000	0201	0	0	1	00000000	0	0	0
`

const sampleIPv6Route = `fd000001000000000000000000000000 40 00000000000000000000000000000000 00 00000000000000000000000000000000 00000100 00000001 00000000 00000001 enp3s0
00000000000000000000000000000000 00 00000000000000000000000000000000 00 fe800000000000000000000000000001 00000400 00000001 00000000 00000003 wlp2s0
00000000000000000000000000000000 00 00000000000000000000000000000000 00 00000000000000000000000000000000 ffffffff 00000001 00000000 00200200 lo
`

func TestDefaultRouteIPv4PicksLowestMetric(t *testing.T) {
	if got := defaultRouteIPv4(strings.NewReader(sampleRoute)); got != "enp3s0" {
		t.Fatalf("got %q, want enp3s0 (metric 100 beats 600; reject route ignored)", got)
	}
	if got := defaultRouteIPv4(strings.NewReader("Iface\tDestination\n")); got != "" {
		t.Fatalf("no default route: got %q", got)
	}
}

func TestDefaultRouteIPv6(t *testing.T) {
	if got := defaultRouteIPv6(strings.NewReader(sampleIPv6Route)); got != "wlp2s0" {
		t.Fatalf("got %q, want wlp2s0 (lo reject route ignored)", got)
	}
}

func TestMarkDefault(t *testing.T) {
	ifs := []Iface{
		{Name: "any", Up: true, Addrs: []string{"0.0.0.0"}},
		{Name: "lo", Up: true, Loop: true, Addrs: []string{"127.0.0.1"}},
		{Name: "enp3s0", Up: true, Addrs: []string{"10.0.0.5"}},
		{Name: "wlp2s0", Up: true, Addrs: []string{"192.168.1.5"}},
	}
	markDefault(ifs, "wlp2s0")
	if !ifs[3].Default || ifs[2].Default {
		t.Fatalf("route interface not chosen: %+v", ifs)
	}

	fallback := []Iface{
		{Name: "any", Up: true, Addrs: []string{"0.0.0.0"}},
		{Name: "lo", Up: true, Loop: true, Addrs: []string{"127.0.0.1"}},
		{Name: "eth1", Up: false, Addrs: []string{"10.1.0.1"}},
		{Name: "eth0", Up: true, Addrs: []string{"10.0.0.5"}},
	}
	markDefault(fallback, "")
	if !fallback[3].Default {
		t.Fatalf("fallback should pick the first up, non-loopback interface with an address: %+v", fallback)
	}
}

func TestPrintInterfacesMarksDefault(t *testing.T) {
	var b bytes.Buffer
	PrintInterfaces(&b, []Iface{{Name: "eth0", Up: true, Default: true, Addrs: []string{"10.0.0.5"}}, {Name: "lo", Up: true, Loop: true}})
	if !strings.Contains(b.String(), "* eth0") || strings.Contains(b.String(), "* lo") {
		t.Fatalf("output:\n%s", b.String())
	}
}
