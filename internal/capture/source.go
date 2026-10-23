// SPDX-License-Identifier: MIT

package capture

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
	"github.com/gopacket/gopacket/pcapgo"
)

// Source yields raw frames. Next returns io.EOF when a file source is exhausted.
type Source interface {
	Next() (data []byte, ci gopacket.CaptureInfo, err error)
	// Live reports whether timestamps track the wall clock.
	Live() bool
	// Stats returns cumulative kernel and interface drops (0 for files).
	Stats() (kernelDrops, ifDrops uint64)
	// SetFilter applies a BPF capture filter (docs/CONFIG.md --bpf).
	SetFilter(expr string) error
	Close() error
}

// loopGap separates the end of one pass of a looped file from the start of the next.
const loopGap = time.Second

// File replays a pcap file, optionally looping with timestamps shifted so that
// time keeps increasing (docs/CONFIG.md --loop).
type File struct {
	path   string
	loop   bool
	f      *os.File
	r      *pcapgo.Reader
	offset time.Duration
	first  time.Time
	last   time.Time
	filter *pcap.BPF
	// restarted is set when the file rewinds; Restarted reports and clears it.
	restarted bool
}

// OpenFile opens a pcap capture file with an Ethernet link type.
func OpenFile(path string, loop bool) (*File, error) {
	s := &File{path: path, loop: loop}
	if err := s.open(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *File) open() error {
	f, err := os.Open(s.path)
	if err != nil {
		return fmt.Errorf("read %s: %w", s.path, err)
	}
	r, err := pcapgo.NewReader(f)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("read %s: %w", s.path, err)
	}
	if lt := r.LinkType(); lt != layers.LinkTypeEthernet {
		_ = f.Close()
		return fmt.Errorf("read %s: link type %s not supported (Ethernet only)", s.path, lt)
	}
	s.f, s.r = f, r
	return nil
}

func (s *File) rewind() error {
	if _, err := s.f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind %s: %w", s.path, err)
	}
	r, err := pcapgo.NewReader(s.f)
	if err != nil {
		return fmt.Errorf("rewind %s: %w", s.path, err)
	}
	s.r = r
	return nil
}

// Next returns the next frame, restarting the file at EOF when looping.
func (s *File) Next() ([]byte, gopacket.CaptureInfo, error) {
	for {
		data, ci, err := s.r.ReadPacketData()
		if errors.Is(err, io.EOF) && s.loop && !s.first.IsZero() {
			// Rewind the open file rather than reopening the path: after a
			// privilege drop the path may no longer be readable (D22).
			if err := s.rewind(); err != nil {
				return nil, ci, err
			}
			s.offset += s.last.Sub(s.first) + loopGap
			s.first = time.Time{}
			s.restarted = true
			continue
		}
		if err != nil {
			return nil, ci, err
		}
		if s.first.IsZero() {
			s.first = ci.Timestamp
		}
		s.last = ci.Timestamp
		if s.filter != nil && !s.filter.Matches(ci, data) {
			continue
		}
		ci.Timestamp = ci.Timestamp.Add(s.offset)
		return data, ci, nil
	}
}

// SetFilter compiles expr with libpcap and drops non-matching frames.
func (s *File) SetFilter(expr string) error {
	bpf, err := pcap.NewBPF(layers.LinkTypeEthernet, 65536, expr)
	if err != nil {
		return fmt.Errorf("--bpf %q: %w", expr, err)
	}
	s.filter = bpf
	return nil
}

// Restarted reports, once, that the file looped back to its start: the same
// connections and sequence numbers will replay.
func (s *File) Restarted() bool {
	r := s.restarted
	s.restarted = false
	return r
}

// Live is false: file timestamps are replayed, not observed.
func (s *File) Live() bool { return false }

// Stats is always zero for files.
func (s *File) Stats() (uint64, uint64) { return 0, 0 }

// Close closes the file.
func (s *File) Close() error { return s.f.Close() }
