package detect

import (
	"math"
	"net/netip"
	"time"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// FloodConfig tunes the flood detector (docs/CONFIG.md detect.flood.*).
type FloodConfig struct {
	Window           time.Duration
	BaselineHalfLife time.Duration
	MinHistory       time.Duration
	Multiplier       float64
	MinPPS           int
	MinSources       int
}

const (
	maxFloodTargets   = 50_000
	maxBucketSources  = 4096
	floodIdleHalfLife = 5 // forget a quiet target after this many half-lives
)

// bucket counts one second of packets towards a target.
type bucket struct {
	sec  int64
	pkts int
	srcs map[netip.Addr]struct{}
}

type floodTarget struct {
	first    time.Time
	lastSeen time.Time
	buckets  []bucket // ascending sec
	folded   int64    // last second folded into the baseline
	baseline float64  // EWMA of packets/s
	hasBase  bool
	active   bool
}

// Flood flags a destination whose packet rate jumps far above its own
// baseline, from many sources (docs/CONFIG.md "Flood").
type Flood struct {
	cfg   FloodConfig
	alpha float64 // per-second EWMA weight
	byDst map[netip.Addr]*floodTarget
}

// NewFlood returns a flood detector.
func NewFlood(cfg FloodConfig) *Flood {
	alpha := 1.0
	if cfg.BaselineHalfLife > 0 {
		alpha = 1 - math.Exp2(-float64(time.Second)/float64(cfg.BaselineHalfLife))
	}
	return &Flood{cfg: cfg, alpha: alpha, byDst: make(map[netip.Addr]*floodTarget)}
}

// Packet counts the packet against its destination's current second.
func (f *Flood) Packet(p *Packet) {
	t := f.byDst[p.Dst]
	if t == nil {
		if len(f.byDst) >= maxFloodTargets {
			return
		}
		t = &floodTarget{first: p.TS, folded: p.TS.Unix() - 1}
		f.byDst[p.Dst] = t
	}
	t.lastSeen = p.TS
	sec := p.TS.Unix()
	if n := len(t.buckets); n == 0 || t.buckets[n-1].sec < sec {
		t.buckets = append(t.buckets, bucket{sec: sec, srcs: make(map[netip.Addr]struct{})})
	}
	b := &t.buckets[len(t.buckets)-1]
	b.pkts++
	if len(b.srcs) < maxBucketSources {
		b.srcs[p.Src] = struct{}{}
	}
}

// fold feeds every completed second before now into the baseline; quiet
// seconds count as zero. The baseline is frozen while a flood is active.
func (f *Flood) fold(t *floodTarget, now int64) {
	i := 0
	for t.folded+1 < now {
		sec := t.folded + 1
		for i < len(t.buckets) && t.buckets[i].sec < sec {
			i++
		}
		if i < len(t.buckets) && t.buckets[i].sec == sec {
			sample := float64(t.buckets[i].pkts)
			switch {
			case t.active, t.hasBase && f.suspect(sample, t.baseline):
				// A flood's ramp-up must not raise the bar it is measured against.
			case !t.hasBase:
				t.baseline, t.hasBase = sample, true
			default:
				t.baseline += f.alpha * (sample - t.baseline)
			}
			t.folded = sec
			continue
		}
		// Run of empty seconds up to the next bucket (or now).
		next := now
		if i < len(t.buckets) && t.buckets[i].sec < now {
			next = t.buckets[i].sec
		}
		if t.hasBase && !t.active {
			t.baseline *= math.Pow(1-f.alpha, float64(next-sec))
		}
		t.folded = next - 1
	}
}

// suspect reports whether a rate is above the flood threshold for baseline.
func (f *Flood) suspect(pps, baseline float64) bool {
	return pps >= math.Max(f.cfg.Multiplier*baseline, float64(f.cfg.MinPPS))
}

// Window evaluates each destination over the window ending at end.
func (f *Flood) Window(end time.Time) []Candidate {
	var out []Candidate
	now := end.Unix()
	win := int64(math.Ceil(f.cfg.Window.Seconds()))
	for dst, t := range f.byDst {
		f.fold(t, now)
		i := 0
		for i < len(t.buckets) && t.buckets[i].sec <= now-win {
			i++
		}
		t.buckets = t.buckets[i:]
		if len(t.buckets) == 0 && end.Sub(t.lastSeen) > floodIdleHalfLife*f.cfg.BaselineHalfLife {
			delete(f.byDst, dst)
			continue
		}
		pkts := 0
		srcs := map[netip.Addr]struct{}{}
		for _, b := range t.buckets {
			pkts += b.pkts
			for s := range b.srcs {
				srcs[s] = struct{}{}
			}
		}
		pps := float64(pkts) / f.cfg.Window.Seconds()
		t.active = t.hasBase && end.Sub(t.first) >= f.cfg.MinHistory &&
			f.suspect(pps, t.baseline) &&
			len(srcs) >= f.cfg.MinSources
		if !t.active {
			continue
		}
		target := dst
		c := Candidate{
			Kind:     protocol.EventFlood,
			Key:      dst.String(),
			Dst:      &target,
			Targets:  []netip.Addr{target},
			Severity: protocol.SeverityAlert,
			Duration: 8 * time.Second,
			Evidence: map[string]float64{
				"pps":              math.Round(pps),
				"baseline_pps":     math.Round(t.baseline*10) / 10,
				"distinct_sources": float64(len(srcs)),
				"window_s":         f.cfg.Window.Seconds(),
			},
		}
		if len(srcs) == 1 {
			for s := range srcs {
				c.Src = &s
			}
		}
		out = append(out, c)
	}
	return out
}
