// SPDX-License-Identifier: MIT

// Package config holds runtime settings: built-in defaults, an optional YAML
// file and command-line flags, in that order of precedence (docs/CONFIG.md).
package config

import (
	"fmt"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config is the complete runtime configuration.
type Config struct {
	Listen       string   `yaml:"listen"`
	AllowOrigins []string `yaml:"allow_origins"`
	Iface        string   `yaml:"iface"`
	BPF          string   `yaml:"bpf"`
	Snaplen      int      `yaml:"snaplen"`
	Promiscuous  bool     `yaml:"promiscuous"`
	User         string   `yaml:"user"`
	ShowMAC      bool     `yaml:"show_mac"`
	Anonymize    bool     `yaml:"anonymize"`
	Districts    string   `yaml:"districts"`
	GeoIP        string   `yaml:"geoip"`
	Window       Duration `yaml:"window"`
	MaxHosts     int      `yaml:"max_hosts"`
	LogLevel     string   `yaml:"log_level"`

	Server ServerConfig `yaml:"server"`
	Flows  FlowsConfig  `yaml:"flows"`
	Hosts  HostsConfig  `yaml:"hosts"`
	Enrich EnrichConfig `yaml:"enrich"`
	Detect DetectConfig `yaml:"detect"`
	Geo    GeoConfig    `yaml:"geo"`

	// Flag-only settings.
	ConfigFile  string  `yaml:"-"`
	URLFile     string  `yaml:"-"`
	Read        string  `yaml:"-"`
	ReplaySpeed float64 `yaml:"-"`
	Loop        bool    `yaml:"-"`
	DumpJSON    string  `yaml:"-"`
	ListIfaces  bool    `yaml:"-"`
	ShowVersion bool    `yaml:"-"`
}

// ServerConfig tunes the HTTP/WebSocket server.
type ServerConfig struct {
	MaxClients   int      `yaml:"max_clients"`
	ClientQueue  int      `yaml:"client_queue"`
	WriteTimeout Duration `yaml:"write_timeout"`
	PingInterval Duration `yaml:"ping_interval"`
	Heartbeat    Duration `yaml:"heartbeat"`
}

// FlowsConfig bounds flow state.
type FlowsConfig struct {
	MaxTracked int `yaml:"max_tracked"`
}

// HostsConfig tunes the host table.
type HostsConfig struct {
	IdleTimeout Duration `yaml:"idle_timeout"`
	MaxServices int      `yaml:"max_services"`
}

// EnrichConfig tunes passive enrichment.
type EnrichConfig struct {
	HandshakeTimeout Duration `yaml:"handshake_timeout"`
	RTTSamples       int      `yaml:"rtt_samples"`
	RetransWindow    Duration `yaml:"retrans_window"`
	ClassInterval    Duration `yaml:"class_interval"`
	GatewayMinIPs    int      `yaml:"gateway_min_ips"`
}

// DetectConfig tunes the heuristic detectors.
type DetectConfig struct {
	Scan  ScanConfig  `yaml:"scan"`
	Flood FloodConfig `yaml:"flood"`
}

// ScanConfig tunes the port-scan detector.
type ScanConfig struct {
	Window             Duration `yaml:"window"`
	MinPorts           int      `yaml:"min_ports"`
	MinHosts           int      `yaml:"min_hosts"`
	MinIncompleteRatio float64  `yaml:"min_incomplete_ratio"`
	UDPMinPorts        int      `yaml:"udp_min_ports"`
	Cooldown           Duration `yaml:"cooldown"`
}

// FloodConfig tunes the flood detector.
type FloodConfig struct {
	Window           Duration `yaml:"window"`
	BaselineHalfLife Duration `yaml:"baseline_half_life"`
	MinHistory       Duration `yaml:"min_history"`
	Multiplier       float64  `yaml:"multiplier"`
	MinPPS           int      `yaml:"min_pps"`
	MinSources       int      `yaml:"min_sources"`
	Cooldown         Duration `yaml:"cooldown"`
}

// GeoConfig holds the sensor location used for GeoIP bearings.
type GeoConfig struct {
	Home *[2]float64 `yaml:"home"`
}

// Duration is a time.Duration written as a Go duration string ("100ms") in YAML.
type Duration time.Duration

// D returns the value as a time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// UnmarshalYAML parses a duration string.
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("line %d: %w", n.Line, err)
	}
	*d = Duration(v)
	return nil
}

// String formats like time.Duration.
func (d Duration) String() string { return time.Duration(d).String() }
