// SPDX-License-Identifier: MIT

// Package capture opens packet sources: live interfaces (libpcap) and capture files.
package capture

import (
	"errors"
	"fmt"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcap"
)

// Live is an open live capture.
type Live struct {
	Device string
	handle *pcap.Handle
}

// OpenFirstDevice opens the first interface libpcap reports. This is the
// prototype's behaviour; T2.1 replaces it with default-route selection.
func OpenFirstDevice(snaplen int32, promisc bool) (*Live, error) {
	devices, err := pcap.FindAllDevs()
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	if len(devices) == 0 {
		return nil, errors.New("no capture devices found")
	}
	return OpenLive(devices[0].Name, snaplen, promisc)
}

// OpenLive opens the named interface.
func OpenLive(device string, snaplen int32, promisc bool) (*Live, error) {
	handle, err := pcap.OpenLive(device, snaplen, promisc, 100*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", device, err)
	}
	return &Live{Device: device, handle: handle}, nil
}

// Next returns the next frame, skipping read timeouts.
func (l *Live) Next() ([]byte, gopacket.CaptureInfo, error) {
	for {
		data, ci, err := l.handle.ReadPacketData()
		if errors.Is(err, pcap.NextErrorTimeoutExpired) {
			continue
		}
		return data, ci, err
	}
}

// SetFilter installs a kernel BPF filter.
func (l *Live) SetFilter(expr string) error {
	if err := l.handle.SetBPFFilter(expr); err != nil {
		return fmt.Errorf("--bpf %q: %w", expr, err)
	}
	return nil
}

// Live is true: timestamps track the wall clock.
func (l *Live) Live() bool { return true }

// Stats returns cumulative kernel and interface drops.
func (l *Live) Stats() (uint64, uint64) {
	s, err := l.handle.Stats()
	if err != nil {
		return 0, 0
	}
	return uint64(max(0, s.PacketsDropped)), uint64(max(0, s.PacketsIfDropped)) //nolint:gosec // non-negative
}

// Close releases the capture handle.
func (l *Live) Close() error { l.handle.Close(); return nil }
