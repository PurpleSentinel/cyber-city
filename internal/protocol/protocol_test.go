// SPDX-License-Identifier: MIT

package protocol

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/protocol examples")

func TestExamplesMatchTestdata(t *testing.T) {
	cases := map[string]func() ([]byte, error){
		"hello.json":  func() ([]byte, error) { return EncodeHello(ExampleHello()) },
		"tick.json":   func() ([]byte, error) { return EncodeTick(ExampleTick()) },
		"events.json": func() ([]byte, error) { return EncodeTick(ExampleEventsTick()) },
	}
	for name, encode := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := encode()
			if err != nil {
				t.Fatal(err)
			}
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, raw, "", "  "); err != nil {
				t.Fatal(err)
			}
			pretty.WriteByte('\n')
			path := filepath.Join("..", "..", "testdata", "protocol", name)
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, pretty.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run with -update to create)", err)
			}
			if !bytes.Equal(pretty.Bytes(), want) {
				t.Fatalf("%s is stale; run go test ./internal/protocol -update", path)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	raw, err := EncodeTick(ExampleEventsTick())
	if err != nil {
		t.Fatal(err)
	}
	var got Tick
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	again, err := EncodeTick(&got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, again) {
		t.Fatalf("round trip changed output:\n%s\n%s", raw, again)
	}
}

func TestEmptyArraysAreNotNull(t *testing.T) {
	raw, err := EncodeTick(&Tick{Flows: []Flow{{}}, Hosts: []Host{{}}, Events: []Event{{}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"districts", "flows", "hosts", "removed_hosts", "events", "tcp_flags", "services", "targets"} {
		if strings.Contains(string(raw), `"`+field+`":null`) {
			t.Errorf("%s marshalled as null: %s", field, raw)
		}
	}
	if !strings.Contains(string(raw), `"evidence":{}`) {
		t.Errorf("nil evidence should be {}: %s", raw)
	}
}

func TestSortTick(t *testing.T) {
	tick := &Tick{
		Flows: []Flow{
			{Src: "b", Dst: "x", Bytes: 10, TCPFlags: []string{"ACK", "SYN", "ACK", "BOGUS"}},
			{Src: "a", Dst: "y", Bytes: 10, ServicePort: Ptr(443)},
			{Src: "a", Dst: "y", Bytes: 10},
			{Src: "z", Dst: "z", Bytes: 99},
		},
		Hosts:        []Host{{ID: "b", Services: []Service{{Port: 443, Proto: "udp"}, {Port: 22}, {Port: 443, Proto: "tcp"}}}, {ID: "a"}},
		RemovedHosts: []string{"c", "a"},
		Events:       []Event{{ID: "e-2", TS: 5}, {ID: "e-1", TS: 5, Targets: []string{"y", "x"}}, {ID: "e-3", TS: 1}},
	}
	SortTick(tick)

	gotFlows := make([]string, len(tick.Flows))
	for i, f := range tick.Flows {
		gotFlows[i] = f.Src + f.Dst + string(rune('0'+portKey(f.ServicePort)%10))
	}
	if want := []string{"zz/", "ay/", "ay3", "bx/"}; !reflect.DeepEqual(gotFlows, want) {
		t.Errorf("flow order %v, want %v", gotFlows, want)
	}
	if got := tick.Flows[3].TCPFlags; !reflect.DeepEqual(got, []string{"SYN", "ACK"}) {
		t.Errorf("flags %v", got)
	}
	if tick.Hosts[0].ID != "a" {
		t.Errorf("hosts not sorted")
	}
	svc := tick.Hosts[1].Services
	if svc[0].Port != 22 || svc[1].Proto != "tcp" || svc[2].Proto != "udp" {
		t.Errorf("services order %+v", svc)
	}
	if !reflect.DeepEqual(tick.RemovedHosts, []string{"a", "c"}) {
		t.Errorf("removed %v", tick.RemovedHosts)
	}
	if ids := []string{tick.Events[0].ID, tick.Events[1].ID, tick.Events[2].ID}; !reflect.DeepEqual(ids, []string{"e-3", "e-1", "e-2"}) {
		t.Errorf("event order %v", ids)
	}
	if !reflect.DeepEqual(tick.Events[1].Targets, []string{"x", "y"}) {
		t.Errorf("targets %v", tick.Events[1].Targets)
	}
}

func TestTargetsCapped(t *testing.T) {
	targets := make([]string, 100)
	for i := range targets {
		targets[i] = string(rune('A' + i%26))
	}
	tick := &Tick{Events: []Event{{Targets: targets}}}
	if _, err := EncodeTick(tick); err != nil {
		t.Fatal(err)
	}
	if n := len(tick.Events[0].Targets); n != MaxTargets {
		t.Fatalf("targets = %d, want %d", n, MaxTargets)
	}
}
