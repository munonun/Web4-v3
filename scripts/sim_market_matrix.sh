#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/go-build}"
STEPS="${STEPS:-5000}"
OUT_DIR="${OUT_DIR:-out/market_matrix}"

mkdir -p "$OUT_DIR"

echo "building web4"
GOCACHE="$GOCACHE" go build -buildvcs=false -o web4 ./cmd/web4

scenarios=(split clustered collapse random fragmented cycle-basic cycle-fragmented cycle-random)
topologies=(full chain clustered)
alphas=(0.05 0.2 0.5)
min_profits=(0.01 0.05)

summary_value() {
  local key="$1"
  local path="$2"
  local value=""

  while IFS= read -r line; do
    case "$line" in
      "${key}:"*)
        value="${line#*: }"
        ;;
    esac
  done < "$path"

  printf '%s' "$value"
}

echo "scenario topology alpha min_profit converged fragmented collapsed total_trades final_spread"

for scenario in "${scenarios[@]}"; do
  for topology in "${topologies[@]}"; do
    for alpha in "${alphas[@]}"; do
      for min_profit in "${min_profits[@]}"; do
        name="${scenario}_${topology}_alpha-${alpha}_min-${min_profit}_steps-${STEPS}"
        csv_path="${OUT_DIR}/${name}.csv"
        json_path="${OUT_DIR}/${name}.json"
        summary_path="${OUT_DIR}/${name}.txt"
        extra_args=()
        case "$scenario" in
          cycle-*)
            extra_args=(--enable-demand --enable-cycle --consumption-rate 0.1 --production-rate 0.3)
            ;;
        esac

        ./web4 sim market \
          --scenario "$scenario" \
          --topology "$topology" \
          --steps "$STEPS" \
          --alpha "$alpha" \
          --min-profit "$min_profit" \
          --csv "$csv_path" \
          "${extra_args[@]}" > "$summary_path"

        ./web4 sim market \
          --scenario "$scenario" \
          --topology "$topology" \
          --steps "$STEPS" \
          --alpha "$alpha" \
          --min-profit "$min_profit" \
          --json \
          "${extra_args[@]}" > "$json_path"

        converged="$(summary_value converged "$summary_path")"
        fragmented="$(summary_value fragmented "$summary_path")"
        collapsed="$(summary_value collapsed "$summary_path")"
        total_trades="$(summary_value total_trades "$summary_path")"
        final_spread="$(summary_value price_spread "$summary_path")"

        printf '%s %s %s %s %s %s %s %s %s\n' \
          "$scenario" "$topology" "$alpha" "$min_profit" \
          "$converged" "$fragmented" "$collapsed" "$total_trades" "$final_spread"
      done
    done
  done
done

echo
echo "outputs: ${OUT_DIR}"
