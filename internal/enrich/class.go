package enrich

import (
	"net/netip"
	"strings"
	"time"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// Vendor keywords for device classes (docs/CONFIG.md "Device class"),
// matched case-insensitively as substrings of the OUI vendor name.
var (
	networkVendors = []string{"cisco", "juniper", "ubiquiti", "mikrotik", "aruba", "netgear", "tp-link", "fortinet", "palo alto", "zyxel", "arista"}
	iotVendors     = []string{"espressif", "tuya", "shelly", "allterco", "signify", "philips lighting", "sonos", "hikvision", "dahua", "xiaomi", "amazon technologies", "nest labs", "roku", "ring", "wyze", "raspberry pi"}
)

const (
	classWindow  = 5 * time.Minute // peer windows in the server/workstation rules
	peersPerHost = 8               // rules need at least 3; keep a few recent ones
)

// Classifier assigns heuristic device classes from passive observations.
type Classifier struct {
	servedPeers map[netip.Addr]map[netip.Addr]time.Time // server → clients of its services
	httpsDests  map[netip.Addr]map[netip.Addr]time.Time // client → non-local https servers
	mqttClient  map[netip.Addr]bool
}

// NewClassifier returns an empty classifier.
func NewClassifier() *Classifier {
	return &Classifier{
		servedPeers: make(map[netip.Addr]map[netip.Addr]time.Time),
		httpsDests:  make(map[netip.Addr]map[netip.Addr]time.Time),
		mqttClient:  make(map[netip.Addr]bool),
	}
}

// Connection records traffic from client to a service on server.
func (c *Classifier) Connection(client, server netip.Addr, port uint16, category string, serverLocal bool, ts time.Time) {
	remember(c.servedPeers, server, client, ts)
	if category == protocol.CatHTTPS && !serverLocal {
		remember(c.httpsDests, client, server, ts)
	}
	if port == 1883 || port == 8883 {
		c.mqttClient[client] = true
	}
}

// remember keeps the most recent peersPerHost peers of host.
func remember(m map[netip.Addr]map[netip.Addr]time.Time, host, peer netip.Addr, ts time.Time) {
	peers := m[host]
	if peers == nil {
		peers = make(map[netip.Addr]time.Time)
		m[host] = peers
	}
	if _, ok := peers[peer]; !ok && len(peers) >= peersPerHost {
		var oldest netip.Addr
		var oldestTS time.Time
		for p, t := range peers {
			if oldestTS.IsZero() || t.Before(oldestTS) || (t.Equal(oldestTS) && p.Less(oldest)) {
				oldest, oldestTS = p, t
			}
		}
		delete(peers, oldest)
	}
	if ts.After(peers[peer]) {
		peers[peer] = ts
	}
}

func recentCount(peers map[netip.Addr]time.Time, now time.Time) int {
	n := 0
	for _, t := range peers {
		if now.Sub(t) <= classWindow {
			n++
		}
	}
	return n
}

// ClassInput is what the rules need to know about one host.
type ClassInput struct {
	Host              netip.Addr
	Local             bool
	Vendor            string
	Gateway           bool
	ConfirmedServices int
	Now               time.Time
}

// Classify applies the rules in order; the first match wins.
func (c *Classifier) Classify(in ClassInput) string {
	vendor := strings.ToLower(in.Vendor)
	switch {
	case in.Gateway || containsAny(vendor, networkVendors):
		return protocol.ClassNetwork
	case containsAny(vendor, iotVendors) || c.mqttClient[in.Host]:
		return protocol.ClassIoT
	case in.ConfirmedServices > 0 && recentCount(c.servedPeers[in.Host], in.Now) >= 3:
		return protocol.ClassServer
	case in.Local && recentCount(c.httpsDests[in.Host], in.Now) >= 3:
		return protocol.ClassWorkstation
	default:
		return protocol.ClassUnknown
	}
}

func containsAny(s string, words []string) bool {
	if s == "" {
		return false
	}
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// Forget drops a removed host's state.
func (c *Classifier) Forget(host netip.Addr) {
	delete(c.servedPeers, host)
	delete(c.httpsDests, host)
	delete(c.mqttClient, host)
}
