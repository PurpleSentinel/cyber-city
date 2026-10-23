// SPDX-License-Identifier: MIT

package pipeline

import (
	"context"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"

	"github.com/PurpleSentinel/cyber-city/internal/capture"
	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// fakeLive is a live source that replays a few frames with current
// timestamps, then idles, reporting fixed drop counters.
type fakeLive struct {
	frames    [][]byte
	statCalls atomic.Int32
	done      chan struct{}
}

func (f *fakeLive) Next() ([]byte, gopacket.CaptureInfo, error) {
	if len(f.frames) > 0 {
		data := f.frames[0]
		f.frames = f.frames[1:]
		return data, gopacket.CaptureInfo{Timestamp: time.Now(), CaptureLength: len(data), Length: len(data)}, nil
	}
	<-f.done
	return nil, gopacket.CaptureInfo{}, io.EOF
}
func (f *fakeLive) Live() bool              { return true }
func (f *fakeLive) SetFilter(string) error  { return nil }
func (f *fakeLive) Close() error            { return nil }
func (f *fakeLive) Stats() (uint64, uint64) { f.statCalls.Add(1); return 5, 2 }

// rawFrames returns the undecoded frames of a fixture.
func rawFrames(t *testing.T, name string) [][]byte {
	t.Helper()
	src, err := capture.OpenFile(filepath.Join("..", "..", "testdata", "pcap", name+".pcap"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Close() }()
	var out [][]byte
	for {
		data, _, err := src.Next()
		if err != nil {
			return out
		}
		out = append(out, append([]byte(nil), data...))
	}
}

type collectSink struct {
	mu    sync.Mutex
	ticks []*protocol.Tick
}

func (c *collectSink) Tick(t *protocol.Tick) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ticks = append(c.ticks, t)
	return nil
}

func TestLiveRunReportsDropStats(t *testing.T) {
	src := &fakeLive{done: make(chan struct{}), frames: rawFrames(t, "dns")[:4]}
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	sink := &collectSink{}
	err := Run(ctx, src, New(testOptions()), sink, 1)
	close(src.done)
	if err != context.DeadlineExceeded {
		t.Fatalf("Run returned %v", err)
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	var withDrops bool
	for _, tk := range sink.ticks {
		if tk.Stats.KernelDrops == 5 && tk.Stats.IfDrops == 2 {
			withDrops = true
		}
	}
	if !withDrops {
		t.Fatalf("no tick carried kernel_drops=5 if_drops=2 (%d ticks)", len(sink.ticks))
	}
	// Read about once per second, not on every 100 ms window.
	if n := src.statCalls.Load(); n < 2 || n > 4 {
		t.Fatalf("Stats called %d times in 2.5 s, want ~3", n)
	}
}
