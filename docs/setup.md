---
title: Agent Setup
layout: default
nav_order: 8
description: "Install and configure Provasign using your AI coding agent — always at the latest version."
permalink: /setup/
---

# Agent-Driven Setup

**The fastest way to install Provasign is to let your AI agent do it.**

Point any agent at the setup prompt below — it will detect your platform, check for the latest version, ask which products you want, download and verify the binaries, initialize everything in your project, and run a smoke test.

Works with Claude Code, Cursor, Codex CLI, GitHub Copilot, Windsurf, and any agent that can read a file or URL and run shell commands.

---

## One-Liner

**Claude Code** — from inside any project directory:
```bash
claude "Follow the setup instructions at https://provasign.dev/assets/AGENT_SETUP_PROMPT.md"
```

**Any other agent:**

Paste this URL into your agent's chat and say *"follow the setup instructions in this file"*:
```
https://provasign.dev/assets/AGENT_SETUP_PROMPT.md
```

The prompt fetches the latest version of its own instructions first — so it's always current regardless of when you saved or bookmarked it.

---

## What the Agent Does

The [AGENT_SETUP_PROMPT.md](https://provasign.dev/assets/AGENT_SETUP_PROMPT.md) prompt walks the agent through:

1. **Refresh** — fetches the latest version of these instructions from GitHub before doing anything
2. **Ask** — which products to install (full suite / Prism only / custom), and where to put the binaries
3. **Detect** — OS and architecture (linux/darwin/windows × amd64/arm64)
4. **Latest version** — queries the GitHub Releases API; never installs an outdated binary
5. **Check existing** — detects already-installed products, compares versions, asks before upgrading
6. **Download + verify** — fetches the correct binary and verifies SHA-256 against `checksums.txt`
7. **Install** — moves binary to the chosen path, handles macOS Gatekeeper quarantine removal
8. **Initialize** — runs `prism init`, `fuse install`, `provasign init --stack=<detected>`, then `provasign tools install --with-sonar` and `provasign doctor`; this pre-downloads analyzer dependencies so first use is deterministic. In VS Code, it can optionally install `prism.prism-vscode` and remove Prism MCP wiring from `.vscode/mcp.json` to avoid duplicate providers
9. **Smoke test** — verifies each binary works end-to-end
10. **Summary** — prints what's installed, where, and what to do next

---

## Manual Setup

Prefer to do it yourself? The [full installation guide]({{ '/installation/' | relative_url }}) covers:

- Pre-built binaries for all platforms (macOS Apple Silicon / Intel, Linux amd64 / arm64, Windows)
- Checksum verification
- Building from source
- After-install wiring per product

---

## Upgrading

Run the same prompt again at any time. It detects your current version, checks the latest release, and only downloads what has changed.

Or check manually:

```bash
# Check installed versions
grove version && prism version && fuse version && provasign version

# Check latest release
curl -sf https://api.github.com/repos/provasign/provasign/releases/latest \
  | grep '"tag_name"'
```

---

## Need Uninstall Instead?

The same agent prompt supports full uninstall/reset mode too.

Use the same one-liner, then tell your agent:

> Follow the uninstall/reset flow in this prompt.

Under the hood, the agent will run the repo script:

```bash
cd /path/to/provasign
./scripts/uninstall-provasign.sh /path/to/target/project
```

For manual details, see the [installation guide uninstall section]({{ '/installation/#uninstall' | relative_url }}).

---

## Troubleshooting Setup

| Symptom | Fix |
|---------|-----|
| `command not found` after install | Install dir not on `$PATH` — add it and restart shell |
| macOS "developer cannot be verified" | `xattr -d com.apple.quarantine $(which grove)` |
| `grove: connection refused` on Prism start | Should not happen in embedded mode — upgrade to the latest release |
| VS Code shows duplicate Prism providers/tools | Use either Prism MCP or the Prism VS Code extension, not both. If using extension mode, remove `prism` from `.vscode/mcp.json` |
| Agent can't reach GitHub API | Check network, or download manually from [GitHub Releases](https://github.com/provasign/provasign/releases) |

Full troubleshooting: [Troubleshooting]({{ '/troubleshooting/' | relative_url }})
