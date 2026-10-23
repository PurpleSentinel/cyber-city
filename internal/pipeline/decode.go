// SPDX-License-Identifier: MIT

package pipeline

import (
	"net"
	"net/netip"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// Proto is a transport protocol as reported on the wire (docs/PROTOCOL.md "Flow").
type Proto uint8

// Transport protocols.
const (
	ProtoOther Proto = iota
	ProtoTCP
	ProtoUDP
	ProtoICMP
	ProtoICMPv6
)

// String returns the protocol's wire name.
func (p Proto) String() string {
	switch p {
	case ProtoTCP:
		return "tcp"
	case ProtoUDP:
		return "udp"
	case ProtoICMP:
		return "icmp"
	case ProtoICMPv6:
		return "icmpv6"
	default:
		return "other"
	}
}

// TCP flag bits in Record.Flags, in canonical order (docs/PROTOCOL.md "Flow").
const (
	FlagSYN uint8 = 1 << iota
	FlagACK
	FlagFIN
	FlagRST
	FlagPSH
	FlagURG
)

var flagNames = [...]string{"SYN", "ACK", "FIN", "RST", "PSH", "URG"}

// FlagNames lists the set flags in canonical order.
func FlagNames(f uint8) []string {
	out := make([]string, 0, 2)
	for i, n := range flagNames {
		if f&(1<<i) != 0 {
			out = append(out, n)
		}
	}
	return out
}

// Record is the metadata of one IP packet. Payload bytes are never kept
// (docs/DECISIONS.md D8).
type Record struct {
	TS               time.Time
	WireLen          int
	SrcMAC, DstMAC   net.HardwareAddr
	VLAN             uint16
	Src, Dst         netip.Addr
	Proto            Proto
	SrcPort, DstPort uint16
	Flags            uint8
	Seq              uint32
	// PayloadLen comes from IP/TCP/UDP header lengths, not captured bytes,
	// because the snap length truncates payloads.
	PayloadLen int
}

// Decoder turns raw frames into Records without per-packet allocation of
// layer structs.
type Decoder struct {
	parser  *gopacket.DecodingLayerParser
	eth     layers.Ethernet
	dot1q   layers.Dot1Q
	ip4     layers.IPv4
	ip6     layers.IPv6
	tcp     layers.TCP
	udp     layers.UDP
	icmp4   layers.ICMPv4
	icmp6   layers.ICMPv6
	payload gopacket.Payload
	decoded []gopacket.LayerType
}

// NewDecoder returns a decoder for Ethernet frames.
func NewDecoder() *Decoder {
	d := &Decoder{}
	d.parser = gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet,
		&d.eth, &d.dot1q, &d.ip4, &d.ip6, &d.tcp, &d.udp, &d.icmp4, &d.icmp6, &d.payload)
	// Stop quietly at layers we do not model (IPv6 extension headers, ARP, …).
	d.parser.IgnoreUnsupported = true
	d.decoded = make([]gopacket.LayerType, 0, 8)
	return d
}

// Decode extracts a Record from one frame. ok is false for frames without an
// IPv4 or IPv6 header.
func (d *Decoder) Decode(data []byte, ci gopacket.CaptureInfo) (rec Record, ok bool) {
	// Truncated frames still yield the layers decoded so far; errors are not fatal.
	_ = d.parser.DecodeLayers(data, &d.decoded)

	rec.TS = ci.Timestamp
	rec.WireLen = ci.Length
	if rec.WireLen == 0 {
		rec.WireLen = len(data)
	}
	var ipHeaderLen, ipTotalLen int
	for _, lt := range d.decoded {
		switch lt {
		case layers.LayerTypeEthernet:
			rec.SrcMAC = append(net.HardwareAddr(nil), d.eth.SrcMAC...)
			rec.DstMAC = append(net.HardwareAddr(nil), d.eth.DstMAC...)
		case layers.LayerTypeDot1Q:
			rec.VLAN = d.dot1q.VLANIdentifier
		case layers.LayerTypeIPv4:
			rec.Src, _ = netip.AddrFromSlice(d.ip4.SrcIP.To4())
			rec.Dst, _ = netip.AddrFromSlice(d.ip4.DstIP.To4())
			ipHeaderLen, ipTotalLen = int(d.ip4.IHL)*4, int(d.ip4.Length)
			ok = true
		case layers.LayerTypeIPv6:
			rec.Src, _ = netip.AddrFromSlice(d.ip6.SrcIP)
			rec.Dst, _ = netip.AddrFromSlice(d.ip6.DstIP)
			ipHeaderLen, ipTotalLen = 40, 40+int(d.ip6.Length)
			ok = true
		case layers.LayerTypeTCP:
			rec.Proto = ProtoTCP
			rec.SrcPort, rec.DstPort = uint16(d.tcp.SrcPort), uint16(d.tcp.DstPort)
			rec.Seq = d.tcp.Seq
			rec.Flags = tcpFlags(&d.tcp)
			rec.PayloadLen = max(0, ipTotalLen-ipHeaderLen-int(d.tcp.DataOffset)*4)
		case layers.LayerTypeUDP:
			rec.Proto = ProtoUDP
			rec.SrcPort, rec.DstPort = uint16(d.udp.SrcPort), uint16(d.udp.DstPort)
			rec.PayloadLen = max(0, int(d.udp.Length)-8)
		case layers.LayerTypeICMPv4:
			rec.Proto = ProtoICMP
		case layers.LayerTypeICMPv6:
			rec.Proto = ProtoICMPv6
		}
	}
	if ok {
		rec.Src, rec.Dst = rec.Src.Unmap(), rec.Dst.Unmap()
	}
	return rec, ok
}

func tcpFlags(t *layers.TCP) uint8 {
	var f uint8
	for i, set := range [...]bool{t.SYN, t.ACK, t.FIN, t.RST, t.PSH, t.URG} {
		if set {
			f |= 1 << i
		}
	}
	return f
}

// IsLocal reports whether addr is on the local side of the sensor
// (docs/CONFIG.md "Local address").
func IsLocal(addr netip.Addr) bool {
	return addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast()
}
