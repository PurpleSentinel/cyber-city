// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

// expectedSpan bounds each fixture's first-to-last packet time.
var expectedSpan = map[string][2]time.Duration{
	"web_https":  {3 * time.Second, 5 * time.Second},
	"dns":        {9 * time.Second, 10 * time.Second},
	"ssh":        {59 * time.Second, 61 * time.Second},
	"retrans":    {3 * time.Second, 5 * time.Second},
	"scan_syn":   {7 * time.Second, 9 * time.Second},
	"scan_sweep": {4 * time.Second, 6 * time.Second},
	"flood":      {54 * time.Second, 56 * time.Second},
	"idle":       {139 * time.Second, 142 * time.Second},
	"ipv6":       {0, time.Second},
	"vlan":       {4 * time.Second, 6 * time.Second},
	"gateway":    {4 * time.Second, 6 * time.Second},
	"demo":       {110 * time.Second, 121 * time.Second},
	"many_hosts": {15 * time.Second, 17 * time.Second},
}

func hashDir(t *testing.T, dir string) map[string][32]byte {
	t.Helper()
	sums := map[string][32]byte{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		sums[rel] = sha256.Sum256(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sums
}

func TestDeterministicAndCommitted(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	if _, err := generate(a); err != nil {
		t.Fatal(err)
	}
	if _, err := generate(b); err != nil {
		t.Fatal(err)
	}
	sumA, sumB := hashDir(t, a), hashDir(t, b)
	if len(sumA) != len(fixtures)+1 {
		t.Fatalf("generated %d files, want %d", len(sumA), len(fixtures)+1)
	}
	for name, s := range sumA {
		if sumB[name] != s {
			t.Errorf("%s differs between runs", name)
		}
		committed, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
		if err != nil {
			t.Errorf("%s not committed: %v (run make fixtures)", name, err)
			continue
		}
		if sha256.Sum256(committed) != s {
			t.Errorf("testdata/%s is stale; run make fixtures", name)
		}
	}
}

func TestReadBack(t *testing.T) {
	dir := t.TempDir()
	results, err := generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	var total int
	for _, r := range results {
		total += r.size
		t.Run(r.name, func(t *testing.T) {
			n, first, last := readPcap(t, filepath.Join(dir, "pcap", r.name+".pcap"))
			if n != r.packets {
				t.Errorf("read %d packets, generated %d", n, r.packets)
			}
			span := last.Sub(first)
			if want := expectedSpan[r.name]; span < want[0] || span > want[1] {
				t.Errorf("span %s outside %s..%s", span, want[0], want[1])
			}
			if !first.Equal(baseTime) && first.Before(baseTime) {
				t.Errorf("first packet %s before base time", first)
			}
		})
	}
	if total > 10<<20 {
		t.Errorf("fixtures total %d bytes, limit 10 MB", total)
	}
	for _, r := range results {
		if r.name == "demo" && r.size > 5<<20 {
			t.Errorf("demo.pcap is %d bytes, limit 5 MB", r.size)
		}
	}
}

// readPcap decodes every packet and fails on decode errors or bad lengths.
func readPcap(t *testing.T, path string) (int, time.Time, time.Time) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	r, err := pcapgo.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	var first, last time.Time
	for {
		data, ci, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if ci.Length < ci.CaptureLength {
			t.Fatalf("packet %d: wire length %d < captured %d", n, ci.Length, ci.CaptureLength)
		}
		p := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.Default)
		if el := p.ErrorLayer(); el != nil {
			t.Fatalf("packet %d: decode error: %v", n, el.Error())
		}
		if n == 0 {
			first = ci.Timestamp
		}
		last = ci.Timestamp
		n++
	}
	return n, first, last
}

func TestWebHTTPSHandshakeTiming(t *testing.T) {
	v, err := loadVendors()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, _, err := webHTTPS(v).write(&buf); err != nil {
		t.Fatal(err)
	}
	r, err := pcapgo.NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var synAt time.Time
	for {
		data, ci, err := r.ReadPacketData()
		if err != nil {
			t.Fatal("no handshake ACK found")
		}
		tcp, _ := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.Default).Layer(layers.LayerTypeTCP).(*layers.TCP)
		if tcp == nil {
			continue
		}
		switch {
		case tcp.SYN && !tcp.ACK:
			synAt = ci.Timestamp
		case !tcp.SYN && tcp.ACK && !synAt.IsZero():
			if rtt := ci.Timestamp.Sub(synAt); rtt != 12*time.Millisecond {
				t.Fatalf("SYN→ACK = %s, want 12ms", rtt)
			}
			return
		}
	}
}

func TestVLANTags(t *testing.T) {
	v, err := loadVendors()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, _, err := vlan(v).write(&buf); err != nil {
		t.Fatal(err)
	}
	r, err := pcapgo.NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[uint16]bool{}
	for {
		data, _, err := r.ReadPacketData()
		if err != nil {
			break
		}
		if q, ok := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.Default).Layer(layers.LayerTypeDot1Q).(*layers.Dot1Q); ok {
			seen[q.VLANIdentifier] = true
		} else {
			t.Fatal("untagged frame in vlan fixture")
		}
	}
	if !seen[10] || !seen[20] {
		t.Fatalf("VLANs seen %v, want 10 and 20", seen)
	}
}
