package config

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

// newFlagSet binds every flag in docs/CONFIG.md to cfg. Flags default to cfg's
// current values, so parsing only changes what was given on the command line.
func newFlagSet(cfg *Config, out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("cybercity", flag.ContinueOnError)
	fs.SetOutput(out)

	fs.StringVar(&cfg.ConfigFile, "config", cfg.ConfigFile, "YAML config file (format: config/cybercity.example.yaml)")
	fs.StringVar(&cfg.Listen, "listen", cfg.Listen, "HTTP/WebSocket `host:port`; port 0 picks a free port")
	fs.Var(&originList{target: &cfg.AllowOrigins}, "allow-origin", "extra allowed `origin` (repeatable), e.g. http://localhost:5173")
	fs.StringVar(&cfg.URLFile, "url-file", cfg.URLFile, "write the ready-to-open URL to this `path` (mode 0600)")
	fs.StringVar(&cfg.Iface, "iface", cfg.Iface, "live capture `interface` (default: default-route interface)")
	fs.BoolVar(&cfg.ListIfaces, "list-ifaces", cfg.ListIfaces, "print capture interfaces and exit")
	fs.StringVar(&cfg.Read, "read", cfg.Read, "replay a capture `file` instead of live capture")
	fs.Float64Var(&cfg.ReplaySpeed, "replay-speed", cfg.ReplaySpeed, "replay speed `multiplier`; 0 = as fast as possible")
	fs.BoolVar(&cfg.Loop, "loop", cfg.Loop, "restart the file at EOF")
	fs.StringVar(&cfg.DumpJSON, "dump-json", cfg.DumpJSON, "write hello + ticks as JSON Lines to `path` (- = stdout) and exit; needs --read")
	fs.StringVar(&cfg.BPF, "bpf", cfg.BPF, "capture filter `expression`")
	fs.IntVar(&cfg.Snaplen, "snaplen", cfg.Snaplen, "`bytes` captured per packet (>= 128)")
	fs.Var(negatedBool{target: &cfg.Promiscuous}, "no-promisc", "disable promiscuous mode")
	fs.StringVar(&cfg.User, "user", cfg.User, "drop to this `user` after opening the capture (default nobody when root)")
	fs.BoolVar(&cfg.ShowMAC, "show-mac", cfg.ShowMAC, "include MAC addresses in host data")
	fs.BoolVar(&cfg.Anonymize, "anonymize", cfg.Anonymize, "pseudonymous host ids and no IP addresses")
	fs.StringVar(&cfg.Districts, "districts", cfg.Districts, "district map `file`")
	fs.StringVar(&cfg.GeoIP, "geoip", cfg.GeoIP, "GeoLite2-City `mmdb` file (needs geo.home)")
	fs.Var(durationValue{target: &cfg.Window}, "window", "aggregation `window` (50ms..1s)")
	fs.IntVar(&cfg.MaxHosts, "max-hosts", cfg.MaxHosts, "tracked hosts before least-recently-seen eviction")
	fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "debug, info, warn or error")
	fs.BoolVar(&cfg.ShowVersion, "version", cfg.ShowVersion, "print version and exit")

	fs.Usage = func() {
		_, _ = fmt.Fprintf(out, "Usage: cybercity [flags]\n\nFlags:\n")
		fs.VisitAll(func(f *flag.Flag) {
			name, usage := flag.UnquoteUsage(f)
			line := "  --" + f.Name
			if name != "" {
				line += " " + name
			}
			_, _ = fmt.Fprintf(out, "%s\n    \t%s", line, strings.ReplaceAll(usage, "\n", "\n    \t"))
			if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "[]" {
				_, _ = fmt.Fprintf(out, " (default %s)", f.DefValue)
			}
			_, _ = fmt.Fprintln(out)
		})
	}
	return fs
}

// originList is a repeatable flag. The first use on the command line replaces
// any origins from the config file (flags override the file).
type originList struct {
	target *[]string
	set    bool
}

func (o *originList) String() string {
	if o == nil || o.target == nil {
		return ""
	}
	return strings.Join(*o.target, ",")
}

func (o *originList) Set(v string) error {
	if !o.set {
		*o.target = nil
		o.set = true
	}
	*o.target = append(*o.target, v)
	return nil
}

// negatedBool is a boolean flag that clears target when given (--no-promisc).
type negatedBool struct{ target *bool }

func (n negatedBool) String() string   { return "false" }
func (n negatedBool) IsBoolFlag() bool { return true }
func (n negatedBool) Set(v string) error {
	switch v {
	case "true", "1":
		*n.target = false
	case "false", "0":
	default:
		return fmt.Errorf("invalid boolean %q", v)
	}
	return nil
}

type durationValue struct{ target *Duration }

func (d durationValue) String() string {
	if d.target == nil {
		return ""
	}
	return d.target.String()
}

func (d durationValue) Set(v string) error {
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return err
	}
	*d.target = Duration(parsed)
	return nil
}
