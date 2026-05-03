# Repository Instructions

## Scope
- `Web4_Whitepaper.md` is the project whitepaper.
- Go foundation code lives under `core/crypto` and `core/canonical`; keep higher-level Web4 model logic out of these packages.
- Value/transaction types and structural validation live in `core/model`; local node acceptance policy lives in `core/policy`.
- Multi-node acceptance math lives in `core/sim`; it is an in-memory simulation layer, not networking.
- CLI entrypoint is `cmd/web4`; CLI implementation lives in `internal/cli` and currently only runs local acceptance dynamics scenarios.

## Working Notes
- Preserve the concise whitepaper style: short sections, direct claims, and minimal prose.
- Keep mathematical notation readable in plain Markdown; the existing document uses inline Unicode symbols such as `∈`, `θ_i`, and `τ`.
- Canonical encoding is a length-prefixed binary format in `core/canonical`; map keys must stay lexicographically sorted and field order must remain caller-defined.
- Crypto primitives are fixed to BLAKE3, Ed25519, and XChaCha20-Poly1305; do not substitute algorithms or add consensus/ledger behavior in this layer.
- Model IDs exclude signatures and ID fields; preserve deterministic canonical preimages when changing `core/model`.
- Policy must call model validation first and remain local/scored; do not add double-spend prevention, persistence, networking, blocks, mining, or global ledger behavior here.
- Simulation maps policy decisions to whitepaper math: `ACCEPT` is `accept_i(tx)`, acceptance ratio is `M(tx)`, and survival is threshold-based only.
- Simulation dynamics in `core/sim` include both global feedback and topology-based local feedback; keep them deterministic and separate from policy mutation.
- The CLI scenario presets construct acceptance scores directly; they do not create real model transactions yet.

## Verification
- Run `gofmt -w core/crypto/*.go core/canonical/*.go core/model/*.go core/policy/*.go core/sim/*.go` after Go edits.
- Run `go test ./...` for the full current test suite.
- Build the CLI with `go build -o web4 ./cmd/web4`; smoke-test with `./web4 sim acceptance --scenario partial --alpha 0.5 --tau 0.5 --steps 10` and optionally compare `--topology global` vs `--topology clustered`.
- For documentation-only edits, verify by reading the rendered Markdown or checking the changed section directly.
