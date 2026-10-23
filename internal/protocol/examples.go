// SPDX-License-Identifier: MIT

package protocol

// Canonical example messages. They are written to testdata/protocol/*.json by
// `go test ./internal/protocol -update` and parsed by web/test/protocol.test.ts,
// which is how the Go and TypeScript definitions are kept in step.

func exampleHost() Host {
	return Host{
		ID:          "10.0.1.10",
		IP:          Ptr("10.0.1.10"),
		Local:       true,
		District:    "corp",
		MAC:         nil,
		Vendor:      Ptr("Raspberry Pi Foundation"),
		DeviceClass: ClassIoT,
		Services:    []Service{{Port: 22, Proto: "tcp", Category: CatSSH, Confirmed: true}},
		Geo:         nil,
		FirstSeen:   1767225600000,
		LastSeen:    1767225610090,
		RTTMS:       Ptr(11.8),
		RetransRate: 0,
	}
}

func exampleExternalHost() Host {
	return Host{
		ID:          "203.0.113.10",
		IP:          Ptr("203.0.113.10"),
		Local:       false,
		District:    OutlandsID,
		DeviceClass: ClassServer,
		Services:    []Service{{Port: 443, Proto: "tcp", Category: CatHTTPS, Confirmed: true}},
		Geo:         &Geo{Sector: "NE", Country: "JP"},
		FirstSeen:   1767225600000,
		LastSeen:    1767225610090,
		RTTMS:       Ptr(12.1),
		RetransRate: 0.002,
	}
}

// ExampleHello is the canonical hello message.
func ExampleHello() *Hello {
	return &Hello{
		V:    Version,
		Type: TypeHello,
		Server: ServerInfo{
			Version: "test", Source: "live", Iface: Ptr("enp3s0"), WindowMS: 100,
			Anonymized: false, ShowMAC: false, GeoIP: true,
		},
		Districts: []District{
			{ID: "core", Name: "Server Core", Kind: KindServer, CIDRs: []string{"10.0.9.0/24"}},
			{ID: "corp", Name: "Corporate", Kind: KindLAN, CIDRs: []string{"10.0.1.0/24"}},
			{ID: OutlandsID, Name: "The Outlands", Kind: KindOutlands},
		},
		Hosts: []Host{exampleHost(), exampleExternalHost()},
	}
}

// ExampleTick is the canonical tick with flows and host updates.
func ExampleTick() *Tick {
	return &Tick{
		V: Version, Type: TypeTick, Seq: 4182, TS: 1767225610100, WindowMS: 100,
		Stats: Stats{Packets: 812, Bytes: 603112, Flows: 2, HostsTotal: 142},
		Flows: []Flow{
			{
				Src: "10.0.1.10", Dst: "203.0.113.10", Proto: "tcp",
				ServicePort: Ptr(443), ServerSide: Ptr(SideDst), Category: CatHTTPS,
				Packets: 12, Bytes: 9812, TCPFlags: []string{"PSH", "ACK"}, RTTMS: Ptr(12.1),
			},
			{
				Src: "fe80::1", Dst: "ff02::1", Proto: "icmpv6",
				Category: CatControl, Packets: 1, Bytes: 86,
			},
		},
		Hosts:        []Host{exampleHost()},
		RemovedHosts: []string{"203.0.113.77"},
	}
}

// ExampleEventsTick is a tick carrying one event of each kind.
func ExampleEventsTick() *Tick {
	return &Tick{
		V: Version, Type: TypeTick, Seq: 4300, TS: 1767225660000, WindowMS: 100,
		Stats:        Stats{Packets: 3000, Bytes: 180000, Flows: 0, HostsTotal: 150},
		RemovedHosts: []string{"10.0.1.30"},
		Events: []Event{
			{
				ID: "e-17", Kind: EventPortScan, TS: 1767225660000, Severity: SeverityAlert,
				Src: Ptr("10.0.1.66"), Dst: Ptr("10.0.9.5"), Targets: []string{"10.0.9.5"},
				Evidence:   map[string]float64{"distinct_ports": 1024, "distinct_hosts": 1, "incomplete_ratio": 0.99, "window_s": 10},
				DurationMS: 6000,
			},
			{
				ID: "e-18", Kind: EventFlood, TS: 1767225660000, Severity: SeverityAlert,
				Dst: Ptr("10.0.9.5"), Targets: []string{"10.0.9.5"},
				Evidence:   map[string]float64{"pps": 3000, "baseline_pps": 50, "distinct_sources": 300, "window_s": 5},
				DurationMS: 8000,
			},
			{
				ID: "e-19", Kind: EventHostIdle, TS: 1767225660000, Severity: SeverityInfo,
				Src: Ptr("10.0.1.30"), Targets: []string{},
				Evidence:   map[string]float64{"idle_s": 120},
				DurationMS: 5000,
			},
		},
	}
}
