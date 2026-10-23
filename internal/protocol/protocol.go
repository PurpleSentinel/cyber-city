// Package protocol defines the v1 wire messages exchanged with the browser.
// It mirrors docs/PROTOCOL.md and web/src/protocol.ts; change all three together.
package protocol

import (
	"encoding/json"
	"math"
)

// Version is the protocol version carried in every message's "v" field.
const Version = 1

// Message types.
const (
	TypeHello = "hello"
	TypeTick  = "tick"
)

// District kinds.
const (
	KindLAN      = "lan"
	KindServer   = "server"
	KindOutlands = "outlands"
)

// OutlandsID is the id of the single district holding all non-local hosts.
const OutlandsID = "outlands"

// Traffic categories (docs/CONFIG.md "Traffic categories"), in facade-window bit order.
const (
	CatHTTPS           = "https"
	CatWebCleartext    = "web_cleartext"
	CatDNS             = "dns"
	CatSSH             = "ssh"
	CatLegacyCleartext = "legacy_cleartext"
	CatDiscovery       = "discovery"
	CatControl         = "control"
	CatOther           = "other"
)

// Categories lists every category in bit order.
var Categories = []string{CatHTTPS, CatWebCleartext, CatDNS, CatSSH, CatLegacyCleartext, CatDiscovery, CatControl, CatOther}

// Device classes.
const (
	ClassWorkstation = "workstation"
	ClassServer      = "server"
	ClassIoT         = "iot"
	ClassNetwork     = "network"
	ClassUnknown     = "unknown"
)

// Event kinds and severities.
const (
	EventPortScan = "port_scan"
	EventFlood    = "flood"
	EventHostIdle = "host_idle"

	SeverityInfo  = "info"
	SeverityWarn  = "warn"
	SeverityAlert = "alert"
)

// Server sides of a flow.
const (
	SideSrc = "src"
	SideDst = "dst"
)

// Hello is the first message on every connection: configuration plus a full snapshot.
type Hello struct {
	V         int        `json:"v"`
	Type      string     `json:"type"`
	Server    ServerInfo `json:"server"`
	Districts []District `json:"districts"`
	Hosts     []Host     `json:"hosts"`
}

// ServerInfo describes the running sensor.
type ServerInfo struct {
	Version    string  `json:"version"`
	Source     string  `json:"source"`
	Iface      *string `json:"iface"`
	WindowMS   int     `json:"window_ms"`
	Anonymized bool    `json:"anonymized"`
	ShowMAC    bool    `json:"show_mac"`
	GeoIP      bool    `json:"geoip"`
}

// District groups hosts into a city block.
type District struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Kind  string   `json:"kind"`
	CIDRs []string `json:"cidrs"`
}

// Tick carries one aggregation window.
type Tick struct {
	V            int        `json:"v"`
	Type         string     `json:"type"`
	Seq          uint64     `json:"seq"`
	TS           int64      `json:"ts"`
	WindowMS     int        `json:"window_ms"`
	Stats        Stats      `json:"stats"`
	Districts    []District `json:"districts"`
	Flows        []Flow     `json:"flows"`
	Hosts        []Host     `json:"hosts"`
	RemovedHosts []string   `json:"removed_hosts"`
	Events       []Event    `json:"events"`
}

// Stats summarises a window plus cumulative capture drop counters.
type Stats struct {
	Packets     uint64 `json:"packets"`
	Bytes       uint64 `json:"bytes"`
	Flows       int    `json:"flows"`
	HostsTotal  int    `json:"hosts_total"`
	KernelDrops uint64 `json:"kernel_drops"`
	IfDrops     uint64 `json:"if_drops"`
}

// Flow is traffic between two hosts within one window.
type Flow struct {
	Src         string   `json:"src"`
	Dst         string   `json:"dst"`
	Proto       string   `json:"proto"`
	ServicePort *int     `json:"service_port"`
	ServerSide  *string  `json:"server_side"`
	Category    string   `json:"category"`
	Packets     uint64   `json:"packets"`
	Bytes       uint64   `json:"bytes"`
	TCPFlags    []string `json:"tcp_flags"`
	RTTMS       *float64 `json:"rtt_ms"`
	Retrans     int      `json:"retrans"`
}

// Host is one building in the city.
type Host struct {
	ID          string    `json:"id"`
	IP          *string   `json:"ip"`
	Local       bool      `json:"local"`
	District    string    `json:"district"`
	MAC         *string   `json:"mac"`
	Vendor      *string   `json:"vendor"`
	DeviceClass string    `json:"device_class"`
	Services    []Service `json:"services"`
	Geo         *Geo      `json:"geo"`
	FirstSeen   int64     `json:"first_seen"`
	LastSeen    int64     `json:"last_seen"`
	RTTMS       *float64  `json:"rtt_ms"`
	RetransRate float64   `json:"retrans_rate"`
}

// Service is a port a host serves.
type Service struct {
	Port      int    `json:"port"`
	Proto     string `json:"proto"`
	Category  string `json:"category"`
	Confirmed bool   `json:"confirmed"`
}

// Geo places a non-local host in a compass sector.
type Geo struct {
	Sector  string `json:"sector"`
	Country string `json:"country"`
}

// Event is a heuristic detection for the client to animate and log.
type Event struct {
	ID         string             `json:"id"`
	Kind       string             `json:"kind"`
	TS         int64              `json:"ts"`
	Severity   string             `json:"severity"`
	Src        *string            `json:"src"`
	Dst        *string            `json:"dst"`
	Targets    []string           `json:"targets"`
	Evidence   map[string]float64 `json:"evidence"`
	DurationMS int                `json:"duration_ms"`
}

// MaxTargets caps Event.Targets.
const MaxTargets = 64

// EncodeHello normalises, sorts and marshals a hello message.
func EncodeHello(h *Hello) ([]byte, error) {
	h.V, h.Type = Version, TypeHello
	h.normalize()
	SortHello(h)
	return json.Marshal(h)
}

// EncodeTick normalises, sorts and marshals a tick message.
func EncodeTick(t *Tick) ([]byte, error) {
	t.V, t.Type = Version, TypeTick
	t.normalize()
	SortTick(t)
	return json.Marshal(t)
}

// Arrays are always [] on the wire, never null (docs/PROTOCOL.md "Common conventions").
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (h *Hello) normalize() {
	h.Districts = normalizeDistricts(h.Districts)
	h.Hosts = normalizeHosts(h.Hosts)
}

func (t *Tick) normalize() {
	t.Districts = normalizeDistricts(t.Districts)
	t.Flows = nonNil(t.Flows)
	for i := range t.Flows {
		t.Flows[i].TCPFlags = nonNil(t.Flows[i].TCPFlags)
	}
	t.Hosts = normalizeHosts(t.Hosts)
	t.RemovedHosts = nonNil(t.RemovedHosts)
	t.Events = nonNil(t.Events)
	for i := range t.Events {
		e := &t.Events[i]
		e.Targets = nonNil(e.Targets)
		if len(e.Targets) > MaxTargets {
			e.Targets = e.Targets[:MaxTargets]
		}
		if e.Evidence == nil {
			e.Evidence = map[string]float64{}
		}
	}
}

func normalizeDistricts(ds []District) []District {
	ds = nonNil(ds)
	for i := range ds {
		ds[i].CIDRs = nonNil(ds[i].CIDRs)
	}
	return ds
}

func normalizeHosts(hs []Host) []Host {
	hs = nonNil(hs)
	for i := range hs {
		hs[i].Services = nonNil(hs[i].Services)
	}
	return hs
}

// Round1 rounds to one decimal place (rtt_ms).
func Round1(v float64) float64 { return math.Round(v*10) / 10 }

// Round3 rounds to three decimal places (retrans_rate).
func Round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// Ptr returns a pointer to v, for optional fields.
func Ptr[T any](v T) *T { return &v }
