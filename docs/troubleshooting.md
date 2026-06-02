---
title: Troubleshooting
layout: default
nav_order: 12
description: "Common issues with Provasign — how to diagnose and fix them."
permalink: /troubleshooting/
---

# Troubleshooting

Common issues, how to diagnose, how to fix.

If your issue isn't here, the next stop is [GitHub Issues](https://github.com/provasign/provasign/issues).

---

## Grove

### Embedded mode — no daemon, no ports

Grove is linked directly into Prism, Fuse, and Provasign. It does not open TCP
ports, does not write `.grove/.token`, and does not need `grove serve`. If you
see old documentation or scripts referencing those, they're pre-embedded and
should be ignored.

### Indexing is slow on a large repo

Cold index on 10,000 files is ~34 seconds on a 2024 MacBook. If you're seeing dramatically slower:

1. Check disk speed — Grove writes to `.grove/grove.db` during indexing
2. Check available cores — Grove parallelizes across all available CPUs
3. Excluded directories — confirm `node_modules`, `vendor`, `.git`, etc. are being skipped (they should be by default)

After the first index, subsequent runs touch only changed files (delta indexing by git blob SHA). One-file changes complete in milliseconds.

### Symbols are missing for a file Grove should support

1. `grove status .` — confirm the file is indexed at all
2. Check the file's language detection: `grove --debug-language <file>`
3. If the file is syntactically invalid, Tree-sitter falls back to regex extraction — some symbols may be missed. Fix the syntax error and re-index.

### Grove crashes on a specific file

This is a bug. Please file an issue with:
- The file (or a minimal reproducer)
- The Grove version (`grove --version`)
- The crash output

Grove should never crash on bad input — Tree-sitter has graceful syntax error recovery.

### My `.grove/grove.db` file is huge

The database scales with project size — roughly 2 MB per 1,000 files indexed. If yours seems disproportionate:

```bash
# Check what's indexed
sqlite3 .grove/grove.db "SELECT COUNT(*) FROM symbols;"

# Check for stale entries (files that no longer exist)
grove index .   # this prunes stale entries
```

Force a clean re-index:
```bash
rm -rf .grove/grove.db
grove index .
```

### Bearer token errors

In the embedded model there is no bearer token — every reader links Grove
directly. If a tool still reports an auth error, you're on a pre-embedded
build; upgrade to the latest release.

---

## Prism

### `prism savings` shows 0 even after I used the agent

The agent isn't calling Prism tools. Check in order:

1. **Was `prism init` run in this directory?**
   ```bash
   ls .mcp.json .cursor/mcp.json 2>/dev/null
   ```
   If neither file exists, `prism init` wasn't run here. For Claude Code, also
   verify the server is approved: run `claude mcp list` and look for prism. If
   it shows ⏸ Pending, restart Claude Code and approve when prompted.

2. **Did you restart the coding tool after `prism init`?**
   The MCP config is read at startup. New configs require a restart.

3. **Are the agent steering instructions present?**
   ```bash
   grep -l 'prism' CLAUDE.md .cursorrules .github/copilot-instructions.md 2>/dev/null
   ```
   `prism init` writes a "Prism — context delivery (ALWAYS use these tools)" section. Without it, the agent doesn't know to call Prism.

4. **Is the binary on the PATH?**
   ```bash
   which prism && prism version
   ```

### Token savings are lower than expected

A few possible reasons:

- **Small project, every symbol is relevant.** When your project has 50 files and the query touches 30 of them, there's nothing to filter. This is correct behaviour.
- **First-read penalty.** Progressive disclosure only kicks in on the second+ read of the same file in the same session. First reads return the full source.
- **Cache miss.** Session cache is keyed by content SHA — if files have changed since the last read, you get a full disclosure again.

Run `prism savings --json` for a detailed breakdown.

### Prism query latency is high

Target is sub-200ms end-to-end. If you're consistently slower:

1. Profile with `--verbose`: which stage is slow? Grove BFS? Ranking? Disclosure?
2. Project size: very large monorepos (>50K files) can push BFS depth-3 over budget. Try `--max-depth=2`.
3. Embedding backend: `embeddings_backend: openai` uses a remote API and adds latency. Switch to `local` for Model2Vec.

### My agent doesn't see Prism tools

The MCP server registration didn't take. Re-run:
```bash
prism init --force
# Restart your coding tool
```

For VS Code Copilot Agent, the native extension is the integration path — not MCP. Make sure the Prism VS Code extension is installed.

### Progressive disclosure is too aggressive — I want full source every time

Set `disclosure: full` in `prism.yaml`:
```yaml
disclosure: full   # disable progressive disclosure
```

Or `--disclosure=full` per command. You'll get full source on every read but lose ~57–68% of session savings.

---

## Fuse

### Fuse declares a conflict on something that should auto-resolve

Most common cause: the language isn't in `.gitattributes`.

```bash
cat .gitattributes
# Should have: *.go merge=fuse  (and similar for each language)
```

If the file extension is included but Fuse still declares a conflict, the conflict may be genuinely ambiguous — Fuse falls back to conflict markers when confidence < 0.70. Read the handoff at `.git/fuse/conflict-<hash>.md` for full context.

### Fuse merged something incorrectly

This is serious — please file an issue with the three input files (base, ours, theirs) and the produced merge. Fuse aims for symbol-correct merges; an incorrect merge is a bug.

To recover the unmerged state:
```bash
git merge --abort
```

To force line-level merge for this file:
```bash
git -c merge.fuse.driver='git-merge-file %A %O %B' merge <branch>
```

### `.git/fuse/conflict-*.md` files are accumulating

These are conflict handoff prompts left behind after Fuse failed to auto-resolve. Once you've resolved the conflict (manually or via AI), you can delete them:

```bash
rm .git/fuse/conflict-*.md
```

Or add `.git/fuse/` to your `.gitignore` if it isn't already.

### Fuse is slow

Per-file merge target is < 1 second. If you're seeing seconds:

1. Grove must be running and reachable — Fuse waits for `/impact` and `/deps` responses
2. Very large files (10K+ lines) take longer to parse with Tree-sitter
3. Check `.git/fuse/audit.json` for the per-phase timing

---

## Provasign

### `provasign init` didn't detect my coding tool

Detection looks at standard config locations. If your tool is installed unusually:

```bash
provasign mcp install-for claude-code    # writes ~/.claude.json (global user config)
provasign mcp install-for cursor         # writes .cursor/mcp.json
provasign mcp install-for windsurf       # writes .windsurf/mcp.json
provasign mcp install-for continue       # writes .continue/config.json
```

### Stage 1 fails — build or tests aren't passing

```bash
provasign certify --verbose
```

The output shows the exact failing test or build error. Reproduce outside Provasign:

```bash
# For Go projects
cd .provasign/.cache/worktree && go test ./...

# For Node
cd .provasign/.cache/worktree && npm test

# For Python
cd .provasign/.cache/worktree && pytest
```

If it passes outside Provasign but fails inside, the issue is likely:
- Worktree state — Provasign uses a clean git worktree, so uncommitted changes aren't included
- Toolchain — Provasign uses whatever's on the PATH at `.provasign/provasign.yaml` evaluation time

### Stage 2 finds something I want to suppress

Use a per-rule suppression in `.provasign/policies/`:

```yaml
# .provasign/policies/sast.yaml
suppressions:
  - rule: G101
    file: internal/legacy/credentials.go
    reason: "Historical credential format, replaced by KMS in next release"
```

Suppressions are audited — every suppression is logged with the policy version that allowed it.

### Pre-push hook is blocking me — I need to push now

```bash
git push --no-verify
```

This is audited in `.provasign/.cache/audit.log`. Used occasionally for emergency hotfixes, this is fine. Used constantly, it means your gates are too strict — adjust `.provasign/provasign.yaml`.

### Certificate verification fails

```
provasign cert verify abc123
# verification failed: signature mismatch
```

Possible causes:
1. The certificate was tampered with — investigate
2. The Ed25519 key was rotated — old certs require the old key to verify
3. The clock skew between signing and verification is large (rare)

To inspect the certificate manually:
```bash
provasign cert show --raw abc123
```

### `provasign cert replay` returns `tool_drift`

This means the gates would still pass, but with different tool versions than at the time of admission. The cert isn't *invalid* — it's not byte-reproducible because tools have moved.

To get back to byte-reproducible:
```bash
# Pin tool versions in .provasign/provasign.yaml
tools:
  semgrep: 1.42.0
  gitleaks: 8.18.2
  govulncheck: 1.1.0
```

Re-running `provasign certify` with pinned tools produces byte-reproducible certs from this point forward.

### Provasign can't reach Grove

In the embedded model Provasign links Grove directly — there is nothing to reach.
If you see a "grove unreachable" error, you're on a pre-embedded build;
upgrade to the latest release.

### Intent capture is creating duplicate intents

`provasign intent open` creates a new draft each call. If your wrapper is calling it on every iteration, you'll get duplicates. Capture the intent once at task start; reference it via `--id` in subsequent calls.

For agent-driven workflows, the recommended pattern is in [docs/agent-prompt.md](https://github.com/provasign/provasign/blob/main/provasign/docs/agent-prompt.md).

---

## Environment / Installation

### `make install` fails — Go not found

Provasign requires Go 1.22+.

```bash
# macOS
brew install go

# Linux
# See https://go.dev/doc/install

# After install, ensure $GOPATH/bin is on your PATH
export PATH="$HOME/go/bin:$PATH"
```

### Binaries aren't on the PATH after install

Add `$GOPATH/bin` (typically `~/go/bin`) to your shell rc file:

```bash
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

### CGO errors on install

Grove's `parser` package uses Tree-sitter (CGO). If you see CGO errors:

```bash
# macOS — install Xcode CLI tools
xcode-select --install

# Linux — install gcc
sudo apt install build-essential   # Debian/Ubuntu
sudo dnf install gcc               # Fedora
```

### Wrong binary version

```bash
which prism      # check the path
prism version    # check the version

# If they don't match, you may have multiple installations
type -a prism
```

Remove old installations and reinstall:
```bash
rm $(which prism)
cd /provasign/prism && make install
```

---

## Logs and Diagnostics

Each product writes structured logs.

| Product | Log location | Verbose flag |
|---------|--------------|--------------|
| Grove | stderr | `grove --verbose serve` |
| Prism | stderr | `prism --verbose query ...` |
| Fuse | `.git/fuse/audit.json` (decisions) + stderr | `fuse --verbose merge ...` |
| Provasign | `.provasign/.cache/provasign.log` | `provasign --verbose certify` |

For bug reports, the relevant log + the command used + the platform (`uname -a`) usually suffices.

---

## Last Resort

If something is fundamentally broken, the reset path:

```bash
# Per-project reset (Grove only)
rm -rf .grove

# Per-project reset (Prism, Fuse, Provasign all preserve project config)
rm -rf .grove .git/fuse .provasign/.cache

# Per-user reset (re-init from scratch)
rm -rf ~/.provasign/keys ~/.cache/prism
provasign init --stack=<stack>   # regenerates Ed25519 key
```

**Never** run `rm -rf .provasign/` — that destroys your committed configuration.
