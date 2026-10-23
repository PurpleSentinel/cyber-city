// Package pipeline turns packets into the per-window messages sent to clients.
package pipeline

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// PacketMetrics is one flow's totals for a window, in the prototype wire
// format. T0.4/T0.8 replace it with the v1 protocol (docs/PROTOCOL.md).
type PacketMetrics struct {
	Timestamp   int64    `json:"timestamp"`
	SrcIP       string   `json:"src_ip"`
	SrcMac      string   `json:"src_mac"`
	SrcIsLocal  bool     `json:"src_is_local"`
	DstIP       string   `json:"dst_ip"`
	DstMac      string   `json:"dst_mac"`
	DstIsLocal  bool     `json:"dst_is_local"`
	Protocol    string   `json:"protocol"`
	Port        int      `json:"port"`
	PacketCount int      `json:"packet_count"`
	ByteCount   int      `json:"byte_count"`
	Flags       []string `json:"flags"`
}

// Aggregator accumulates flows until Flush.
type Aggregator struct {
	mu      sync.Mutex
	metrics map[string]*PacketMetrics
	now     func() time.Time
}

// NewAggregator returns an empty aggregator.
func NewAggregator() *Aggregator {
	return &Aggregator{metrics: make(map[string]*PacketMetrics), now: time.Now}
}

// Add folds one packet into the current window. Non-IPv4 packets are ignored.
func (a *Aggregator) Add(packet gopacket.Packet) {
	netLayer := packet.NetworkLayer()
	if netLayer == nil {
		return
	}
	ip4, ok := netLayer.(*layers.IPv4)
	if !ok {
		return
	}

	var srcMac, dstMac string
	if link := packet.LinkLayer(); link != nil {
		srcMac = link.LinkFlow().Src().String()
		dstMac = link.LinkFlow().Dst().String()
	}

	srcIP, dstIP := ip4.SrcIP.String(), ip4.DstIP.String()
	protocol := ip4.Protocol.String()
	var port int
	var flags []string
	switch t := packet.TransportLayer().(type) {
	case *layers.TCP:
		port, protocol = int(t.DstPort), "TCP"
		if t.SYN {
			flags = append(flags, "SYN")
		}
		if t.ACK {
			flags = append(flags, "ACK")
		}
	case *layers.UDP:
		port, protocol = int(t.DstPort), "UDP"
	}
	bytes := len(packet.Data())

	key := fmt.Sprintf("%s->%s:%d", srcIP, dstIP, port)
	a.mu.Lock()
	defer a.mu.Unlock()
	if m, exists := a.metrics[key]; exists {
		m.PacketCount++
		m.ByteCount += bytes
		return
	}
	a.metrics[key] = &PacketMetrics{
		Timestamp:   a.now().UnixMilli(),
		SrcIP:       srcIP,
		SrcMac:      srcMac,
		SrcIsLocal:  isLocal(srcIP),
		DstIP:       dstIP,
		DstMac:      dstMac,
		DstIsLocal:  isLocal(dstIP),
		Protocol:    protocol,
		Port:        port,
		PacketCount: 1,
		ByteCount:   bytes,
		Flags:       flags,
	}
}

// Flush returns the window's flows and starts a new window.
func (a *Aggregator) Flush() []*PacketMetrics {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]*PacketMetrics, 0, len(a.metrics))
	for _, m := range a.metrics {
		out = append(out, m)
	}
	a.metrics = make(map[string]*PacketMetrics)
	return out
}

func isLocal(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && (parsed.IsPrivate() || parsed.IsLoopback())
}
