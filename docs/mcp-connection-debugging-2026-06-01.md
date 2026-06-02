# MCP Connection Failure — Root Cause Analysis (2026-06-01)

## Symptoms

After installing Provasign, the prism and provasign MCP servers would not connect
in Claude Code. `claude mcp list` showed either:
- `✗ Failed to connect`
- `⏸ Pending approval (run 'claude' to approve)`

Restarting Claude Code did not resolve the issue.

## Root Causes Found

### 1. `grove_binary` defaults to bare `"grove"`, not an absolute path

**Where:** `prism/internal/config/config.go` (Default), `fuse/internal/config/config.go`  
**Effect:** When `prism init` runs, it writes `grove_binary: "grove"` to `prism.yaml`.
When Claude Code later spawns the prism MCP server, it provides a minimal
environment where `~/bin` is not on `$PATH`. If grove is not already running,
prism tries to auto-start it via `exec.Command("grove", ...)`, which fails
silently with "executable not found". The prism MCP process then sits with no
backing grove instance; tool calls fail.

**Fix:** `prism init` and `fuse init` must detect and write the absolute path to
the grove binary (looking in the same directory as the prism binary, then
`~/bin`, then `$PATH`).

### 2. `prism init` rewrites `.mcp.json` unconditionally

**Where:** `prism/internal/cli/commands.go` → `initRegisterMCPTools`  
**Effect:** Every `prism init` run (including repeated runs during setup or
troubleshooting) modifies `.mcp.json`. Claude Code detects any change to
`.mcp.json` and resets the MCP approval state. The servers move to
"Pending approval" and will not connect until the user interactively re-approves
in a new `claude` session. This is an invisible reset — no warning is shown.

**Fix:** `mergeOrCreate` already avoids overwriting unrelated keys, but the outer
`prism init` call still touches the file even when the prism entry is already
present and correct. Add an idempotency check: skip writing `.mcp.json` if the
existing prism entry already points to the correct binary and args.

### 3. `provasign init` writes `.mcp.json` with `"env": {}` 

**Where:** `provasign/internal/cli/mcp_install_commands.go` (or equivalent)  
**Effect:** The provasign entry in `.mcp.json` includes `"env": {}`. While harmless,
it differs from prism's format (no `env` key) and causes unnecessary diffs on
repeated `provasign init` runs, again triggering approval resets.

**Fix:** Omit `"env"` from provasign's `.mcp.json` entry when it is empty.

### 4. No post-install MCP connectivity verification

**Where:** `AGENT_SETUP_PROMPT.md` Step 9 smoke test  
**Effect:** The smoke test runs `grove version`, `prism version`, etc. — it does
not verify that MCP servers actually connect. Installation is declared complete
while the MCP layer is broken, with no user-visible failure until the agent
tries to use a Prism/Provasign tool.

**Fix:** Add a `claude mcp list` check to Step 9 with explicit pass/fail
criteria, and add a "Pending approval" troubleshooting entry to the help table.

### 5. `provasign/bin/provasign` binary committed in modified state

**Where:** git tracked file `provasign/bin/provasign`  
**Effect:** The binary committed to the repo diverged from the released v0.2.3
binary (different size: 21 MB repo vs 14 MB release). This could cause
confusion during source builds and git operations.

**Fix:** `make build` output should go to `bin/` inside the module and that path
should be in `.gitignore`, not committed.

## What Was Done During Investigation

### Session 1 (2026-06-01)

1. Confirmed all four binaries exist in `~/bin/` at v0.2.3.
2. Confirmed `~/bin` registered in `/etc/paths.d/provasign` but not in the
   current shell's `$PATH` (expected — only applies to new login sessions).
3. Manually verified MCP protocol handshake (initialize → initialized →
   tools/list) works for both prism and provasign — the binaries are correct.
4. Confirmed grove at `localhost:7777` is running and authenticated (token at
   `.grove/.token`).
5. Found that editing `.mcp.json` (even to restore it) resets Claude Code's
   MCP approval, causing "Pending approval" instead of "Failed to connect".
6. Fixed `prism.yaml` in this project to use absolute path for `grove_binary`.
7. Ran `prism init` to regenerate `prism.yaml` and re-register MCP configs.
8. Wrote all source-code fixes for root causes 1–3 into the working tree
   (`prism/internal/cli/commands.go`, `prism/internal/grove/client.go`,
   `fuse/internal/cli/commands.go`, `fuse/internal/grove/client.go`,
   `provasign/internal/cli/mcp_install_commands.go`,
   `provasign/internal/cli/init_agent_wiring.go`).
9. **Did NOT rebuild the binaries.** Session ended without running
   `go build` — the fixes existed only in source, not in `~/bin/`.

### Session 2 (2026-06-01)

**Root cause of continued failure:** All source fixes from Session 1 were
present and correct in the working tree, but `~/bin/prism`, `~/bin/provasign`, and
`~/bin/fuse` were never rebuilt. The running binaries were still the pre-fix
versions. MCP servers continued to fail for the same underlying reasons despite
the code being fixed.

**Resolution:** Rebuilt and reinstalled all three binaries directly:

```bash
cd prism  && /usr/local/go/bin/go build -o ~/bin/prism  ./cmd/prism
cd fuse   && /usr/local/go/bin/go build -o ~/bin/fuse   ./cmd/fuse
cd provasign  && /usr/local/go/bin/go build -o ~/bin/provasign  ./cmd/provasign
```

Verified both `prism` and `provasign` respond correctly to the MCP `initialize`
handshake after rebuild.

**Note:** `make install` fails in this environment because `/usr/local/go/bin`
is not on `$PATH` when Claude Code runs shell commands. Always use the absolute
path `/usr/local/go/bin/go` for Go builds in this session.

### Session 3 (2026-06-01) — ACTUAL ROOT CAUSE FOUND

The five "root causes" in Sessions 1–2 were all real-but-secondary cleanups.
**None of them was why MCP connection failed.** The Claude Code MCP logs
(`~/Library/Caches/claude-cli-nodejs/<project>/mcp-logs-{prism,provasign}/*.jsonl`)
showed the true error:

```
Connection failed: MCP server "prism" connection timed out after 30000ms
```

A 30s timeout = Claude sent `initialize` and never received a parseable reply.

**THE BUG: wrong stdio wire framing.** All three stdio MCP servers (grove,
prism, provasign) wrote responses with LSP-style `Content-Length: N\r\n\r\n{json}`
framing (copied from grove's server.go). But the **MCP stdio transport requires
newline-delimited JSON** — one compact JSON object per line, no headers. Every
newline-delimited MCP client (Claude Code, Cursor, VS Code, Copilot) reads the
first line as `Content-Length: 140\r`, fails to parse it as JSON, and then
blocks forever because the JSON body that followed had **no terminating
newline** → 30s connection timeout → "Failed to connect".

Why earlier manual handshake tests looked fine: a human reading the terminal can
see `Content-Length: 140` + JSON, but a newline-delimited client parser cannot.
The read side was already lenient (accepted both framings), which is why the
servers received Claude's requests — only the **write** side was wrong.

**Fix (the only change that actually mattered):** `writeMessage` in all three
servers now emits `fmt.Fprintf(w, "%s\n", payload)` — newline-delimited compact
JSON. Readers stay lenient (still accept legacy Content-Length input).

**Secondary fix:** `initialize` now negotiates `protocolVersion` — echoes the
client's requested version when supported (2024-11-05 / 2025-03-26 / 2025-06-18),
else falls back to 2025-03-26. Previously each server hardcoded a version, a
spec violation that stricter non-Claude clients may reject.

Verified: `claude mcp list` shows both prism and provasign **✓ Connected**; full
`initialize → notifications/initialized → tools/list` handshake succeeds for
both with a newline-delimited client.

**Files changed:** `grove/internal/mcp/server.go`,
`prism/internal/mcp/server.go`, `provasign/internal/api/mcp/server.go` (+ their
`*_test.go` which had been asserting the buggy Content-Length framing).

## Required User Action After Fixes

After a fresh install from source:
1. Rebuild all binaries with `/usr/local/go/bin/go build` (not just `make install`
   — `go` is not on PATH in the Claude Code shell environment).
2. Start a new Claude Code session from the project root.
3. When prompted "Allow MCP servers from .mcp.json?", click **Allow**.
4. Verify with `claude mcp list` — both prism and provasign should show as connected.

## Checklist: Fix is complete only when ALL of these are done

- [ ] Source changes written (commands.go, client.go, mcp_install_commands.go, etc.)
- [ ] `~/bin/prism` rebuilt from source
- [ ] `~/bin/fuse` rebuilt from source
- [ ] `~/bin/provasign` rebuilt from source
- [ ] New Claude Code session started
- [ ] MCP servers approved (or already in `enabledMcpjsonServers`)
- [ ] `claude mcp list` shows both prism and provasign as connected
