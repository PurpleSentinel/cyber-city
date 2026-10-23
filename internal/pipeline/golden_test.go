package pipeline

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PurpleSentinel/cyber-city/internal/capture"
	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

var update = flag.Bool("update", false, "rewrite testdata/golden")

var fixtureNames = []string{
	"web_https", "dns", "ssh", "retrans", "scan_syn", "scan_sweep",
	"flood", "idle", "ipv6", "vlan", "gateway", "demo",
}

func testOptions() Options {
	return Options{
		Window:     100 * time.Millisecond,
		Heartbeat:  time.Second,
		MaxTracked: 50000,
		Server:     protocol.ServerInfo{Version: "test", Source: "file"},
	}
}

// runFixture replays a fixture as fast as possible and returns the JSONL output.
func runFixture(t testing.TB, name string, opts Options) []byte {
	t.Helper()
	src, err := capture.OpenFile(filepath.Join("..", "..", "testdata", "pcap", name+".pcap"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Close() }()
	p := New(opts)
	var buf bytes.Buffer
	sink, err := NewJSONLSink(&buf, p.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), src, p, sink, 0); err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestGolden(t *testing.T) {
	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			got := runFixture(t, name, testOptions())
			path := filepath.Join("..", "..", "testdata", "golden", name+".jsonl")
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run go test ./internal/pipeline -run Golden -update)", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%s differs from golden; inspect with -update and git diff", path)
			}
		})
	}
}

func TestBPFFilterYieldsNoFlows(t *testing.T) {
	src, err := capture.OpenFile(filepath.Join("..", "..", "testdata", "pcap", "dns.pcap"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Close() }()
	if err := src.SetFilter("tcp"); err != nil {
		t.Fatal(err)
	}
	p := New(testOptions())
	var buf bytes.Buffer
	sink, err := NewJSONLSink(&buf, p.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), src, p, sink, 0); err != nil {
		t.Fatal(err)
	}
	_ = sink.Close()
	for _, tk := range ticks(t, buf.Bytes()) {
		if len(tk.Flows) != 0 {
			t.Fatalf("--bpf tcp on dns.pcap produced flows: %+v", tk.Flows)
		}
	}
}

func TestDeterministic(t *testing.T) {
	a, b := runFixture(t, "demo", testOptions()), runFixture(t, "demo", testOptions())
	if !bytes.Equal(a, b) {
		t.Fatal("two runs of demo.pcap differ")
	}
}

// ticks parses JSONL output into hello + ticks.
func ticks(t testing.TB, out []byte) []protocol.Tick {
	t.Helper()
	var res []protocol.Tick
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for i := 0; sc.Scan(); i++ {
		if i == 0 {
			continue // hello
		}
		var tk protocol.Tick
		if err := json.Unmarshal(sc.Bytes(), &tk); err != nil {
			t.Fatal(err)
		}
		res = append(res, tk)
	}
	return res
}

func TestWebHTTPSBytesAreOnWireLength(t *testing.T) {
	var total uint64
	var sawHTTPS bool
	for _, tk := range ticks(t, runFixture(t, "web_https", testOptions())) {
		for _, f := range tk.Flows {
			total += f.Bytes
			if f.Category == protocol.CatHTTPS && f.Src == "10.0.1.10" && f.Dst == "203.0.113.10" &&
				f.ServerSide != nil && *f.ServerSide == "dst" && f.ServicePort != nil && *f.ServicePort == 443 {
				sawHTTPS = true
			}
		}
	}
	if !sawHTTPS {
		t.Fatal("no https flow 10.0.1.10 → 203.0.113.10:443 with server_side dst")
	}
	// ~200 kB of payload plus headers: far more than the ≤64 stored bytes per packet.
	if total < 200_000 {
		t.Fatalf("total bytes %d: byte counts must use on-wire length", total)
	}
}

func TestTicksAreOrderedAndHeartbeat(t *testing.T) {
	ts := ticks(t, runFixture(t, "idle", testOptions()))
	for i := 1; i < len(ts); i++ {
		if ts[i].Seq != ts[i-1].Seq+1 || ts[i].TS <= ts[i-1].TS {
			t.Fatalf("tick %d out of order: seq %d ts %d after seq %d ts %d", i, ts[i].Seq, ts[i].TS, ts[i-1].Seq, ts[i-1].TS)
		}
		if gap := ts[i].TS - ts[i-1].TS; gap > 1000 {
			t.Fatalf("gap of %d ms between ticks exceeds the 1 s heartbeat", gap)
		}
	}
}

func TestGroupAddressesAreNotHosts(t *testing.T) {
	out := runFixture(t, "demo", testOptions())
	for _, tk := range ticks(t, out) {
		for _, h := range tk.Hosts {
			if h.ID == "224.0.0.251" || h.ID == "239.255.255.250" {
				t.Fatalf("multicast group %s became a host", h.ID)
			}
		}
	}
	if !bytes.Contains(out, []byte(`"dst":"224.0.0.251"`)) {
		t.Fatal("flows to multicast groups should still be reported")
	}
}

func TestAutoDistricts(t *testing.T) {
	for addr, want := range map[string]string{
		"10.0.1.10":  "lan-10.0.1",
		"fd00:1::10": "lan-fd00-1-0-0",
	} {
		if got := autoDistrict(mustAddr(addr)).ID; got != want {
			t.Errorf("autoDistrict(%s) = %s, want %s", addr, got, want)
		}
	}
}
