#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/go-build}"
STEPS="${STEPS:-100000}"
OUT_DIR="${OUT_DIR:-out/substitution}"
ASSETS="${ASSETS:-SKUG,WEB4}"
PRICE_MODEL="${PRICE_MODEL:-pipeline}"

mkdir -p "$OUT_DIR"

echo "building web4"
GOCACHE="$GOCACHE" go build -buildvcs=false -o web4 ./cmd/web4

run_case() {
  local scenario="$1"
  local topology="$2"
  local utility="$3"
  local name="${scenario}_${topology}_${utility}_${STEPS}"
  local csv_path="${OUT_DIR}/${name}.csv"
  local json_path="${OUT_DIR}/${name}.json"
  local txt_path="${OUT_DIR}/${name}.txt"

  echo
  echo "=== ${scenario}/${topology}/${utility} steps=${STEPS} price_model=${PRICE_MODEL} ==="
  ./web4 sim market \
    --scenario "$scenario" \
    --multi-asset \
    --assets "$ASSETS" \
    --topology "$topology" \
    --steps "$STEPS" \
    --enable-demand \
    --enable-cycle \
    --enable-substitution \
    --utility-mode "$utility" \
    --price-model "$PRICE_MODEL" \
    --csv "$csv_path" > "$txt_path"

  ./web4 sim market \
    --scenario "$scenario" \
    --multi-asset \
    --assets "$ASSETS" \
    --topology "$topology" \
    --steps "$STEPS" \
    --enable-demand \
    --enable-cycle \
    --enable-substitution \
    --utility-mode "$utility" \
    --price-model "$PRICE_MODEL" \
    --json > "$json_path"

  cat "$txt_path"
  echo "csv: ${csv_path}"
  echo "json: ${json_path}"
  echo "summary: ${txt_path}"
}

run_case multi-basic full fixed
run_case multi-compete full fixed
run_case multi-flight full fixed
run_case multi-coexist clustered clustered
run_case multi-fragmented clustered clustered
run_case multi-basic full random
