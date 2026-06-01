# Grove Suite — Architecture

> **Embedded mode:** Grove is now linked into Prism, Fuse, and Relay as a Go library (`grove/pkg/grove`). The HTTP/gRPC daemon and bearer-token model described below were the v1 design; the current implementation opens the on-disk index in-process. Sections that mention `:7777`, `:7778`, `GROVE_URL`, or `.grove/.token` are historical.

## Dependency Map

Grove is the only product with its own storage. All others are clients.

```
┌────────────────────────────────────────────────────────────────┐
│                        Grove Suite                             │
│                                                                │
│   ┌──────────────────────────────────────────────────────┐    │
│   │                        Grove                         │    │
│   │  Tree-sitter parser · SQLite graph · BFS traversal  │    │
│   │  11 languages · 8 edge types · MCP + HTTP + gRPC    │    │
│   │  127.0.0.1:7777 (HTTP) · 127.0.0.1:7778 (gRPC)     │    │
│   └───────────────────────┬──────────────────────────────┘    │
│                           │  HTTP + shared-secret token        │
│              ┌────────────┼────────────────┐                   │
│              ▼            ▼                ▼                   │
│         ┌────────┐   ┌────────┐    ┌───────────┐              │
│         │ Prism  │   │  Fuse  │    │   Relay   │              │
│         │ :8888  │   │  (git  │    │   :9000   │              │
│         │  MCP   │   │ driver)│    │           │              │
│         └────────┘   └────────┘    └───────────┘              │
└────────────────────────────────────────────────────────────────┘
```

---

## Grove HTTP API

All consumers use this API. All endpoints except `/health` require `Authorization: Bearer <token>`.

| Endpoint | Method | Request | Consumer |
|----------|--------|---------|----------|
| `/health` | GET | — | Prism, Fuse, Relay (startup check) |
| `/status` | GET | — | Prism, Relay, VS Code extension |
| `/index` | POST | `{"dir": string}` | All |
| `/query` | POST | `{"intent": string, "limit": int}` | Prism, Relay (agent context) |
| `/impact` | POST | `{"file": string, "line": int}` | Fuse, Relay (blast radius) |
| `/deps` | POST | `{"file": string}` | Fuse, Prism, Relay (dependency graph) |
| `/symbols` | POST | `{"query": string}` | Prism, Fuse, Relay |
| `/tests` | POST | `{"query": string}` | Prism, Relay (certification) |
| `/icr` | POST | `{"intent": string}` | Relay (GS scoring + decomposition) |

---

## Grove's Role Across the Full Pipeline

| Phase | Grove endpoint | What it decides |
|-------|---------------|-----------------|
| Intake: GS scoring | `/icr` | Is the intent scoped tightly enough to execute? |
| Intake: decomposition (Phase 3) | `/icr` → connected components via `/deps` | Can a broad intent be split into independent parallel sub-intents? |
| Execution: agent context | `/query` `/symbols` `/deps` via Prism | What code context does each agent need? |
| Certification: test selection | `/impact` + `/tests` | Which tests cover the changed symbols? |
| Certification: blast radius | `/impact` + `/deps` | What else might break? |
| Merge: parallel agent output (Phase 3) | `/impact` + `/deps` via Fuse | Semantic merge of independent ChangeSets |

---

## Startup Contract

Prism, Fuse, and Relay all implement identical startup logic:

1. Read `$GROVE_URL` (default `http://localhost:7777`)
2. `GET $GROVE_URL/health`
3. If unreachable: `exec grove serve --port 7777 .`
4. Poll `/health` with 500ms backoff for up to 10 seconds
5. If still unreachable after 10s: fatal exit with setup instructions

---

## Security Model

**Network binding.** All HTTP servers bind to `127.0.0.1`, not `0.0.0.0`. There is no LAN exposure.

**Shared secret token.** Grove generates a 64-character hex token from `crypto/rand` on first start, writes it to `.grove/.token` with mode 0600, and requires `Authorization: Bearer <token>` on all non-health requests. Prism reads the token from the same path via `WithTokenFromDir`. Fuse and Relay follow the same pattern.

**Ed25519 admission key.** Relay generates a local Ed25519 keypair at `~/.relay/keys/admission.ed25519` (mode 0600) on `relay init`. Every admitted commit is signed over CanonicalBytes (config hash + ChangeSet + test results + findings). The admitted commit SHA is recorded in the trailer *after* signing — the cert is valid before the commit exists.

**Threat boundary.** This model stops network-adjacent and browser-based attacks. It does not stop other local processes running as the same user — those can read token files. Full isolation would require OS-level sandboxing outside the scope of these tools.

---

## Data Flows

### AI agent using Prism (MCP or VS Code)

```
Agent task description ("add rate limiting to login endpoint")
     │
     ▼
prism_query
     │
     ├──► POST grove:7777/symbols  → seed symbols
     ├──► POST grove:7777/query    → BFS-ranked candidates
     │
     ▼
5-signal ranking
(graph distance · semantic similarity · recency · test relevance · edit frequency)
     │
     ▼
Budget allocation
(target 35% · deps 25% · tests 20% · doc 10% · summary 10%)
     │
     ▼
Progressive disclosure
(full source on first read · signature on second · reference on third+)
     │
     ▼
Session deduplication (LRU — already-seen symbols downranked)
     │
     ▼
Token-optimized context pack → agent
```

### VS Code (Prism extension)

```
VS Code workspace
     │
     ├── Left status bar: "$(database) Grove N syms"  — Grove symbol count
     ├── Left status bar: "$(graph) Prism X.X%"       — Prism session savings
     │
     ├── Copilot Agent mode:
     │     #prismQuery, #prismRead, #prismSearch, #prismLookup,
     │     #prismIndex, #prismSavings, #prismCompact, #prismFeedback
     │
     └── Auto-index on file save
```

The standalone Grove VS Code extension (grove-vscode) has been retired. The Prism extension owns the VS Code surface and provides full Grove + Prism integration.

### Git merge with Fuse

```
git merge <branch>
     │  .gitattributes: *.go merge=fuse
     ▼
fuse merge <base> <ours> <theirs> <path>
     │
     ├──► POST grove:7777/impact   → blast radius of changed symbols
     ├──► POST grove:7777/deps     → cross-file dependencies
     │
     ▼
IntelliMerge 7-phase pipeline:
  context building → symbol extraction → recency analysis →
  graph context → breaking change detection →
  conflict classification → strategy selection
     │
     ├── clean merge  → write merged file · exit 0
     └── conflict     → write conflict markers
                        write .git/fuse/conflict-<sha>.md (AI handoff)
                        exit 1
```

### Relay certification pipeline

```
agent writes code
     │
     ▼
relay_intent_open → intent YAML saved to .relay/.cache/intents/
     │
     ▼
relay_check  (fast, in-loop — sub-10 s target)
  └── SAST on changed files + Grove-affected unit tests only
     │  structured findings (file, line, rule, severity, fix-hint) returned to agent
     ▼  agent self-corrects; loops up to 3× until Allowed=true
     │
     ▼  (agent calls relay_certify only after relay_check Allowed=true)
relay_certify
     │
     ├── Stage 1: build + full test suite (git worktree isolation)
     │     └── coverage-of-changed-symbols gate (vs Grove /tests edges)
     │
     ├── Stage 2: static analysis (semgrep, gitleaks, govulncheck, linters)
     │
     ├── Risk heatmap: ICR (0.30) + severity (0.30) + coverage (0.25) + touch (0.15)
     │
     ├── Policy gates (path, secrets, fileclass, deps, size, coverage)
     │
     └── Admission
           ├── rebase onto target branch
           ├── Ed25519 sign CanonicalBytes
           ├── linear commit with full trailer
           └── certificate persisted + intent-store git snapshot

relay_intent_close → intent promoted to .relay/intents/ + committed
```

---

## Relay Data Model

```
Repo
  id, name, url, default_branch

    └── Project (many per repo — monorepo support)
          id, name, repo_id, source_path, gs_threshold, auto_approve, owner

            └── ProjectIntegration (M:M with external boards)
                  type: jira | github_issues | github_projects | linear
                  external_id: "AUTH" | "acme/backend"
                  config: trigger_status, label_trigger, component_filter, auto_approve

            └── Intent
                  project_id → Project
                  description, status, gs_score, icr_symbols
                  source: jira | github_issue | native | mcp
                  source_ref: AUTH-123 | acme/backend#42

            └── ChangeSet (Phase 2A)
                  intent_id → Intent
                  diff, agent_id, model_version

            └── Certificate (Phase 2A)
                  changeset_id → ChangeSet
                  stage1_result, stage2_findings, risk_score
                  signature (Ed25519), effective_config_hash
                  admitted_commit_sha, admitted_branch
```

---

## Module Paths

```
github.com/tabladrum/grove-suite/grove     go.mod: grove/go.mod
github.com/tabladrum/grove-suite/prism     go.mod: prism/go.mod
github.com/tabladrum/grove-suite/fuse      go.mod: fuse/go.mod
github.com/tabladrum/grove-suite/relay     go.mod: relay/go.mod
github.com/tabladrum/grove-suite/astkit    go.mod: astkit/go.mod
```

Go workspace: `go.work` at the repo root references all modules.

---

## Key Invariants

1. **Grove is always the single source of graph truth.** No product rebuilds the symbol graph internally.
2. **All HTTP servers bind to `127.0.0.1` only.** Never `0.0.0.0`.
3. **Token files use mode 0600.** Generated from `crypto/rand`. Never committed.
4. **Relay's CanonicalBytes excludes `admitted_commit_sha`.** The cert is signed before the commit exists; the SHA is appended to the trailer post-commit.
5. **Delta indexing by git blob SHA.** If the SHA matches, the file is never re-parsed, regardless of project size.
6. **Fuse parses merge versions in-memory.** It does not write base/ours/theirs to disk or call Grove's indexer. Grove is queried only for cross-file context (impact, deps).
7. **The Prism extension is the sole VS Code surface.** The standalone Grove extension (grove-vscode) has been retired; Grove status is surfaced via the Prism extension's left status bar items.
8. **`relay init` is the one-command agent wiring step.** It writes Pre-Flight Autopilot instructions to every per-agent instruction file and registers the relay MCP server with every detected tool — idempotent on re-run.
