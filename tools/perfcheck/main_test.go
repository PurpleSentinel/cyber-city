package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCheck(t *testing.T) {
	dir := t.TempDir()
	base := write(t, dir, "baseline.json", `{"replay_pkts_per_s": 200000, "frame_ms_p50": 100, "frame_ms_p95": 200}`)
	bench := write(t, dir, "bench.txt", strings.Join([]string{
		"goos: linux",
		"BenchmarkReplay-4   10   300000000 ns/op   150000 pkts/s   27000000 B/op",
		"BenchmarkReplay-4   10   300000000 ns/op   170000 pkts/s   27000000 B/op",
		"BenchmarkReplay-4   10   300000000 ns/op   190000 pkts/s   27000000 B/op",
		"PASS",
	}, "\n"))
	e2e := write(t, dir, "perf.json", `{"hosts": 2000, "frame_ms_p50": 110, "frame_ms_p95": 260, "quality": "low"}`)

	rows, err := check(base, bench, e2e, 0.2)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]row{}
	for _, r := range rows {
		got[r.name] = r
	}
	// Median 170k vs 200k: -15%, within tolerance.
	if r := got["replay_pkts_per_s"]; r.got != 170000 || r.regressed {
		t.Errorf("pkts/s row %+v", r)
	}
	if r := got["frame_ms_p50"]; r.regressed || r.change > -0.09 || r.change < -0.11 {
		t.Errorf("p50 row %+v", r)
	}
	// 260 ms vs 200 ms: 30% slower.
	if r := got["frame_ms_p95"]; !r.regressed {
		t.Errorf("p95 should regress: %+v", r)
	}
	var sb strings.Builder
	report(&sb, rows, 0.2)
	if !strings.Contains(sb.String(), "**regression**") {
		t.Errorf("report:\n%s", sb.String())
	}
}

func TestMissingInputsAreSkipped(t *testing.T) {
	dir := t.TempDir()
	base := write(t, dir, "baseline.json", `{"replay_pkts_per_s": 200000}`)
	rows, err := check(base, "", filepath.Join(dir, "absent.json"), 0.2)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.have || r.regressed {
			t.Errorf("row %+v should be skipped", r)
		}
	}
}
