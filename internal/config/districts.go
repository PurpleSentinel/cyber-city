package config

import (
	"bytes"
	"fmt"
	"net/netip"
	"os"
	"regexp"

	"go.yaml.in/yaml/v3"
)

// DistrictDef is one entry of a districts file (docs/CONFIG.md "Districts file").
type DistrictDef struct {
	ID    string   `yaml:"id"`
	Name  string   `yaml:"name"`
	Kind  string   `yaml:"kind"`
	CIDRs []string `yaml:"cidrs"`
	VLANs []int    `yaml:"vlans"`

	// Prefixes holds CIDRs parsed and masked by LoadDistricts.
	Prefixes []netip.Prefix `yaml:"-"`
}

var districtID = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// LoadDistricts reads and validates a districts file, preserving its order
// (first match wins).
func LoadDistricts(path string) ([]DistrictDef, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied path
	if err != nil {
		return nil, fmt.Errorf("districts: %w", err)
	}
	var file struct {
		Districts []DistrictDef `yaml:"districts"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("districts %s: %w", path, err)
	}
	if err := validateDistricts(file.Districts); err != nil {
		return nil, fmt.Errorf("districts %s: %w", path, err)
	}
	return file.Districts, nil
}

func validateDistricts(ds []DistrictDef) error {
	seen := map[string]bool{}
	for i := range ds {
		d := &ds[i]
		switch {
		case !districtID.MatchString(d.ID):
			return fmt.Errorf("district %d: id %q must match [a-z0-9-]{1,32}", i+1, d.ID)
		case d.ID == "outlands":
			return fmt.Errorf("district %d: id %q is reserved", i+1, d.ID)
		case seen[d.ID]:
			return fmt.Errorf("district %q defined twice", d.ID)
		case d.Name == "":
			return fmt.Errorf("district %q: name required", d.ID)
		case d.Kind != "lan" && d.Kind != "server":
			return fmt.Errorf("district %q: kind %q must be lan or server", d.ID, d.Kind)
		case len(d.CIDRs) == 0 && len(d.VLANs) == 0:
			return fmt.Errorf("district %q: needs cidrs or vlans", d.ID)
		}
		seen[d.ID] = true
		d.Prefixes = d.Prefixes[:0]
		for _, c := range d.CIDRs {
			p, err := netip.ParsePrefix(c)
			if err != nil {
				return fmt.Errorf("district %q: %w", d.ID, err)
			}
			d.Prefixes = append(d.Prefixes, p.Masked())
		}
		for _, v := range d.VLANs {
			if v < 1 || v > 4094 {
				return fmt.Errorf("district %q: vlan %d out of range 1..4094", d.ID, v)
			}
		}
	}
	return nil
}
