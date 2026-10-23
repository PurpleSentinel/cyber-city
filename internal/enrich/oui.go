// Package enrich derives per-host and per-flow facts from passive observation.
package enrich

import (
	"net"
	"sort"
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
