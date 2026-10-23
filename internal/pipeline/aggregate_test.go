package pipeline

import (
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

func tcpPacket(t *testing.T, src, dst string, dport uint16, syn bool) gopacket.Packet {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x02, 0, 0, 0, 0, 0x10},
		DstMAC:       net.HardwareAddr{0x02, 0, 0, 0, 0, 0x01},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP,
		SrcIP: net.ParseIP(src).To4(), DstIP: net.ParseIP(dst).To4()}
	tcp := &layers.TCP{SrcPort: 40000, DstPort: layers.TCPPort(dport), SYN: syn, ACK: !syn}
	if err := tcp.SetNetworkLayerForChecksum(ip); err != nil {
		t.Fatal(err)
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, eth, ip, tcp); err != nil {
		t.Fatal(err)
	}
	return gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
}

func TestAggregatorGroupsByFlowAndResets(t *testing.T) {
	a := NewAggregator()
	a.Add(tcpPacket(t, "10.0.1.10", "203.0.113.10", 443, true))
	a.Add(tcpPacket(t, "10.0.1.10", "203.0.113.10", 443, false))
	a.Add(tcpPacket(t, "10.0.1.10", "203.0.113.11", 80, false))

	got := a.Flush()
	if len(got) != 2 {
		t.Fatalf("flows = %d, want 2", len(got))
	}
	for _, m := range got {
		if m.DstIP == "203.0.113.10" {
			if m.PacketCount != 2 || m.Protocol != "TCP" || m.Port != 443 {
				t.Errorf("unexpected flow %+v", m)
			}
			if !m.SrcIsLocal || m.DstIsLocal {
				t.Errorf("locality wrong: %+v", m)
			}
		}
	}
	if n := len(a.Flush()); n != 0 {
		t.Fatalf("after flush: %d flows, want 0", n)
	}
}
