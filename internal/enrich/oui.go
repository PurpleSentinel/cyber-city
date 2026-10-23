// SPDX-License-Identifier: MIT

// Package enrich derives per-host and per-flow facts from passive observation.
package enrich

import (
	"net"
	"sort"
	"strings"
)

//go:generate go run ../../tools/ouigen -out oui_table.go

// Lookup returns the IEEE-registered vendor for mac's OUI. Locally administered
// and group (multicast) addresses have no vendor.
func Lookup(mac net.HardwareAddr) (string, bool) {
	if len(mac) < 3 || mac[0]&0x03 != 0 {
		return "", false
	}
	prefix := uint32(mac[0])<<16 | uint32(mac[1])<<8 | uint32(mac[2])
	i := sort.Search(len(ouiPrefixes), func(i int) bool { return ouiPrefixes[i] >= prefix })
	if i == len(ouiPrefixes) || ouiPrefixes[i] != prefix {
		return "", false
	}
	return ouiVendorNames[ouiVendorIndex[i]], true
}

// FindPrefix returns the lowest registered OUI whose vendor name contains
// substr (case-sensitive). Used by tools/fixturegen to pick realistic MACs.
func FindPrefix(substr string) (net.HardwareAddr, bool) {
	for i, p := range ouiPrefixes {
		if strings.Contains(ouiVendorNames[ouiVendorIndex[i]], substr) {
			return net.HardwareAddr{byte(p >> 16 & 0xff), byte(p >> 8 & 0xff), byte(p & 0xff)}, true //nolint:gosec // masked to 8 bits
		}
	}
	return nil, false
}
