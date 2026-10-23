// SPDX-License-Identifier: MIT

package enrich

import (
	"fmt"
	"math"
	"net/netip"

	"github.com/oschwald/maxminddb-golang/v2"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// sectors in compass order, 45° each, centred on N (docs/CONFIG.md "Geo sector").
var sectors = [...]string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}

// Geo places external hosts in compass sectors from an offline GeoLite2-City
// database (docs/DECISIONS.md D10). Nothing is fetched over the network.
type Geo struct {
	db    *maxminddb.Reader
	home  [2]float64
	cache map[netip.Addr]*protocol.Geo
}

type cityRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	Location struct {
		Latitude  *float64 `maxminddb:"latitude"`
		Longitude *float64 `maxminddb:"longitude"`
	} `maxminddb:"location"`
}

// OpenGeo opens a GeoLite2-City .mmdb; bearings are measured from home [lat, lon].
func OpenGeo(path string, home [2]float64) (*Geo, error) {
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("geoip %s: %w", path, err)
	}
	return &Geo{db: db, home: home, cache: make(map[netip.Addr]*protocol.Geo)}, nil
}

// Lookup returns the sector and country of addr, or nil when the database has
// no location for it.
func (g *Geo) Lookup(addr netip.Addr) *protocol.Geo {
	if g == nil {
		return nil
	}
	if v, ok := g.cache[addr]; ok {
		return v
	}
	var rec cityRecord
	var out *protocol.Geo
	if err := g.db.Lookup(addr).Decode(&rec); err == nil && rec.Location.Latitude != nil && rec.Location.Longitude != nil {
		b := Bearing(g.home[0], g.home[1], *rec.Location.Latitude, *rec.Location.Longitude)
		out = &protocol.Geo{Sector: SectorFor(b), Country: rec.Country.ISOCode}
	}
	g.cache[addr] = out
	return out
}

// Forget drops a removed host's cached result.
func (g *Geo) Forget(addr netip.Addr) {
	if g != nil {
		delete(g.cache, addr)
	}
}

// Close releases the database.
func (g *Geo) Close() error {
	if g == nil {
		return nil
	}
	return g.db.Close()
}

// Bearing is the initial great-circle bearing in degrees [0, 360) from
// (lat1, lon1) to (lat2, lon2).
func Bearing(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180
	φ1, φ2, Δλ := lat1*rad, lat2*rad, (lon2-lon1)*rad
	y := math.Sin(Δλ) * math.Cos(φ2)
	x := math.Cos(φ1)*math.Sin(φ2) - math.Sin(φ1)*math.Cos(φ2)*math.Cos(Δλ)
	return math.Mod(math.Atan2(y, x)/rad+360, 360)
}

// SectorFor quantises a bearing to one of 8 sectors centred on N, NE, ….
func SectorFor(bearing float64) string {
	return sectors[int(math.Round(bearing/45))%8]
}
