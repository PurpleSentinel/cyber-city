package config

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

const repoRoot = "../.."

func TestExampleMatchesDefaults(t *testing.T) {
	cfg, err := Load([]string{"--config", filepath.Join(repoRoot, "config", "cybercity.example.yaml")}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConfigFile = ""
	if want := Defaults(); !reflect.DeepEqual(cfg, want) {
		t.Fatalf("example config differs from Defaults()\n got: %+v\nwant: %+v", cfg, want)
	}
}

func TestDefaultsAreValid(t *testing.T) {
	cfg := Defaults()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

// Every flag in docs/CONFIG.md's table exists, and nothing undocumented does.
func TestFlagsMatchDocs(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join(repoRoot, "docs", "CONFIG.md"))
	if err != nil {
		t.Fatal(err)
	}
	var documented []string
	for _, m := range regexp.MustCompile("(?m)^\\| `--([a-z-]+)").FindAllStringSubmatch(string(doc), -1) {
		documented = append(documented, m[1])
	}
	var defined []string
	cfg := Defaults()
	newFlagSet(&cfg, &bytes.Buffer{}).VisitAll(func(f *flag.Flag) { defined = append(defined, f.Name) })
	slices.Sort(documented)
	slices.Sort(defined)
	if !reflect.DeepEqual(documented, defined) {
		t.Fatalf("flags differ\n docs: %v\n code: %v", documented, defined)
	}
}

func TestHelpListsEveryFlag(t *testing.T) {
	var out bytes.Buffer
	_, err := Load([]string{"--help"}, &out)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err = %v, want ErrHelp", err)
	}
	cfg := Defaults()
	newFlagSet(&cfg, &bytes.Buffer{}).VisitAll(func(f *flag.Flag) {
		if !strings.Contains(out.String(), "--"+f.Name) {
			t.Errorf("--help is missing --%s", f.Name)
		}
	})
}

func writeFile(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPrecedence(t *testing.T) {
	file := writeFile(t, "c.yaml", "listen: 127.0.0.1:9000\nwindow: 200ms\nallow_origins: [\"http://a.example\"]\npromiscuous: true\n")
	cfg, err := Load([]string{"--config", file, "--window", "300ms", "--allow-origin", "http://b.example", "--no-promisc"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:9000" {
		t.Errorf("file value lost: listen=%s", cfg.Listen)
	}
	if cfg.Window.D() != 300*time.Millisecond {
		t.Errorf("flag should override file: window=%s", cfg.Window)
	}
	if !reflect.DeepEqual(cfg.AllowOrigins, []string{"http://b.example"}) {
		t.Errorf("flag should replace file origins: %v", cfg.AllowOrigins)
	}
	if cfg.Promiscuous {
		t.Error("--no-promisc ignored")
	}
}

func TestDumpJSONImpliesFullSpeed(t *testing.T) {
	cfg, err := Load([]string{"--read", "x.pcap", "--dump-json", "-", "--replay-speed", "4"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ReplaySpeed != 0 {
		t.Fatalf("replay speed = %v, want 0", cfg.ReplaySpeed)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		file string
		want string
	}{
		{"read and iface", []string{"--read", "a.pcap", "--iface", "eth0"}, "", "mutually exclusive"},
		{"dump without read", []string{"--dump-json", "-"}, "", "requires --read"},
		{"loop without read", []string{"--loop"}, "", "--loop requires --read"},
		{"dump with loop", []string{"--read", "a", "--dump-json", "-", "--loop"}, "", "cannot be combined"},
		{"negative speed", []string{"--read", "a", "--replay-speed", "-1"}, "", "replay-speed"},
		{"window too small", []string{"--window", "10ms"}, "", "out of range"},
		{"window too large", []string{"--window", "2s"}, "", "out of range"},
		{"snaplen too small", []string{"--snaplen", "64"}, "", "snaplen"},
		{"snaplen too large", []string{"--snaplen", "70000"}, "", "snaplen"},
		{"max hosts", []string{"--max-hosts", "0"}, "", "max-hosts"},
		{"geoip without home", []string{"--geoip", "x.mmdb"}, "", "geo.home"},
		{"bad home", nil, "geo:\n  home: [100, 0]\n", "not a valid"},
		{"log level", []string{"--log-level", "loud"}, "", "log-level"},
		{"listen no host", []string{"--listen", ":8080"}, "", "host required"},
		{"listen bad port", []string{"--listen", "127.0.0.1:99999"}, "", "invalid port"},
		{"listen garbage", []string{"--listen", "nope"}, "", "--listen"},
		{"origin scheme", []string{"--allow-origin", "localhost:5173"}, "", "allow-origin"},
		{"unknown key", nil, "listne: x\n", "not found"},
		{"bad duration", nil, "window: fast\n", "invalid duration"},
		{"zero positive", nil, "server:\n  max_clients: 0\n", "server.max_clients"},
		{"ratio above one", nil, "detect:\n  scan:\n    min_incomplete_ratio: 1.5\n", "min_incomplete_ratio"},
		{"unknown flag", []string{"--bogus"}, "", "bogus"},
		{"stray argument", []string{"extra"}, "", "unexpected argument"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := tc.args
			if tc.file != "" {
				args = append([]string{"--config", writeFile(t, "c.yaml", tc.file)}, args...)
			}
			_, err := Load(args, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestLoadDistrictsExample(t *testing.T) {
	ds, err := LoadDistricts(filepath.Join(repoRoot, "config", "districts.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 2 || ds[0].ID != "core" || ds[1].ID != "corp" {
		t.Fatalf("unexpected districts %+v", ds)
	}
	if got := ds[1].Prefixes[1].String(); got != "fd00:1::/64" {
		t.Errorf("prefix = %s", got)
	}
}

func TestDistrictValidation(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"bad id":    {"districts:\n  - {id: Core, name: x, kind: lan, cidrs: [10.0.0.0/8]}\n", "must match"},
		"reserved":  {"districts:\n  - {id: outlands, name: x, kind: lan, cidrs: [10.0.0.0/8]}\n", "reserved"},
		"duplicate": {"districts:\n  - {id: a, name: x, kind: lan, cidrs: [10.0.0.0/8]}\n  - {id: a, name: y, kind: lan, cidrs: [10.1.0.0/16]}\n", "twice"},
		"no name":   {"districts:\n  - {id: a, kind: lan, cidrs: [10.0.0.0/8]}\n", "name required"},
		"bad kind":  {"districts:\n  - {id: a, name: x, kind: dmz, cidrs: [10.0.0.0/8]}\n", "kind"},
		"empty":     {"districts:\n  - {id: a, name: x, kind: lan}\n", "needs cidrs or vlans"},
		"bad cidr":  {"districts:\n  - {id: a, name: x, kind: lan, cidrs: [10.0.0.0/33]}\n", "netip"},
		"bad vlan":  {"districts:\n  - {id: a, name: x, kind: lan, vlans: [5000]}\n", "out of range"},
		"unknown":   {"districts:\n  - {id: a, name: x, kind: lan, vlans: [5], colour: red}\n", "not found"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := LoadDistricts(writeFile(t, "d.yaml", tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}
