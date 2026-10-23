#!/usr/bin/env bash
# CI check (docs/TASKS.md T5.3): runs the README's no-root commands against a
# built bin/cybercity. `make setup` and `make build` run in the e2e job before
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

echo "README commands OK"
