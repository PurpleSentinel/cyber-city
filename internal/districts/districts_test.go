package districts

import (
	"net/netip"
	"path/filepath"
	"testing"

	"github.com/PurpleSentinel/cyber-city/internal/config"
)

func example(t *testing.T) *Matcher {
	t.Helper()
	defs, err := config.LoadDistricts(filepath.Join("..", "..", "config", "districts.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return New(defs)
}

func TestMatchVLANBeforeCIDR(t *testing.T) {
	m := example(t)
	cases := []struct {
		addr string
		vlan uint16
		want string
		ok   bool
	}{
		{"10.0.1.10", 10, "corp", true},
		{"10.0.9.5", 20, "core", true},
		{"10.0.9.5", 10, "corp", true}, // VLAN wins over CIDR
		{"10.0.9.5", 0, "core", true},  // untagged: CIDR
		{"fd00:1::10", 0, "corp", true},
		{"10.0.5.5", 0, "", false}, // no match: auto district
		{"10.0.5.5", 99, "", false},
	}
	for _, tc := range cases {
		got, ok := m.Match(netip.MustParseAddr(tc.addr), tc.vlan)
		if got != tc.want || ok != tc.ok {
			t.Errorf("Match(%s, vlan %d) = %q %v, want %q %v", tc.addr, tc.vlan, got, ok, tc.want, tc.ok)
		}
	}
}

func TestDistrictsHideCIDRsWhenAnonymized(t *testing.T) {
	m := example(t)
	plain := m.Districts(false)
	if len(plain) != 2 || plain[0].ID != "core" || plain[0].CIDRs[0] != "10.0.9.0/24" || plain[0].Kind != "server" {
		t.Fatalf("districts %+v", plain)
	}
	for _, d := range m.Districts(true) {
		if len(d.CIDRs) != 0 {
			t.Fatalf("anonymised district %s leaks %v", d.ID, d.CIDRs)
		}
	}
	var nilMatcher *Matcher
	if _, ok := nilMatcher.Match(netip.MustParseAddr("10.0.1.1"), 0); ok || nilMatcher.Districts(false) != nil {
		t.Fatal("nil matcher should match nothing")
	}
}
