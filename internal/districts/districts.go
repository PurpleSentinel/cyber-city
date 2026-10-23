// SPDX-License-Identifier: MIT

// Package districts assigns local hosts to configured districts
// (docs/CONFIG.md "Districts file").
package districts

import (
	"net/netip"
	"slices"

	"github.com/PurpleSentinel/cyber-city/internal/config"
	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// Matcher maps a local address and VLAN to a configured district.
type Matcher struct {
	defs []config.DistrictDef
}

// New builds a matcher from a validated districts file (config.LoadDistricts).
func New(defs []config.DistrictDef) *Matcher { return &Matcher{defs: defs} }

// Match returns the first district whose VLANs include vlan, else the first
// whose CIDRs contain addr (docs/CONFIG.md: VLAN first, then CIDR).
func (m *Matcher) Match(addr netip.Addr, vlan uint16) (string, bool) {
	if m == nil {
		return "", false
	}
	if vlan != 0 {
		for _, d := range m.defs {
			if slices.Contains(d.VLANs, int(vlan)) {
				return d.ID, true
			}
		}
	}
	for _, d := range m.defs {
		for _, p := range d.Prefixes {
			if p.Contains(addr) {
				return d.ID, true
			}
		}
	}
	return "", false
}

// Districts returns the configured districts in protocol form. With
// anonymize, CIDRs are withheld (docs/PROTOCOL.md "hello").
func (m *Matcher) Districts(anonymize bool) []protocol.District {
	if m == nil {
		return nil
	}
	out := make([]protocol.District, 0, len(m.defs))
	for _, d := range m.defs {
		cidrs := []string{}
		if !anonymize {
			for _, p := range d.Prefixes {
				cidrs = append(cidrs, p.String())
			}
		}
		out = append(out, protocol.District{ID: d.ID, Name: d.Name, Kind: d.Kind, CIDRs: cidrs})
	}
	return out
}
