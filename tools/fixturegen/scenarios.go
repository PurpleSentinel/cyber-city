// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/PurpleSentinel/cyber-city/internal/enrich"
)

// vendors are MAC prefixes looked up by IEEE vendor name (docs/TESTING.md).
type vendors struct {
	rpi, espressif, cisco, apple, sonos net.HardwareAddr
}

func loadVendors() (vendors, error) {
	var v vendors
	for _, want := range []struct {
		name string
		dst  *net.HardwareAddr
	}{
		{"Raspberry Pi", &v.rpi},
		{"Espressif", &v.espressif},
		{"Cisco Systems", &v.cisco},
		{"Apple, Inc.", &v.apple},
		{"Sonos", &v.sonos},
	} {
		p, ok := enrich.FindPrefix(want.name)
		if !ok {
			return v, fmt.Errorf("vendor %q missing from the OUI table", want.name)
		}
		*want.dst = p
	}
	return v, nil
}

// noVendor is a locally administered prefix: hosts using it have no vendor.
var noVendor = net.HardwareAddr{0x02, 0x00, 0x00}

func gateway() host {
	return host{ip: netip.MustParseAddr("10.0.1.1"), mac: gatewayMAC, local: true}
}

// fixture is one generated capture.
type fixture struct {
	name  string
	build func(v vendors) *gen
}

var fixtures = []fixture{
	{"web_https", webHTTPS},
	{"dns", dnsFixture},
	{"ssh", sshFixture},
	{"retrans", retransFixture},
	{"scan_syn", scanSYN},
	{"scan_sweep", scanSweep},
	{"flood", flood},
	{"idle", idle},
	{"ipv6", ipv6},
	{"vlan", vlan},
	{"gateway", gatewayFixture},
	{"demo", demo},
	{"many_hosts", manyHosts},
}

// many_hosts: 2000 hosts for the frame-time check (docs/TASKS.md T5.4). 1000
// local clients in 10.0.16.0/22 each exchange a UDP datagram with their own
// server every 4 s for 16 s: 762 servers in the documentation ranges and 238
// local ones in 10.0.30.0/24.
func manyHosts(v vendors) *gen {
	g := newGen(13)
	var servers []host
	for _, prefix := range []string{"192.0.2.", "198.51.100.", "203.0.113."} {
		for i := 1; i <= 254; i++ {
			servers = append(servers, externalHost(fmt.Sprintf("%s%d", prefix, i)))
		}
	}
	for i := 1; len(servers) < 1000; i++ {
		servers = append(servers, localHost(fmt.Sprintf("10.0.30.%d", i), noVendor))
	}
	for i, s := range servers {
		c := localHost(fmt.Sprintf("10.0.%d.%d", 16+i/250, 1+i%250), v.apple)
		start := time.Duration(i) * 4 * time.Millisecond
		for k := range 4 {
			at := start + time.Duration(k)*4*time.Second
			cport := uint16(40000 + i) //nolint:gosec // < 65536
			g.udp(at, c, s, cport, 443, 120)
			g.udp(at+ms(15), s, c, 443, cport, 900)
		}
	}
	return g
}

// web_https: 10.0.1.10 makes 3 TLS-port connections to 203.0.113.10 with a
// 12 ms handshake RTT, about 200 kB down in total.
func webHTTPS(v vendors) *gen {
	g := newGen(1)
	c, s := localHost("10.0.1.10", v.rpi), externalHost("203.0.113.10")
	for i := range 3 {
		g.webSession(time.Duration(i)*2*time.Second, c, s, 443, ms(6), ms(6), 517, 66_700, 0)
	}
	return g
}

// dns: 20 queries from 10.0.1.10 to the gateway resolver, all answered.
func dnsFixture(v vendors) *gen {
	g := newGen(2)
	c := localHost("10.0.1.10", v.rpi)
	for i := range 20 {
		g.dns(time.Duration(i)*500*time.Millisecond, c, gateway(), ms(3))
	}
	return g
}

// ssh: 60 s interactive session from 10.0.1.20 to 10.0.9.5:22.
func sshFixture(v vendors) *gen {
	g := newGen(3)
	c, s := localHost("10.0.1.20", v.apple), localHost("10.0.9.5", noVendor)
	k := g.newConn(c, s, 22)
	t := g.handshake(0, k, ms(0.4), ms(0.4))
	for t < 60*time.Second {
		g.send(t+ms(1), k, true, 36)
		g.send(t+ms(2), k, false, 36)
		g.ack(t+ms(3), k, true)
		t += 500 * time.Millisecond
	}
	g.close(t, k)
	return g
}

// retrans: as web_https but every 20th data segment is retransmitted (~5%).
func retransFixture(v vendors) *gen {
	g := newGen(4)
	c, s := localHost("10.0.1.10", v.rpi), externalHost("203.0.113.10")
	for i := range 3 {
		g.webSession(time.Duration(i)*2*time.Second, c, s, 443, ms(6), ms(6), 517, 66_700, 20)
	}
	return g
}

// scanPorts probes ports 1..1024 of target over 8 s; 22 and 80 are open.
func scanPorts(g *gen, start time.Duration, scanner, target host) {
	open, closed := true, false
	cport := uint16(40000 + g.rng.IntN(20000)) //nolint:gosec // < 65536
	step := 8 * time.Second / 1024
	for p := 1; p <= 1024; p++ {
		state := &closed
		if p == 22 || p == 80 {
			state = &open
		}
		g.synProbe(start+time.Duration(p-1)*step, scanner, target, cport, uint16(p), state) //nolint:gosec // p <= 1024
	}
}

// scan_syn: 10.0.1.66 SYN-scans ports 1..1024 of 10.0.9.5 in 8 s.
func scanSYN(v vendors) *gen {
	g := newGen(5)
	scanPorts(g, 0, localHost("10.0.1.66", v.apple), localHost("10.0.9.5", noVendor))
	return g
}

// scan_sweep: 10.0.1.66 probes port 445 on every other host of 10.0.1.0/24; no replies.
func scanSweep(v vendors) *gen {
	g := newGen(6)
	scanner := localHost("10.0.1.66", v.apple)
	cport := uint16(40000 + g.rng.IntN(20000)) //nolint:gosec // < 65536
	t := time.Duration(0)
	for i := 2; i <= 254; i++ {
		if i == 66 {
			continue
		}
		g.synProbe(t, scanner, localHost(fmt.Sprintf("10.0.1.%d", i), noVendor), cport, 445, nil)
		t += 20 * time.Millisecond
	}
	return g
}

// flood: 55 s of 50 pps from 5 clients to 10.0.9.5:80, with a 5 s SYN flood of
// 3000 pps from 60 documentation-range sources starting at 40 s (min_sources is 50).
func flood(v vendors) *gen {
	g := newGen(7)
	server := localHost("10.0.9.5", noVendor)
	for i := range 5 {
		k := g.newConn(localHost(fmt.Sprintf("10.0.1.%d", 20+i), v.apple), server, 80)
		t := g.handshake(time.Duration(i)*10*time.Millisecond, k, ms(0.4), ms(0.4))
		for t < 55*time.Second {
			g.send(t+ms(1), k, true, 300)
			g.send(t+ms(3), k, false, 1200)
			t += 200 * time.Millisecond
		}
	}
	sources := make([]host, 0, 60)
	for i := 1; i <= 30; i++ {
		sources = append(sources, externalHost(fmt.Sprintf("198.51.100.%d", i)), externalHost(fmt.Sprintf("203.0.113.%d", i)))
	}
	for n := range 15000 {
		at := 40*time.Second + time.Duration(n)*time.Second/3000
		src := sources[n%len(sources)]
		cport := uint16(1024 + g.rng.IntN(60000)) //nolint:gosec // < 65536
		g.tcp(at, src, server, tcpSeg{sport: cport, dport: 80, seq: g.rng.Uint32(), syn: true})
	}
	return g
}

// idle: 10.0.1.30 is active for 10 s then silent; 10.0.1.10 keeps a 1 query/s
// heartbeat until 140 s so time keeps advancing.
func idle(v vendors) *gen {
	g := newGen(8)
	quiet, busy := localHost("10.0.1.30", v.apple), localHost("10.0.1.10", v.rpi)
	for i := range 10 {
		g.dns(time.Duration(i)*time.Second, quiet, gateway(), ms(3))
	}
	g.webSession(2*time.Second, quiet, externalHost("203.0.113.20"), 443, ms(8), ms(8), 517, 20_000, 0)
	for i := range 141 {
		g.dns(time.Duration(i)*time.Second+ms(500), busy, gateway(), ms(3))
	}
	return g
}

// ipv6: fd00:1::10 talks HTTPS to 2001:db8:ffff::1, plus neighbour discovery
// with the router fd00:1::1.
func ipv6(v vendors) *gen {
	g := newGen(9)
	c := localHost("fd00:1::10", v.rpi)
	router := host{ip: netip.MustParseAddr("fd00:1::1"), mac: gatewayMAC, local: true}
	g.icmpv6NS(0, c, router, router.ip)
	g.icmpv6NA(ms(1), router, c)
	g.webSession(ms(10), c, externalHost("2001:db8:ffff::1"), 443, ms(9), ms(9), 517, 30_000, 0)
	return g
}

// vlan: VLAN 10 carries corp hosts and their resolver; VLAN 20 carries a core
// SSH session.
func vlan(v vendors) *gen {
	g := newGen(10)
	gw := gateway().withVLAN(10)
	for i, c := range []host{localHost("10.0.1.10", v.rpi).withVLAN(10), localHost("10.0.1.11", v.espressif).withVLAN(10)} {
		for j := range 5 {
			g.dns(time.Duration(j)*time.Second+time.Duration(i)*ms(100), c, gw, ms(2))
		}
	}
	k := g.newConn(localHost("10.0.9.6", noVendor).withVLAN(20), localHost("10.0.9.5", noVendor).withVLAN(20), 22)
	t := g.handshake(0, k, ms(0.3), ms(0.3))
	for range 10 {
		g.send(t+ms(1), k, true, 36)
		g.send(t+ms(2), k, false, 36)
		t += 500 * time.Millisecond
	}
	g.close(t, k)
	return g
}

// gateway: 10 external hosts reached through the router's MAC, plus DNS from
// the router itself, so its MAC can be attributed to 10.0.1.1 (D13).
func gatewayFixture(v vendors) *gen {
	g := newGen(11)
	c := localHost("10.0.1.10", v.rpi)
	for i := range 10 {
		at := time.Duration(i) * 500 * time.Millisecond
		g.dns(at, c, gateway(), ms(3))
		g.webSession(at+ms(10), c, externalHost(fmt.Sprintf("203.0.113.%d", 30+i)), 443, ms(10), ms(10), 517, 8_000, 0)
	}
	return g
}

// demo: 120 s of mixed traffic on a small office network, with one SYN scan at
// 60 s. Used by end-to-end tests and the README recording.
func demo(v vendors) *gen {
	g := newGen(12)
	const duration = 120 * time.Second

	corpVendors := []net.HardwareAddr{v.apple, v.rpi, v.apple, v.espressif, v.apple, v.sonos}
	var corp []host
	for i := range 30 {
		corp = append(corp, localHost(fmt.Sprintf("10.0.1.%d", 10+i), corpVendors[i%len(corpVendors)]))
	}
	var core []host
	for i := range 10 {
		prefix := noVendor
		if i == 0 {
			prefix = v.cisco
		}
		core = append(core, localHost(fmt.Sprintf("10.0.9.%d", 2+i), prefix))
	}
	sshServer, webServer, broker := core[3], core[4], core[5] // 10.0.9.5, .6, .7
	var external []host
	for i := 1; i <= 100; i++ {
		external = append(external, externalHost(fmt.Sprintf("203.0.113.%d", i)), externalHost(fmt.Sprintf("198.51.100.%d", i)))
	}
	mdns := host{ip: netip.MustParseAddr("224.0.0.251"), mac: net.HardwareAddr{0x01, 0, 0x5e, 0, 0, 0xfb}, local: true}
	ssdp := host{ip: netip.MustParseAddr("239.255.255.250"), mac: net.HardwareAddr{0x01, 0, 0x5e, 0x7f, 0xff, 0xfa}, local: true}

	for i, c := range corp {
		iot := corpVendors[i%len(corpVendors)].String() == v.espressif.String() || corpVendors[i%len(corpVendors)].String() == v.sonos.String()
		if iot {
			for t := time.Duration(g.rng.IntN(5000)) * time.Millisecond; t < duration; t += 15 * time.Second {
				g.udp(t, c, mdns, 5353, 5353, 120)
				g.webSession(t+ms(50), c, broker, 1883, ms(1), ms(1), 60, 200, 0)
			}
			if corpVendors[i%len(corpVendors)].String() == v.sonos.String() {
				for t := time.Duration(g.rng.IntN(9000)) * time.Millisecond; t < duration; t += 30 * time.Second {
					g.udp(t, c, ssdp, 1900, 1900, 300)
				}
			}
			continue
		}
		period := time.Duration(4000+g.rng.IntN(6000)) * time.Millisecond
		for t := time.Duration(g.rng.IntN(4000)) * time.Millisecond; t < duration-2*time.Second; t += period {
			dst := external[g.rng.IntN(len(external))]
			half := ms(float64(3 + g.rng.IntN(25)))
			g.dns(t, c, gateway(), ms(2))
			g.webSession(t+ms(5), c, dst, 443, half, half, 517, 5_000+g.rng.IntN(55_000), 0)
		}
		if i%6 == 0 { // some hosts also use the internal web server and SSH
			g.webSession(time.Duration(g.rng.IntN(60))*time.Second, c, webServer, 80, ms(0.5), ms(0.5), 400, 30_000, 0)
			k := g.newConn(c, sshServer, 22)
			t := g.handshake(time.Duration(10+g.rng.IntN(90))*time.Second, k, ms(0.4), ms(0.4))
			for range 20 {
				g.send(t+ms(1), k, true, 36)
				g.send(t+ms(2), k, false, 36)
				t += 400 * time.Millisecond
			}
			g.close(t, k)
		}
	}
	scanPorts(g, 60*time.Second, localHost("10.0.1.66", v.apple), sshServer)
	return g
}
