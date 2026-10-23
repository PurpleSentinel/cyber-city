package pipeline

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/gopacket/gopacket"

	"github.com/PurpleSentinel/cyber-city/internal/capture"
	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// statsInterval is how often capture drop counters are read (T2.13).
const statsInterval = time.Second

// Sink receives completed ticks in order.
type Sink interface {
	Tick(t *protocol.Tick) error
}

type frame struct {
	data    []byte
	ci      gopacket.CaptureInfo
	restart bool // first frame after a looped file rewound
}

// restarter is implemented by sources that can loop (capture.File).
type restarter interface{ Restarted() bool }

// replayClock maps wall time to packet time for paced replay.
type replayClock struct {
	mu             sync.Mutex
	packetBase     time.Time
	wallBase       time.Time
	speed          float64
	started        bool
	now            func() time.Time
	sleepUntilWall func(ctx context.Context, wall time.Time) bool
}

// due returns the wall time at which a packet stamped ts should be released.
func (c *replayClock) due(ts time.Time) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		c.packetBase, c.wallBase, c.started = ts, c.now(), true
	}
	return c.wallBase.Add(time.Duration(float64(ts.Sub(c.packetBase)) / c.speed))
}

// packetNow is the packet time corresponding to the current wall time.
func (c *replayClock) packetNow() (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		return time.Time{}, false
	}
	return c.packetBase.Add(time.Duration(float64(c.now().Sub(c.wallBase)) * c.speed)), true
}

func sleepUntil(ctx context.Context, wall time.Time) bool {
	d := time.Until(wall)
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Run reads src until it is exhausted or ctx is cancelled, sending ticks to
// sink. speed paces file replay (0 = as fast as possible); live sources are
// never paced. A file source ends with a final flush and a nil error.
func Run(ctx context.Context, src capture.Source, p *Pipeline, sink Sink, speed float64) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var clock *replayClock
	if !src.Live() && speed > 0 {
		clock = &replayClock{speed: speed, now: time.Now, sleepUntilWall: sleepUntil}
	}

	frames := make(chan frame, 1024)
	readErr := make(chan error, 1)
	go func() {
		defer close(frames)
		for {
			data, ci, err := src.Next()
			if err != nil {
				readErr <- err
				return
			}
			if clock != nil && !clock.sleepUntilWall(ctx, clock.due(ci.Timestamp)) {
				return
			}
			restart := false
			if r, ok := src.(restarter); ok {
				restart = r.Restarted()
			}
			select {
			case frames <- frame{data, ci, restart}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wall-clock ticks advance windows when packets are sparse.
	var tickC <-chan time.Time
	if src.Live() || clock != nil {
		ticker := time.NewTicker(p.opts.Window)
		defer ticker.Stop()
		tickC = ticker.C
	}

	var lastStats time.Time

	emit := func(ticks []*protocol.Tick) error {
		for _, t := range ticks {
			if err := sink.Tick(t); err != nil {
				return err
			}
		}
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case f, ok := <-frames:
			if !ok {
				err := <-readErr
				if errors.Is(err, io.EOF) {
					return emit(p.Flush())
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return err
			}
			if f.restart {
				p.ResetConnections()
			}
			if err := emit(p.Frame(f.data, f.ci)); err != nil {
				return err
			}
		case <-tickC:
			now := time.Now()
			if now.Sub(lastStats) >= statsInterval {
				p.SetDrops(src.Stats())
				lastStats = now
			}
			if clock != nil {
				var ok bool
				if now, ok = clock.packetNow(); !ok {
					continue
				}
			}
			if err := emit(p.Advance(now)); err != nil {
				return err
			}
		}
	}
}
