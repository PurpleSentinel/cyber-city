package pipeline

import (
	"net/netip"
	"sync"
	"time"

	"github.com/gopacket/gopacket"

	"github.com/PurpleSentinel/cyber-city/internal/districts"
	"github.com/PurpleSentinel/cyber-city/internal/enrich"
	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// Options configure a Pipeline.
type Options struct {
	Window     time.Duration
	Heartbeat  time.Duration
	MaxTracked int
	// MaxHosts bounds the host table; the least recently seen host is evicted.
	MaxHosts int
	// IdleTimeout removes hosts silent for this long (packet time).
	IdleTimeout time.Duration
	// MaxServices bounds services per host (hosts.max_services).
	MaxServices int
	// HandshakeTimeout and RTTSamples tune handshake RTT (enrich.*).
	HandshakeTimeout time.Duration
	RTTSamples       int
	// RetransWindow is the per-host retransmission-rate window.
	RetransWindow time.Duration
	// GatewayMinIPs marks a MAC as a router's (enrich.gateway_min_ips, D13).
	GatewayMinIPs int
	// ClassInterval is how often device classes are re-evaluated.
	ClassInterval time.Duration
	// Districts assigns local hosts to configured districts (nil: auto only).
	Districts *districts.Matcher
	// Geo places external hosts in compass sectors (nil: --geoip not set).
	Geo *enrich.Geo
	// Server describes the sensor in hello messages.
	Server protocol.ServerInfo
	// Anonymize pseudonymises host ids and hides addresses (--anonymize).
	Anonymize bool
	// AnonKey fixes the pseudonym key (tests); nil means a random per-run key.
	AnonKey []byte
}

type flowKey struct {
	proto    Proto
	src, dst netip.Addr
	port     uint16
	hasPort  bool
}

type flowAgg struct {
	side    Side
	packets uint64
	bytes   uint64
	flags   uint8
	retrans int
}

// Pipeline aggregates packet records into protocol ticks. Add and Advance
// must be called from one goroutine; Snapshot may be called concurrently.
type Pipeline struct {
	opts Options
	id   *identity
	svc  *enrich.Services
	rtt  *enrich.RTT
	retx *enrich.Retrans
	macs *enrich.MACs
	cls  *enrich.Classifier
	// lastClassify is the packet time of the last device-class pass.
	lastClassify time.Time
	flowRTT      map[flowKey]float64 // most recent handshake RTT per flow key
	dec          *Decoder
	tracker      *ServerTracker

	// Window state, owned by the pipeline goroutine.
	started     bool
	windowStart time.Time
	lastEmitted time.Time
	flows       map[flowKey]*flowAgg
	packets     uint64
	bytes       uint64
	kernelDrops uint64
	ifDrops     uint64

	mu        sync.Mutex // guards hosts and districts for Snapshot
	hosts     map[netip.Addr]*hostState
	districts map[string]protocol.District
	newDist   []protocol.District
	removed   []string // host ids removed since the last tick
}

// New returns an empty pipeline.
func New(opts Options) *Pipeline {
	p := &Pipeline{
		opts:      opts,
		id:        newIdentity(opts.Anonymize, opts.AnonKey),
		svc:       enrich.NewServices(max(1, opts.MaxServices)),
		rtt:       enrich.NewRTT(opts.HandshakeTimeout, max(1, opts.MaxTracked), max(1, opts.RTTSamples)),
		flowRTT:   make(map[flowKey]float64),
		retx:      enrich.NewRetrans(opts.RetransWindow, max(1, opts.MaxTracked)),
		macs:      enrich.NewMACs(max(1, opts.GatewayMinIPs), max(1, opts.MaxHosts)),
		cls:       enrich.NewClassifier(),
		dec:       NewDecoder(),
		tracker:   NewServerTracker(opts.MaxTracked),
		flows:     make(map[flowKey]*flowAgg),
		hosts:     make(map[netip.Addr]*hostState),
		districts: map[string]protocol.District{protocol.OutlandsID: outlands()},
	}
	for _, d := range opts.Districts.Districts(opts.Anonymize) {
		p.districts[d.ID] = d
	}
	return p
}

// Frame decodes and adds one raw frame, returning any ticks completed by its
// timestamp.
func (p *Pipeline) Frame(data []byte, ci gopacket.CaptureInfo) []*protocol.Tick {
	ticks := p.Advance(ci.Timestamp)
	if !p.started {
		p.start(ci.Timestamp)
	}
	p.packets++
	p.bytes += uint64(max(ci.Length, len(data))) //nolint:gosec // non-negative
	if rec, ok := p.dec.Decode(data, ci); ok {
		p.add(&rec)
	}
	return ticks
}

// SetDrops records cumulative capture drop counters for the next tick.
func (p *Pipeline) SetDrops(kernel, iface uint64) { p.kernelDrops, p.ifDrops = kernel, iface }

func (p *Pipeline) start(t time.Time) {
	p.started = true
	p.windowStart = t.Truncate(p.opts.Window)
	p.lastEmitted = p.windowStart
}

func (p *Pipeline) add(r *Record) {
	d := p.tracker.Direction(r)
	key := flowKey{proto: r.Proto, src: d.Src, dst: d.Dst}
	if d.ServicePort != nil {
		key.port, key.hasPort = *d.ServicePort, true
	}
	f := p.flows[key]
	if f == nil {
		f = &flowAgg{side: d.ServerSide}
		p.flows[key] = f
	}
	f.packets++
	f.bytes += uint64(r.WireLen) //nolint:gosec // non-negative
	f.flags |= r.Flags

	p.mu.Lock()
	p.touch(r.Src, r.TS, r.VLAN)
	p.touch(r.Dst, r.TS, r.VLAN)
	for _, a := range p.macs.Observe(r.SrcMAC, r.Src, IsLocal(r.Src), r.DstMAC, r.Dst, IsLocal(r.Dst)) {
		if h := p.hosts[a]; h != nil {
			h.dirty = true
		}
	}
	if d.ServerSide == SideDst && d.ServicePort != nil && !isGroup(d.Dst) {
		port := *d.ServicePort
		p.cls.Connection(d.Src, d.Dst, port, Category(r.Proto, &port), IsLocal(d.Dst), r.TS)
	}
	p.observeService(r, &d)
	p.observeHandshake(r, &d, key)
	if r.Proto == ProtoTCP && r.PayloadLen > 0 {
		dir := enrich.Direction{From: netip.AddrPortFrom(r.Src, r.SrcPort), To: netip.AddrPortFrom(r.Dst, r.DstPort)}
		if p.retx.Segment(dir, r.Seq, r.PayloadLen, r.TS) {
			f.retrans++
			if h := p.hosts[r.Src]; h != nil {
				h.dirty = true
			}
		}
	}
	p.mu.Unlock()
}

// observeHandshake feeds TCP handshake packets to the RTT tracker; callers
// hold p.mu.
func (p *Pipeline) observeHandshake(r *Record, d *Direction, key flowKey) {
	if r.Proto != ProtoTCP || d.ServerSide != SideDst || d.ServicePort == nil {
		return
	}
	fromClient := r.Src == d.Src
	cport := r.SrcPort
	if !fromClient {
		cport = r.DstPort
	}
	c := enrich.Conn{Client: netip.AddrPortFrom(d.Src, cport), Server: netip.AddrPortFrom(d.Dst, *d.ServicePort)}
	syn, ack := r.Flags&FlagSYN != 0, r.Flags&FlagACK != 0
	switch {
	case fromClient && syn && !ack:
		p.rtt.SYN(c, r.TS)
	case !fromClient && syn && ack:
		p.rtt.SYNACK(c)
	case fromClient && ack && !syn:
		ms, ok := p.rtt.ACK(c, r.TS)
		if !ok {
			return
		}
		if len(p.flowRTT) >= p.opts.MaxTracked {
			clear(p.flowRTT) // bounded memory; values refill from new handshakes
		}
		p.flowRTT[key] = protocol.Round1(ms)
		for _, a := range []netip.Addr{d.Src, d.Dst} {
			if h := p.hosts[a]; h != nil {
				h.dirty = true
			}
		}
	}
}

// observeService records the service a server is seen providing. Only
// packets sent by the server count, so probes alone (scans) never create
// services (docs/DECISIONS.md D23). Callers hold p.mu.
func (p *Pipeline) observeService(r *Record, d *Direction) {
	if d.ServerSide != SideDst || d.ServicePort == nil || r.Src != d.Dst || isGroup(d.Dst) {
		return
	}
	port := *d.ServicePort
	var confirmed bool
	switch r.Proto {
	case ProtoTCP:
		if r.Flags&FlagRST != 0 {
			return
		}
		confirmed = r.Flags&(FlagSYN|FlagACK) == FlagSYN|FlagACK
	case ProtoUDP:
		confirmed = r.SrcPort == port
	default:
		return
	}
	if p.svc.Observe(d.Dst, r.Proto.String(), port, Category(r.Proto, &port), confirmed, r.TS) {
		if h := p.hosts[d.Dst]; h != nil {
			h.dirty = true
		}
	}
}

// touch records activity for a host seen in a frame on vlan (0: untagged);
// callers hold p.mu.
func (p *Pipeline) touch(a netip.Addr, ts time.Time, vlan uint16) {
	if isGroup(a) {
		return
	}
	h := p.hosts[a]
	if h == nil {
		h = &hostState{addr: a, local: IsLocal(a), firstSeen: ts, dirty: true}
		if id, ok := p.opts.Districts.Match(a, vlan); h.local && ok {
			h.district = id
		} else if h.local {
			d, isNew := p.id.district(a)
			h.district = d.ID
			if isNew {
				p.districts[d.ID] = d
				p.newDist = append(p.newDist, d)
			}
		} else {
			h.district = protocol.OutlandsID
		}
		if p.opts.MaxHosts > 0 && len(p.hosts) >= p.opts.MaxHosts {
			p.evictOldest()
		}
		p.hosts[a] = h
	}
	if ts.After(h.lastSeen) {
		h.lastSeen = ts
	}
	if h.lastSeen.Sub(h.sentSeen) >= hostResend {
		h.dirty = true
	}
}

// Advance completes every window that ends at or before t. Windows with
// traffic produce ticks; empty stretches produce a heartbeat tick at least
// every Heartbeat.
func (p *Pipeline) Advance(t time.Time) []*protocol.Tick {
	if !p.started {
		return nil
	}
	var out []*protocol.Tick
	for end := p.windowStart.Add(p.opts.Window); !t.Before(end); end = p.windowStart.Add(p.opts.Window) {
		if p.packets > 0 || end.Sub(p.lastEmitted) >= p.opts.Heartbeat {
			out = append(out, p.tick(end))
			p.lastEmitted = end
		}
		p.windowStart = end
	}
	return out
}

// Flush completes the current window if it saw traffic (used at end of input).
func (p *Pipeline) Flush() []*protocol.Tick {
	if !p.started || p.packets == 0 {
		return nil
	}
	end := p.windowStart.Add(p.opts.Window)
	t := p.tick(end)
	p.windowStart, p.lastEmitted = end, end
	return []*protocol.Tick{t}
}

func (p *Pipeline) tick(end time.Time) *protocol.Tick {
	t := &protocol.Tick{
		TS:       end.UnixMilli(),
		WindowMS: int(p.opts.Window / time.Millisecond),
		Stats: protocol.Stats{
			Packets: p.packets, Bytes: p.bytes, Flows: len(p.flows),
			KernelDrops: p.kernelDrops, IfDrops: p.ifDrops,
		},
		Flows: make([]protocol.Flow, 0, len(p.flows)),
	}
	for k, f := range p.flows {
		wf := protocol.Flow{
			Src: p.id.hostID(k.src), Dst: p.id.hostID(k.dst), Proto: k.proto.String(),
			Category: Category(k.proto, nil), Packets: f.packets, Bytes: f.bytes,
			TCPFlags: FlagNames(f.flags), Retrans: f.retrans,
		}
		if k.hasPort {
			port := k.port
			wf.ServicePort = protocol.Ptr(int(port))
			wf.Category = Category(k.proto, &port)
		}
		if f.side == SideDst {
			wf.ServerSide = protocol.Ptr(protocol.SideDst)
		}
		if rtt, ok := p.flowRTT[k]; ok {
			wf.RTTMS = protocol.Ptr(rtt)
		}
		t.Flows = append(t.Flows, wf)
	}
	clear(p.flows)
	p.packets, p.bytes = 0, 0

	p.mu.Lock()
	t.Stats.HostsTotal = len(p.hosts)
	t.Districts, p.newDist = p.newDist, nil
	p.removeIdle(end)
	if end.Sub(p.lastClassify) >= p.opts.ClassInterval {
		p.classify(end)
	}
	t.RemovedHosts, p.removed = p.removed, nil
	for _, h := range p.hosts {
		if h.dirty {
			t.Hosts = append(t.Hosts, p.wire(h))
			h.dirty, h.sentSeen = false, h.lastSeen
		}
	}
	p.mu.Unlock()
	protocol.SortTick(t)
	return t
}

// Snapshot returns a hello message describing the current state.
func (p *Pipeline) Snapshot() *protocol.Hello {
	p.mu.Lock()
	defer p.mu.Unlock()
	h := &protocol.Hello{
		V: protocol.Version, Type: protocol.TypeHello, Server: p.opts.Server,
		Districts: make([]protocol.District, 0, len(p.districts)),
		Hosts:     make([]protocol.Host, 0, len(p.hosts)),
	}
	h.Server.WindowMS = int(p.opts.Window / time.Millisecond)
	h.Server.Anonymized = p.opts.Anonymize
	h.Server.GeoIP = p.opts.Geo != nil
	for _, d := range p.districts {
		h.Districts = append(h.Districts, d)
	}
	for _, hs := range p.hosts {
		h.Hosts = append(h.Hosts, p.wire(hs))
	}
	protocol.SortHello(h)
	return h
}

// evictOldest removes the least recently seen host (ties: lowest address);
// callers hold p.mu.
func (p *Pipeline) evictOldest() {
	var oldest *hostState
	for _, h := range p.hosts {
		if oldest == nil || h.lastSeen.Before(oldest.lastSeen) ||
			(h.lastSeen.Equal(oldest.lastSeen) && h.addr.Less(oldest.addr)) {
			oldest = h
		}
	}
	if oldest != nil {
		p.remove(oldest)
	}
}

// removeIdle drops hosts silent for IdleTimeout as of packet time now;
// callers hold p.mu.
func (p *Pipeline) removeIdle(now time.Time) {
	if p.opts.IdleTimeout <= 0 {
		return
	}
	for _, h := range p.hosts {
		if now.Sub(h.lastSeen) >= p.opts.IdleTimeout {
			p.remove(h)
		}
	}
}

func (p *Pipeline) remove(h *hostState) {
	delete(p.hosts, h.addr)
	p.svc.Forget(h.addr)
	p.rtt.Forget(h.addr)
	p.retx.Forget(h.addr)
	p.macs.Forget(h.addr)
	p.cls.Forget(h.addr)
	p.opts.Geo.Forget(h.addr)
	p.removed = append(p.removed, p.id.hostID(h.addr))
}

// classify re-evaluates every host's device class as of packet time now
// (docs/CONFIG.md "Device class"); callers hold p.mu.
func (p *Pipeline) classify(now time.Time) {
	p.lastClassify = now
	for _, h := range p.hosts {
		_, vendor, _ := p.macs.For(h.addr)
		confirmed := 0
		for _, s := range p.svc.For(h.addr) {
			if s.Confirmed {
				confirmed++
			}
		}
		class := p.cls.Classify(enrich.ClassInput{
			Host: h.addr, Local: h.local, Vendor: vendor, Gateway: p.macs.IsGateway(h.addr),
			ConfirmedServices: confirmed, Now: now,
		})
		if class != h.class {
			h.class, h.dirty = class, true
		}
	}
}
