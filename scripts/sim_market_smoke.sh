#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/go-build}"

echo "building web4"
GOCACHE="$GOCACHE" go build -buildvcs=false -o web4 ./cmd/web4

run_case() {
  local scenario="$1"
  local topology="$2"

  echo
  echo "=== ${scenario}/${topology} ==="
  ./web4 sim market \
    --scenario "$scenario" \
    --topology "$topology" \
    --steps 100
}

run_case split full
run_case clustered clustered
run_case collapse full
