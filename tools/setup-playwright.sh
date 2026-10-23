#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Installs Playwright's Chromium for the e2e tests (`make setup`). Playwright
# can install Chromium's system libraries only through apt-get, so --with-deps
# is used on Debian/Ubuntu (and CI) and skipped elsewhere, e.g. Fedora.
set -euo pipefail
cd "$(dirname "$0")/../web"
if command -v apt-get >/dev/null 2>&1; then
  npx playwright install --with-deps chromium
else
  npx playwright install chromium
  cat <<'EOF'
note: system libraries for Chromium were not installed (no apt-get). Only the
      e2e tests need them; building and running cybercity does not. If
      `make e2e` fails to launch the browser on Fedora, install:
      sudo dnf install nss atk at-spi2-atk cups-libs libdrm libXcomposite \
        libXdamage libXrandr mesa-libgbm pango alsa-lib libxkbcommon
EOF
fi
