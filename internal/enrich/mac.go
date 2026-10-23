// SPDX-License-Identifier: MIT

package enrich

import (
	"net"
	"net/netip"
)

// MACs attributes link-layer addresses to hosts (docs/DECISIONS.md D13).
// Every frame to or from an external host carries the router's MAC, so:
// a MAC seen with at least gatewayMinIPs distinct non-local IPs is a gateway
// MAC; only the local IP that sends from it most (the router itself) gets
// it; other local IPs seen behind it, and all non-local hosts, get none.
type MACs struct {
	gatewayMinIPs int
	maxHosts      int

	external map[string]map[netip.Addr]struct{} // MAC → non-local IPs (capped)
	gateway  map[string]bool
	srcCount map[netip.Addr]map[string]int // local IP → source MAC → frames
}

// NewMACs uses enrich.gateway_min_ips and bounds per-host state.
func NewMACs(gatewayMinIPs, maxHosts int) *MACs {
	return &MACs{
		gatewayMinIPs: gatewayMinIPs, maxHosts: maxHosts,
		external: make(map[string]map[netip.Addr]struct{}),
		gateway:  make(map[string]bool),
		srcCount: make(map[netip.Addr]map[string]int),
	}
}

// Observe records one frame. It returns the local IPs whose attribution may
// have changed (new pairing, or a MAC newly recognised as a gateway).
func (m *MACs) Observe(srcMAC net.HardwareAddr, src netip.Addr, srcLocal bool, dstMAC net.HardwareAddr, dst netip.Addr, dstLocal bool) []netip.Addr {
	var changed []netip.Addr
	for _, side := range []struct {
		mac   net.HardwareAddr
		ip    netip.Addr
		local bool
	}{{srcMAC, src, srcLocal}, {dstMAC, dst, dstLocal}} {
		if len(side.mac) == 0 || side.local {
			continue
		}
		key := side.mac.String()
		ips := m.external[key]
		if ips == nil {
			ips = make(map[netip.Addr]struct{})
			m.external[key] = ips
		}
		if len(ips) < m.gatewayMinIPs {
			ips[side.ip] = struct{}{}
			if len(ips) == m.gatewayMinIPs && !m.gateway[key] {
				m.gateway[key] = true
				changed = append(changed, m.localsUsing(key)...)
			}
		}
	}
	if srcLocal && len(srcMAC) > 0 {
		counts := m.srcCount[src]
		if counts == nil {
			if len(m.srcCount) >= m.maxHosts {
				return changed // bounded: new hosts beyond the cap get no MAC
			}
			counts = make(map[string]int)
			m.srcCount[src] = counts
		}
		key := srcMAC.String()
		if counts[key] == 0 {
			changed = append(changed, src)
		}
		counts[key]++
	}
	return changed
}

func (m *MACs) localsUsing(mac string) []netip.Addr {
	var out []netip.Addr
	for ip, counts := range m.srcCount {
		if counts[mac] > 0 {
			out = append(out, ip)
		}
	}
	return out
}

// For returns host's attributed MAC and vendor (vendor is "" when the OUI is
// unregistered or locally administered).
func (m *MACs) For(host netip.Addr) (mac, vendor string, ok bool) {
	counts := m.srcCount[host]
	best, bestN := "", 0
	for k, n := range counts {
		if n > bestN || (n == bestN && k < best) {
			best, bestN = k, n
		}
	}
	if best == "" {
		return "", "", false
	}
	if m.gateway[best] && m.gatewayIP(best) != host {
		return "", "", false // behind the router: the MAC is the router's
	}
	hw, err := net.ParseMAC(best)
	if err != nil {
		return "", "", false
	}
	vendor, _ = Lookup(hw)
	return best, vendor, true
}

// gatewayIP is the local IP sending most frames from a gateway MAC.
func (m *MACs) gatewayIP(mac string) netip.Addr {
	var best netip.Addr
	bestN := 0
	for ip, counts := range m.srcCount {
		if n := counts[mac]; n > bestN || (n == bestN && n > 0 && ip.Less(best)) {
			best, bestN = ip, n
		}
	}
	return best
}

// IsGateway reports whether host is the router behind a detected gateway MAC.
func (m *MACs) IsGateway(host netip.Addr) bool {
	mac, _, ok := m.For(host)
	return ok && m.gateway[mac]
}

// Forget drops a removed host's state.
func (m *MACs) Forget(host netip.Addr) { delete(m.srcCount, host) }
