#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# CI check (docs/TASKS.md T5.3): the README's live quick start,
# `make setcap && bin/cybercity`, captures as an unprivileged user.
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
echo "live quick start OK (uid $(id -u))"
