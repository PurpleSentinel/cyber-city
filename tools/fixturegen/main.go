// SPDX-License-Identifier: MIT

// Command fixturegen writes the synthetic capture fixtures and test GeoIP
// database under testdata/ (docs/TESTING.md "Fixtures"). Output is
// byte-identical on every run; CI fails if the committed files differ.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

func main() {
	out := flag.String("out", "testdata", "output directory")
	flag.Parse()
	results, err := generate(*out)
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range results {
		fmt.Printf("%-12s %7d packets %8.1fs %9d bytes\n", r.name, r.packets, r.span.Seconds(), r.size)
	}
}

type result struct {
	name    string
	packets int
	span    time.Duration
	size    int
}

// generate writes every fixture into dir/pcap and the GeoIP database into dir/geo.
func generate(dir string) ([]result, error) {
	v, err := loadVendors()
	if err != nil {
		return nil, err
	}
	for _, sub := range []string{"pcap", "geo"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil { //nolint:gosec // test data directory
			return nil, err
		}
	}
	var results []result
	for _, f := range fixtures {
		var buf bytes.Buffer
		n, span, err := f.build(v).write(&buf)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "pcap", f.name+".pcap"), buf.Bytes(), 0o644); err != nil { //nolint:gosec // test data
			return nil, err
		}
		results = append(results, result{f.name, n, span, buf.Len()})
	}
	var geo bytes.Buffer
	if err := writeGeo(&geo); err != nil {
		return nil, fmt.Errorf("geo: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "geo", "test-city.mmdb"), geo.Bytes(), 0o644); err != nil { //nolint:gosec // test data
		return nil, err
	}
	return results, nil
}
