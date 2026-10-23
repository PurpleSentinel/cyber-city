package enrich

import (
	"cmp"
	"net/netip"
	"slices"
	"time"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// Services tracks the ports each host serves (docs/PROTOCOL.md "Host",
// services). A service is confirmed once the host is seen answering on it.
type Services struct {
	max    int
	byHost map[netip.Addr]map[serviceKey]*serviceState
}

type serviceKey struct {
	port  uint16
	proto string
}

type serviceState struct {
	category  string
	confirmed bool
	lastSeen  time.Time
}

// NewServices keeps at most limit services per host (hosts.max_services).
func NewServices(limit int) *Services {
	return &Services{max: limit, byHost: make(map[netip.Addr]map[serviceKey]*serviceState)}
}

// Observe records that server serves port/proto, seen at ts. confirmed means
// the server itself answered (TCP SYN-ACK, or UDP from that port). It reports
// whether the host's service list changed.
func (s *Services) Observe(server netip.Addr, proto string, port uint16, category string, confirmed bool, ts time.Time) bool {
	m := s.byHost[server]
	if m == nil {
		m = make(map[serviceKey]*serviceState)
		s.byHost[server] = m
	}
	k := serviceKey{port, proto}
	st := m[k]
	changed := false
	if st == nil {
		if len(m) >= s.max {
			evictOldestService(m)
		}
		st = &serviceState{category: category}
		m[k] = st
		changed = true
	}
	if confirmed && !st.confirmed {
		st.confirmed, changed = true, true
	}
	if ts.After(st.lastSeen) {
		st.lastSeen = ts
	}
	return changed
}

func evictOldestService(m map[serviceKey]*serviceState) {
	var oldestKey serviceKey
	var oldest *serviceState
	for k, st := range m {
		if oldest == nil || st.lastSeen.Before(oldest.lastSeen) ||
			(st.lastSeen.Equal(oldest.lastSeen) && (k.port < oldestKey.port || (k.port == oldestKey.port && k.proto < oldestKey.proto))) {
			oldestKey, oldest = k, st
		}
	}
	delete(m, oldestKey)
}

// For returns host's services sorted by port then protocol.
func (s *Services) For(host netip.Addr) []protocol.Service {
	m := s.byHost[host]
	out := make([]protocol.Service, 0, len(m))
	for k, st := range m {
		out = append(out, protocol.Service{Port: int(k.port), Proto: k.proto, Category: st.category, Confirmed: st.confirmed})
	}
	slices.SortFunc(out, func(a, b protocol.Service) int {
		return cmp.Or(cmp.Compare(a.Port, b.Port), cmp.Compare(a.Proto, b.Proto))
	})
	return out
}

// Forget drops a removed host's state.
func (s *Services) Forget(host netip.Addr) { delete(s.byHost, host) }
