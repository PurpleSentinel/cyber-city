package capture

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

// Source yields raw frames. Next returns io.EOF when a file source is exhausted.
type Source interface {
	Next() (data []byte, ci gopacket.CaptureInfo, err error)
	// Live reports whether timestamps track the wall clock.
	Live() bool
	// Stats returns cumulative kernel and interface drops (0 for files).
	Stats() (kernelDrops, ifDrops uint64)
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

// Next returns the next frame, restarting the file at EOF when looping.
func (s *File) Next() ([]byte, gopacket.CaptureInfo, error) {
	for {
		data, ci, err := s.r.ReadPacketData()
		if errors.Is(err, io.EOF) && s.loop && !s.first.IsZero() {
			_ = s.f.Close()
			if err := s.open(); err != nil {
				return nil, ci, err
			}
			s.offset += s.last.Sub(s.first) + loopGap
			s.first = time.Time{}
			continue
		}
		if err != nil {
			return nil, ci, err
		}
		if s.first.IsZero() {
			s.first = ci.Timestamp
		}
		s.last = ci.Timestamp
		ci.Timestamp = ci.Timestamp.Add(s.offset)
		return data, ci, nil
	}
}

// Live is false: file timestamps are replayed, not observed.
func (s *File) Live() bool { return false }

// Stats is always zero for files.
func (s *File) Stats() (uint64, uint64) { return 0, 0 }

// Close closes the file.
func (s *File) Close() error { return s.f.Close() }
