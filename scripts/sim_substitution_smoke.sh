#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/go-build}"
ASSETS="${ASSETS:-SKUG,WEB4}"

echo "building web4"
GOCACHE="$GOCACHE" go build -buildvcs=false -o web4 ./cmd/web4

run_case() {
  local scenario="$1"
  local topology="$2"
  local utility="$3"

  echo
  echo "=== ${scenario}/${topology}/${utility} ==="
  ./web4 sim market \
    --scenario "$scenario" \
    --multi-asset \
    --assets "$ASSETS" \
    --topology "$topology" \
    --steps 100 \
    --enable-demand \
    --enable-cycle \
    --enable-substitution \
    --utility-mode "$utility"
}

run_case multi-basic full fixed
run_case multi-flight full fixed
run_case multi-coexist clustered clustered
