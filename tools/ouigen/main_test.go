// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

const sample = `Registry,Assignment,Organization Name,Organization Address
MA-L,B827EB,Raspberry Pi Foundation,Cambridge
MA-L,00000C,"Cisco Systems,   Inc",San Jose
MA-L,00000D,"Cisco Systems, Inc",San Jose
MA-L,AAAAAA,Private,
MA-M,123456,Not MA-L,
`

func TestParseAndRender(t *testing.T) {
	entries, err := parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3 (Private and MA-M skipped)", len(entries))
	}
	if entries[0].prefix != 0x00000C || entries[2].prefix != 0xB827EB {
		t.Fatalf("not sorted: %+v", entries)
	}
	if entries[0].vendor != "Cisco Systems, Inc" {
		t.Fatalf("whitespace not collapsed: %q", entries[0].vendor)
	}
	code, err := render("enrich", entries)
	if err != nil {
		t.Fatal(err)
	}
	src := string(code)
	if !strings.Contains(src, "DO NOT EDIT") || strings.Count(src, `"Cisco Systems, Inc"`) != 1 {
		t.Fatalf("names not de-duplicated or header missing:\n%s", src)
	}
}

func TestParseRejectsUnexpectedHeader(t *testing.T) {
	if _, err := parse(strings.NewReader("a,b,c,d\nMA-L,000000,x,y\n")); err == nil {
		t.Fatal("expected header error")
	}
}
