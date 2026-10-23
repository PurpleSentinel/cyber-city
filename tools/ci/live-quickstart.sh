#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# CI check (docs/TASKS.md T5.3, T5.5): the live quick start from the README
# and docs/SETUP.md captures as an unprivileged user after `make setcap`.
set -euo pipefail
cd "$(dirname "$0")/../.."
dir=$(mktemp -d)
trap 'kill "${pid:-}" 2>/dev/null || true; rm -rf "$dir"' EXIT

make setcap
getcap bin/cybercity
bin/cybercity --listen 127.0.0.1:0 --url-file "$dir/url" >"$dir/log" 2>&1 &
pid=$!
for _ in $(seq 100); do [ -s "$dir/url" ] && break; sleep 0.1; done
sleep 1
if ! kill -0 "$pid" 2>/dev/null || [ ! -s "$dir/url" ]; then
  echo "FAIL: live capture without root did not start"; cat "$dir/log"; exit 1
fi
[ "$(stat -c %u "/proc/$pid")" = "$(id -u)" ] || { echo "FAIL: not running as the invoking user"; exit 1; }
grep -i "capture" "$dir/log" | head -3 || true
kill "$pid"
wait "$pid" 2>/dev/null || true

# docs/SETUP.md "Choosing what to capture".
# Via a file: under pipefail, `| grep -q` can kill the writer with SIGPIPE.
bin/cybercity --list-ifaces >"$dir/ifaces"
grep -q '^ *lo ' "$dir/ifaces" || { echo "FAIL: lo not listed"; cat "$dir/ifaces"; exit 1; }
rm -f "$dir/url"
bin/cybercity --iface lo --bpf "tcp or udp" --no-promisc --listen 127.0.0.1:0 --url-file "$dir/url" >"$dir/log2" 2>&1 &
pid=$!
for _ in $(seq 100); do [ -s "$dir/url" ] && break; sleep 0.1; done
sleep 1
kill -0 "$pid" 2>/dev/null && [ -s "$dir/url" ] || { echo "FAIL: filtered capture on lo"; cat "$dir/log2"; exit 1; }
echo "live quick start OK (uid $(id -u))"
