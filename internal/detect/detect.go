// Package detect turns packet metadata into heuristic events: port scans,
// floods and hosts going dark (docs/CONFIG.md "Event detectors"). These are
// visualisation heuristics, not an IDS (DECISIONS D14).
package detect

import (
	"cmp"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// Packet is the metadata a detector sees for one packet.
type Packet struct {
	TS               time.Time
	Src, Dst         netip.Addr
	TCP, UDP         bool
	SrcPort, DstPort uint16
	SYN, ACK, RST    bool
}

// Candidate is an event before id assignment and cooldown filtering.
type Candidate struct {
	Kind     string
	Key      string // cooldown key, e.g. the scanner or flood target
	Src, Dst *netip.Addr
	Targets  []netip.Addr
	Evidence map[string]float64
	Severity string
	Duration time.Duration
}

// Detector consumes packets and reports candidates when a window closes.
type Detector interface {
	Packet(p *Packet)
	Window(end time.Time) []Candidate
}

// Manager runs detectors, applies per-kind cooldowns and numbers events.
type Manager struct {
	detectors []Detector
	cooldowns map[string]time.Duration // per kind
	last      map[string]time.Time     // kind|key → last emission
	nextID    int
}

// NewManager runs detectors with the given per-kind cooldowns.
func NewManager(cooldowns map[string]time.Duration, detectors ...Detector) *Manager {
	return &Manager{detectors: detectors, cooldowns: cooldowns, last: make(map[string]time.Time)}
}

// Packet feeds every detector.
func (m *Manager) Packet(p *Packet) {
	for _, d := range m.detectors {
		d.Packet(p)
	}
}

// Window closes a window at end and returns its events. id maps addresses to
// wire host ids (pseudonyms under --anonymize).
func (m *Manager) Window(end time.Time, id func(netip.Addr) string) []protocol.Event {
	var out []protocol.Event
	for _, d := range m.detectors {
		for _, c := range d.Window(end) {
			if ev, ok := m.admit(end, c, id); ok {
				out = append(out, ev)
			}
		}
	}
	return out
}

// Emit admits a candidate produced outside a detector (host idle).
func (m *Manager) Emit(at time.Time, c Candidate, id func(netip.Addr) string) (protocol.Event, bool) {
	return m.admit(at, c, id)
}

func (m *Manager) admit(at time.Time, c Candidate, id func(netip.Addr) string) (protocol.Event, bool) {
	k := c.Kind + "|" + c.Key
	if last, ok := m.last[k]; ok && at.Sub(last) < m.cooldowns[c.Kind] {
		return protocol.Event{}, false
	}
	m.last[k] = at
	m.nextID++
	ev := protocol.Event{
		ID:         fmt.Sprintf("e-%d", m.nextID),
		Kind:       c.Kind,
		TS:         at.UnixMilli(),
		Severity:   c.Severity,
		Evidence:   c.Evidence,
		DurationMS: int(c.Duration / time.Millisecond),
	}
	if c.Src != nil {
		ev.Src = protocol.Ptr(id(*c.Src))
	}
	if c.Dst != nil {
		ev.Dst = protocol.Ptr(id(*c.Dst))
	}
	targets := slices.Clone(c.Targets)
	slices.SortFunc(targets, func(a, b netip.Addr) int { return cmp.Compare(id(a), id(b)) })
	if len(targets) > protocol.MaxTargets {
		targets = targets[:protocol.MaxTargets]
	}
	for _, t := range targets {
		ev.Targets = append(ev.Targets, id(t))
	}
	return ev, true
}

// HostIdle is the event for a local host that went silent (docs/CONFIG.md
// "Host idle"); its removal is reported in the same tick.
func HostIdle(host netip.Addr, idle time.Duration) Candidate {
	return Candidate{
		Kind:     protocol.EventHostIdle,
		Key:      host.String(),
		Src:      &host,
		Evidence: map[string]float64{"idle_s": idle.Seconds()},
		Severity: protocol.SeverityInfo,
		Duration: 5 * time.Second,
	}
}
