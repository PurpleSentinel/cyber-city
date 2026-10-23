package enrich

import (
	"net"
	"os"
	"strings"
	"testing"
)

func TestLookup(t *testing.T) {
	cases := []struct {
		mac    string
		vendor string // substring; "" means not found
	}{
		{"b8:27:eb:12:34:56", "Raspberry Pi"},
		{"00:00:0c:aa:bb:cc", "Cisco"},
		{"24:0a:c4:00:00:01", "Espressif"},
		{"02:00:00:00:00:01", ""}, // locally administered
		{"01:00:5e:00:00:fb", ""}, // multicast
		{"ff:ff:ff:ff:ff:ff", ""}, // broadcast
	}
	for _, tc := range cases {
		mac, err := net.ParseMAC(tc.mac)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := Lookup(mac)
		switch {
		case tc.vendor == "" && ok:
			t.Errorf("%s: got %q, want not found", tc.mac, got)
		case tc.vendor != "" && (!ok || !strings.Contains(got, tc.vendor)):
			t.Errorf("%s: got %q (%v), want containing %q", tc.mac, got, ok, tc.vendor)
		}
	}
	if _, ok := Lookup(net.HardwareAddr{0xb8}); ok {
		t.Error("short address should not match")
	}
}

func TestFindPrefix(t *testing.T) {
	p, ok := FindPrefix("Raspberry Pi")
	if !ok {
		t.Fatal("Raspberry Pi prefix not found")
	}
	if v, _ := Lookup(append(p, 0, 0, 1)); !strings.Contains(v, "Raspberry Pi") {
		t.Fatalf("round trip vendor = %q", v)
	}
	if _, ok := FindPrefix("No Such Vendor Corp 12345"); ok {
		t.Fatal("unexpected match")
	}
}

func TestTableIsSortedAndConsistent(t *testing.T) {
	if len(ouiPrefixes) != len(ouiVendorIndex) {
		t.Fatalf("table lengths differ: %d vs %d", len(ouiPrefixes), len(ouiVendorIndex))
	}
	for i := 1; i < len(ouiPrefixes); i++ {
		if ouiPrefixes[i-1] >= ouiPrefixes[i] {
			t.Fatalf("prefixes not strictly sorted at %d", i)
		}
	}
	for i, idx := range ouiVendorIndex {
		if int(idx) >= len(ouiVendorNames) {
			t.Fatalf("index %d out of range at %d", idx, i)
		}
	}
}

func TestGeneratedFileSize(t *testing.T) {
	info, err := os.Stat("oui_table.go")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 3<<20 {
		t.Fatalf("oui_table.go is %d bytes, limit 3 MB (docs/TASKS.md T0.6)", info.Size())
	}
}
