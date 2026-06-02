#!/usr/bin/env bash
# bootstrap-relay.sh — one-command local install for Relay.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/tabladrum/grove-suite/main/relay/scripts/bootstrap-relay.sh | bash
#   # or, from a local checkout:
#   ./relay/scripts/bootstrap-relay.sh [workspace-dir]
#
# What it does:
#   1. Builds (or installs) the `relay` binary into $GOBIN.
#   2. Runs `relay local init` against the target workspace:
#        - installs Stage-2 analyzer binaries (jre, sonarlint-ls, plugins,
#          semgrep hints, gitleaks hints, ...).
#        - installs the relay-managed git pre-commit + pre-push hooks.
#        - writes agent steering files (CLAUDE.md, AGENTS.md, GEMINI.md, ...).
#        - registers the relay MCP server with every detected client.
#   3. Prints next-step hints.
#
# Idempotent: re-running upgrades without disturbing unrelated config.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RELAY_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
WORKSPACE_DIR="${1:-$PWD}"

if ! command -v go >/dev/null 2>&1; then
  echo "error: go is required but was not found in PATH" >&2
  exit 1
fi

GOBIN="${GOBIN:-$(go env GOPATH)/bin}"
mkdir -p "$GOBIN"

if [[ ! -f "$RELAY_ROOT/go.mod" ]]; then
  echo "error: relay go.mod not found at $RELAY_ROOT" >&2
  exit 1
fi

echo "[bootstrap-relay] building relay from $RELAY_ROOT"
(
  cd "$RELAY_ROOT"
  go build -o "$GOBIN/relay" ./cmd/relay
)
echo "[bootstrap-relay] installed: $GOBIN/relay"

if [[ ":$PATH:" != *":$GOBIN:"* ]]; then
  echo "[bootstrap-relay] note: $GOBIN is not in PATH for this shell"
  echo "[bootstrap-relay] run: export PATH=\"$GOBIN:\$PATH\""
fi

echo "[bootstrap-relay] running: relay local init --repo $WORKSPACE_DIR"
"$GOBIN/relay" local init --repo "$WORKSPACE_DIR"

echo
echo "[bootstrap-relay] done. Next:"
echo "  relay doctor                   # verify environment"
echo "  relay daemon start             # background MCP daemon"
echo "  relay submit ...               # gate a change set"
