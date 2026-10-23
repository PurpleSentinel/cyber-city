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

type scanSource struct {
	tcp []probe
	udp []probe
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
		st = &scanSource{}
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
			st.tcp = appendBounded(st.tcp, probe{dst: p.Dst, port: p.DstPort, ts: p.TS})
		}
	case p.TCP && p.SYN && p.ACK:
		// Reply from the probed host: complete the matching probe.
		if st := s.bySrc[p.Dst]; st != nil {
			for i := len(st.tcp) - 1; i >= 0; i-- {
				if pr := &st.tcp[i]; pr.dst == p.Src && pr.port == p.SrcPort && !pr.complete {
					pr.complete = true
					break
				}
			}
		}
	case p.UDP && p.SrcPort >= 1024:
		// Replies from well-known service ports (DNS answers to many client
		// ports) are not probes.
		if st := s.source(p.Src); st != nil {
			st.udp = appendBounded(st.udp, probe{dst: p.Dst, port: p.DstPort, ts: p.TS})
		}
	}
}

func appendBounded(ps []probe, p probe) []probe {
	if len(ps) >= maxProbesPerSource {
		ps = ps[1:]
	}
	return append(ps, p)
}

func trim(ps []probe, since time.Time) []probe {
	i := 0
	for i < len(ps) && ps[i].ts.Before(since) {
		i++
	}
	return ps[i:]
}

// Window evaluates each source over the sliding window ending at end.
func (s *Scan) Window(end time.Time) []Candidate {
	var out []Candidate
	since := end.Add(-s.cfg.Window)
	for src, st := range s.bySrc {
		st.tcp = trim(st.tcp, since)
		st.udp = trim(st.udp, since)
		if len(st.tcp) == 0 && len(st.udp) == 0 {
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

func (s *Scan) tcpScan(src netip.Addr, probes []probe) (Candidate, bool) {
	if len(probes) == 0 {
		return Candidate{}, false
	}
	portsByDst := map[netip.Addr]map[uint16]bool{}
	dstsByPort := map[uint16]map[netip.Addr]bool{}
	incomplete := 0
	for _, p := range probes {
		if portsByDst[p.dst] == nil {
			portsByDst[p.dst] = map[uint16]bool{}
		}
		portsByDst[p.dst][p.port] = true
		if dstsByPort[p.port] == nil {
			dstsByPort[p.port] = map[netip.Addr]bool{}
		}
		dstsByPort[p.port][p.dst] = true
		if !p.complete {
			incomplete++
		}
	}
	var bestDst netip.Addr
	maxPorts := 0
	for d, ports := range portsByDst {
		if len(ports) > maxPorts || (len(ports) == maxPorts && d.Less(bestDst)) {
			bestDst, maxPorts = d, len(ports)
		}
	}
	var bestPort uint16
	maxHosts := 0
	for port, dsts := range dstsByPort {
		if len(dsts) > maxHosts || (len(dsts) == maxHosts && port < bestPort) {
			bestPort, maxHosts = port, len(dsts)
		}
	}
	ratio := float64(incomplete) / float64(len(probes))
	if (maxPorts < s.cfg.MinPorts && maxHosts < s.cfg.MinHosts) || ratio < s.cfg.MinIncompleteRatio {
		return Candidate{}, false
	}
	c := s.candidate(src, maxPorts, maxHosts, ratio)
	if maxPorts >= s.cfg.MinPorts {
		dst := bestDst
		c.Dst = &dst
		c.Targets = []netip.Addr{dst}
	} else {
		for d := range dstsByPort[bestPort] {
			c.Targets = append(c.Targets, d)
		}
	}
	return c, true
}

func (s *Scan) udpScan(src netip.Addr, probes []probe) (Candidate, bool) {
	portsByDst := map[netip.Addr]map[uint16]bool{}
	for _, p := range probes {
		if portsByDst[p.dst] == nil {
			portsByDst[p.dst] = map[uint16]bool{}
		}
		portsByDst[p.dst][p.port] = true
	}
	for d, ports := range portsByDst {
		if len(ports) >= s.cfg.UDPMinPorts {
			dst := d
			c := s.candidate(src, len(ports), 1, 1)
			c.Dst = &dst
			c.Targets = []netip.Addr{dst}
			return c, true
		}
	}
	return Candidate{}, false
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
