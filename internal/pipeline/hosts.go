package pipeline

import (
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// hostResend is how far last_seen must advance before an otherwise unchanged
// host is sent again (docs/PROTOCOL.md "tick").
const hostResend = 5 * time.Second

type hostState struct {
	addr      netip.Addr
	local     bool
	district  string
	firstSeen time.Time
	lastSeen  time.Time
	sentSeen  time.Time // last_seen value in the most recent message
	dirty     bool      // new or changed since the last tick
}

// wire builds the protocol view of a host; callers hold p.mu.
func (p *Pipeline) wire(h *hostState) protocol.Host {
	var rtt *float64
	if med, ok := p.rtt.HostMedian(h.addr); ok {
		rtt = protocol.Ptr(protocol.Round1(med))
	}
	return protocol.Host{
		RTTMS:       rtt,
		RetransRate: protocol.Round3(p.retx.Rate(h.addr, h.lastSeen)),
		ID:          p.id.hostID(h.addr),
		IP:          p.id.hostIP(h.addr),
		Services:    p.svc.For(h.addr),
		Local:       h.local,
		District:    h.district,
		DeviceClass: protocol.ClassUnknown,
		FirstSeen:   h.firstSeen.UnixMilli(),
		LastSeen:    h.lastSeen.UnixMilli(),
	}
}

// isGroup reports multicast and broadcast addresses. They are not hosts
// (docs/DECISIONS.md D21): flows to them are reported, but only the sender is
// a building.
func isGroup(a netip.Addr) bool {
	return a.IsMulticast() || a == netip.IPv4Unspecified() || a == netip.AddrFrom4([4]byte{255, 255, 255, 255})
}

// autoDistrict returns the automatic district of a local address: its /24
// (IPv4) or /64 (IPv6) (docs/PROTOCOL.md "hello").
func autoDistrict(a netip.Addr) protocol.District {
	if a.Is4() {
		p := netip.PrefixFrom(a, 24).Masked()
		b := p.Addr().As4()
		return protocol.District{
			ID:    fmt.Sprintf("lan-%d.%d.%d", b[0], b[1], b[2]),
			Name:  "LAN " + p.String(),
			Kind:  protocol.KindLAN,
			CIDRs: []string{p.String()},
		}
	}
	p := netip.PrefixFrom(a, 64).Masked()
	b := p.Addr().As16()
	parts := make([]string, 4)
	for i := range parts {
		parts[i] = fmt.Sprintf("%x", uint16(b[2*i])<<8|uint16(b[2*i+1]))
	}
	return protocol.District{
		ID:    "lan-" + strings.Join(parts, "-"),
		Name:  "LAN " + p.String(),
		Kind:  protocol.KindLAN,
		CIDRs: []string{p.String()},
	}
}

func outlands() protocol.District {
	return protocol.District{ID: protocol.OutlandsID, Name: "The Outlands", Kind: protocol.KindOutlands, CIDRs: []string{}}
}
