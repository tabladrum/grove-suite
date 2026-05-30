# Grove Suite — Architecture

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
| `/status` | GET | — | Prism, Relay |
| `/index` | POST | `{"dir": string}` | All |
| `/query` | POST | `{"intent": string, "limit": int}` | Prism, Relay (agent context) |
| `/impact` | POST | `{"file": string, "line": int}` | Fuse, Relay (blast radius) |
| `/deps` | POST | `{"file": string}` | Fuse, Prism, Relay (dependency graph) |
| `/symbols` | POST | `{"query": string}` | Prism, Fuse, Relay |
| `/tests` | POST | `{"query": string}` | Prism, Relay (certification) |
| `/icr` | POST | `{"intent": string}` | Relay (GS scoring + decomposition) |

---

## Grove's Role Across the Full Pipeline

Grove is not just a context delivery backend — it drives three distinct decisions in Relay's pipeline:

| Phase | Grove endpoint | What it decides |
|-------|---------------|-----------------|
| Intake: GS scoring | `/icr` | Is the intent scoped tightly enough to execute? (symbol count) |
| Intake: decomposition | `/icr` → connected components via `/deps` | Can a broad intent be split into independent parallel sub-intents? |
| Execution: agent context | `/query` `/symbols` `/deps` via Prism | What code context does each agent need? |
| Certification: test selection | `/impact` `/tests` | Which tests cover the changed symbols? |
| Certification: blast radius | `/impact` `/deps` | What else might break? |
| Merge: parallel agent output | `/impact` `/deps` via Fuse | Semantic merge of independent ChangeSets |

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

**Threat boundary.** This model stops network-adjacent and browser-based attacks. It does not stop other local processes running as the same user — those can read `.grove/.token`. Full isolation would require OS-level sandboxing outside the scope of these tools.

---

## Data Flows

### AI agent using Prism

```
Agent task description ("add rate limiting to login")
     │
     ▼
prism_query
     │
     ├──► POST grove:7777/symbols  → seed symbols
     ├──► POST grove:7777/query    → BFS-ranked candidates
     │
     ▼
5-signal ranking
(graph distance + semantic similarity + recency + test relevance + edit frequency)
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

### Relay intent lifecycle (full pipeline)

#### Phase 1 — Intake and routing (built)

```
Work item arrives (Jira webhook / GitHub webhook / CLI / API)
     │
     ▼
Project routing  (B → C → A)
     │
     ├── B: explicit "Relay Project" field on ticket → direct route
     ├── C: no unique match → status: unrouted (human assigns via dashboard)
     └── A: component/label filter → route if exactly one project matches
     │
     ▼
Intent created with project_id
     │
     ▼
GS check — two stages:
     │
     ├── Stage 1: heuristic (word count, vagueness, specificity signals)
     │
     ├── Stage 2: POST grove:7777/icr → affected symbol count
     │           1–50 symbols  → GS 0.70–0.95 (well-scoped)
     │           51–150        → GS 0.40–0.70 (borderline)
     │           151+          → GS < 0.40    (too broad)
     │
     │   Final GS = 40% heuristic + 60% ICR
     │
     ├── GS ≥ threshold → status: queued (auto or manual approval)
     └── GS < threshold → status: needs_info + feedback to source system
```

#### Phase 2 — Execution (planned)

```
Queued intent
     │
     ▼
Grove decomposition
     │
     ├──► POST grove:7777/icr   → affected symbol list
     ├──► POST grove:7777/deps  → edges between affected symbols
     │
     │   union-find on edge set → connected components
     │
     │   Component A         Component B         (no edges between them)
     │   RateLimiter         Logger
     │   LoginHandler        RequestLogger
     │   TokenValidator      AuditWriter
     │
     ▼
One K8s Job per independent component (parallel)

     ┌─────────────────────┐   ┌─────────────────────┐
     │  Agent Pod A        │   │  Agent Pod B        │
     │                     │   │                     │
     │  grove index        │   │  grove index        │
     │  prism_query →      │   │  prism_query →      │
     │    context for A    │   │    context for B    │
     │  claude implements  │   │  claude implements  │
     │  git diff → patch   │   │  git diff → patch   │
     └────────┬────────────┘   └──────────┬──────────┘
              │                           │
              └──────────┬────────────────┘
                         ▼
                   Fuse semantic merge
                   (parallel agent output →
                    single unified diff)
                         │
                         ▼
                   Certification
                   ├── POST grove:7777/impact → blast radius
                   ├── POST grove:7777/tests  → which tests to run
                   ├── lint + test suite
                   └── deploy dry-run
                         │
                         ▼
                   Admission
                   rebase → linear commit to main (no branches)
                         │
                         ▼
                   Canary deployment
                   metric validation → intent marked "realized"
```

**Why Grove drives decomposition:** Two symbols are independently executable if they share no `calls`, `uses-type`, or `imports` edges in the code graph. Grove's BFS traversal over the affected symbol set finds these connected components in O(V+E). This is the same graph used by Fuse for blast radius — the data is already there.

**Why Fuse is the merge layer:** When parallel agents produce ChangeSets, Fuse resolves them at symbol granularity rather than line granularity. Agents working on structurally independent symbols cannot produce conflicts at the symbol level — but they can touch adjacent lines. Fuse's symbol-aware pipeline handles this correctly; line-level merge would produce false conflicts.

---

## Relay Data Model

```
Repo
  id, name, url, default_branch
  e.g. "backend" → https://github.com/acme/backend
    │
    │ one repo → many projects (monorepo support)
    ▼
Project
  id, name, repo_id, source_path, gs_threshold, auto_approve, owner
  e.g. "auth-service"  path: /services/auth
  e.g. "payments"      path: /services/payments
    │
    ├── many ProjectIntegrations (M:M with external boards)
    │     id, project_id, type, external_id, config (JSONB)
    │     type: "jira" | "github_issues" | "github_projects" | "linear"
    │     external_id: "AUTH" (Jira board key), "acme/backend" (GitHub repo)
    │
    └── many Intents
          id, project_id, description, source, source_ref
          status, gs_score, icr_symbols, author
          created_at, updated_at, approved_at, rejected_at
```

### Intent state machine

```
                    ingest
                      │
         B resolved?  │  B failed?
              │       │       │
              ▼       │       ▼
           draft      │    unrouted ──► human assigns project
              │       │       │               │
              ▼       └───────┘               │
         validating ◄──────────────────────────┘
              │
     ┌────────┼────────┐
     ▼        ▼        ▼
needs_info  queued  rejected
     │
  resubmit
     │
     ▼
validating
```

### Routing algorithm (B → C → A)

```
Webhook received (type="jira", external_id="AUTH", ticket data)
     │
     ▼
1. Find ProjectIntegrations where type=jira AND external_id=AUTH
   → zero matches: ignore (board not registered)
   → one or more: proceed
     │
     ▼
2. [B] Extract explicit relay-project value from ticket:
       Jira:   custom field "Relay Project"
       GitHub: label "relay-project:<name>"
   → match found: route to named project
     │
     ▼
3. [A] Apply component/label filter on each matching integration config
   → exactly one match: route to that project
   → multiple or zero: fall through
     │
     ▼
4. [C] Create intent with status=unrouted
       Human assigns via dashboard or:
       relay intent assign <id> --project auth-service
```

---

## Module Paths

| Directory | Module |
|-----------|--------|
| `grove/` | `github.com/tabladrum/grove-suite/grove` |
| `prism/` | `github.com/tabladrum/grove-suite/prism` |
| `fuse/` | `github.com/tabladrum/grove-suite/fuse` |
| `relay/` | `github.com/tabladrum/grove-suite/relay` |

All four are wired into a Go workspace at `go.work` in the repository root.

---

## Key Invariants

- **CGO is isolated.** Only `grove/internal/parser/` contains CGO (tree-sitter). The rest of Grove and all of Prism/Fuse/Relay are pure Go.
- **Delta indexing.** Grove never re-parses a file whose git blob SHA hasn't changed. Tests that force re-parsing must either change file content or clear the SHA cache.
- **Scoped edges.** `calls` and `uses-type` edges are only created within the same file or across `imports` edges. Violating this produces false positives proportional to the number of symbols with common names.
- **Symbol IDs are content-addressed.** An ID `path::name@sha` becomes stale as soon as the file content changes. Stale IDs must not be stored in external systems without a revalidation step.
- **Decomposition requires edge absence.** Two work items are independently executable only when no `calls`, `uses-type`, or `imports` edge connects their symbol sets. Relay must verify this before spawning parallel agents.
- **Fuse is the merge layer for parallel agents.** When multiple agents produce ChangeSets from the same base commit, Fuse's semantic merge resolves them. Line-level merge is not sufficient.
- **Postgres is operational state (Relay).** Intent proposals, project config, routing rules live in Postgres. Approved intent snapshots and certificates live in git. These are separate concerns.
