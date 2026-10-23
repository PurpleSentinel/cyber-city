package main

import (
	"io"
	"net"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

// geoRecords maps the documentation ranges to countries and city locations
// (docs/TESTING.md "Fixtures").
var geoRecords = []struct {
	cidr     string
	country  string
	lat, lon float64
}{
	{"203.0.113.0/24", "JP", 35.68, 139.69},
	{"198.51.100.0/24", "US", 40.71, -74.0},
	{"192.0.2.0/24", "DE", 52.52, 13.40},
}

// writeGeo writes a tiny GeoLite2-City-shaped database.
func writeGeo(w io.Writer) error {
	tree, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType:            "GeoLite2-City",
		Description:             map[string]string{"en": "cybercity test fixture"},
		RecordSize:              24,
		IncludeReservedNetworks: true, // the fixtures use documentation ranges
		BuildEpoch:              baseTime.Unix(),
	})
	if err != nil {
		return err
	}
	for _, r := range geoRecords {
		_, network, err := net.ParseCIDR(r.cidr)
		if err != nil {
			return err
		}
		record := mmdbtype.Map{
			"country":  mmdbtype.Map{"iso_code": mmdbtype.String(r.country)},
			"location": mmdbtype.Map{"latitude": mmdbtype.Float64(r.lat), "longitude": mmdbtype.Float64(r.lon)},
		}
		if err := tree.Insert(network, record); err != nil {
			return err
		}
	}
	_, err = tree.WriteTo(w)
	return err
}
