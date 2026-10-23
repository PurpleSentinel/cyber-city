// Package capture opens packet sources: live interfaces (libpcap) and, from
// T0.8, capture files.
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
	device := devices[0].Name
	handle, err := pcap.OpenLive(device, snaplen, promisc, 100*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", device, err)
	}
	return &Live{Device: device, handle: handle}, nil
}

// Packets returns the decoded packet stream; it closes when the handle closes.
func (l *Live) Packets() <-chan gopacket.Packet {
	return gopacket.NewPacketSource(l.handle, l.handle.LinkType()).Packets()
}

// Close releases the capture handle.
func (l *Live) Close() { l.handle.Close() }
