#!/usr/bin/env bash
# Verify the read-only pre-registration archive against its recorded SHA-256 sums (D40).
# Usage: scripts/check-preregistration.sh
set -euo pipefail
here=$(cd "$(dirname "$0")/.." && pwd)
cd "$here/docs/preregistration"
sha256sum -c SHA256SUMS
