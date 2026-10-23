// SPDX-License-Identifier: MIT

package pipeline

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"

	"github.com/PurpleSentinel/cyber-city/internal/capture"
)

// memSource replays frames held in memory, `passes` times, shifting
// timestamps like --loop so packet time keeps increasing.
type memSource struct {
	data      [][]byte
	ci        []gopacket.CaptureInfo
	span      time.Duration
	passes    int
	pass, i   int
	restarted bool
}

func loadFrames(tb testing.TB, name string) *memSource {
	tb.Helper()
	src, err := capture.OpenFile(filepath.Join("..", "..", "testdata", "pcap", name+".pcap"), false)
	if err != nil {
		tb.Fatal(err)
	}
	defer func() { _ = src.Close() }()
	m := &memSource{}
	for {
		data, ci, err := src.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			tb.Fatal(err)
		}
		m.data = append(m.data, append([]byte(nil), data...))
		m.ci = append(m.ci, ci)
	}
	m.span = m.ci[len(m.ci)-1].Timestamp.Sub(m.ci[0].Timestamp) + time.Second
	return m
}

func (m *memSource) Next() ([]byte, gopacket.CaptureInfo, error) {
	if m.i == len(m.data) {
		m.pass++
		m.i = 0
		m.restarted = true
	}
	if m.pass >= m.passes {
		return nil, gopacket.CaptureInfo{}, io.EOF
	}
	ci := m.ci[m.i]
	ci.Timestamp = ci.Timestamp.Add(time.Duration(m.pass) * m.span)
	d := m.data[m.i]
	m.i++
	return d, ci, nil
}

func (m *memSource) Restarted() bool {
	r := m.restarted
	m.restarted = false
	return r
}

func (m *memSource) Live() bool                  { return false }
func (m *memSource) Stats() (uint64, uint64)     { return 0, 0 }
func (m *memSource) SetFilter(string) error      { return nil }
func (m *memSource) Close() error                { return nil }
func (m *memSource) reset(passes int) *memSource { m.pass, m.i, m.passes = 0, 0, passes; return m }

// BenchmarkReplay measures the whole pipeline (decode, enrichment, detectors,
// windows and JSON encoding) replaying demo.pcap as fast as possible. The
// pkts/s metric is the T5.4 throughput figure (target >= 200k without -race).
func BenchmarkReplay(b *testing.B) {
	src := loadFrames(b, "demo")
	b.ReportAllocs()
	packets := 0
	for b.Loop() {
		p := New(testOptions())
		sink, err := NewJSONLSink(io.Discard, p.Snapshot())
		if err != nil {
			b.Fatal(err)
		}
		if err := Run(context.Background(), src.reset(3), p, sink, 0); err != nil {
			b.Fatal(err)
		}
		packets += 3 * len(src.data)
	}
	b.ReportMetric(float64(packets)/b.Elapsed().Seconds(), "pkts/s")
}
