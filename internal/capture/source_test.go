package capture

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixture(name string) string {
	return filepath.Join("..", "..", "testdata", "pcap", name+".pcap")
}

func countFrames(t *testing.T, s *File) (int, time.Time, time.Time) {
	t.Helper()
	var n int
	var first, last time.Time
	for {
		_, ci, err := s.Next()
		if errors.Is(err, io.EOF) {
			return n, first, last
		}
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			first = ci.Timestamp
		}
		last = ci.Timestamp
		n++
	}
}

func TestFileReplaysOnce(t *testing.T) {
	s, err := OpenFile(fixture("dns"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if n, _, _ := countFrames(t, s); n != 40 {
		t.Fatalf("frames = %d, want 40", n)
	}
}

func TestFileLoopKeepsTimeIncreasing(t *testing.T) {
	// Copy the fixture and remove read permission after opening: looping must
	// not depend on reopening the path (privilege drop, D22).
	path := filepath.Join(t.TempDir(), "dns.pcap")
	data, err := os.ReadFile(fixture("dns"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenFile(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}

	var prev time.Time
	for i := range 40 * 3 {
		_, ci, err := s.Next()
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if !prev.IsZero() && ci.Timestamp.Before(prev) {
			t.Fatalf("frame %d: time went backwards (%s < %s)", i, ci.Timestamp, prev)
		}
		prev = ci.Timestamp
	}
}

func TestFileBPFFilter(t *testing.T) {
	cases := []struct {
		expr string
		want int
	}{
		{"tcp", 0},
		{"udp port 53", 40},
		{"src host 10.0.1.10", 20},
	}
	for _, tc := range cases {
		s, err := OpenFile(fixture("dns"), false)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SetFilter(tc.expr); err != nil {
			t.Fatal(err)
		}
		if n, _, _ := countFrames(t, s); n != tc.want {
			t.Errorf("--bpf %q: %d frames, want %d", tc.expr, n, tc.want)
		}
		_ = s.Close()
	}

	s, err := OpenFile(fixture("dns"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.SetFilter("tcp and and"); err == nil {
		t.Fatal("invalid expression accepted")
	}
}

func TestFileRejectsNonEthernet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.pcap")
	// Classic pcap header with link type 101 (raw IP).
	hdr := []byte{0xd4, 0xc3, 0xb2, 0xa1, 2, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 101, 0, 0, 0}
	if err := os.WriteFile(path, hdr, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenFile(path, false); err == nil {
		t.Fatal("non-Ethernet capture accepted")
	}
}
