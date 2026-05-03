#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/go-build}"
STEPS="${STEPS:-10000}"
OUT_DIR="${OUT_DIR:-out/market}"

mkdir -p "$OUT_DIR"

echo "building web4"
GOCACHE="$GOCACHE" go build -buildvcs=false -o web4 ./cmd/web4

run_case() {
  local scenario="$1"
  local topology="$2"
  shift 2
  local name="${scenario}_${topology}_${STEPS}"
  local csv_path="${OUT_DIR}/${name}.csv"
  local json_path="${OUT_DIR}/${name}.json"
  local summary_path="${OUT_DIR}/${name}.txt"

  echo
  echo "=== ${scenario}/${topology} steps=${STEPS} ==="
  ./web4 sim market \
    --scenario "$scenario" \
    --topology "$topology" \
    --steps "$STEPS" \
    --csv "$csv_path" \
    "$@" | tee "$summary_path"

  ./web4 sim market \
    --scenario "$scenario" \
    --topology "$topology" \
    --steps "$STEPS" \
    --json \
    "$@" > "$json_path"

  echo "csv: ${csv_path}"
  echo "json: ${json_path}"
  echo "summary: ${summary_path}"
}

run_case split full
run_case clustered clustered
run_case collapse full
run_case random full
run_case fragmented clustered
run_case cycle-basic full --enable-demand --enable-cycle --consumption-rate 0.1 --production-rate 0.3
run_case cycle-fragmented clustered --enable-demand --enable-cycle --consumption-rate 0.1 --production-rate 0.3
run_case cycle-random full --enable-demand --enable-cycle --consumption-rate 0.1 --production-rate 0.3
