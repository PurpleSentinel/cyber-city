// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/netip"
	"slices"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

// baseTime is the timestamp of t=0 in every fixture (docs/TESTING.md "Fixtures").
var baseTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// maxStoredPayload bounds the payload bytes written per packet. Header lengths
// still describe the full payload, like a snaplen-truncated capture, which keeps
// fixtures small while byte counts stay realistic.
const maxStoredPayload = 64

// gatewayMAC is the router's MAC: every frame to or from a non-local host
// carries it (docs/DECISIONS.md D13).
var gatewayMAC = net.HardwareAddr{0x02, 0, 0, 0, 0, 0x01}

// host is an address on the capture link (or behind the gateway when !local).
type host struct {
	ip    netip.Addr
	mac   net.HardwareAddr
	local bool
	vlan  uint16
}

type packet struct {
	at      time.Duration
	order   int // tie-break for identical timestamps: generation order
	data    []byte
	wireLen int
}

// gen accumulates packets for one fixture and writes them sorted by time.
type gen struct {
	rng     *rand.Rand
	packets []packet
	err     error
}

func newGen(seed uint64) *gen {
	return &gen{rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
}

func (g *gen) fail(err error) {
	if g.err == nil {
		g.err = err
	}
}

// localHost returns a host on the link with a MAC derived from prefix and the address.
func localHost(ip string, prefix net.HardwareAddr) host {
	a := netip.MustParseAddr(ip)
	b := a.As16()
	mac := net.HardwareAddr{prefix[0], prefix[1], prefix[2], b[13], b[14], b[15]}
	return host{ip: a, mac: mac, local: true}
}

// externalHost returns a host behind the gateway.
func externalHost(ip string) host {
	return host{ip: netip.MustParseAddr(ip)}
}

func (h host) withVLAN(id uint16) host { h.vlan = id; return h }

func frameMACs(src, dst host) (net.HardwareAddr, net.HardwareAddr) {
	s, d := gatewayMAC, gatewayMAC
	if src.local {
		s = src.mac
	}
	if dst.local {
		d = dst.mac
	}
	return s, d
}

// tcpSeg describes one TCP segment.
type tcpSeg struct {
	sport, dport uint16
	seq, ack     uint32
	syn, ackF    bool
	fin, rst     bool
	psh          bool
	payload      int
}

func (g *gen) tcp(at time.Duration, src, dst host, s tcpSeg) {
	t := &layers.TCP{
		SrcPort: layers.TCPPort(s.sport), DstPort: layers.TCPPort(s.dport),
		Seq: s.seq, Ack: s.ack, DataOffset: 5, Window: 64240,
		SYN: s.syn, ACK: s.ackF, FIN: s.fin, RST: s.rst, PSH: s.psh,
	}
	g.emit(at, src, dst, layers.IPProtocolTCP, t, 20, s.payload)
}

func (g *gen) udp(at time.Duration, src, dst host, sport, dport uint16, payload int) {
	u := &layers.UDP{SrcPort: layers.UDPPort(sport), DstPort: layers.UDPPort(dport), Length: uint16(8 + payload)} //nolint:gosec // small payloads
	g.emit(at, src, dst, layers.IPProtocolUDP, u, 8, payload)
}

// icmpv6NS emits a neighbour solicitation from src for target (category "control").
func (g *gen) icmpv6NS(at time.Duration, src, dst host, target netip.Addr) {
	icmp := &layers.ICMPv6{TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeNeighborSolicitation, 0)}
	ns := &layers.ICMPv6NeighborSolicitation{TargetAddress: net.IP(target.AsSlice())}
	g.emitICMPv6(at, src, dst, icmp, ns)
}

// icmpv6NA emits a neighbour advertisement from src about itself.
func (g *gen) icmpv6NA(at time.Duration, src, dst host) {
	icmp := &layers.ICMPv6{TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeNeighborAdvertisement, 0)}
	na := &layers.ICMPv6NeighborAdvertisement{TargetAddress: net.IP(src.ip.AsSlice()), Flags: 0x60}
	g.emitICMPv6(at, src, dst, icmp, na)
}

func (g *gen) emitICMPv6(at time.Duration, src, dst host, icmp *layers.ICMPv6, body gopacket.SerializableLayer) {
	ip := &layers.IPv6{Version: 6, NextHeader: layers.IPProtocolICMPv6, HopLimit: 255,
		SrcIP: net.IP(src.ip.AsSlice()), DstIP: net.IP(dst.ip.AsSlice())}
	if err := icmp.SetNetworkLayerForChecksum(ip); err != nil {
		g.fail(err)
		return
	}
	layersOut := append(g.link(src, dst, layers.EthernetTypeIPv6), ip, icmp, body)
	g.serialize(at, layersOut, true, 0)
}

// emit builds Ethernet[/Dot1Q]/IP/transport with header lengths covering the
// full payload but at most maxStoredPayload bytes stored.
func (g *gen) emit(at time.Duration, src, dst host, proto layers.IPProtocol, transport gopacket.SerializableLayer, transportLen, payload int) {
	stored := min(payload, maxStoredPayload)
	var ipLayer interface {
		gopacket.SerializableLayer
		gopacket.NetworkLayer
	}
	var ipLen int
	var etherType layers.EthernetType
	if src.ip.Is4() {
		ipLen = 20 + transportLen + payload
		ipLayer = &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: proto, Length: uint16(ipLen), //nolint:gosec // bounded sizes
			Flags: layers.IPv4DontFragment, SrcIP: net.IP(src.ip.AsSlice()), DstIP: net.IP(dst.ip.AsSlice())}
		etherType = layers.EthernetTypeIPv4
	} else {
		ipLen = 40 + transportLen + payload
		ipLayer = &layers.IPv6{Version: 6, NextHeader: proto, HopLimit: 64, Length: uint16(transportLen + payload), //nolint:gosec // bounded sizes
			SrcIP: net.IP(src.ip.AsSlice()), DstIP: net.IP(dst.ip.AsSlice())}
		etherType = layers.EthernetTypeIPv6
	}
	if cs, ok := transport.(interface {
		SetNetworkLayerForChecksum(gopacket.NetworkLayer) error
	}); ok {
		if err := cs.SetNetworkLayerForChecksum(ipLayer); err != nil {
			g.fail(err)
			return
		}
	}
	out := append(g.link(src, dst, etherType), ipLayer, transport, gopacket.Payload(make([]byte, stored)))
	g.serialize(at, out, false, ipLen)
}

func (g *gen) link(src, dst host, etherType layers.EthernetType) []gopacket.SerializableLayer {
	smac, dmac := frameMACs(src, dst)
	if src.vlan == 0 {
		return []gopacket.SerializableLayer{&layers.Ethernet{SrcMAC: smac, DstMAC: dmac, EthernetType: etherType}}
	}
	return []gopacket.SerializableLayer{
		&layers.Ethernet{SrcMAC: smac, DstMAC: dmac, EthernetType: layers.EthernetTypeDot1Q},
		&layers.Dot1Q{VLANIdentifier: src.vlan, Type: etherType},
	}
}

// serialize writes the layers. With fixLengths, lengths are computed from the
// stored bytes (ICMPv6 carries no truncated payload); otherwise the caller's
// header lengths are kept and the wire length is linkHeader + ipLen.
func (g *gen) serialize(at time.Duration, ls []gopacket.SerializableLayer, fixLengths bool, ipLen int) {
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: fixLengths, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, ls...); err != nil {
		g.fail(fmt.Errorf("serialize at %s: %w", at, err))
		return
	}
	data := slices.Clone(buf.Bytes())
	wire := len(data)
	if !fixLengths {
		linkLen := 14
		if _, tagged := ls[1].(*layers.Dot1Q); tagged {
			linkLen = 18
		}
		// Short frames are padded to Ethernet's 60-byte minimum, as on the wire.
		wire = max(linkLen+ipLen, len(data))
	}
	g.packets = append(g.packets, packet{at: at, order: len(g.packets), data: data, wireLen: wire})
}

// write sorts by time and writes a pcap; it returns the packet count and span.
func (g *gen) write(w io.Writer) (int, time.Duration, error) {
	if g.err != nil {
		return 0, 0, g.err
	}
	slices.SortStableFunc(g.packets, func(a, b packet) int {
		if a.at != b.at {
			return int(a.at - b.at)
		}
		return a.order - b.order
	})
	pw := pcapgo.NewWriter(w)
	if err := pw.WriteFileHeader(65536, layers.LinkTypeEthernet); err != nil {
		return 0, 0, err
	}
	for _, p := range g.packets {
		ci := gopacket.CaptureInfo{Timestamp: baseTime.Add(p.at), CaptureLength: len(p.data), Length: p.wireLen}
		if err := pw.WritePacket(ci, p.data); err != nil {
			return 0, 0, err
		}
	}
	var span time.Duration
	if n := len(g.packets); n > 0 {
		span = g.packets[n-1].at - g.packets[0].at
	}
	return len(g.packets), span, nil
}

// ms converts milliseconds (possibly fractional) to a Duration.
func ms(v float64) time.Duration { return time.Duration(v * float64(time.Millisecond)) }
