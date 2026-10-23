package pipeline

import (
	"container/list"
	"net/netip"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// Category table (docs/CONFIG.md "Traffic categories"), keyed by protocol and port.
var (
	tcpCategories = map[uint16]string{
		443: protocol.CatHTTPS, 8443: protocol.CatHTTPS,
		80: protocol.CatWebCleartext, 8000: protocol.CatWebCleartext, 8080: protocol.CatWebCleartext,
		53: protocol.CatDNS, 853: protocol.CatDNS,
		22: protocol.CatSSH,
		21: protocol.CatLegacyCleartext, 23: protocol.CatLegacyCleartext, 25: protocol.CatLegacyCleartext,
		110: protocol.CatLegacyCleartext, 143: protocol.CatLegacyCleartext, 512: protocol.CatLegacyCleartext,
		513: protocol.CatLegacyCleartext, 514: protocol.CatLegacyCleartext,
	}
	udpCategories = map[uint16]string{
		443: protocol.CatHTTPS,
		53:  protocol.CatDNS,
		69:  protocol.CatLegacyCleartext, 161: protocol.CatLegacyCleartext,
		67: protocol.CatDiscovery, 68: protocol.CatDiscovery, 137: protocol.CatDiscovery,
		138: protocol.CatDiscovery, 546: protocol.CatDiscovery, 547: protocol.CatDiscovery,
		1900: protocol.CatDiscovery, 5353: protocol.CatDiscovery, 5355: protocol.CatDiscovery,
	}
)

// PortCategory returns the category of a server port, or "" if the port is
// not in the table.
func PortCategory(p Proto, port uint16) string {
	switch p {
	case ProtoTCP:
		return tcpCategories[port]
	case ProtoUDP:
		return udpCategories[port]
	}
	return ""
}

// Category returns the category of traffic to a service.
func Category(p Proto, servicePort *uint16) string {
	switch p {
	case ProtoICMP, ProtoICMPv6:
		return protocol.CatControl
	}
	if servicePort != nil {
		if c := PortCategory(p, *servicePort); c != "" {
			return c
		}
	}
	return protocol.CatOther
}

// Side identifies the server end of a connection.
type Side uint8

// Server sides; SideUnknown maps to server_side: null.
const (
	SideUnknown Side = iota
	SideSrc
	SideDst
)

// decisionStrength orders how a server side was decided: handshake evidence
// (rules 1 and 2) overrides a guess from port numbers (rules 3 to 5).
type decisionStrength uint8

const (
	byPorts decisionStrength = iota
	byHandshake
)

// connKey identifies a connection independent of direction: endpoint a is
// the lexically smaller (address, port).
type connKey struct {
	proto Proto
	a, b  netip.AddrPort
}

func newConnKey(r *Record) (connKey, bool) {
	src := netip.AddrPortFrom(r.Src, r.SrcPort)
	dst := netip.AddrPortFrom(r.Dst, r.DstPort)
	if src.Compare(dst) <= 0 {
		return connKey{r.Proto, src, dst}, false
	}
	return connKey{r.Proto, dst, src}, true
}

// serverDecision says which endpoint of a connKey is the server.
type serverDecision struct {
	serverIsB bool
	known     bool // false: rule 5, no server side
	strength  decisionStrength
	port      uint16
}

// Direction is a flow's orientation and service, per docs/CONFIG.md
// "Service port and server side".
type Direction struct {
	Src, Dst    netip.Addr
	ServicePort *uint16
	ServerSide  Side
}

// ServerTracker decides and caches each connection's server side, keeping at
// most max connections (least recently seen evicted first).
type ServerTracker struct {
	max   int
	order *list.List // front = most recently seen; values are connKey
	items map[connKey]*trackerItem
}

type trackerItem struct {
	decision serverDecision
	elem     *list.Element
}

// NewServerTracker returns a tracker bounded to limit connections.
func NewServerTracker(limit int) *ServerTracker {
	return &ServerTracker{max: limit, order: list.New(), items: make(map[connKey]*trackerItem)}
}

// Len returns the number of tracked connections.
func (t *ServerTracker) Len() int { return len(t.items) }

// Direction returns the orientation of r's flow, updating the cache.
func (t *ServerTracker) Direction(r *Record) Direction {
	if r.Proto != ProtoTCP && r.Proto != ProtoUDP {
		src, dst := r.Src, r.Dst
		if dst.Less(src) { // stable orientation so both directions aggregate together
			src, dst = dst, src
		}
		return Direction{Src: src, Dst: dst}
	}

	key, swapped := newConnKey(r)
	d := t.decide(key, swapped, r)

	server, client := key.a, key.b
	if d.serverIsB {
		server, client = key.b, key.a
	}
	port := d.port
	if !d.known {
		// Rule 5: lower port, no server side; orient lower address first.
		src, dst := key.a.Addr(), key.b.Addr()
		return Direction{Src: src, Dst: dst, ServicePort: &port}
	}
	return Direction{Src: client.Addr(), Dst: server.Addr(), ServicePort: &port, ServerSide: SideDst}
}

func (t *ServerTracker) decide(key connKey, swapped bool, r *Record) serverDecision {
	// srcIsB: the packet's source is endpoint b of the key.
	srcIsB := swapped
	var fromHandshake *serverDecision
	if r.Proto == ProtoTCP && r.Flags&FlagSYN != 0 {
		// Rule 1: SYN without ACK, the server is the destination.
		// Rule 2: SYN+ACK, the server is the source.
		serverIsB := !srcIsB
		port := r.DstPort
		if r.Flags&FlagACK != 0 {
			serverIsB, port = srcIsB, r.SrcPort
		}
		fromHandshake = &serverDecision{serverIsB: serverIsB, known: true, strength: byHandshake, port: port}
	}

	if it, ok := t.items[key]; ok {
		t.order.MoveToFront(it.elem)
		if fromHandshake != nil && it.decision.strength < byHandshake {
			it.decision = *fromHandshake
		}
		return it.decision
	}

	d := byPortRules(key)
	if fromHandshake != nil {
		d = *fromHandshake
	}
	t.items[key] = &trackerItem{decision: d, elem: t.order.PushFront(key)}
	for len(t.items) > t.max {
		oldest := t.order.Back()
		delete(t.items, oldest.Value.(connKey))
		t.order.Remove(oldest)
	}
	return d
}

// byPortRules applies rules 3 to 5.
func byPortRules(key connKey) serverDecision {
	pa, pb := key.a.Port(), key.b.Port()
	ca, cb := PortCategory(key.proto, pa) != "", PortCategory(key.proto, pb) != ""
	switch {
	case ca && !cb: // rule 3
		return serverDecision{serverIsB: false, known: true, port: pa}
	case cb && !ca:
		return serverDecision{serverIsB: true, known: true, port: pb}
	case pa < 1024 || pb < 1024: // rule 4
		if pa <= pb {
			return serverDecision{serverIsB: false, known: true, port: pa}
		}
		return serverDecision{serverIsB: true, known: true, port: pb}
	default: // rule 5
		return serverDecision{known: false, port: min(pa, pb)}
	}
}
