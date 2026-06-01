# Grove Suite — AI Agent Setup Prompt

*This file is a prompt for your AI coding agent. Point any agent at it — Claude Code, Cursor, Codex CLI, GitHub Copilot, Windsurf — and it will install and configure Grove Suite for you, always at the latest version.*

---

## How to Use This File

**Claude Code:**
```
/read AGENT_SETUP_PROMPT.md  — then say "follow the setup instructions"
```
Or from outside the repo:
```
claude "Follow the setup instructions at https://tabladrum.github.io/grove-suite/assets/AGENT_SETUP_PROMPT.md"
```

**Cursor / Codex CLI / any agent with web fetch:**
Paste the URL below into chat and say "follow the setup instructions in this file":
```
https://tabladrum.github.io/grove-suite/assets/AGENT_SETUP_PROMPT.md
```

---

## Instructions for the Agent

You are setting up Grove Suite — infrastructure that makes AI coding agents production-safe. Follow each step in order. Ask the user when a decision is needed. Do not skip checksum verification.

Before Step 1, ask:

> Do you want to **install/update** Grove Suite, or **fully uninstall/reset** it?

- If the user says **install/update**, continue with Step 1 as written.
- If the user says **uninstall/reset**, skip to **Step U1 — Full Uninstall / Reset** below.

### Step 0 — Refresh These Instructions

Before doing anything else, fetch the latest version of this file:

```
https://tabladrum.github.io/grove-suite/assets/AGENT_SETUP_PROMPT.md
```

If the fetched content differs from what you are reading now, use the fetched version for all remaining steps. This ensures you always follow the most current setup procedure.

---

### Step 1 — Ask What to Install

Ask the user:

> Which Grove Suite products would you like to install?
>
> **1. Full suite** *(recommended)* — Grove + Prism + Fuse + Relay
> **2. Prism only** — token-optimized context for your AI agent (most common single install)
> **3. Grove + Prism** — context delivery with the full knowledge graph
> **4. Grove + Fuse** — symbol-aware merge for teams running parallel agents
> **5. Grove + Relay** — quality gates and certified commits for your agent
> **6. Custom** — I'll choose individually

Grove is always included — Prism, Fuse, and Relay all depend on it.

Also ask:

> Where should the binaries be installed?
>
> **1.** `~/bin` *(recommended for agent-driven install — no sudo required)*
> **2.** `/usr/local/bin` — system-wide; requires sudo. The agent will download
>        and verify the binaries, then give you a single paste-ready terminal
>        command to do the privileged move. You run that one command yourself.
> **3.** Let me specify a path

**Important:** AI agents cannot run `sudo` interactively. If you choose `/usr/local/bin`
(or any path requiring elevated privileges), the agent will complete all steps up to
the file move, then hand you a single command to run in your own terminal. All other
steps — download, checksum verification, initialization — are fully automated.

On Windows, ask for the target directory and confirm it is on `%PATH%`.

If the user selected any option that includes Prism, ask one more question:

> Are you running this setup from VS Code and using GitHub Copilot Chat?
>
> If yes, do you want to install the Prism VS Code extension and use native
> VS Code tools instead of MCP for Prism?

Behavior rules:
- If user says **yes** to extension mode: install extension `prism.prism-vscode`
  and do **not** keep Prism VS Code MCP wiring (`.vscode/mcp.json`) for Prism.
- If user says **no**: keep standard Prism MCP setup from `prism init`.

---

### Step 2 — Detect the Platform

Run the following to identify OS and architecture:

**Linux / macOS:**
```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)        ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
esac
echo "Detected platform: ${OS}-${ARCH}"
```

**Windows (PowerShell):**
```powershell
$OS = "windows"
$ARCH = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
Write-Host "Detected platform: $OS-$ARCH"
```

Confirm with the user if the detected platform looks wrong.

---

### Step 3 — Get the Latest Version

Fetch the current release tag from the GitHub API:

**Linux / macOS:**
```bash
VERSION=$(curl -sf https://api.github.com/repos/tabladrum/grove-suite/releases/latest \
  | grep '"tag_name"' \
  | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
echo "Latest release: ${VERSION}"
```

If `curl` is unavailable:
```bash
VERSION=$(wget -qO- https://api.github.com/repos/tabladrum/grove-suite/releases/latest \
  | grep '"tag_name"' \
  | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
```

**Windows (PowerShell):**
```powershell
$VERSION = (Invoke-RestMethod https://api.github.com/repos/tabladrum/grove-suite/releases/latest).tag_name
Write-Host "Latest release: $VERSION"
```

If the API is unreachable, tell the user to check https://github.com/tabladrum/grove-suite/releases and provide the version manually.

---

### Step 4 — Check for Existing Installations

For each product the user selected, check what is already installed and at what version:

**Linux / macOS:**
```bash
for bin in grove prism fuse relay; do
  if command -v "$bin" &>/dev/null; then
    LOC=$(which "$bin")
    VER=$("$bin" version 2>/dev/null | head -1 || echo "version unknown")
    echo "  $bin: INSTALLED at $LOC — $VER"
  else
    echo "  $bin: not found"
  fi
done
```

**Windows (PowerShell):**
```powershell
foreach ($bin in @("grove","prism","fuse","relay")) {
  $path = Get-Command $bin -ErrorAction SilentlyContinue
  if ($path) {
    $ver = & $bin version 2>$null | Select-Object -First 1
    Write-Host "  $bin: INSTALLED at $($path.Source) — $ver"
  } else {
    Write-Host "  $bin: not found"
  }
}
```

For any product already installed:
- If the installed version matches `$VERSION`: tell the user it is up to date and skip it.
- If the installed version is older: tell the user and ask whether to upgrade.
- If the version cannot be determined: ask whether to reinstall.

---

### Step 5 — Download Binaries

Set the base URL:

**Linux / macOS:**
```bash
BASE="https://github.com/tabladrum/grove-suite/releases/download/${VERSION}"
```

Download the checksums file first:

**Linux / macOS:**
```bash
curl -fL "${BASE}/checksums.txt" -o /tmp/grove-suite-checksums.txt
echo "Checksums downloaded."
```

**Windows (PowerShell):**
```powershell
$BASE = "https://github.com/tabladrum/grove-suite/releases/download/$VERSION"
Invoke-WebRequest "$BASE/checksums.txt" -OutFile "$env:TEMP\grove-suite-checksums.txt"
Write-Host "Checksums downloaded."
```

Now download each selected binary. Install **Grove first** — the others depend on it.

**Linux / macOS — for each product:**
```bash
PRODUCT=grove   # repeat for: prism, fuse, relay
FILENAME="${PRODUCT}-${VERSION}-${OS}-${ARCH}"
curl -fL "${BASE}/${FILENAME}" -o "/tmp/${FILENAME}"
echo "Downloaded ${FILENAME}"
```

**Windows (PowerShell) — for each product:**
```powershell
$PRODUCT = "grove"  # repeat for: prism, fuse, relay
$FILENAME = "$PRODUCT-$VERSION-windows-$ARCH.exe"
Invoke-WebRequest "$BASE/$FILENAME" -OutFile "$env:TEMP\$FILENAME"
Write-Host "Downloaded $FILENAME"
```

---

### Step 6 — Verify Checksums

**Never skip this step.** Verify each downloaded binary against the checksums file before installing.

**Linux / macOS:**
```bash
PRODUCT=grove   # repeat for each binary
FILENAME="${PRODUCT}-${VERSION}-${OS}-${ARCH}"

EXPECTED=$(grep "^[a-f0-9]*  ${FILENAME}$\|^[a-f0-9]*  \./${FILENAME}$" \
           /tmp/grove-suite-checksums.txt | awk '{print $1}')

if command -v sha256sum &>/dev/null; then
  ACTUAL=$(sha256sum "/tmp/${FILENAME}" | awk '{print $1}')
else
  ACTUAL=$(shasum -a 256 "/tmp/${FILENAME}" | awk '{print $1}')
fi

if [ "$EXPECTED" = "$ACTUAL" ]; then
  echo "✅ ${FILENAME}: checksum OK"
else
  echo "❌ ${FILENAME}: CHECKSUM MISMATCH — do not install"
  echo "   expected: $EXPECTED"
  echo "   actual:   $ACTUAL"
  exit 1
fi
```

**Windows (PowerShell):**
```powershell
$PRODUCT = "grove"  # repeat for each binary
$FILENAME = "$PRODUCT-$VERSION-windows-$ARCH.exe"

$checksums = Get-Content "$env:TEMP\grove-suite-checksums.txt"
$expected = ($checksums | Where-Object { $_ -match $FILENAME }) -split '\s+' | Select-Object -First 1

$actual = (Get-FileHash "$env:TEMP\$FILENAME" -Algorithm SHA256).Hash.ToLower()

if ($expected -eq $actual) {
  Write-Host "✅ $FILENAME checksum OK"
} else {
  Write-Error "❌ $FILENAME CHECKSUM MISMATCH — do not install"
  exit 1
}
```

If any checksum fails, stop. Do not install the binary. Tell the user and suggest re-downloading.

---

### Step 7 — Install Binaries

Install Grove first — the others depend on it. The method depends on the
install directory chosen in Step 1.

---

**Path A — `~/bin` (no sudo; agent runs this directly):**

```bash
mkdir -p ~/bin
for PRODUCT in grove prism fuse relay; do   # only selected products
  FILENAME="${PRODUCT}-${VERSION}-${OS}-${ARCH}"
  mv "/tmp/${FILENAME}" ~/bin/${PRODUCT}
  chmod +x ~/bin/${PRODUCT}
  echo "✅ ${PRODUCT} → ~/bin/${PRODUCT}"
done
```

Check `~/bin` is on `$PATH` (run this too):
```bash
echo "$PATH" | grep -q "$HOME/bin" || {
  SHELL_RC="$HOME/.zshrc"
  [ -f "$HOME/.bashrc" ] && SHELL_RC="$HOME/.bashrc"
  echo 'export PATH="$HOME/bin:$PATH"' >> "$SHELL_RC"
  export PATH="$HOME/bin:$PATH"
  echo "Added ~/bin to PATH in ${SHELL_RC} — restart your shell or: source ${SHELL_RC}"
}
```

macOS — clear Gatekeeper quarantine:
```bash
for PRODUCT in grove prism fuse relay; do
  xattr -d com.apple.quarantine ~/bin/${PRODUCT} 2>/dev/null || true
done
```

---

**Path B — `/usr/local/bin` or any sudo-required path:**

The agent cannot run `sudo` interactively. Do the following:

1. Tell the user: *"All binaries are downloaded and checksum-verified in `/tmp/`.
   Please run the command below in your terminal to complete the install, then
   come back and I will continue with initialization."*

2. Print this exact block for the user to paste into their terminal (substituting
   the actual `$VERSION`, `$OS`, `$ARCH`, and the selected products):

```bash
# Paste this in your terminal:
VERSION="v0.x.x"   # filled in by agent
OS="darwin"        # filled in by agent
ARCH="arm64"       # filled in by agent
INSTALL_DIR="/usr/local/bin"

for PRODUCT in grove prism fuse relay; do
  sudo mv "/tmp/${PRODUCT}-${VERSION}-${OS}-${ARCH}" "${INSTALL_DIR}/${PRODUCT}"
  sudo chmod +x "${INSTALL_DIR}/${PRODUCT}"
  xattr -d com.apple.quarantine "${INSTALL_DIR}/${PRODUCT}" 2>/dev/null || true
  echo "✅ ${PRODUCT} installed"
done
```

3. Wait for the user to confirm they ran it before continuing to Step 8.

---

**Windows — move to target directory (agent runs this):**
```powershell
$PRODUCTS = @("grove","prism","fuse","relay")   # only selected products
$TARGET = "$env:USERPROFILE\bin"                # or user-specified path
New-Item -ItemType Directory -Force -Path $TARGET | Out-Null
foreach ($PRODUCT in $PRODUCTS) {
  $FILENAME = "$PRODUCT-$VERSION-windows-$ARCH.exe"
  Move-Item "$env:TEMP\$FILENAME" "$TARGET\$PRODUCT.exe" -Force
  Write-Host "✅ $PRODUCT → $TARGET\$PRODUCT.exe"
}
```

---

### Step 8 — Initialize in the Project

Ask the user for the path to their project (the repository they want to instrument). Then run from that directory:

**Grove (always):**
```bash
cd /path/to/your/project
grove index .
echo "Grove: knowledge graph built."
```

**Prism (if selected):**
```bash
prism init    # detects installed AI tools, writes MCP configs automatically
              # supports: Claude Code, GitHub Copilot, Cursor, Codex CLI,
              #           Windsurf, Zed, Continue, and any MCP-capable tool
prism index   # initial index (delta-aware — subsequent runs only touch changed files)
echo "Prism: initialized. Restart your AI coding tool to activate the MCP server."
```

If the user chose **VS Code extension mode**:

```bash
# Install Prism VS Code extension (native tools; no Prism MCP in VS Code)
code --install-extension prism.prism-vscode

# Keep prism init outputs, but remove Prism MCP wiring for VS Code only to avoid
# duplicate Prism providers in Copilot Chat.
if [ -f .vscode/mcp.json ]; then
  cp .vscode/mcp.json .vscode/mcp.json.bak
  python3 - << 'PY'
import json, pathlib
p = pathlib.Path('.vscode/mcp.json')
doc = json.loads(p.read_text()) if p.exists() else {}
servers = doc.get('servers', {})
if 'prism' in servers:
    del servers['prism']
doc['servers'] = servers
p.write_text(json.dumps(doc, indent=2) + '\n')
print('Updated .vscode/mcp.json: removed prism MCP entry for VS Code extension mode')
PY
fi

echo "Prism VS Code extension installed. Restart VS Code to activate native Prism tools."
```

Notes:
- Keep Prism MCP for non-VS Code tools (Claude Code, Cursor, etc.) if configured.
- If `code` CLI is unavailable, tell the user to install the extension manually in
  VS Code Extensions: `prism.prism-vscode`.

**Fuse (if selected):**
```bash
fuse install   # registers 'fuse' as a git merge driver in ~/.gitconfig

# Add to the project's .gitattributes — only the languages you use:
cat >> .gitattributes << 'EOF'
*.go   merge=fuse
*.ts   merge=fuse
*.tsx  merge=fuse
*.py   merge=fuse
*.java merge=fuse
*.rs   merge=fuse
*.cs   merge=fuse
EOF

git add .gitattributes
echo "Fuse: installed. Next git merge will use symbol-aware resolution."
```

Ask the user which languages they use and only add those lines.

**Relay (if selected):**

Then run:
```bash
relay init --list-stacks  # show available stacks: go-microservice, java-spring,
                          # node-api, python-service
relay init --stack=<stack> # pick the stack that matches your project;
                          # scaffolds .relay/ config, generates Ed25519 key,
                          # writes agent steering instructions to CLAUDE.md /
                          # .cursorrules / AGENTS.md / .clinerules automatically
relay hook install        # installs pre-push backstop

# Pre-download analyzer dependencies now so there is no first-use delay.
# (Downloads JRE + sonarlint-ls.jar + plugins; roughly 500+ MB total with --with-sonar.)
# Ask the user before running --with-sonar if bandwidth is a concern.
relay tools install --with-sonar
echo "Relay tools installed."

# govulncheck requires Go in PATH. Add it if missing:
export PATH="/usr/local/go/bin:$PATH"

# Semgrep and Ruff are Python packages; install via pipx (preferred).
# If pipx is missing, install it first: brew install pipx (macOS) or pip install pipx.
if command -v pipx &>/dev/null; then
  pipx install semgrep && echo "✅ semgrep installed"
  pipx install ruff    && echo "✅ ruff installed"
elif command -v pip3 &>/dev/null; then
  # Modern Python (3.12+) blocks pip system installs — try pipx first.
  pip3 install --user semgrep && echo "✅ semgrep (pip3 --user)" \
    || echo "ℹ️  pip3 blocked by system Python; run: brew install pipx && pipx install semgrep"
  pip3 install --user ruff && echo "✅ ruff (pip3 --user)" \
    || echo "ℹ️  pip3 blocked by system Python; run: brew install pipx && pipx install ruff"
else
  echo "ℹ️  pipx not found — install it first: brew install pipx"
  echo "    then: pipx install semgrep && pipx install ruff"
fi

# eslint is optional — only needed if JS/TS SAST is enabled in relay.yaml.
# npm install -g eslint

# Verify all tools after install.
# relay doctor exits non-zero if any check needs attention — read its output
# rather than treating a non-zero exit as a hard failure. Only stop if a
# tool that is enabled AND required for your stack shows "missing".
relay doctor || true
# If govulncheck shows missing: ensure Go is on PATH and re-run relay tools install.
# If eslint shows missing: safe to ignore for Go/Python projects.

git add .relay/
echo "Relay: initialized. Your agent will call relay_check before every commit."
```

---

### Step 9 — Smoke Test

Run a smoke test for each installed binary:

**Linux / macOS:**
```bash
echo "=== Grove Suite smoke test ==="
grove version  && echo "✅ grove ok"  || echo "❌ grove failed"
command -v prism &>/dev/null && { prism version && echo "✅ prism ok" || echo "❌ prism failed"; }
command -v fuse  &>/dev/null && { fuse version  && echo "✅ fuse ok"  || echo "❌ fuse failed";  }
command -v relay &>/dev/null && { relay version && echo "✅ relay ok" || echo "❌ relay failed"; }
```

**Windows (PowerShell):**
```powershell
Write-Host "=== Grove Suite smoke test ==="
& grove version  && Write-Host "✅ grove ok"  || Write-Host "❌ grove failed"
if (Get-Command prism  -EA 0) { & prism version  && Write-Host "✅ prism ok"  || Write-Host "❌ prism failed" }
if (Get-Command fuse   -EA 0) { & fuse version   && Write-Host "✅ fuse ok"   || Write-Host "❌ fuse failed"  }
if (Get-Command relay  -EA 0) { & relay version  && Write-Host "✅ relay ok"  || Write-Host "❌ relay failed" }
```

**Common failures and fixes:**

| Symptom | Fix |
|---------|-----|
| `command not found` | Install directory not on `$PATH` — add it and restart shell |
| macOS "cannot be opened because the developer cannot be verified" | Run `xattr -d com.apple.quarantine $(which grove)` |
| macOS `zsh: killed` (exit 137) when binary is in `/opt/homebrew/bin` | Run `codesign -f -s - $(which grove)` (repeat for each binary) |
| `grove: connection refused` on Prism/Fuse/Relay | Run `grove serve` once; it auto-starts on subsequent calls |
| `relay: key not found` | Run `relay init` from the project root |
| `relay init --stack=auto` fails with "unknown stack" | Use `relay init --list-stacks` to see valid stack names, then `relay init --stack=<name>` |
| `relay_check` passes but no SAST/secrets findings appear | Run `relay tools install` — analyzers are silently skipped when not pre-downloaded |
| semgrep not running in relay_check | Install separately: `pipx install semgrep` |
| `relay doctor` shows `govulncheck missing` | Go is not in PATH — run `export PATH="/usr/local/go/bin:$PATH"` then re-run `relay tools install` |
| `relay doctor` shows `eslint missing` | Safe to ignore for Go/Python projects; only needed for JS/TS SAST. Install with `npm install -g eslint` if required |
| `relay doctor` exits non-zero but output shows only `eslint missing` | Not a hard failure — relay doctor exits 1 whenever any check needs attention, even optional ones. Read the output to distinguish required vs optional gaps |
| `pipx install semgrep` fails: "externally-managed-environment" | System Python blocks pip — install pipx first: `brew install pipx`, then retry |

If anything fails, diagnose and fix before reporting done.

---

### Step 10 — Report to the User

Print a clear summary of what was installed, where, and what to do next:

```
Grove Suite installation complete
══════════════════════════════════════════════════════════
 grove  v0.x.x  ✅  /usr/local/bin/grove
 prism  v0.x.x  ✅  /usr/local/bin/prism
 fuse   v0.x.x  ✅  /usr/local/bin/fuse
 relay  v0.x.x  ✅  /usr/local/bin/relay
══════════════════════════════════════════════════════════

Next steps
──────────
  Prism  → If using MCP mode: restart your AI coding tool to activate the MCP server
           If using VS Code extension mode: restart VS Code, then use #prismQuery
           Then run: prism savings   (see token savings after your first task)

  Fuse   → Your next `git merge` uses symbol-aware resolution automatically
           Audit log: .git/fuse/audit.json

  Relay  → Your agent now calls relay_check before every commit automatically
           After your first commit: relay cert show HEAD

Documentation
─────────────
  Full docs:  https://tabladrum.github.io/grove-suite/
  Setup help: https://tabladrum.github.io/grove-suite/setup/
  Issues:     https://github.com/tabladrum/grove-suite/issues
```

List any products that were skipped or failed, with specific next steps for each.

---

## Step U1 — Full Uninstall / Reset

Use this flow when the user asks to remove Grove Suite and start from a clean slate.

Ask for the target project path first (the repo where MCP/hook wiring should be removed).

Then run:

**macOS / Linux:**
```bash
cd /path/to/grove-suite
./scripts/uninstall-grove-suite.sh /path/to/target/project
```

What this removes:
- Grove/Prism/Fuse/Relay binaries from common install paths
- Relay-managed hooks and MCP registrations
- Relay tool cache (`~/.relay/tools`) and user runtime/cache state
- Project-local runtime state (`.grove`, `.git/fuse`, workspace MCP config files)
- Lingering local processes (`grove serve`, relay MCP, etc.)

After uninstall, verify:

```bash
command -v grove || echo "grove removed"
command -v prism || echo "prism removed"
command -v fuse  || echo "fuse removed"
command -v relay || echo "relay removed"
```

Then report a short uninstall summary to the user and confirm they can now rerun this prompt for a clean install.

**Windows:**
- Full-script uninstall automation is not yet available.
- Tell the user to follow the uninstall section in docs/installation.md for now.

---

*Grove, Prism, and Fuse are MIT licensed. Relay is AGPL-3.0 licensed. No telemetry. Your code never leaves your machine.*
