#!/usr/bin/env bash
#
# Grove Suite one-command installer.
#
#   curl -fsSL https://tabladrum.github.io/provasign/assets/install.sh | bash
#
# Installs grove, prism, fuse, provasign from GitHub Releases, verifies checksums,
# puts them on PATH, and (optionally) initializes a project. Non-interactive:
# everything is driven by environment variables so it works in CI, Dockerfiles,
# and "one click" flows.
#
# Environment variables (all optional):
#   GROVE_SUITE_VERSION       release tag to install            (default: latest)
#   PROVASIGN_PRODUCTS      space-separated product list       (default: "grove prism fuse provasign")
#   GROVE_SUITE_INSTALL_DIR   install directory                  (default: $HOME/bin)
#   GROVE_SUITE_PROJECT       project dir to init after install  (default: none)
#   PROVASIGN_REPO          github owner/repo                  (default: tabladrum/provasign)
#
# This script NEVER skips checksum verification. Grove is always installed first
# (prism/fuse/provasign depend on it). Canonical copy: scripts/install.sh — keep
# docs/assets/install.sh byte-identical so the published URL matches.
set -euo pipefail

REPO="${PROVASIGN_REPO:-tabladrum/provasign}"
PRODUCTS="${PROVASIGN_PRODUCTS:-grove prism fuse provasign}"
INSTALL_DIR="${GROVE_SUITE_INSTALL_DIR:-$HOME/bin}"
PROJECT="${GROVE_SUITE_PROJECT:-}"

info()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
ok()    { printf '\033[1;32m✅\033[0m %s\n' "$*"; }
err()   { printf '\033[1;31m❌\033[0m %s\n' "$*" >&2; }
die()   { err "$*"; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "required tool not found: $1"; }
need curl

# ---- 1. Detect platform -----------------------------------------------------
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)        ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) die "unsupported architecture: $ARCH" ;;
esac
case "$OS" in
  linux|darwin) ;;
  *) die "unsupported OS: $OS (use the PowerShell instructions on Windows)" ;;
esac
info "Platform: ${OS}-${ARCH}"

# ---- 2. Resolve version -----------------------------------------------------
VERSION="${GROVE_SUITE_VERSION:-}"
if [ -z "$VERSION" ]; then
  info "Resolving latest release…"
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
  [ -n "$VERSION" ] || die "could not determine latest version; set GROVE_SUITE_VERSION"
fi
info "Version: ${VERSION}"

BASE="https://github.com/${REPO}/releases/download/${VERSION}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# ---- 3. Download checksums ---------------------------------------------------
info "Downloading checksums…"
curl -fSL "${BASE}/checksums.txt" -o "${TMP}/checksums.txt" || die "failed to fetch checksums.txt"

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}';
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# ---- 4. Download + verify each binary ---------------------------------------
# Always install grove first; preserve requested order otherwise.
ORDERED="grove"
for p in $PRODUCTS; do [ "$p" = "grove" ] || ORDERED="$ORDERED $p"; done
case " $PRODUCTS " in *" grove "*) ;; *) ORDERED="$(echo "$ORDERED" | sed 's/^grove //')";; esac

mkdir -p "$INSTALL_DIR"
for PRODUCT in $ORDERED; do
  FILE="${PRODUCT}-${VERSION}-${OS}-${ARCH}"
  info "Downloading ${FILE}…"
  curl -fSL "${BASE}/${FILE}" -o "${TMP}/${FILE}" || die "download failed: ${FILE}"

  EXPECTED="$(grep -E "  (\./)?${FILE}\$" "${TMP}/checksums.txt" | awk '{print $1}' | head -1)"
  [ -n "$EXPECTED" ] || die "no checksum entry for ${FILE}"
  ACTUAL="$(sha256 "${TMP}/${FILE}")"
  if [ "$EXPECTED" != "$ACTUAL" ]; then
    err "CHECKSUM MISMATCH for ${FILE} — refusing to install"
    err "  expected: $EXPECTED"
    err "  actual:   $ACTUAL"
    exit 1
  fi
  ok "${FILE}: checksum verified"

  mv "${TMP}/${FILE}" "${INSTALL_DIR}/${PRODUCT}"
  chmod +x "${INSTALL_DIR}/${PRODUCT}"
  [ "$OS" = "darwin" ] && xattr -d com.apple.quarantine "${INSTALL_DIR}/${PRODUCT}" 2>/dev/null || true
  ok "${PRODUCT} → ${INSTALL_DIR}/${PRODUCT}"
done

# ---- 5. Put install dir on PATH ---------------------------------------------
if [ "$OS" = "darwin" ] && [ "$INSTALL_DIR" = "$HOME/bin" ]; then
  if command -v sudo >/dev/null 2>&1 && [ ! -f /etc/paths.d/provasign ]; then
    echo "$INSTALL_DIR" | sudo tee /etc/paths.d/provasign >/dev/null 2>&1 \
      && ok "Registered ${INSTALL_DIR} system-wide via /etc/paths.d/provasign" || true
  fi
fi
SHELL_RC="$HOME/.zshrc"; [ -n "${BASH_VERSION:-}" ] && SHELL_RC="$HOME/.bashrc"
LINE="export PATH=\"${INSTALL_DIR}:\$PATH\""
if ! grep -qsF "$LINE" "$SHELL_RC" 2>/dev/null; then
  echo "$LINE" >> "$SHELL_RC" && info "Added ${INSTALL_DIR} to PATH in ${SHELL_RC}"
fi
export PATH="${INSTALL_DIR}:$PATH"

# ---- 6. Optional project init -----------------------------------------------
if [ -n "$PROJECT" ]; then
  [ -d "$PROJECT" ] || die "project dir not found: $PROJECT"
  info "Initializing project: $PROJECT"
  ( cd "$PROJECT"
    "${INSTALL_DIR}/grove" index . >/dev/null 2>&1 && ok "grove: indexed" || err "grove index failed"
    case " $PRODUCTS " in *" prism "*) "${INSTALL_DIR}/prism" init && "${INSTALL_DIR}/prism" index >/dev/null 2>&1 && ok "prism: initialized";; esac
    case " $PRODUCTS " in *" provasign "*) "${INSTALL_DIR}/provasign" init >/dev/null 2>&1 && ok "provasign: initialized (run 'provasign tools install' for analyzers)";; esac
  )
fi

# ---- 7. Smoke test ----------------------------------------------------------
info "Smoke test:"
for PRODUCT in $ORDERED; do
  if "${INSTALL_DIR}/${PRODUCT}" version >/dev/null 2>&1; then
    ok "${PRODUCT} $("${INSTALL_DIR}/${PRODUCT}" version 2>/dev/null | head -1)"
  else
    err "${PRODUCT} failed to run"
  fi
done

cat <<EOF

Grove Suite ${VERSION} installed to ${INSTALL_DIR}.

Next steps:
  • Open a NEW terminal (or: export PATH="${INSTALL_DIR}:\$PATH") so the binaries are on PATH.
  • If you initialized a project, RESTART your AI coding tool (Claude Code, Cursor,
    VS Code, Copilot) so it spawns the freshly-registered MCP servers.
  • Verify MCP wiring in Claude Code:  claude mcp list   → prism/provasign should show ✓ Connected
  • Docs: https://tabladrum.github.io/provasign/
EOF
