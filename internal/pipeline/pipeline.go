package pipeline

import (
	"net/netip"
	"sync"
	"time"

	"github.com/gopacket/gopacket"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// Options configure a Pipeline.
type Options struct {
	Window     time.Duration
	Heartbeat  time.Duration
	MaxTracked int
	// Server describes the sensor in hello messages.
	Server protocol.ServerInfo
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
}

// Pipeline aggregates packet records into protocol ticks. Add and Advance
// must be called from one goroutine; Snapshot may be called concurrently.
type Pipeline struct {
	opts    Options
	dec     *Decoder
	tracker *ServerTracker

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
}

// New returns an empty pipeline.
func New(opts Options) *Pipeline {
	return &Pipeline{
		opts:      opts,
		dec:       NewDecoder(),
		tracker:   NewServerTracker(opts.MaxTracked),
		flows:     make(map[flowKey]*flowAgg),
		hosts:     make(map[netip.Addr]*hostState),
		districts: map[string]protocol.District{protocol.OutlandsID: outlands()},
	}
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
	p.touch(r.Src, r.TS)
	p.touch(r.Dst, r.TS)
	p.mu.Unlock()
}

// touch records activity for a host; callers hold p.mu.
func (p *Pipeline) touch(a netip.Addr, ts time.Time) {
	if isGroup(a) {
		return
	}
	h := p.hosts[a]
	if h == nil {
		h = &hostState{addr: a, local: IsLocal(a), firstSeen: ts, dirty: true}
		if h.local {
			d := autoDistrict(a)
			h.district = d.ID
			if _, ok := p.districts[d.ID]; !ok {
				p.districts[d.ID] = d
				p.newDist = append(p.newDist, d)
			}
		} else {
			h.district = protocol.OutlandsID
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
			Src: k.src.String(), Dst: k.dst.String(), Proto: k.proto.String(),
			Category: Category(k.proto, nil), Packets: f.packets, Bytes: f.bytes,
			TCPFlags: FlagNames(f.flags),
		}
		if k.hasPort {
			port := k.port
			wf.ServicePort = protocol.Ptr(int(port))
			wf.Category = Category(k.proto, &port)
		}
		if f.side == SideDst {
			wf.ServerSide = protocol.Ptr(protocol.SideDst)
		}
		t.Flows = append(t.Flows, wf)
	}
	clear(p.flows)
	p.packets, p.bytes = 0, 0

	p.mu.Lock()
	t.Stats.HostsTotal = len(p.hosts)
	t.Districts, p.newDist = p.newDist, nil
	for _, h := range p.hosts {
		if h.dirty {
			t.Hosts = append(t.Hosts, h.wire())
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
	for _, d := range p.districts {
		h.Districts = append(h.Districts, d)
	}
	for _, hs := range p.hosts {
		h.Hosts = append(h.Hosts, hs.wire())
	}
	protocol.SortHello(h)
	return h
}
