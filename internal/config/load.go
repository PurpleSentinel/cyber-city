package config

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// ErrHelp is returned by Load when --help was requested; the usage text has
// already been written to the output.
var ErrHelp = flag.ErrHelp

// Load builds the configuration from defaults, the --config file (if any) and
// args, then validates it. Usage and parse errors are written to out.
func Load(args []string, out io.Writer) (Config, error) {
	cfg := Defaults()

	// Pass 1: only to find --config; every flag must be defined so parsing succeeds.
	scratch := Defaults()
	pre := newFlagSet(&scratch, io.Discard)
	if err := pre.Parse(args); err != nil && !errors.Is(err, flag.ErrHelp) {
		fs := newFlagSet(&cfg, out)
		_ = fs.Parse(args) // repeat to print the error and usage to out
		return cfg, err
	}
	if scratch.ConfigFile != "" {
		if err := loadFile(scratch.ConfigFile, &cfg); err != nil {
			return cfg, err
		}
	}

	// Pass 2: flags override the file.
	fs := newFlagSet(&cfg, out)
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() > 0 {
		return cfg, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if cfg.DumpJSON != "" {
		cfg.ReplaySpeed = 0 // --dump-json implies as fast as possible
	}
	if cfg.ListIfaces || cfg.ShowVersion {
		return cfg, nil
	}
	return cfg, cfg.Validate()
}

func loadFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied config path
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("config %s: %w", path, err)
	}
	return nil
}

// Validate reports the first invalid setting.
func (c *Config) Validate() error {
	if err := validateListen(c.Listen); err != nil {
		return err
	}
	switch {
	case c.Read != "" && c.Iface != "":
		return errors.New("--read and --iface are mutually exclusive")
	case c.DumpJSON != "" && c.Read == "":
		return errors.New("--dump-json requires --read")
	case c.Loop && c.Read == "":
		return errors.New("--loop requires --read")
	case c.ReplaySpeed < 0:
		return errors.New("--replay-speed must be >= 0")
	case c.Window.D() < 50*time.Millisecond || c.Window.D() > time.Second:
		return fmt.Errorf("--window %s out of range 50ms..1s", c.Window)
	case c.Snaplen < 128 || c.Snaplen > 65535:
		return fmt.Errorf("--snaplen %d out of range 128..65535", c.Snaplen)
	case c.MaxHosts < 1:
		return errors.New("--max-hosts must be >= 1")
	case c.GeoIP != "" && c.Geo.Home == nil:
		return errors.New("--geoip requires geo.home [lat, lon] in the config file")
	}
	if c.Geo.Home != nil {
		if lat, lon := c.Geo.Home[0], c.Geo.Home[1]; lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			return fmt.Errorf("geo.home [%v, %v] is not a valid [lat, lon]", lat, lon)
		}
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("--log-level %q must be debug, info, warn or error", c.LogLevel)
	}
	for _, o := range c.AllowOrigins {
		if !strings.HasPrefix(o, "http://") && !strings.HasPrefix(o, "https://") {
			return fmt.Errorf("--allow-origin %q must start with http:// or https://", o)
		}
	}
	return validatePositive(c)
}

func validateListen(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("--listen %q: %w", addr, err)
	}
	if host == "" {
		return fmt.Errorf("--listen %q: host required (use 127.0.0.1 or an explicit address)", addr)
	}
	if p, err := strconv.Atoi(port); err != nil || p < 0 || p > 65535 {
		return fmt.Errorf("--listen %q: invalid port", addr)
	}
	return nil
}

func validatePositive(c *Config) error {
	checks := []struct {
		name string
		ok   bool
	}{
		{"server.max_clients", c.Server.MaxClients > 0},
		{"server.client_queue", c.Server.ClientQueue > 0},
		{"server.write_timeout", c.Server.WriteTimeout > 0},
		{"server.ping_interval", c.Server.PingInterval > 0},
		{"server.heartbeat", c.Server.Heartbeat > 0},
		{"flows.max_tracked", c.Flows.MaxTracked > 0},
		{"hosts.idle_timeout", c.Hosts.IdleTimeout > 0},
		{"hosts.max_services", c.Hosts.MaxServices > 0},
		{"enrich.handshake_timeout", c.Enrich.HandshakeTimeout > 0},
		{"enrich.rtt_samples", c.Enrich.RTTSamples > 0},
		{"enrich.retrans_window", c.Enrich.RetransWindow > 0},
		{"enrich.class_interval", c.Enrich.ClassInterval > 0},
		{"enrich.gateway_min_ips", c.Enrich.GatewayMinIPs > 0},
		{"detect.scan.window", c.Detect.Scan.Window > 0},
		{"detect.scan.min_ports", c.Detect.Scan.MinPorts > 0},
		{"detect.scan.min_hosts", c.Detect.Scan.MinHosts > 0},
		{"detect.scan.min_incomplete_ratio", c.Detect.Scan.MinIncompleteRatio > 0 && c.Detect.Scan.MinIncompleteRatio <= 1},
		{"detect.scan.udp_min_ports", c.Detect.Scan.UDPMinPorts > 0},
		{"detect.flood.window", c.Detect.Flood.Window > 0},
		{"detect.flood.baseline_half_life", c.Detect.Flood.BaselineHalfLife > 0},
		{"detect.flood.multiplier", c.Detect.Flood.Multiplier > 0},
		{"detect.flood.min_pps", c.Detect.Flood.MinPPS > 0},
		{"detect.flood.min_sources", c.Detect.Flood.MinSources > 0},
	}
	for _, chk := range checks {
		if !chk.ok {
			return fmt.Errorf("%s must be positive", chk.name)
		}
	}
	return nil
}
