// SPDX-License-Identifier: MIT

package protocol

import (
	"cmp"
	"slices"
)

// tcpFlagOrder is the canonical order of Flow.TCPFlags (docs/PROTOCOL.md "Flow").
var tcpFlagOrder = []string{"SYN", "ACK", "FIN", "RST", "PSH", "URG"}

// SortHello applies the ordering rules (docs/PROTOCOL.md "Ordering", DECISIONS D16).
func SortHello(h *Hello) {
	sortDistricts(h.Districts)
	sortHosts(h.Hosts)
}

// SortTick applies the ordering rules (docs/PROTOCOL.md "Ordering", DECISIONS D16).
func SortTick(t *Tick) {
	sortDistricts(t.Districts)
	for i := range t.Flows {
		t.Flows[i].TCPFlags = CanonicalFlags(t.Flows[i].TCPFlags)
	}
	slices.SortFunc(t.Flows, compareFlows)
	sortHosts(t.Hosts)
	slices.Sort(t.RemovedHosts)
	for i := range t.Events {
		slices.Sort(t.Events[i].Targets)
	}
	slices.SortFunc(t.Events, func(a, b Event) int {
		return cmp.Or(cmp.Compare(a.TS, b.TS), cmp.Compare(a.ID, b.ID))
	})
}

// CanonicalFlags de-duplicates flags and orders them SYN, ACK, FIN, RST, PSH, URG,
// dropping unknown names.
func CanonicalFlags(flags []string) []string {
	out := make([]string, 0, len(flags))
	for _, f := range tcpFlagOrder {
		if slices.Contains(flags, f) {
			out = append(out, f)
		}
	}
	return out
}

// Flows: bytes descending, then src, dst, service_port (null first).
func compareFlows(a, b Flow) int {
	return cmp.Or(
		cmp.Compare(b.Bytes, a.Bytes),
		cmp.Compare(a.Src, b.Src),
		cmp.Compare(a.Dst, b.Dst),
		cmp.Compare(portKey(a.ServicePort), portKey(b.ServicePort)),
		cmp.Compare(a.Proto, b.Proto),
	)
}

func portKey(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}

func sortDistricts(ds []District) {
	slices.SortFunc(ds, func(a, b District) int { return cmp.Compare(a.ID, b.ID) })
}

func sortHosts(hs []Host) {
	slices.SortFunc(hs, func(a, b Host) int { return cmp.Compare(a.ID, b.ID) })
	for i := range hs {
		slices.SortFunc(hs[i].Services, func(a, b Service) int {
			return cmp.Or(cmp.Compare(a.Port, b.Port), cmp.Compare(a.Proto, b.Proto))
		})
	}
}
