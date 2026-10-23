#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# CI smoke test (T1.8): start the binary as root for live capture and check it
# drops root once the capture is open (docs/CONFIG.md "Startup order").
# Usage: tools/ci/privdrop-smoke.sh path/to/cybercity
set -euo pipefail
bin=${1:?usage: privdrop-smoke.sh path/to/cybercity}
dir=$(mktemp -d)
url_file="$dir/url.txt"
log="$dir/server.log"

sudo "$bin" --listen 127.0.0.1:0 --url-file "$url_file" >"$log" 2>&1 &
for _ in $(seq 100); do [ -s "$url_file" ] && break; sleep 0.1; done
sleep 1 # let the privilege drop complete after the URL file is written

pid=$(pgrep -x cybercity || true)
cleanup() { [ -n "$pid" ] && sudo kill "$pid" 2>/dev/null || true; }
trap cleanup EXIT
if [ -z "$pid" ]; then
  echo "cybercity is not running"; cat "$log"; exit 1
fi

read -r _ ruid euid _ < <(grep '^Uid:' "/proc/$pid/status")
echo "cybercity pid=$pid ruid=$ruid euid=$euid"
if [ "$ruid" = 0 ] || [ "$euid" = 0 ]; then
  echo "FAIL: still running as root"; cat "$log"; exit 1
fi
grep -q "dropped root privileges" "$log" || { echo "FAIL: no drop logged"; cat "$log"; exit 1; }

# The URL file was handed to the invoking (sudo) user and is private.
[ "$(stat -c '%u %a' "$url_file")" = "$(id -u) 600" ] || { echo "FAIL: url file owner/mode $(stat -c '%u %a' "$url_file")"; exit 1; }
url=$(head -1 "$url_file")
code=$(curl -s -o /dev/null -w '%{http_code}' "${url%%/auth*}/api/session")
[ "$code" = 401 ] || { echo "FAIL: /api/session without cookie returned $code"; exit 1; }
echo "OK: running unprivileged, url file private to $(id -un), API requires a session"
