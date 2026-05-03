#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/go-build}"
STEPS="${STEPS:-10000}"
OUT_DIR="${OUT_DIR:-out/substitution_matrix}"
ASSETS="${ASSETS:-SKUG,WEB4}"

mkdir -p "$OUT_DIR"

echo "building web4"
GOCACHE="$GOCACHE" go build -buildvcs=false -o web4 ./cmd/web4

scenarios=(multi-basic multi-compete multi-flight multi-coexist)
topologies=(full chain clustered)
utilities=(fixed random clustered)
alphas=(0.05 0.2 0.5)
spreads=(0.01 0.05)

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

echo "scenario topology utility alpha spread dominant_holdings dominant_flow flow_concentration SKUG_share WEB4_share SKUG_flow_share WEB4_flow_share switch_count cross_asset_trades total_volume"

for scenario in "${scenarios[@]}"; do
  for topology in "${topologies[@]}"; do
    for utility in "${utilities[@]}"; do
      for alpha in "${alphas[@]}"; do
        for spread in "${spreads[@]}"; do
          name="${scenario}_${topology}_${utility}_alpha-${alpha}_spread-${spread}_steps-${STEPS}"
          csv_path="${OUT_DIR}/${name}.csv"
          json_path="${OUT_DIR}/${name}.json"
          txt_path="${OUT_DIR}/${name}.txt"

          ./web4 sim market \
            --scenario "$scenario" \
            --multi-asset \
            --assets "$ASSETS" \
            --topology "$topology" \
            --steps "$STEPS" \
            --alpha "$alpha" \
            --spread "$spread" \
            --enable-demand \
            --enable-cycle \
            --enable-substitution \
            --utility-mode "$utility" \
            --csv "$csv_path" > "$txt_path"

          ./web4 sim market \
            --scenario "$scenario" \
            --multi-asset \
            --assets "$ASSETS" \
            --topology "$topology" \
            --steps "$STEPS" \
            --alpha "$alpha" \
            --spread "$spread" \
            --enable-demand \
            --enable-cycle \
            --enable-substitution \
            --utility-mode "$utility" \
            --json > "$json_path"

          dominant_holdings="$(summary_value dominant_asset_by_holdings "$txt_path")"
          dominant_flow="$(summary_value dominant_asset_by_flow "$txt_path")"
          flow_concentration="$(summary_value flow_concentration "$txt_path")"
          skug_share="$(summary_value SKUG_share "$txt_path")"
          web4_share="$(summary_value WEB4_share "$txt_path")"
          skug_flow_share="$(summary_value SKUG_flow_share "$txt_path")"
          web4_flow_share="$(summary_value WEB4_flow_share "$txt_path")"
          switch_count="$(summary_value switch_count "$txt_path")"
          cross_asset_trades="$(summary_value total_cross_asset_trades "$txt_path")"
          total_volume="$(summary_value total_volume "$txt_path")"

          printf '%s %s %s %s %s %s %s %s %s %s %s %s %s %s %s\n' \
            "$scenario" "$topology" "$utility" "$alpha" "$spread" \
            "$dominant_holdings" "$dominant_flow" "$flow_concentration" \
            "$skug_share" "$web4_share" "$skug_flow_share" "$web4_flow_share" \
            "$switch_count" "$cross_asset_trades" "$total_volume"
        done
      done
    done
  done
done

echo
echo "outputs: ${OUT_DIR}"
