# Master Prompt: Intent-Driven Software Delivery Platform

## Instructions for Agent

You are building an **Intent-Driven Software Delivery Platform** — a system that replaces traditional CI/CD (Jenkins, GitHub PRs, branch-based workflows) with an agent-native, intent-first orchestration platform designed for the era of AI-generated code.

This is a greenfield project. Build it phase by phase. Follow the architecture precisely. Ask clarifying questions only when genuinely blocked — otherwise execute autonomously.

**Language:** Go (entire platform). Single language for all components. **Infrastructure:** Kubernetes, Redis (no persistence), Git (3 repos), OCI container registry. **No other databases.** No PostgreSQL, no etcd, no Kafka, no S3. **Why Go:** K8s native ecosystem, goroutines for concurrency, 3–5x faster than Node.js for graph operations, low memory footprint, single binary deployment, industry standard for infrastructure software.

-----

## What You Are Building

A platform where:

1. Humans define **intents** (structured descriptions of what should change)
1. Intents go through collaborative refinement and multi-party sign-off
1. Sign-off triggers autonomous **agent execution** (Claude, Codex, etc.)
1. Agents write code within **isolated change regions** (preventing merge conflicts)
1. Code passes an optional **human review gate** (team-configurable, shifted left for fast failure)
1. Code is **certified** (build, test, security, semantic validation) without traditional CI
1. Certified code is **admitted** to a linear main branch (no PRs, no branches)
1. Artifacts are **progressively deployed** with intent-metric validation
1. Production continuously validates **intent preservation**

This replaces: Jira, GitHub PRs, Jenkins/Actions, branch management, and traditional deployment pipelines. Human code review is retained as an optional policy — not eliminated.

-----

## Core Design Decisions (Non-Negotiable)

### Storage: Git-Native

- **Source code repo:** Linear history, no branches. Every commit has trailers linking to intent ID and certificate.
- **Intent store repo:** Intent YAML files organized by state (`active/`, `completed/`, `failed/`). Every state change = a git commit. Git log = full audit trail.
- **Platform config repo:** Policies, domain definitions, agent configs as YAML.
- **Redis:** Transient coordination only — ICR locks (SET NX EX), execution queue (sorted sets), agent heartbeats (key TTL), graph cache (rebuilt from git on loss). Zero persistence configured.
- **OCI registry:** Container images only.

### No Branches (Core Architectural Strength)

Git is used for storage and audit, not for branching workflows. Source repo has a single `main` branch with linear history. Agents work on cloned snapshots, produce diffs, diffs are rebased and committed by the admission controller.

**Why no branches is the killer feature:**

- Eliminates merge conflicts entirely (ICR isolation replaces branches)
- Forces proper intent decomposition (each commit must be independently deployable)
- Makes audit trail trivial (linear = sequential events)
- Simplifies CKG (one graph state, not per-branch)
- Enables simple rollback (revert any commit on linear line)

**Allowed:** Single `main` branch, git tags (release marking), agent pod workspaces (ephemeral). **Not allowed:** Feature branches, release branches, hotfix branches, long-lived branches.

**Edge cases solved without branches:**

- Hotfixes → Priority Queue (high-priority intents skip the queue)
- Atomic multi-change operations → Intent Groups
- Experimentation → Agent pods (disposable, never persisted to git)
- Large refactoring → Incremental migration + feature flags
- Rollback → Deploy any previous certified SHA
- Release tracking → Git tags

-----

## Intent Groups (Atomic Multi-Intent Composition)

When multiple sub-intents MUST merge and deploy together (e.g., database migration + code change):

```yaml
apiVersion: intent/v1
kind: IntentGroup
metadata:
  id: "IG-{year}-{sequence}"
  created: "ISO8601"
  created_by: "email"

spec:
  title: "Group description"
  intents: ["INT-2026-001-A", "INT-2026-001-B", "INT-2026-001-C"]
  merge_strategy: atomic    # all ChangeSets merge together or none do
  deploy_strategy: atomic   # deploy as single unit
  ordering: sequential      # or "parallel" if sub-intents are independent

status:
  state:
    "pending|executing|review_pending|certifying|certified|merged|deployed|realized|failed"
  intent_states:
    INT-2026-001-A: "certified"
    INT-2026-001-B: "certified"
    INT-2026-001-C: "executing"
```

**Behavior:**

- Each sub-intent executes independently (may be parallel if non-overlapping ICRs)
- Each sub-intent is certified independently
- Admission controller HOLDS all ChangeSets until ALL sub-intents are certified
- Then merges ALL as a sequential batch commit (preserving linear history)
- Deployment treats the group as one deployable unit
- If ANY sub-intent fails certification after 3 retries, the entire group fails
- Each sub-intent must still pass granularity scoring individually
- Each sub-intent SHOULD leave main in a valid state (but group constraint relaxes this for `atomic` merge strategy)

**Constraint:** Intent groups are limited to 10 sub-intents maximum. Larger groups indicate insufficient decomposition.

-----

## Priority Queue (Hotfix Fast-Path)

Intents can be assigned priority levels that affect queue ordering and execution:

```yaml
# In intent spec:
priority:
  level: critical    # critical|high|normal|low
  reason: "Production P1 incident - auth service failing"
  bypass_rules:
    skip_staging_e2e: true    # for critical only
    skip_canary: false        # never skip canary
    skip_queue: true          # execute immediately, don't wait for queue
```

**Priority Levels:**

|Level   |Queue Behavior                     |Certification                     |Deployment                                |
|--------|-----------------------------------|----------------------------------|------------------------------------------|
|critical|Immediate execution, preempts queue|Static only (skip integration env)|Canary 1% → 100% (accelerated, 2min gates)|
|high    |Front of queue, next to execute    |Full certification, parallelized  |Normal canary (shortened gates)           |
|normal  |FIFO within priority band          |Full certification                |Normal canary                             |
|low     |Back of queue, yield to others     |Full certification                |Normal canary (extended gates)            |

**Rules:**

- `critical` requires explicit justification and `security_champion` approval
- `critical` intents still run SAST and unit tests (never skip security)
- `critical` intents still deploy via canary (never direct to 100%)
- Maximum 3 `critical` intents per 24-hour window (prevents abuse)
- All priority overrides are logged in intent audit trail

**Lock Preemption:**

- `critical` intents can preempt `low` priority locks (force-release + requeue the low intent)
- `critical` CANNOT preempt `normal` or `high` (to prevent cascading disruption)
- Preempted intents are automatically re-queued and re-executed from scratch

-----

## Code Knowledge Graph (CKG)

Based on the `gctx/InfiniContext` architecture:

- Tree-sitter parsing (TypeScript, JavaScript, Python, Go, Java)
- 8 edge types: `defines`, `imports`, `calls`, `extends`, `implements`, `uses-type`, `tests`, `contains`
- Confidence scoring per edge (0.0–1.0)
- BFS traversal for neighbor discovery and distance computation
- Delta indexing via git blob SHAs (only re-parse changed files)
- In-memory adjacency lists cached in Redis
- Rebuilt from source repo on cache loss (~5s for 5000 files)

### Intent Granularity Enforcement

Before an intent can be signed, it must pass granularity scoring:

```
Granularity Score (GS) = Specificity × (1 − Decomposability)
Minimum threshold: GS >= 0.7
```

Decomposability factors: estimated file count, domain crossing count, distinct behavior count, external dependency count. Specificity factors: testable acceptance criteria, defined constraints, unambiguous language, error cases specified.

Intents below 0.7 are blocked with AI-generated decomposition suggestions.

### Isolation via Graph-Based Locking

- Intent → CKG query → identify target nodes → BFS impact radius → classify as exclusive/shared-read/boundary = ICR
- ICR mapped to Redis lock keys
- Lock acquired atomically (SET NX EX) before agent starts
- Lock auto-expires on TTL (dead agent safety)
- Conflicting intents are queued or serialized

### Conflict Detection (3 Layers)

1. **Structural** (pure set operations): File/symbol overlap between ICRs
1. **Contract** (graph + analysis): API path overlap, shared type modification, event schema conflicts
1. **Semantic** (LLM-powered): Behavioral interference detected by reasoning about intent pairs. Only invoked for same-domain intents with shared boundaries.

-----

## Human Review Gate (Team-Configurable)

Human code review is an **optional, team-configurable policy gate** positioned between agent output and certification (shifted left — rejection wastes zero certification compute).

**Policy modes:** `none` (skip entirely), `optional` (auto-advances on timeout), `mandatory` (blocks until reviewed).

```yaml
# platform-config repo: policies/review.yaml
apiVersion: platform/v1
kind: ReviewPolicy
metadata:
  name: default
spec:
  default_mode: optional     # none|optional|mandatory
  default_timeout: 4h        # auto-advance if no response (optional mode only)
  min_reviewers: 1
  overrides:
    - match: { domains: ["payments", "auth", "pii"] }
      mode: mandatory
      min_reviewers: 2
      required_roles: ["tech_lead"]

    - match: { domains: ["internal-tooling", "docs"] }
      mode: none

    - match: { priority: "critical" }
      mode: optional
      timeout: 15m            # don't block hotfixes

review_context:               # what reviewer sees
  - intent_spec
  - diff
  - icr_boundaries
  - affected_tests
  - granularity_score
```

**Reviewer decisions:**

- `approve` → proceed to certification
- `reject` (reason: “intent is wrong”) → intent fails, notify author
- `reject` (reason: “implementation wrong”) → re-execute agent with reviewer feedback injected as context
- `request_changes` + comments → re-execute agent with comments as additional guidance

**Fail-fast behavior:** Rejection immediately terminates the pipeline for that ChangeSet. No certification compute wasted.

**Key principle:** Review is NOT a replacement for certification. Even after human approval, full machine certification still runs. Reviewers focus on *design/approach correctness*; machines handle *implementation correctness* (build, tests, security).

-----

## Certification (Not CI)

Multi-stage validation pipeline:

- **Stage 1 (static):** Build verification, unit tests, contract tests, SAST, semantic validation
- **Stage 2 (ephemeral env):** Integration tests in auto-provisioned K8s namespace, DAST
- **Stage 3 (staging):** E2E tests from intent acceptance criteria, performance tests (conditional)
- Produces cryptographic certificate attached to intent record

-----

## Agent Execution

- Agents run in ephemeral K8s pods (NOT on developer laptops)
- Each pod: isolated filesystem (read-only except ICR paths), scoped credentials, time limit, cost budget
- Agent receives: intent spec, ICR boundaries, code snapshot, CKG context
- Agent produces: ChangeSet (code diff + tests + metadata)
- Tests are PART of the ChangeSet — a ChangeSet without tests fails certification

-----

## Architecture

```
HUMAN LAYER:
  Intent CLI + Intent Dashboard (web UI)
        |
        ↓ (git commits to intent-store repo)

CONTROL PLANE:
  Orchestrator ——— watches intent-store, triggers workflows
        |
        ├── Granularity Checker (library) ——— enforces GS >= 0.7
        ├── CKG Service ——— graph queries, ICR computation, test selection
        ├── Isolation Engine ——— Redis locks, queue, heartbeats
        └── Conflict Detector ——— 3-layer detection + resolution

EXECUTION LAYER:
  Agent Execution Platform (K8s operator)
        | produces ChangeSet
        ↓

REVIEW LAYER (configurable):
  Review Gate ——— team-configurable human review (none/optional/mandatory)
        | approved or auto-advanced
        ↓

CERTIFICATION LAYER:
  Certification Engine ——— build, test, security, semantic validation
        | produces Certificate
        ↓

DELIVERY LAYER:
  Admission Controller ——— rebase + merge to source repo main
        |
  Deployment Engine ——— canary → progressive → full (with intent metric validation)
```

### Component List

|Component               |Language|Type        |Replicas          |
|------------------------|--------|------------|------------------|
|Orchestrator            |Go      |Long-running|2 (active-passive)|
|CKG Service             |Go      |Long-running|2 (both active)   |
|Isolation Engine        |Go      |Long-running|1                 |
|Conflict Detector       |Go      |Long-running|2                 |
|Agent Execution Platform|Go      |K8s Operator|1                 |
|Certification Engine    |Go      |Job runner  |1 (spawns pods)   |
|Admission Controller    |Go      |Triggered   |1                 |
|Deployment Engine       |Go      |Long-running|1                 |
|Dashboard               |Go + SPA|Long-running|2                 |
|Intent CLI              |Go      |CLI tool    |N/A               |

-----

## Data Schemas

### Intent YAML (stored in intent-store git repo)

```yaml
apiVersion: intent/v1
kind: Intent
metadata:
  id: "INT-{year}-{sequence}"
  parent: null  # or parent intent ID if decomposed
  created: "ISO8601"
  created_by: "email"

spec:
  title: "Short imperative description"
  description: "Detailed description of what should change and why"
  domain: "architectural-domain"
  capability: "specific-capability"
  acceptance_criteria:
    - "Testable assertion 1"
    - "Testable assertion 2"
  constraints:
    - "What must NOT change"
    - "Boundaries and limits"
  estimated_scope:
    files: <number>
    domains: ["list"]

group: "IG-{year}-{sequence}"  # null if standalone, group ID if part of intent group
priority:
  level: "critical|high|normal|low"  # default: normal
  reason: "justification for non-normal priority"
  bypass_rules:
    skip_staging_e2e: false
    skip_queue: false

status:
  state:
    "draft|proposed|signed|executing|review_pending|certifying|certified|merged|deployed|realized|failed"
  granularity_score: <float>

approvals:
  - approver: "email"
    role: "product_owner|tech_lead|security_champion"
    decision: "approved|rejected"
    timestamp: "ISO8601"
    commit: "git-sha"

execution:
  started: "ISO8601"
  agent: "model-name"
  pod: "pod-id"
  icr_hash: "sha256:..."
  cost_so_far: "$X.XX"
  retries: 0

review:
  policy_mode: "none|optional|mandatory"  # resolved from ReviewPolicy at execution time
  status: "skipped|pending|approved|rejected"
  reviewers:
    - reviewer: "email"
      decision: "approved|rejected|request_changes"
      comments: "optional feedback"
      timestamp: "ISO8601"
  auto_advanced: false  # true if timeout elapsed in optional mode

certification:
  id: "cert-id"
  timestamp: "ISO8601"
  build: { status: "pass|fail", duration_ms: N }
  unit_tests: { status, total, passed, failed, coverage }
  integration_tests: { status, total, passed, failed }
  security: { sast: "pass|fail", dast: "pass|fail", vulnerabilities: N }
  semantic: { invariants_preserved: bool, contracts_compatible: bool }

history:
  - event: "state-name"
    timestamp: "ISO8601"
    commit: "git-sha"
```

### Source Repo Commit Format

```
<type>: <description>

<body>

Intent-ID: INT-2026-XXXX
Certificate: cert-YYYY-MM-DD-XXXXX
Agent: <model>:<pod-id>
ICR-Hash: sha256:<hash>
Test-Plan: <N>-unit/<N>-integration/<N>-e2e
Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

### ICR Structure

```go
type IsolatedChangeRegion struct {
    IntentID      string   `json:"intentId"`
    Exclusive     []string `json:"exclusive"`      // node IDs agent will modify
    SharedRead    []string `json:"sharedRead"`     // node IDs agent may read
    Boundary      []string `json:"boundary"`       // edge nodes (interface to rest of graph)
    ExclusiveFiles []string `json:"exclusiveFiles"` // file paths (for human readability)
    ReadableFiles  []string `json:"readableFiles"`
    Confidence    float64  `json:"confidence"`     // 0.0–1.0 (how sure ICR is complete)
    LockKeys      []string `json:"lockKeys"`       // Redis keys for this ICR
}
```

### ChangeSet Structure

```go
type ChangeSet struct {
    IntentID      string   `json:"intentId"`
    Diff          string   `json:"diff"`           // unified diff (git format)
    NewFiles      []string `json:"newFiles"`
    ModifiedFiles []string `json:"modifiedFiles"`
    DeletedFiles  []string `json:"deletedFiles"`
    Tests         struct {
        Unit        []string `json:"unit"`
        Integration []string `json:"integration"`
        E2E         []string `json:"e2e"`
    } `json:"tests"`
    Metadata struct {
        Agent         string  `json:"agent"`
        Model         string  `json:"model"`
        ExecutionTime int64   `json:"executionTime"` // milliseconds
        Cost          float64 `json:"cost"`          // dollars
        TokensInput   int     `json:"tokensInput"`
        TokensOutput  int     `json:"tokensOutput"`
    } `json:"metadata"`
}
```

-----

## CKG Service Specification

### Graph Model (Go structs, ported from gctx TypeScript)

```go
package ckg

type SymbolKind string

const (
    KindFunction    SymbolKind = "function"
    KindMethod      SymbolKind = "method"
    KindClass       SymbolKind = "class"
    KindInterface   SymbolKind = "interface"
    KindType        SymbolKind = "type"
    KindConst       SymbolKind = "const"
    KindEnum        SymbolKind = "enum"
    KindModule      SymbolKind = "module"
    KindVariable    SymbolKind = "variable"
    KindConstructor SymbolKind = "constructor"
    KindField       SymbolKind = "field"
)

type EdgeType string

const (
    EdgeDefines      EdgeType = "defines"
    EdgeImports      EdgeType = "imports"
    EdgeCalls        EdgeType = "calls"
    EdgeExtends      EdgeType = "extends"
    EdgeImplements   EdgeType = "implements"
    EdgeUsesType     EdgeType = "uses-type"
    EdgeTests        EdgeType = "tests"
    EdgeContains     EdgeType = "contains"
)

type LineRange struct {
    Start int `json:"start"`
    End   int `json:"end"`
}

type SymbolRecord struct {
    ID            string     `json:"id"`
    FilePath      string     `json:"filePath"`
    BlobSHA       string     `json:"blobSHA"`
    Language      string     `json:"language"`
    Kind          SymbolKind `json:"kind"`
    Name          string     `json:"name"`
    QualifiedName string     `json:"qualifiedName"`
    Signature     string     `json:"signature"`
    Docstring     string     `json:"docstring,omitempty"`
    Span          LineRange  `json:"span"`
    Imports       []string   `json:"imports"`
    Exports       bool       `json:"exports"`
    RawText       string     `json:"rawText"`
    ParentSymbol  string     `json:"parentSymbol,omitempty"`
    TokenEstimate int        `json:"tokenEstimate"`
    CallSites     []string   `json:"callSites,omitempty"`
    Modifiers     []string   `json:"modifiers,omitempty"`
    Annotations   []string   `json:"annotations,omitempty"`
    TypeParameters []string  `json:"typeParameters,omitempty"`
}

type TKGEdge struct {
    From       string   `json:"from"`
    To         string   `json:"to"`
    Type       EdgeType `json:"type"`
    Confidence float64  `json:"confidence"` // 0.0–1.0
}
```

### Graph Core Data Structures

```go
type CodeGraph struct {
    symbols  []SymbolRecord
    edges    []TKGEdge

    // O(1) adjacency lookup (bidirectional)
    adjacency map[string][]Neighbor

    // Directed typed edge indexes
    outEdgesByType map[string]map[EdgeType]map[string]struct{}
    inEdgesByType  map[string]map[EdgeType]map[string]struct{}

    // Symbol indexes
    symbolsByFile map[string][]SymbolRecord
    symbolsByName map[string][]SymbolRecord
    symbolsByID   map[string]*SymbolRecord

    // Import graph (for scoping call edges)
    importedFilesByFile map[string]map[string]struct{}

    // Config
    tsconfigPaths map[string][]string // alias prefix → resolved dirs
}

type Neighbor struct {
    ID   string
    Type EdgeType
}
```

### Edge Building Algorithm

The graph builds 6 edge types in order:

1. **DEFINES** (confidence: 1.0) — For every symbol: file → symbol edge
1. **IMPORTS** (confidence: 0.9) — For every symbol’s import list, resolve import path to actual file
1. **INHERITANCE** (confidence: 0.85) — Parse signature text for “extends X” and “implements Y” clauses
1. **USES-TYPE** (confidence: 0.5) — Scan each symbol’s signature for uppercase identifiers (type names)
1. **CALLS** (confidence: 0.85 AST-based, 0.6 regex-based) — Build callable index: map[name] → []SymbolRecord
1. **TESTS** (confidence: 0.85) — Match test file patterns: `*_test.go`, `test_*.py`, `*.test.ts`, etc.

### Key Implementation Notes

1. **Call edge scoping is critical.** Without scoping to same-file + imported files, false positives explode. This single design decision reduced false edges by ~80%.
1. **AST-based call extraction >> regex.** When tree-sitter provides call sites, use them (0.85 confidence). Regex fallback (0.6 confidence) is for when AST extraction isn’t available.
1. **Strip comments and strings before regex matching.**
1. **Test linking uses directory proximity.** Simple basename matching + directory scoring gives 95%+ accuracy.
1. **Import resolution must handle aliases.** TypeScript’s tsconfig paths, Python’s relative imports, Go’s module paths.
1. **Tree-sitter Go bindings:** Use `github.com/smacker/go-tree-sitter` with language packages.

**Performance Targets:**

- Index 5000 files: < 5 seconds
- BFS depth-3 on 50K-node graph: < 30ms
- Full edge rebuild: < 3 seconds for 50K symbols
- Memory: < 600MB for 50K-symbol graph

### API (gRPC)

```protobuf
service CKGService {
    rpc SearchSymbols(SearchRequest) returns (SymbolList);
    rpc GetNeighbors(NeighborRequest) returns (NodeList);
    rpc GetDistance(DistanceRequest) returns (DistanceResult);
    rpc ComputeICR(ComputeICRRequest) returns (IsolatedChangeRegion);
    rpc SelectTests(SelectTestsRequest) returns (TestPlan);
    rpc ReindexFiles(ReindexRequest) returns (ReindexResult);
}
```

### ICR Computation Algorithm

1. Extract keywords from intent (domain, capability, title words)
1. Search graph for matching symbols (gctx ranker)
1. If targets found: BFS outward (depth 2, edges: calls/extends/implements/uses-type)
1. If no targets (new capability): identify insertion directory from domain mapping
1. Classify nodes: EXCLUSIVE = targets + same-file siblings that share state; SHARED_READ = impact radius nodes not in exclusive set; BOUNDARY = nodes at max BFS depth with inbound edges from outside
1. Compute confidence (penalties for high fan-out, deep inheritance, shared modules)
1. Map exclusive files to Redis lock keys

### Test Selection Algorithm

1. Identify changed symbols (from ChangeSet diff)
1. Find direct tests (1 hop via ‘tests’ edge, inbound)
1. Find transitive dependents (3 hops via calls/uses-type, inbound)
1. Find tests for transitive dependents
1. Add new tests from ChangeSet
1. Apply policy requirements (mandatory tests for certain scopes)
1. Compute confidence (penalize untested changes, low-confidence edges)

-----

## Isolation Engine Specification

### Redis Key Design

```
Lock keys:
  lock:icr:{sha256-of-exclusive-files-sorted}
  Value: { intent_id, acquired_at, pod_id, ttl }
  Operations: SET key value NX EX {ttl_seconds}

Queue:
  queue:pending (sorted set, score = priority × timestamp)
  queue:executing (set of intent IDs)

Heartbeats:
  heartbeat:{pod_id}
  Value: { intent_id, last_seen }
  TTL: 60 seconds (auto-expires = dead agent)

Graph cache:
  graph:edges (hash: edge_id → serialized edge)
  graph:symbols:{file_path} (hash: symbol_id → serialized symbol)
  graph:version (string: source repo HEAD SHA)
```

### Lock Algorithm

```
ACQUIRE(intent_id, icr):
  lock_keys = icr.lockKeys
  for key in lock_keys:
    result = REDIS SET key {intent_id, now(), pod_id} NX EX 1800
    if result == nil:
      // Lock held by another intent
      ROLLBACK(acquired_keys)
      return { granted: false, held_by: GET(key).intent_id, queue_position: ENQUEUE(intent_id) }
  return { granted: true }

RELEASE(intent_id, icr):
  for key in icr.lockKeys:
    if GET(key).intent_id == intent_id:
      DEL(key)
  DEQUEUE_NEXT() // notify next queued intent

HEARTBEAT(pod_id, intent_id):
  SET heartbeat:{pod_id} {intent_id, now()} EX 60

DEAD_AGENT_CHECK (runs every 30s):
  for each lock in SCAN("lock:icr:*"):
    pod_id = lock.value.pod_id
    if NOT EXISTS heartbeat:{pod_id}:
      RELEASE(lock.intent_id, lock.icr)
      REQUEUE(lock.intent_id) // retry with new agent
```

-----

## Conflict Detection Specification

### Layer 1: Structural (No LLM)

Check new ICR against all active ICRs:

- `FILE_OVERLAP` (HIGH): intersection of exclusive files
- `SYMBOL_OVERLAP` (HIGH): intersection of exclusive symbol nodes
- `READ_WRITE` (MEDIUM): new.exclusive ∩ active.sharedRead OR active.exclusive ∩ new.sharedRead

### Layer 2: Contract (No LLM)

Using CKG graph analysis:

- `API_CONFLICT` (HIGH): both modify symbols serving same API route paths
- `TYPE_CONFLICT` (MEDIUM): both modify same shared type definitions

### Layer 3: Semantic (LLM — Claude Opus 4.6)

Only invoked when: same domain AND shared boundary nodes AND no structural/contract conflict found.

```
SYSTEM PROMPT:
You are analyzing two software intents for potential behavioral interference.
Both intents will execute concurrently on the same codebase.
They do NOT modify the same files, but they might still conflict semantically.

Determine if executing both simultaneously could produce an inconsistent system.
Consider: (1) Does one create something the other assumes doesn't exist?
(2) Do both modify behavior that shares a system invariant?
(3) Does one depend on state the other changes?

Output JSON:
{
  "conflict_detected": boolean,
  "confidence": float (0.0–1.0),
  "explanation": string (1–2 sentences),
  "resolution": "serialize" | "merge" | "partition" | "safe_parallel"
}

Be conservative: false positives > false negatives.
```

-----

## Granularity Scoring Specification

```
GS = Specificity × (1 − Decomposability)
Threshold: GS >= 0.7

Decomposability Score (DS) — lower is better:
DS = scope×0.30 + domain×0.25 + behavior×0.25 + deps×0.20
  scope    = min(1.0, estimatedFiles / 50)
  domain   = min(1.0, (domainCount − 1) / 4)
  behavior = min(1.0, (distinctBehaviors − 1) / 7)
  deps     = min(1.0, externalDeps / 5)

Specificity Score (SS) — higher is better:
SS = average of 6 binary/continuous factors:
  - Has acceptance criteria (0 or 1)
  - Testable criteria ratio (0.0–1.0)
  - Has constraints/boundaries (0 or 1)
  - Has domain+capability defined (0.5 or 1.0)
  - Low ambiguity score (1.0 − ambiguousTerms×0.25, min 0)
  - Has error cases specified (0.3 or 1.0)
```

-----

## Certification Pipeline Specification

**Stage 1 — Static** (runs in certification pod, no deployment):

- 1a. Build verification: compile/transpile, type check, dependency resolution
- 1b. Unit test execution: all tests in ChangeSet + CKG-selected affected tests
- 1c. Contract tests: API schema validation, event schema validation
- 1d. SAST: Semgrep rules scan on changed files
- 1e. Semantic: invariant preservation, backward compatibility

**Stage 2 — Integration** (ephemeral K8s namespace):

- 2a. Provision: deploy changed service + dependencies (containers) + DB (migrations + seed)
- 2b. Integration tests: exercise real HTTP/DB/event paths
- 2c. DAST: OWASP ZAP scan against running service
- 2d. Teardown: delete namespace

**Stage 3 — Staging** (persistent environment, conditional):

- 3a. Deploy to staging
- 3b. E2E tests derived from intent acceptance criteria
- 3c. Performance tests (if latency-sensitive scope)

Certificate produced on all-pass. Committed to intent-store repo.

-----

## Agent Execution Specification

### Kubernetes CRD

```yaml
apiVersion: platform.intent/v1
kind: AgentExecution
metadata:
  name: exec-{intent-id}
spec:
  intent:
    id: "INT-2026-XXXX"
    title: "..."
    description: "..."
    acceptance_criteria: [...]
    constraints: [...]
  icr:
    exclusive_paths: ["path/to/dir/**"]
    readable_paths: ["other/dir/**"]
  agent:
    model: "claude-opus-4-6"
    runtime: "claude-code"
  resources:
    cpu: "4"
    memory: "16Gi"
    ephemeral_storage: "50Gi"
    llm_budget_dollars: "15.00"
    time_limit_minutes: 30
  source:
    repo: "git@github.com:org/repo.git"
    ref: "main"
    sha: "abc123"
  output:
    type: "changeset"
```

### Pod Template

```yaml
containers:
  - name: agent
    image: platform/agent-runtime:latest
    securityContext:
      readOnlyRootFilesystem: false  # needs to write within ICR
      runAsNonRoot: true
    volumeMounts:
      - name: workspace
        mountPath: /workspace
    env:
      - name: INTENT_SPEC
        valueFrom: configMapKeyRef
      - name: ICR_BOUNDARIES
        valueFrom: configMapKeyRef
      - name: LLM_API_KEY
        valueFrom: secretKeyRef
  - name: heartbeat-sidecar
    image: platform/heartbeat:latest
    # Reports to isolation engine every 30s
  - name: cost-tracker
    image: platform/cost-tracker:latest
    # Monitors LLM API usage, kills pod on budget exhaustion
```

-----

## Human Interface: Dashboard + Review UX

### Design Principles

- **No IDE required.** All human interaction happens via CLI or web dashboard — humans don’t touch code.
- **Flow visualization is the primary view.** Humans think in terms of intent progress, not files or branches.
- **Review is inline.** Reviewers see diffs in the dashboard, not in a separate tool (no GitHub/GitLab).
- **Notifications drive action.** Humans are notified when their input is needed (review, approval, failure).

### Intent Dashboard Views

1. **FLOW VIEW** (default) — real-time pipeline visualization
1. **QUEUE VIEW** — what’s waiting, what’s executing, what’s blocked
1. **REVIEW VIEW** — code review interface (for mandatory/optional review)
1. **INTENT DETAIL VIEW** — full lifecycle history (state transitions, agent logs, certification results, deployment progress, audit trail)
1. **GRAPH VIEW** — CKG visualization (ICR boundaries, conflict zones, test coverage paths)
1. **METRICS VIEW** — platform health (intents/hour throughput, mean time from signed→deployed, certification pass rate, agent cost P50/P95, queue depth over time)

### Dashboard Technology

- **Backend:** Go (same monorepo, `cmd/dashboard/main.go`)
- **Frontend:** Static SPA (React or htmx — team preference)
- **Data source:** Reads from intent-store git repo + Redis state + gRPC calls to orchestrator
- **Real-time:** WebSocket connection for live state updates
- **Auth:** OIDC/SSO (same as platform auth)
- **No write path through dashboard except:** review decisions (which commit to intent-store via git)

### CLI Review Commands

```
intent review list                                    # show pending reviews assigned to you
intent review show INT-2026-049                       # display diff + intent spec in terminal
intent review approve INT-2026-049                    # approve (GPG-signed)
intent review reject INT-2026-049 --reason "..."
intent review request-changes INT-2026-049 --comments "..."
```

-----

## Orchestrator Workflow

### Single Intent Flow (pseudocode)

```
onIntentSigned(intentId):
  1. Read intent from git store
  2. Verify granularity (GS >= 0.7)
  3. Compute ICR via CKG
  4. Check conflicts against active locks (3-layer detection)
     - If conflict + critical priority + victim is low → preempt
     - If conflict + cannot preempt → enqueue, return
  5. Acquire lock (Redis SET NX EX)
     - If not granted → enqueue, return
  6. Dispatch agent pod (update state → "executing")
  7. On agent completion → review gate (state → "review_pending")
     - Evaluate ReviewPolicy for intent's domain/priority
     - If mode=none → skip to step 8
     - If mode=optional → notify reviewers, start timeout (auto-advance)
     - If mode=mandatory → block until min_reviewers approve
     - On reject → re-execute agent with feedback OR fail intent (fail fast)
  8. Certify (state → "certifying")
     - Priority affects stages: critical skips integration env, parallelizes
     - If cert fails + retries < 3 → retry agent with failure context
     - If cert fails + retries exhausted → fail intent (+ fail group if grouped)
  9. If part of intent group → hold ChangeSet, check if all group intents certified
  10. Admit: rebase onto HEAD + commit with trailers
  11. Release lock, trigger CKG re-index
  12. Deploy: canary → progressive (priority affects gate times)
```

### Intent Group Flow

```
onAllGroupIntentsCertified(groupId):
  1. Retrieve all held ChangeSets
  2. Order by dependency (if sequential) or any order (if parallel)
  3. Atomic batch admission (rebase + commit all sequentially)
  4. Release all locks
  5. Re-index all changed files
  6. Deploy as single atomic unit
  - If batch admission fails (rebase conflict) → fail entire group
```

### Priority Queue (Redis Sorted Set)

```
Score = priority_weight × 1_000_000_000 + unix_timestamp_ms
Weights: critical=1, high=2, normal=3, low=4
Lower score = dequeued first (highest priority + earliest arrival)

Preemption (critical → low only):
  1. Force-release victim's lock
  2. Terminate victim's agent pod
  3. Reset victim state to "signed", re-enqueue
  4. Log preemption event in audit trail
```

-----

## Implementation Phases

### Phase 1: CKG Service (Week 1-3)

Build the Code Knowledge Graph as a shared service. Source: Extract and adapt from gctx (InfiniContext) codebase patterns.

```
cmd/
  ckg-service/
    main.go          ← gRPC service entrypoint

internal/
  ckg/
    graph/
      graph.go       ← Adjacency list, edge builder, BFS, distance
      graph_test.go
    parser/
      typescript.go  ← Tree-sitter TS/JS parser
      python.go
      golang.go
      java.go
      parser.go      ← Common parser interface
    ranker/
      ranker.go      ← Multi-signal scoring for intent-target mapping
    icr/
      icr.go         ← ICR computation algorithm
    testselect/
      selector.go    ← Graph-based test selection
    service/
      server.go      ← gRPC server setup + handlers
      gitwatcher.go  ← Watch source repo for changes, trigger re-index
      rediscache.go  ← Graph serialization to/from Redis

proto/
  ckg.proto          ← Service definition

deployments/
  docker/Dockerfile.ckg
  k8s/ckg-deployment.yaml
  k8s/ckg-service.yaml
```

**Success criteria:**

- Index a real 5000-file TypeScript repo in < 10 seconds
- BFS returns correct neighbors (validate against manual analysis)
- ICR computation produces reasonable isolation boundaries
- Test selection achieves > 90% recall against full test suite

### Phase 2: Intent Registry (Week 1-3, parallel with Phase 1)

Build the intent lifecycle system backed by git.

```
cmd/
  orchestrator/
    main.go          ← Intent registry + orchestrator entrypoint
  intent-cli/
    main.go          ← CLI entrypoint

internal/
  intent/
    schema/
      schema.go      ← Intent YAML schema + validation
      schema_test.go
    statemachine/
      statemachine.go ← Valid state transitions + enforcement
    gitstore/
      store.go       ← Read/write intents as YAML files in git repo
    approval/
      approval.go    ← Multi-party approval workflow (GPG-signed)
    granularity/
      scoring.go     ← GS scoring + decomposition suggestions
    webhook/
      listener.go    ← Git webhook listener (detect changes)
    api/
      handler.go     ← REST API (CRUD + state transitions)
    group/
      group.go       ← Intent group creation and lifecycle
    priority/
      queue.go       ← Priority level management

cli/
  commands/
    create.go        ← Create new intent (interactive or from file)
    propose.go       ← Move to proposed state
    approve.go       ← Add approval (GPG-signed)
    sign.go          ← Sign intent (trigger execution)
    status.go        ← Check intent status
    list.go          ← List intents by state
    decompose.go     ← Suggest decomposition for large intents
    groupcreate.go   ← Create intent group from multiple intents
    groupstatus.go   ← Check group execution status
    priority.go      ← Set/change intent priority level
```

**Success criteria:**

- Create intent via CLI, see YAML file committed to git
- State machine prevents invalid transitions
- Approvals require GPG signature
- Granularity check blocks low-scoring intents
- Git log shows full audit trail of all operations
- Intent groups can be created linking multiple intents
- Priority levels affect queue ordering

### Phase 3: Isolation Engine (Week 3-5)

Build distributed locking for ICR isolation.

```
cmd/
  isolation-engine/
    main.go          ← Isolation engine entrypoint

internal/
  isolation/
    lockmanager/
      manager.go     ← Redis SET NX EX based locking
      manager_test.go ← Concurrency stress tests (100+ concurrent locks)
    queue/
      priority.go    ← Priority queue (Redis sorted sets)
    heartbeat/
      monitor.go     ← Monitor agent liveness
    deadagent/
      recovery.go    ← Detect and recover from agent death
      recovery_test.go ← Lock recovery after agent death
    preemption/
      preempt.go     ← Critical priority preemption logic
    api/
      server.go      ← gRPC API (acquire, release, status)
    tests/
      deadlock_test.go ← Deadlock detection integration tests
```

**Success criteria:**

- 100 concurrent lock requests handled atomically
- Dead agent locks auto-release within 90 seconds
- Queue respects priority ordering
- No deadlocks under any scenario
- Critical preemption correctly force-releases low-priority locks
- Preempted intents are cleanly re-queued and re-executed

### Phase 4: Conflict Detection (Week 5-8)

Build 3-layer conflict detection.

```
cmd/
  conflict-detector/
    main.go          ← Conflict detector entrypoint

internal/
  conflict/
    structural/
      detector.go    ← Set intersection on ICRs
    contract/
      analyzer.go    ← API path + type analysis
    semantic/
      llm.go         ← LLM-powered behavioral analysis
      confidence.go  ← Score LLM results
    resolution/
      resolver.go    ← Recommend serialize/merge/partition
    api/
      server.go      ← gRPC API
    prompts/
      semantic-conflict.md ← LLM prompt template
    testdata/
      known-conflicts.yaml ← Library of known conflict patterns
      known-safe.yaml      ← Known non-conflicting pairs
```

**Success criteria:**

- 100% detection rate for structural conflicts
- 90%+ detection rate for contract conflicts
- < 20% false positive rate for semantic conflicts
- Resolution suggestions are actionable

### Phase 5: Agent Execution Platform (Week 3-5, parallel with Phase 3)

Build K8s operator for agent pod lifecycle.

```
cmd/
  agent-operator/
    main.go          ← Operator entrypoint (kubebuilder scaffolded)

internal/
  operator/
    api/v1/
      agentexecution_types.go ← CRD schema
    controllers/
      agentexecution_controller.go ← Reconciliation loop
    internal/
      pod/builder.go    ← Pod template generation
      icr/enforcer.go   ← Filesystem isolation (seccomp + read-only mounts)
      budget/tracker.go ← Cost monitoring
      heartbeat/reporter.go

agentruntime/
  entrypoint.go      ← Agent execution wrapper
  icrguard.go        ← Prevent writes outside ICR paths
  changeset.go       ← Collect and format output (git diff)
  costmonitor.go     ← Track LLM token usage
```

**Success criteria:**

- Pod starts within 30 seconds of dispatch
- Agent cannot write files outside ICR paths
- Pod terminates on budget or time exhaustion
- ChangeSet correctly extracted from agent output

### Phase 6: Certification Engine (Week 5-8, parallel with Phase 4)

Build multi-stage certification pipeline.

```
cmd/
  certification/
    main.go          ← Certification engine entrypoint

internal/
  certification/
    orchestrator.go  ← Stage sequencing
    stages/
      build.go       ← Compile/transpile verification
      unittest.go    ← Run unit tests
      integration.go ← Provision ephemeral env + run integration tests
      security.go    ← Semgrep SAST + OWASP ZAP DAST
      semantic.go    ← Contract + invariant checks
    environment/
      provisioner.go ← Create/destroy ephemeral K8s namespaces
      templates/     ← Helm templates for test environments
    certificate/
      generator.go   ← Generate + sign certification record
    reporter/
      reporter.go    ← Update intent-store with results
```

**Success criteria:**

- Full certification (build + unit + integration) completes in < 5 minutes
- Ephemeral environment provisions in < 2 minutes
- Certificate correctly signed and committed to intent-store
- Failed certification triggers agent retry

### Phase 7: Admission + Delivery (Week 8-10)

Build admission control and progressive deployment.

```
cmd/
  admission/
    main.go          ← Admission controller entrypoint
  deployment/
    main.go          ← Deployment engine entrypoint

internal/
  admission/
    rebase/
      rebase.go      ← Rebase ChangeSet onto latest main (go-git)
    merge/
      merge.go       ← Commit with trailers + GPG sign
    batch/
      batchmerge.go  ← Atomic batch merge for intent groups
    hold/
      hold.go        ← Hold ChangeSets pending group completion
    postmerge/
      hooks.go       ← Trigger CKG re-index + intent state update

  deployment/
    strategy/
      canary.go      ← Canary deployment (1% → 10% → 50% → 100%)
      rollback.go    ← Automatic rollback on metric violation
    metrics/
      validator.go   ← Map intent criteria to runtime metrics
    controller/
      controller.go  ← Deployment lifecycle management
```

**Success criteria:**

- Rebase + merge is atomic (no partial state on failure)
- Canary deployment progresses through stages on metric validation
- Automatic rollback within 60 seconds of metric violation
- Intent state updated through deployed → realized

### Phase 8: Integration Testing (Week 10-12)

Wire everything together. Run full flows. Fix bugs.

```
tests/
  e2e/
    fullflow_test.go             ← Intent creation → deployment (happy path)
    conflict_handling_test.go    ← Two conflicting intents
    agent_failure_test.go        ← Agent dies mid-execution
    certification_failure_test.go ← Cert fails, retry succeeds
    concurrent_test.go           ← 10 intents executing simultaneously
    intent_group_test.go         ← Atomic group: all certified → batch merge → atomic deploy
    intent_group_failure_test.go ← One sub-intent fails → entire group fails
  deploy/
    priority_queue_test.go       ← Critical skips queue, preempts low-priority
    priority_hotfix_test.go      ← Critical intent: fast-path cert + accelerated deploy
    review_mandatory_test.go     ← Mandatory review blocks until approved
    review_reject_test.go        ← Reviewer rejects → agent re-executes with feedback
    review_timeout_test.go       ← Optional review auto-advances after timeout
  load/
    throughput_test.go           ← Measure max intents/hour
  chaos/
    redis_failure_test.go        ← Redis crashes during execution
    git_failure_test.go          ← Git hosting unavailable
```

### Phase 9: Hardening (Week 12-14)

Security, operational readiness, documentation.

```
ops/
  security/
    mtls-config.yaml
    rbac-policies.yaml
    network-policies.yaml
  monitoring/
    grafana-dashboards/
    prometheus-alerts.yaml
  runbooks/
    incident-response.md
    disaster-recovery.md
    capacity-planning.md
docs/
  user-guide/
    authoring-intents.md
    approving-intents.md
    monitoring-execution.md
  operator-guide/
    deployment.md
    troubleshooting.md
    scaling.md
```

-----

## Key Go Dependencies

|Package                                 |Purpose                                                      |
|----------------------------------------|-------------------------------------------------------------|
|`github.com/smacker/go-tree-sitter`     |Tree-sitter bindings (+ /typescript, /python, /golang, /java)|
|`github.com/go-git/go-git/v5`           |Git operations                                               |
|`github.com/redis/go-redis/v9`          |Redis client                                                 |
|`google.golang.org/grpc`                |gRPC                                                         |
|`google.golang.org/protobuf`            |Protobuf                                                     |
|`sigs.k8s.io/controller-runtime`        |K8s operator framework                                       |
|`k8s.io/client-go`                      |K8s API client                                               |
|`github.com/go-chi/chi/v5`              |HTTP router (REST API)                                       |
|`gopkg.in/yaml.v3`                      |YAML parsing                                                 |
|`github.com/stretchr/testify`           |Testing assertions                                           |
|`github.com/anthropics/anthropic-sdk-go`|Claude API                                                   |

-----

## Project Layout

Go monorepo: `cmd/` (9 binaries: ckg-service, orchestrator, isolation-engine, conflict-detector, certification-engine, admission-controller, deployment-engine, dashboard, intent-cli), `internal/` (domain packages per phase), `pkg/` (shared utilities: git, redis, grpc, llm, models), `proto/` (service definitions), `tests/` (e2e, load, chaos), `deployments/` (docker, k8s, helm).

-----

## Constraints & Rules

1. **No external databases.** Git + Redis only.
1. **No message brokers.** Git webhooks + gRPC + callbacks.
1. **No branches in source repo.** Linear main only. Tags for release marking.
1. **Human review is optional, machine certification is mandatory.** Teams configure review policy per domain/priority. Review is shifted left (before certification) for fast failure. Certification always runs regardless of review outcome.
1. **Tests are mandatory.** A ChangeSet without tests always fails certification.
1. **Intents must score GS >= 0.7.** No exceptions without explicit human override.
1. **Agents run in pods, never on laptops.** Deterministic, auditable, isolated.
1. **Git is source of truth.** Redis is ephemeral cache. On conflict, git wins.
1. **Every commit has trailers.** Intent-ID, Certificate, Agent — always.
1. **Progressive deployment.** Never deploy directly to 100%. Always canary first.
1. **Every commit must be independently deployable.** No commit leaves main in a broken state. (Exception: intent groups with `atomic` merge strategy may have intermediate commits that require the full group.)
1. **Intent groups limited to 10 sub-intents.** Larger groups indicate insufficient decomposition.
1. **Critical priority max 3 per 24 hours.** Prevents abuse of fast-path. Requires `security_champion` approval.
1. **Critical intents still run SAST + unit tests.** Security scanning is never skipped regardless of priority.
1. **Preemption only critical→low.** Critical cannot preempt normal or high to prevent cascading disruption.
1. **Feature flags for incremental large changes.** Large features ship behind flags, enabled via separate intent when complete.

-----

## Begin Implementation

Start with Phase 1 (CKG Service) and Phase 2 (Intent Registry) in parallel. These have no dependencies on each other and are the foundation for everything else.

For each component:

1. Initialize Go module structure under `cmd/` and `internal/`
1. Implement core logic with comprehensive unit tests (`go test`)
1. Add integration tests
1. Containerize with multi-stage Dockerfile
1. Add K8s manifests
1. Document gRPC/REST API

Proceed phase by phase. After each phase, validate against the success criteria before moving to the next.