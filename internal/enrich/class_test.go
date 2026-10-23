// SPDX-License-Identifier: MIT

package enrich

import (
	"fmt"
	"net/netip"
	"testing"
	"time"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

func addr(s string) netip.Addr { return netip.MustParseAddr(s) }

func TestClassifyRules(t *testing.T) {
	now := time.Unix(10_000, 0)
	c := NewClassifier()
	server, client := addr("10.0.9.5"), addr("10.0.1.20")
	for i := 1; i <= 3; i++ {
		c.Connection(addr(fmt.Sprintf("10.0.1.%d", 100+i)), server, 22, protocol.CatSSH, true, now)
		c.Connection(client, addr(fmt.Sprintf("203.0.113.%d", i)), 443, protocol.CatHTTPS, false, now)
	}
	c.Connection(addr("10.0.1.30"), addr("10.0.9.7"), 1883, protocol.CatOther, true, now)

	cases := []struct {
		name string
		in   ClassInput
		want string
	}{
		{"gateway", ClassInput{Host: addr("10.0.1.1"), Local: true, Gateway: true}, protocol.ClassNetwork},
		{"network vendor", ClassInput{Host: addr("10.0.9.2"), Local: true, Vendor: "Cisco Systems, Inc"}, protocol.ClassNetwork},
		{"iot vendor", ClassInput{Host: addr("10.0.1.40"), Local: true, Vendor: "Espressif Inc."}, protocol.ClassIoT},
		{"mqtt client", ClassInput{Host: addr("10.0.1.30"), Local: true}, protocol.ClassIoT},
		{"server", ClassInput{Host: server, Local: true, ConfirmedServices: 1, Now: now}, protocol.ClassServer},
		{"server needs confirmed", ClassInput{Host: server, Local: true, Now: now}, protocol.ClassUnknown},
		{"workstation", ClassInput{Host: client, Local: true, Now: now}, protocol.ClassWorkstation},
		{"workstation must be local", ClassInput{Host: client, Local: false, Now: now}, protocol.ClassUnknown},
		{"unknown", ClassInput{Host: addr("10.0.1.66"), Local: true, Vendor: "Apple, Inc.", Now: now}, protocol.ClassUnknown},
		// Precedence: network beats iot beats server beats workstation.
		{"gateway with iot vendor", ClassInput{Host: addr("10.0.1.1"), Gateway: true, Vendor: "Espressif"}, protocol.ClassNetwork},
		{"iot vendor serving", ClassInput{Host: server, Local: true, Vendor: "Sonos, Inc.", ConfirmedServices: 1, Now: now}, protocol.ClassIoT},
	}
	for _, tc := range cases {
		if got := c.Classify(tc.in); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}

	// Peers older than five minutes no longer count.
	if got := c.Classify(ClassInput{Host: client, Local: true, Now: now.Add(6 * time.Minute)}); got != protocol.ClassUnknown {
		t.Errorf("stale https peers: got %s, want unknown", got)
	}
}

func TestClassifierKeepsFewPeers(t *testing.T) {
	c := NewClassifier()
	server := addr("10.0.9.5")
	for i := range 50 {
		c.Connection(addr(fmt.Sprintf("10.0.1.%d", i+1)), server, 80, protocol.CatWebCleartext, true, time.Unix(int64(i), 0))
	}
	if n := len(c.servedPeers[server]); n != peersPerHost {
		t.Fatalf("kept %d peers, want %d", n, peersPerHost)
	}
	c.Forget(server)
	if _, ok := c.servedPeers[server]; ok {
		t.Fatal("Forget kept state")
	}
}
