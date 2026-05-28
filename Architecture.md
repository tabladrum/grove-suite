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
│         │  MCP   │   │ driver)│    │   gRPC    │              │
│         └────────┘   └────────┘    └───────────┘              │
└────────────────────────────────────────────────────────────────┘
```

## Grove HTTP API

All consumers use this API. All endpoints except `/health` require `Authorization: Bearer <token>`.

| Endpoint | Method | Request | Consumer |
|----------|--------|---------|----------|
| `/health` | GET | — | Prism, Fuse, Relay (startup check) |
| `/status` | GET | — | Prism, Relay |
| `/index` | POST | `{"dir": string}` | All |
| `/query` | POST | `{"intent": string, "limit": int}` | Prism |
| `/impact` | POST | `{"file": string, "line": int}` | Fuse, Relay |
| `/deps` | POST | `{"file": string}` | Fuse, Prism, Relay |
| `/symbols` | POST | `{"query": string}` | Prism, Fuse |
| `/tests` | POST | `{"query": string}` | Prism, Relay |
| `/icr` | POST | `{"intent": string}` | Relay |

## Startup Contract

Prism, Fuse, and Relay all implement identical startup logic:

1. Read `$GROVE_URL` (default `http://localhost:7777`)
2. `GET $GROVE_URL/health`
3. If unreachable: `exec grove serve --port 7777 .`
4. Poll `/health` with 500ms backoff for up to 10 seconds
5. If still unreachable after 10s: fatal exit with setup instructions

This means you can run `prism query "task"` in a project and Grove will start automatically if it isn't running. The Grove process persists after the Prism process exits — subsequent calls find it already running.

## Security Model

**Network binding.** All HTTP servers bind to `127.0.0.1`, not `0.0.0.0`. There is no LAN exposure.

**Shared secret token.** Grove generates a 64-character hex token from `crypto/rand` on first start, writes it to `.grove/.token` with mode 0600, and requires `Authorization: Bearer <token>` on all non-health requests. Prism reads the token from the same path via `WithTokenFromDir`. Fuse and Relay follow the same pattern.

**Threat boundary.** This model stops network-adjacent and browser-based attacks. It does not stop other local processes running as the same user — those can read `.grove/.token`. Full isolation would require OS-level sandboxing outside the scope of these tools.

## Data Flows

### AI agent using Prism

```
Agent task description ("add rate limiting to login")
     │
     ▼
prism_query
     │
     ├──► GET grove:7777/symbols?q=login      → seed symbols
     ├──► GET grove:7777/query?intent=login   → BFS-ranked candidates
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

### Relay intent lifecycle

```
Intent YAML → granularity check (GS ≥ 0.7)
     │
     ├──► POST grove:7777/icr        → complexity rating
     ├──► Redis lock acquisition
     │
     ▼
Agent execution (ephemeral K8s namespace)
     │
     ▼
ChangeSet output
     │
     ▼ [optional review gate]
     │
     ▼
Certification (3 stages: lint + test + deploy-dry-run)
     │
     ▼
Admission: rebase → linear commit to main (no branches)
     │
     ▼
Canary deployment → metric validation → intent marked "realized"
```

## Module Paths

| Directory | Module |
|-----------|--------|
| `grove/` | `github.com/tabladrum/grove-suite/grove` |
| `prism/` | `github.com/tabladrum/grove-suite/prism` |
| `fuse/` | `github.com/tabladrum/grove-suite/fuse` |

All three are wired into a Go workspace at `go.work` in the repository root. Build any product with `go build ./...` from within its directory, or `go build ./grove/...` from the root.

## Key Invariants

These constraints are enforced by tests and should not be relaxed without understanding their downstream effects:

- **CGO is isolated.** Only `grove/internal/parser/` contains CGO (tree-sitter). The rest of Grove and all of Prism/Fuse/Relay are pure Go.
- **Delta indexing.** Grove never re-parses a file whose git blob SHA hasn't changed. Tests that force re-parsing must either change file content or clear the SHA cache.
- **Scoped edges.** `calls` and `uses-type` edges are only created within the same file or across `imports` edges. Violating this produces false positives proportional to the number of symbols with common names.
- **Symbol IDs are content-addressed.** An ID `path::name@sha` becomes stale as soon as the file content changes. Stale IDs must not be stored in external systems without a revalidation step.
- **Redis is transient (Relay).** `appendonly no`. All business state lives in git. A Redis loss must be recoverable by re-reading the intent-store repo.
