// SPDX-License-Identifier: MIT

package pipeline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// testAnonKey is fixed so anonymised golden output is deterministic.
var testAnonKey = []byte("cybercity-test-anonymisation-key")

func anonOptions() Options {
	o := testOptions()
	o.Anonymize, o.AnonKey = true, testAnonKey
	return o
}

func TestAnonymizedGolden(t *testing.T) {
	for _, name := range []string{"web_https", "demo"} {
		t.Run(name, func(t *testing.T) {
			got := runFixture(t, name, anonOptions())
			path := filepath.Join("..", "..", "testdata", "golden", name+".anon.jsonl")
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run with -update)", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%s differs from golden", path)
			}
		})
	}
}

func TestAnonymizedOutputHidesAddresses(t *testing.T) {
	out := runFixture(t, "demo", anonOptions())
	for _, leak := range []string{"10.0.1.", "10.0.9.", "203.0.113.", "198.51.100.", "/24"} {
		if bytes.Contains(out, []byte(leak)) {
			t.Fatalf("anonymised output contains %q", leak)
		}
	}
	lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	var hello protocol.Hello
	if err := json.Unmarshal(lines[0], &hello); err != nil {
		t.Fatal(err)
	}
	if !hello.Server.Anonymized {
		t.Fatal("hello.server.anonymized should be true")
	}

	anon := regexp.MustCompile(`^anon-[0-9a-f]{8}$`)
	hostIDs := map[string]bool{}
	var flowEnds []string
	for _, line := range lines[1:] {
		var tk protocol.Tick
		if err := json.Unmarshal(line, &tk); err != nil {
			t.Fatal(err)
		}
		for _, d := range tk.Districts {
			if !strings.HasPrefix(d.ID, "lan-") || len(d.CIDRs) != 0 {
				t.Fatalf("district leaks its prefix: %+v", d)
			}
		}
		for _, h := range tk.Hosts {
			if !anon.MatchString(h.ID) || h.IP != nil {
				t.Fatalf("host not anonymised: %+v", h)
			}
			hostIDs[h.ID] = true
		}
		for _, f := range tk.Flows {
			flowEnds = append(flowEnds, f.Src, f.Dst)
		}
	}
	// Flows refer to hosts by the same pseudonyms (group addresses aside).
	for _, id := range flowEnds {
		if !hostIDs[id] && !strings.HasPrefix(id, "224.") && !strings.HasPrefix(id, "239.") {
			t.Fatalf("flow endpoint %q matches no host", id)
		}
	}
}

func TestPseudonymsAreStableWithinARun(t *testing.T) {
	id := newIdentity(true, testAnonKey)
	a, b := id.hostID(mustAddr("10.0.1.10")), id.hostID(mustAddr("10.0.1.10"))
	if a != b || a == id.hostID(mustAddr("10.0.1.11")) {
		t.Fatalf("pseudonyms unstable or colliding: %s %s", a, b)
	}
	if other := newIdentity(true, []byte("another-key")).hostID(mustAddr("10.0.1.10")); other == a {
		t.Fatal("pseudonym does not depend on the key")
	}
}

func TestShowMACFlagReachesHello(t *testing.T) {
	o := testOptions()
	o.Server.ShowMAC = true
	if !New(o).Snapshot().Server.ShowMAC {
		t.Fatal("show_mac not reported in hello")
	}
}
