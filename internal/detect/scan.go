// SPDX-License-Identifier: MIT

package detect

import (
	"math"
	"net/netip"
	"time"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// ScanConfig tunes the port-scan detector (docs/CONFIG.md detect.scan.*).
type ScanConfig struct {
	Window             time.Duration
	MinPorts           int
	MinHosts           int
	MinIncompleteRatio float64
	UDPMinPorts        int
}

const (
	maxProbesPerSource = 20_000
	maxScanSources     = 50_000
)

type probe struct {
	dst      netip.Addr
	port     uint16
	ts       time.Time
	complete bool // the target answered with SYN-ACK
}

// probeSet is one source's probes in time order, with distinct-port and
// distinct-host counts kept incrementally as probes arrive and expire.
type probeSet struct {
	probes     []probe
	ports      map[netip.Addr]map[uint16]int // dst → port → probes
	hosts      map[uint16]map[netip.Addr]int // port → dst → probes (TCP only)
	incomplete int
}

func newProbeSet(byPort bool) *probeSet {
	s := &probeSet{ports: map[netip.Addr]map[uint16]int{}}
	if byPort {
		s.hosts = map[uint16]map[netip.Addr]int{}
	}
	return s
}

func (s *probeSet) add(p probe) {
	if len(s.probes) >= maxProbesPerSource {
		s.dropOldest()
	}
	s.probes = append(s.probes, p)
	s.incomplete++
	if s.ports[p.dst] == nil {
		s.ports[p.dst] = map[uint16]int{}
	}
	s.ports[p.dst][p.port]++
	if s.hosts != nil {
		if s.hosts[p.port] == nil {
			s.hosts[p.port] = map[netip.Addr]int{}
		}
		s.hosts[p.port][p.dst]++
	}
}

func (s *probeSet) dropOldest() {
	p := s.probes[0]
	s.probes = s.probes[1:]
	if !p.complete {
		s.incomplete--
	}
	if m := s.ports[p.dst]; m != nil {
		if m[p.port]--; m[p.port] == 0 {
			delete(m, p.port)
			if len(m) == 0 {
				delete(s.ports, p.dst)
			}
		}
	}
	if s.hosts != nil {
		if m := s.hosts[p.port]; m != nil {
			if m[p.dst]--; m[p.dst] == 0 {
				delete(m, p.dst)
				if len(m) == 0 {
					delete(s.hosts, p.port)
				}
			}
		}
	}
}

func (s *probeSet) expire(since time.Time) {
	for len(s.probes) > 0 && s.probes[0].ts.Before(since) {
		s.dropOldest()
	}
}

// complete marks the newest open probe to dst:port as answered.
func (s *probeSet) complete(dst netip.Addr, port uint16) {
	for i := len(s.probes) - 1; i >= 0; i-- {
		if pr := &s.probes[i]; pr.dst == dst && pr.port == port && !pr.complete {
			pr.complete = true
			s.incomplete--
			return
		}
	}
}

// widestDst is the destination probed on the most ports (lowest address on ties).
func (s *probeSet) widestDst() (netip.Addr, int) {
	var best netip.Addr
	n := 0
	for d, ports := range s.ports {
		if len(ports) > n || (len(ports) == n && d.Less(best)) {
			best, n = d, len(ports)
		}
	}
	return best, n
}

// widestPort is the port probed on the most destinations (lowest port on ties).
func (s *probeSet) widestPort() (uint16, int) {
	var best uint16
	n := 0
	for port, dsts := range s.hosts {
		if len(dsts) > n || (len(dsts) == n && port < best) {
			best, n = port, len(dsts)
		}
	}
	return best, n
}

type scanSource struct {
	tcp, udp *probeSet
}

// Scan flags sources probing many ports on one host, or one port on many
// hosts, with mostly incomplete handshakes (docs/CONFIG.md "Port scan").
type Scan struct {
	cfg   ScanConfig
	bySrc map[netip.Addr]*scanSource
}

// NewScan returns a port-scan detector.
func NewScan(cfg ScanConfig) *Scan {
	return &Scan{cfg: cfg, bySrc: make(map[netip.Addr]*scanSource)}
}

func (s *Scan) source(a netip.Addr) *scanSource {
	st := s.bySrc[a]
	if st == nil {
		if len(s.bySrc) >= maxScanSources {
			return nil
		}
		st = &scanSource{tcp: newProbeSet(true), udp: newProbeSet(false)}
		s.bySrc[a] = st
	}
	return st
}

// Packet records SYN probes, completes them on SYN-ACK, and records UDP probes
// sent from unprivileged ports.
func (s *Scan) Packet(p *Packet) {
	switch {
	case p.TCP && p.SYN && !p.ACK:
		if st := s.source(p.Src); st != nil {
			st.tcp.add(probe{dst: p.Dst, port: p.DstPort, ts: p.TS})
		}
	case p.TCP && p.SYN && p.ACK:
		// Reply from the probed host: complete the matching probe.
		if st := s.bySrc[p.Dst]; st != nil {
			st.tcp.complete(p.Src, p.SrcPort)
		}
	case p.UDP && p.SrcPort >= 1024:
		// Replies from well-known service ports (DNS answers to many client
		// ports) are not probes.
		if st := s.source(p.Src); st != nil {
			st.udp.add(probe{dst: p.Dst, port: p.DstPort, ts: p.TS})
		}
	}
}

// Window evaluates each source over the sliding window ending at end.
func (s *Scan) Window(end time.Time) []Candidate {
	var out []Candidate
	since := end.Add(-s.cfg.Window)
	for src, st := range s.bySrc {
		st.tcp.expire(since)
		st.udp.expire(since)
		if len(st.tcp.probes) == 0 && len(st.udp.probes) == 0 {
			delete(s.bySrc, src)
			continue
		}
		if c, ok := s.tcpScan(src, st.tcp); ok {
			out = append(out, c)
		} else if c, ok := s.udpScan(src, st.udp); ok {
			out = append(out, c)
		}
	}
	return out
}

func (s *Scan) tcpScan(src netip.Addr, ps *probeSet) (Candidate, bool) {
	// Distinct ports or hosts never exceed the probe count: most sources stop here.
	if n := len(ps.probes); n == 0 || (n < s.cfg.MinPorts && n < s.cfg.MinHosts) {
		return Candidate{}, false
	}
	ratio := float64(ps.incomplete) / float64(len(ps.probes))
	if ratio < s.cfg.MinIncompleteRatio {
		return Candidate{}, false
	}
	bestDst, maxPorts := ps.widestDst()
	bestPort, maxHosts := ps.widestPort()
	if maxPorts < s.cfg.MinPorts && maxHosts < s.cfg.MinHosts {
		return Candidate{}, false
	}
	c := s.candidate(src, maxPorts, maxHosts, ratio)
	if maxPorts >= s.cfg.MinPorts {
		dst := bestDst
		c.Dst = &dst
		c.Targets = []netip.Addr{dst}
	} else {
		for d := range ps.hosts[bestPort] {
			c.Targets = append(c.Targets, d)
		}
	}
	return c, true
}

func (s *Scan) udpScan(src netip.Addr, ps *probeSet) (Candidate, bool) {
	if len(ps.probes) < s.cfg.UDPMinPorts {
		return Candidate{}, false
	}
	dst, ports := ps.widestDst()
	if ports < s.cfg.UDPMinPorts {
		return Candidate{}, false
	}
	c := s.candidate(src, ports, 1, 1)
	c.Dst = &dst
	c.Targets = []netip.Addr{dst}
	return c, true
}

func (s *Scan) candidate(src netip.Addr, ports, hosts int, ratio float64) Candidate {
	return Candidate{
		Kind:     protocol.EventPortScan,
		Key:      src.String(),
		Src:      &src,
		Severity: protocol.SeverityAlert,
		Duration: 6 * time.Second,
		Evidence: map[string]float64{
			"distinct_ports":   float64(ports),
			"distinct_hosts":   float64(hosts),
			"incomplete_ratio": math.Round(ratio*100) / 100,
			"window_s":         s.cfg.Window.Seconds(),
		},
	}
}
