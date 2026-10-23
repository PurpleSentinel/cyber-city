package capture

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/gopacket/gopacket/pcap"
)

// Iface is a capture-capable interface.
type Iface struct {
	Name    string
	Addrs   []string
	Up      bool
	Loop    bool
	Default bool // carries the default route
}

// route flags from linux/route.h.
const (
	rtfUp     = 0x0001
	rtfReject = 0x0200
)

// defaultRouteIPv4 parses /proc/net/route and returns the interface of the
// lowest-metric usable default route.
func defaultRouteIPv4(r io.Reader) string {
	best, bestMetric := "", int64(-1)
	sc := bufio.NewScanner(r)
	for first := true; sc.Scan(); first = false {
		f := strings.Fields(sc.Text())
		if first || len(f) < 8 {
			continue
		}
		flags, err1 := strconv.ParseInt(f[3], 16, 64)
		metric, err2 := strconv.ParseInt(f[6], 10, 64)
		if err1 != nil || err2 != nil || f[1] != "00000000" || f[7] != "00000000" ||
			flags&rtfUp == 0 || flags&rtfReject != 0 || f[0] == "lo" {
			continue
		}
		if bestMetric < 0 || metric < bestMetric {
			best, bestMetric = f[0], metric
		}
	}
	return best
}

// defaultRouteIPv6 parses /proc/net/ipv6_route likewise (::/0 routes).
func defaultRouteIPv6(r io.Reader) string {
	best, bestMetric := "", int64(-1)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 10 || f[0] != strings.Repeat("0", 32) || f[1] != "00" {
			continue
		}
		metric, err1 := strconv.ParseInt(f[5], 16, 64)
		flags, err2 := strconv.ParseInt(f[8], 16, 64)
		if err1 != nil || err2 != nil || flags&rtfUp == 0 || flags&rtfReject != 0 || f[9] == "lo" {
			continue
		}
		if bestMetric < 0 || metric < bestMetric {
			best, bestMetric = f[9], metric
		}
	}
	return best
}

func defaultRouteIface() string {
	for _, src := range []struct {
		path  string
		parse func(io.Reader) string
	}{
		{"/proc/net/route", defaultRouteIPv4},
		{"/proc/net/ipv6_route", defaultRouteIPv6},
	} {
		f, err := os.Open(src.path)
		if err != nil {
			continue
		}
		name := src.parse(f)
		_ = f.Close()
		if name != "" {
			return name
		}
	}
	return ""
}

// Interfaces lists capture devices, marking the default one.
func Interfaces() ([]Iface, error) {
	devs, err := pcap.FindAllDevs()
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}
	out := make([]Iface, 0, len(devs))
	for _, d := range devs {
		ifc := Iface{
			Name: d.Name,
			Up:   d.Flags&pcapIfUp != 0,
			Loop: d.Flags&pcapIfLoopback != 0,
		}
		for _, a := range d.Addresses {
			ifc.Addrs = append(ifc.Addrs, a.IP.String())
		}
		out = append(out, ifc)
	}
	markDefault(out, defaultRouteIface())
	return out, nil
}

// libpcap PCAP_IF_* flags.
const (
	pcapIfLoopback = 0x1
	pcapIfUp       = 0x2
)

// markDefault flags the route's interface, or else the first interface that
// is up, not loopback and has an address (docs/TASKS.md T2.1).
func markDefault(ifs []Iface, routeIface string) {
	for i := range ifs {
		if routeIface != "" && ifs[i].Name == routeIface {
			ifs[i].Default = true
			return
		}
	}
	for i := range ifs {
		if ifs[i].Up && !ifs[i].Loop && len(ifs[i].Addrs) > 0 && ifs[i].Name != "any" {
			ifs[i].Default = true
			return
		}
	}
}

// ResolveIface returns name if it is a capture device, or the default device
// when name is empty.
func ResolveIface(name string) (string, error) {
	ifs, err := Interfaces()
	if err != nil {
		return "", err
	}
	for _, ifc := range ifs {
		if (name == "" && ifc.Default) || (name != "" && ifc.Name == name) {
			return ifc.Name, nil
		}
	}
	if name == "" {
		return "", fmt.Errorf("no default capture interface found; pass --iface (see --list-ifaces)")
	}
	return "", fmt.Errorf("interface %q not found; see --list-ifaces", name)
}

// PrintInterfaces writes the --list-ifaces table.
func PrintInterfaces(w io.Writer, ifs []Iface) {
	for _, ifc := range ifs {
		mark := " "
		if ifc.Default {
			mark = "*"
		}
		state := "down"
		if ifc.Up {
			state = "up"
		}
		_, _ = fmt.Fprintf(w, "%s %-16s %-4s %s\n", mark, ifc.Name, state, strings.Join(ifc.Addrs, ", "))
	}
	_, _ = fmt.Fprintln(w, "\n* default (carries the default route)")
}
