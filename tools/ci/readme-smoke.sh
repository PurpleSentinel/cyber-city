#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# CI check (docs/TASKS.md T5.3, T5.5): runs the no-root commands from the
# README and docs/SETUP.md against a built bin/cybercity. `make setup` and `make build` run in the e2e job before
# this; the live quick start runs in the live-smoke job (live-quickstart.sh).
set -euo pipefail
cd "$(dirname "$0")/../.."
bin=bin/cybercity
dir=$(mktemp -d)
trap 'kill "${pid:-}" 2>/dev/null || true; rm -rf "$dir"' EXIT

echo "== bin/cybercity --read testdata/pcap/demo.pcap --loop"
"$bin" --read testdata/pcap/demo.pcap --loop --listen 127.0.0.1:0 --url-file "$dir/url" >"$dir/log" 2>&1 &
pid=$!
for _ in $(seq 100); do [ -s "$dir/url" ] && break; sleep 0.1; done
url=$(cat "$dir/url")
case "$url" in http://127.0.0.1:*/auth\?token=*) echo "ok: $url" | sed 's/token=.*/token=…/' ;; *) echo "FAIL: url '$url'"; cat "$dir/log"; exit 1 ;; esac
grep -q "ready" "$dir/log" || { echo "FAIL: no ready line"; cat "$dir/log"; exit 1; }
kill "$pid"
wait "$pid" 2>/dev/null || true

echo "== bin/cybercity --read testdata/pcap/demo.pcap --dump-json -"
"$bin" --read testdata/pcap/demo.pcap --dump-json - >"$dir/dump.jsonl"
first=$(head -1 "$dir/dump.jsonl")
case "$first" in *'"type":"hello"'*) echo "ok: hello first" ;; *) echo "FAIL: first line ${first:0:120}"; exit 1 ;; esac

echo "== bin/cybercity --list-ifaces"
"$bin" --list-ifaces >"$dir/ifaces"
head -5 "$dir/ifaces"

echo "== bin/cybercity --version"
"$bin" --version

echo "== bin/cybercity --config cybercity.yaml (copied from the example)"
cp config/cybercity.example.yaml "$dir/cybercity.yaml"
"$bin" --config "$dir/cybercity.yaml" --read testdata/pcap/demo.pcap --dump-json - >/dev/null

echo "== bin/cybercity --districts config/districts.example.yaml --read testdata/pcap/vlan.pcap"
"$bin" --districts config/districts.example.yaml --read testdata/pcap/vlan.pcap --dump-json - >"$dir/vlan.jsonl"
grep -q '"name":"Server Core"' "$dir/vlan.jsonl" || { echo "FAIL: named district missing"; exit 1; }

echo "== bin/cybercity --anonymize --read testdata/pcap/demo.pcap --dump-json -"
"$bin" --anonymize --read testdata/pcap/demo.pcap --dump-json - >"$dir/anon.jsonl"
if grep -Eq '"ip":"|10\.0\.[0-9]+\.[0-9]+' "$dir/anon.jsonl"; then echo "FAIL: address in anonymized output"; exit 1; fi
grep -q '"id":"anon-' "$dir/anon.jsonl" || { echo "FAIL: no pseudonyms"; exit 1; }

echo "README and SETUP commands OK"
