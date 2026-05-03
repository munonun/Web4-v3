# Repository Guidelines

## Project Structure & Module Organization

This is a Go 1.22 module named `web4-v3`. The CLI entrypoint is in `cmd/web4`, with command implementation in `internal/cli`. Core packages are organized by responsibility: `core/crypto` for BLAKE3, Ed25519, and XChaCha20-Poly1305 primitives; `core/canonical` for deterministic binary encoding; `core/model` for value and transaction types; `core/policy` for local acceptance rules; and `core/sim` for deterministic in-memory acceptance simulations. The whitepaper lives in `Web4_Whitepaper.md`. The root `web4` file is a built CLI artifact, not source.

## Build, Test, and Development Commands

- `go test ./...`: runs the full package test suite.
- `go build -o web4 ./cmd/web4`: builds the CLI binary at the repository root.
- `./web4 sim acceptance --scenario partial --alpha 0.5 --tau 0.5 --steps 10`: smoke-tests the simulation CLI.
- `gofmt -w ./cmd ./core ./internal`: formats Go source after edits.

If the default Go cache is read-only in a sandboxed environment, run tests with a writable cache, for example `GOCACHE=/tmp/go-build go test ./...`.

## Coding Style & Naming Conventions

Use standard Go formatting and idioms. Keep package names short, lowercase, and aligned with existing directories. Export only API surface needed across packages; prefer unexported helpers inside package boundaries. Test files use the standard `*_test.go` suffix and should sit beside the code they exercise. Preserve deterministic behavior in canonical encoding, model IDs, policy scoring, and simulation dynamics.

## Testing Guidelines

Tests use Go’s built-in `testing` package. Add focused table-driven tests when behavior has multiple cases, especially for validation, encoding, cryptographic wrappers, and simulation thresholds. Run `go test ./...` before submitting changes. For CLI changes, also build the binary and run at least one `sim acceptance` smoke test.

## Commit & Pull Request Guidelines

This checkout does not include usable Git history, so use concise imperative commit messages such as `Add topology simulation test` or `Fix canonical map ordering`. Pull requests should describe the behavior changed, list verification commands run, and link related issues or design notes. Include terminal output or screenshots only when they clarify CLI behavior or user-facing output.

## Agent-Specific Instructions

Keep foundational layers separated. Do not add ledger, persistence, networking, mining, or consensus behavior to `core/crypto`, `core/canonical`, `core/model`, or `core/policy`. Treat `core/sim` as deterministic simulation code, not production networking.
