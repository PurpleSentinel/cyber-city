package enrich

import (
	"net/netip"
	"path/filepath"
	"testing"
)

// london is geo.home in the golden tests (docs/TESTING.md "Golden tests").
var london = [2]float64{51.5, -0.12}

func TestGeoSectorsFromTestDatabase(t *testing.T) {
	g, err := OpenGeo(filepath.Join("..", "..", "testdata", "geo", "test-city.mmdb"), london)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = g.Close() }()
	cases := map[string][2]string{
		"203.0.113.10":  {"NE", "JP"},
		"198.51.100.20": {"W", "US"},
		"192.0.2.30":    {"E", "DE"},
	}
	for ip, want := range cases {
		got := g.Lookup(netip.MustParseAddr(ip))
		if got == nil || got.Sector != want[0] || got.Country != want[1] {
			t.Errorf("%s: got %+v, want %v", ip, got, want)
		}
	}
	if got := g.Lookup(netip.MustParseAddr("8.8.8.8")); got != nil {
		t.Errorf("address outside the database: got %+v, want nil", got)
	}
	var none *Geo
	if none.Lookup(netip.MustParseAddr("203.0.113.10")) != nil {
		t.Error("nil Geo should return nil")
	}
}

func TestSectorBoundaries(t *testing.T) {
	cases := map[float64]string{0: "N", 22.4: "N", 22.6: "NE", 90: "E", 180: "S", 270: "W", 337.4: "NW", 337.6: "N", 359.9: "N"}
	for b, want := range cases {
		if got := SectorFor(b); got != want {
			t.Errorf("SectorFor(%v) = %s, want %s", b, got, want)
		}
	}
}
