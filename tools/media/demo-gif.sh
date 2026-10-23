#!/usr/bin/env bash
# Turns the e2e recording (web/e2e/artifacts/demo.webm, from
# web/e2e/record.spec.ts) into docs/media/demo.gif: 10 s around the demo's
# port scan, 720 px wide, 8 fps. Needs ffmpeg. Run via `make media`.
set -euo pipefail
cd "$(dirname "$0")/../.."
art=web/e2e/artifacts
scan=$(sed -n 's/.*"scan_s": *\([0-9.]*\).*/\1/p' "$art/demo.json")
start=$(awk -v s="$scan" 'BEGIN { t = s - 4; if (t < 0) t = 0; print t }')
filters="fps=8,scale=720:-1:flags=lanczos"
ffmpeg -v error -y -ss "$start" -t 10 -i "$art/demo.webm" \
  -vf "$filters,split[a][b];[a]palettegen=max_colors=96:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle" \
  docs/media/demo.gif
ls -l docs/media/demo.gif
