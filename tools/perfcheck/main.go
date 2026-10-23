// SPDX-License-Identifier: MIT

// Command perfcheck compares performance results with the stored baseline
// (docs/TASKS.md T5.4) and exits non-zero on a regression beyond the
// tolerance. Inputs: `go test -bench BenchmarkReplay` output and the e2e
// perf.json. Missing inputs are reported and skipped.
//
//	perfcheck -baseline testdata/perf/baseline.json -bench bench.txt -e2e web/e2e/artifacts/perf.json
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
)

// metric describes how to read and judge one number.
type metric struct {
	name         string
	higherBetter bool
	unit         string
}

var metrics = []metric{
	{"replay_pkts_per_s", true, "pkts/s"},
	{"frame_ms_p50", false, "ms"},
	{"frame_ms_p95", false, "ms"},
}

type row struct {
	metric
	base, got float64
	have      bool
	change    float64 // relative, positive = better
	regressed bool
}

func main() {
	baseline := flag.String("baseline", "testdata/perf/baseline.json", "stored baseline")
	bench := flag.String("bench", "", "go test -bench output")
	e2e := flag.String("e2e", "", "e2e perf.json")
	tolerance := flag.Float64("tolerance", 0.2, "allowed relative regression")
	flag.Parse()

	rows, err := check(*baseline, *bench, *e2e, *tolerance)
	if err != nil {
		fmt.Fprintln(os.Stderr, "perfcheck:", err)
		os.Exit(2)
	}
	out := io.Writer(os.Stdout)
	if p := os.Getenv("GITHUB_STEP_SUMMARY"); p != "" {
		if f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600); err == nil { //nolint:gosec // CI-provided path
			defer func() { _ = f.Close() }()
			out = io.MultiWriter(os.Stdout, f)
		}
	}
	report(out, rows, *tolerance)
	for _, r := range rows {
		if r.regressed {
			os.Exit(1)
		}
	}
}

func check(baselinePath, benchPath, e2ePath string, tolerance float64) ([]row, error) {
	base, err := readJSON(baselinePath)
	if err != nil {
		return nil, err
	}
	got := map[string]float64{}
	if benchPath != "" {
		v, err := benchPktsPerSec(benchPath)
		if err != nil {
			return nil, err
		}
		got["replay_pkts_per_s"] = v
	}
	if e2ePath != "" {
		m, err := readJSON(e2ePath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		for k, v := range m {
			got[k] = v
		}
	}
	var rows []row
	for _, m := range metrics {
		r := row{metric: m, base: base[m.name]}
		r.got, r.have = got[m.name]
		if r.have && r.base > 0 {
			if m.higherBetter {
				r.change = (r.got - r.base) / r.base
			} else {
				r.change = (r.base - r.got) / r.base
			}
			r.regressed = r.change < -tolerance
		}
		rows = append(rows, r)
	}
	return rows, nil
}

func report(w io.Writer, rows []row, tolerance float64) {
	_, _ = fmt.Fprintf(w, "### Performance (regression limit %.0f%%)\n\n", tolerance*100)
	_, _ = fmt.Fprintln(w, "| Metric | Baseline | Current | Change | |")
	_, _ = fmt.Fprintln(w, "|---|---|---|---|---|")
	for _, r := range rows {
		if !r.have {
			_, _ = fmt.Fprintf(w, "| %s | %.4g %s | — | — | skipped (no input) |\n", r.name, r.base, r.unit)
			continue
		}
		verdict := "ok"
		if r.regressed {
			verdict = "**regression**"
		}
		_, _ = fmt.Fprintf(w, "| %s | %.4g %s | %.4g %s | %+.1f%% | %s |\n", r.name, r.base, r.unit, r.got, r.unit, r.change*100, verdict)
	}
}

// readJSON reads the numeric fields of a flat JSON object.
func readJSON(path string) (map[string]float64, error) {
	data, err := os.ReadFile(path) //nolint:gosec // paths come from the command line
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := map[string]float64{}
	for k, v := range raw {
		if f, ok := v.(float64); ok {
			out[k] = f
		}
	}
	return out, nil
}

// benchPktsPerSec returns the median pkts/s over BenchmarkReplay result lines.
func benchPktsPerSec(path string) (float64, error) {
	f, err := os.Open(path) //nolint:gosec // paths come from the command line
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	var vals []float64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "BenchmarkReplay") {
			continue
		}
		for i := 1; i < len(fields); i++ {
			if fields[i] == "pkts/s" {
				if v, err := strconv.ParseFloat(fields[i-1], 64); err == nil {
					vals = append(vals, v)
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	if len(vals) == 0 {
		return 0, fmt.Errorf("%s: no BenchmarkReplay pkts/s results", path)
	}
	slices.Sort(vals)
	return vals[len(vals)/2], nil
}
