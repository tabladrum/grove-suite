# Relay — Implementation Plan

**Project:** Relay (formerly: Next-gen CICD)  
**CLI:** `relay`  
**Role:** Intent-driven autonomous software delivery platform  
**Language:** Go 1.22+ (entire platform)  
**Depends on:** Grove (code knowledge graph — replaces internal CKG)  
**Infrastructure:** Kubernetes, Redis (transient only), Git (3 repos), OCI registry  
**Status:** Pre-build — architecture validated, implementation not started  
**Last Updated:** May 26, 2026

---

## Overview

Relay replaces traditional CI/CD (Jenkins, GitHub Actions, PRs, branch workflows) with an intent-first, agent-native delivery platform. Humans write **intents** — structured declarations of what should change. Relay turns those intents into certified, deployed code changes through autonomous agent execution, with a configurable human review gate and full audit trail.

**Relationship to Next-gen CICD:** Relay is the Next-gen CICD platform rebuilt with Grove as its code intelligence foundation. The internal CKG (Code Knowledge Graph) service previously duplicated gctx's graph infrastructure. In Relay, the CKG service is replaced by a Grove client — all graph queries, ICR computation, test selection, and conflict detection are delegated to a running Grove instance.

### What Relay owns (not Grove)

| Component               | Description                                                           |
|-------------------------|-----------------------------------------------------------------------|
| Orchestrator            | Watches intent-store, drives intent lifecycle state machine           |
| Intent System           | YAML schema, git-based store, lifecycle, granularity enforcement      |
| Isolation Engine        | Redis locks, priority queue, heartbeats, dead-agent recovery          |
| Conflict Detector       | 3-layer conflict detection (structural + contract + semantic/LLM)     |
| Agent Execution Platform| K8s operator, ephemeral pods, ChangeSet output                        |
| Review Gate             | Configurable human review (none/optional/mandatory)                   |
| Certification Engine    | Build, test, security, semantic validation pipeline                   |
| Admission Controller    | Rebase + linear merge to source repo main                             |
| Deployment Engine       | Canary → progressive → full with intent-metric validation             |
| Dashboard               | Web UI for intents, execution, deployment status                      |
| Intent CLI              | `relay` CLI for authoring, signing, monitoring intents                |

---

## Repository Layout

```
relay/
├── cmd/
│   ├── relay/
│   │   └── main.go                   # CLI entry point
│   ├── orchestrator/
│   │   └── main.go                   # Orchestrator service
│   ├── ckg-service/
│   │   └── main.go                   # CKG service (Grove proxy + ICR logic)
│   ├── isolation-engine/
│   │   └── main.go                   # Isolation engine service
│   ├── conflict-detector/
│   │   └── main.go                   # Conflict detector service
│   ├── certification-engine/
│   │   └── main.go                   # Certification engine
│   ├── admission-controller/
│   │   └── main.go                   # Admission controller
│   ├── deployment-engine/
│   │   └── main.go                   # Deployment engine
│   └── dashboard/
│       └── main.go                   # Dashboard server
├── internal/
│   ├── intent/
│   │   ├── schema.go                 # Intent, IntentGroup YAML schema + validation
│   │   ├── store.go                  # Git-based intent store (read/write YAML)
│   │   ├── lifecycle.go              # State machine: draft→proposed→signed→...
│   │   └── granularity.go            # GS = Specificity × (1 − Decomposability)
│   ├── grove/
│   │   ├── client.go                 # Grove HTTP/gRPC client
│   │   └── types.go                  # Grove data type mirrors
│   ├── ckg/
│   │   ├── service.go                # CKG service: delegates to Grove + owns ICR logic
│   │   ├── icr.go                    # ICR computation (uses Grove's grove_icr)
│   │   ├── tests.go                  # Test selection (uses Grove's grove_tests)
│   │   └── conflict.go               # Conflict detection (structural + contract layers)
│   ├── isolation/
│   │   ├── engine.go                 # Redis lock management
│   │   ├── queue.go                  # Priority queue (sorted sets)
│   │   └── heartbeat.go              # Agent heartbeat + dead-agent recovery
│   ├── conflict/
│   │   ├── structural.go             # Layer 1: set-based file/symbol overlap
│   │   ├── contract.go               # Layer 2: API path, type, event schema conflicts
│   │   └── semantic.go               # Layer 3: LLM-powered behavioral interference
│   ├── execution/
│   │   ├── operator.go               # K8s operator: AgentExecution CRD controller
│   │   ├── pod.go                    # Pod spec generation
│   │   └── changeset.go              # ChangeSet validation
│   ├── review/
│   │   ├── gate.go                   # Review gate logic (none/optional/mandatory)
│   │   └── policy.go                 # ReviewPolicy from platform-config repo
│   ├── certification/
│   │   ├── pipeline.go               # 3-stage certification pipeline
│   │   ├── static.go                 # Stage 1: build, unit tests, SAST, semantic
│   │   ├── integration.go            # Stage 2: ephemeral K8s env, integration tests, DAST
│   │   ├── staging.go                # Stage 3: staging E2E + performance
│   │   └── certificate.go            # Certificate generation + storage
│   ├── admission/
│   │   ├── controller.go             # Rebase + linear merge to main
│   │   └── rebase.go                 # Git rebase algorithm
│   ├── deployment/
│   │   ├── engine.go                 # Canary orchestration
│   │   ├── canary.go                 # Progressive traffic shifting
│   │   └── validation.go             # Intent metric validation
│   ├── dashboard/
│   │   ├── server.go                 # HTTP server for SPA
│   │   └── api.go                    # Dashboard REST API
│   └── config/
│       └── config.go                 # Platform config (from platform-config repo)
├── manifests/
│   ├── crds/
│   │   └── agentexecution.yaml       # AgentExecution CRD
│   ├── rbac/
│   └── deployments/
├── proto/
│   └── relay.proto                   # Internal gRPC between Relay services
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## Data Models

### Intent Schema (`internal/intent/schema.go`)

```go
type Intent struct {
    APIVersion string       `yaml:"apiVersion"`  // "intent/v1"
    Kind       string       `yaml:"kind"`        // "Intent"
    Metadata   IntentMeta   `yaml:"metadata"`
    Spec       IntentSpec   `yaml:"spec"`
    Group      string       `yaml:"group,omitempty"`
    Priority   IntentPriority `yaml:"priority"`
    Status     IntentStatus `yaml:"status"`
    Approvals  []Approval   `yaml:"approvals"`
    Execution  ExecutionInfo `yaml:"execution"`
    Review     ReviewInfo   `yaml:"review"`
    Certification CertInfo  `yaml:"certification"`
    History    []HistoryEntry `yaml:"history"`
}

type IntentMeta struct {
    ID        string `yaml:"id"`       // "INT-{year}-{sequence}"
    Parent    string `yaml:"parent,omitempty"`
    Created   string `yaml:"created"`  // ISO8601
    CreatedBy string `yaml:"created_by"`
}

type IntentSpec struct {
    Title              string   `yaml:"title"`
    Description        string   `yaml:"description"`
    Domain             string   `yaml:"domain"`
    Capability         string   `yaml:"capability"`
    AcceptanceCriteria []string `yaml:"acceptance_criteria"`
    Constraints        []string `yaml:"constraints"`
    EstimatedScope     struct {
        Files   int      `yaml:"files"`
        Domains []string `yaml:"domains"`
    } `yaml:"estimated_scope"`
}

type IntentState string
const (
    StateDraft         IntentState = "draft"
    StateProposed      IntentState = "proposed"
    StateSigned        IntentState = "signed"
    StateExecuting     IntentState = "executing"
    StateReviewPending IntentState = "review_pending"
    StateCertifying    IntentState = "certifying"
    StateCertified     IntentState = "certified"
    StateMerged        IntentState = "merged"
    StateDeployed      IntentState = "deployed"
    StateRealized      IntentState = "realized"
    StateFailed        IntentState = "failed"
)

type PriorityLevel string
const (
    PriorityCritical PriorityLevel = "critical"
    PriorityHigh     PriorityLevel = "high"
    PriorityNormal   PriorityLevel = "normal"
    PriorityLow      PriorityLevel = "low"
)
```

### IsolatedChangeRegion (from Grove)

```go
// Relay uses Grove's IsolatedChangeRegion directly
// Grove computes it; Relay stores it in intent and uses lock keys for Redis
type IsolatedChangeRegion struct {
    IntentID       string   `json:"intentId"`
    Exclusive      []string `json:"exclusive"`       // symbol node IDs agent will modify
    SharedRead     []string `json:"sharedRead"`      // symbol node IDs agent may read
    Boundary       []string `json:"boundary"`        // edge nodes
    ExclusiveFiles []string `json:"exclusiveFiles"`  // human-readable
    ReadableFiles  []string `json:"readableFiles"`
    Confidence     float64  `json:"confidence"`
    LockKeys       []string `json:"lockKeys"`        // Redis SET NX EX keys
}
```

### ChangeSet

```go
type ChangeSet struct {
    IntentID      string   `json:"intentId"`
    Diff          string   `json:"diff"`         // unified diff (git format)
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
        ExecutionTime int64   `json:"executionTime"` // ms
        Cost          float64 `json:"cost"`
        TokensInput   int     `json:"tokensInput"`
        TokensOutput  int     `json:"tokensOutput"`
    } `json:"metadata"`
}
```

---

## Phase 1 — Grove Integration (CKG Service)

**Goal:** The CKG service is a thin proxy to Grove, plus the Relay-specific ICR and test selection logic that calls Grove's APIs.

### 1.1 Grove Client (`internal/grove/client.go`)

```go
type Client struct {
    baseURL    string           // GROVE_URL env var
    httpClient *http.Client
    grpcConn   *grpc.ClientConn
}

// Methods Relay calls on Grove
func (c *Client) Index(dir string) error
func (c *Client) ComputeICR(intentID, intent string) (*IsolatedChangeRegion, error)
func (c *Client) DetectConflicts(a, b *IsolatedChangeRegion) (*grove.ConflictReport, error)
func (c *Client) SelectTests(changedFiles []string) ([]string, error)
func (c *Client) QueryByIntent(intent string) ([]grove.SymbolRecord, error)
func (c *Client) GetImpact(file string, line int) ([]grove.ImpactNode, error)
func (c *Client) GetStatus() (*grove.StatusResult, error)
```

### 1.2 CKG Service (`internal/ckg/service.go`)

The CKG service is the interface Relay's other components call. It wraps Grove and adds Relay-specific business logic:

```go
type CKGService struct {
    grove  *grove.Client
    redis  *redis.Client   // for graph cache metadata
}

func (s *CKGService) ComputeICRForIntent(intent *Intent) (*IsolatedChangeRegion, error)
func (s *CKGService) SelectTestsForChangeSet(cs *ChangeSet) (*TestPlan, error)
func (s *CKGService) CheckConflicts(newICR *IsolatedChangeRegion, activeICRs []*IsolatedChangeRegion) ([]*ConflictReport, error)
func (s *CKGService) ScoreGranularity(intent *Intent) (float64, error)
```

**ICR Computation flow:**
1. Call `grove.ComputeICR(intent.ID, intent.Spec.Title + " " + intent.Spec.Description)`
2. Grove returns ICR with symbol IDs, file paths, confidence, lock keys
3. CKG service stores ICR in Redis (`icr:{intentID}`) for use by isolation engine
4. Returns ICR to orchestrator

**Test Selection flow:**
1. From ChangeSet `ModifiedFiles`, call `grove.SelectTests(modifiedFiles)`
2. Grove returns: direct test files (inbound `tests` edges), transitive dependent tests
3. Merge with ChangeSet's own declared tests (`cs.Tests.Unit`, etc.)
4. Deduplicate; apply policy requirements (e.g., auth domain requires security tests)

---

## Phase 2 — Intent System

**Goal:** Define the intent schema, implement the git-based store, and enforce granularity.

### 2.1 Intent Store (`internal/intent/store.go`)

```go
type IntentStore struct {
    repoPath string  // path to intent-store git repo clone
    git      GitOps  // git operations wrapper
}

// CRUD
func (s *IntentStore) Create(intent *Intent) error       // writes YAML, git commit
func (s *IntentStore) Update(intent *Intent) error       // overwrites YAML, git commit
func (s *IntentStore) Get(intentID string) (*Intent, error)
func (s *IntentStore) List(state IntentState) ([]*Intent, error)
func (s *IntentStore) Transition(intentID string, newState IntentState, metadata map[string]string) error
```

**Storage layout in intent-store repo:**
```
active/
  INT-2026-001.yaml
  INT-2026-002.yaml
completed/
  INT-2025-099.yaml
failed/
  INT-2026-003.yaml
groups/
  IG-2026-001.yaml
```

Every state change = one git commit with message: `intent: INT-2026-001 → state:executing`

### 2.2 Granularity Enforcement (`internal/intent/granularity.go`)

**Formula:**
```
GS = Specificity × (1 − Decomposability)
Threshold: GS >= 0.7

Decomposability Score (DS):
DS = scope×0.30 + domain×0.25 + behavior×0.25 + deps×0.20
  scope    = min(1.0, estimatedFiles / 50)
  domain   = min(1.0, (domainCount − 1) / 4)
  behavior = min(1.0, (distinctBehaviors − 1) / 7)
  deps     = min(1.0, externalDeps / 5)

Specificity Score (SS):
SS = average(
  hasAcceptanceCriteria         → 0.0 or 1.0
  testableCriteriaRatio         → 0.0–1.0
  hasConstraints                → 0.0 or 1.0
  hasDomainAndCapability        → 0.5 or 1.0
  lowAmbiguity                  → 1.0 − (ambiguousTermCount × 0.25), min 0
  hasErrorCases                 → 0.3 or 1.0
)
```

If `GS < 0.7`: block the intent from being signed. Use the LLM to generate decomposition suggestions:
```
Prompt: "This intent scored GS=0.52 due to high decomposability (cross-domain, estimated 30 files). 
Suggest 3–5 sub-intents that each score >= 0.7.
Intent: {intent.Spec.Title}: {intent.Spec.Description}"
```

### 2.3 Intent State Machine (`internal/intent/lifecycle.go`)

```
draft
  └→ proposed      (author submits for review)
       └→ signed        (required approvals collected; GS >= 0.7 enforced)
            └→ executing     (orchestrator starts agent)
                 └→ review_pending   (agent done; review gate triggers)
                      └→ certifying      (review approved/skipped)
                           └→ certified      (all certification stages pass)
                                └→ merged        (admission controller commits to main)
                                     └→ deployed      (deployment engine confirms rollout)
                                          └→ realized    (intent metrics validated in production)
Any state → failed (on unrecoverable error; requeue possible)
```

Allowed transitions are strictly enforced. Invalid transitions are rejected with error.

---

## Phase 3 — Orchestrator

**Goal:** Watch the intent-store, drive intents through the lifecycle, coordinate all services.

### 3.1 Orchestrator Service (`internal` + `cmd/orchestrator/`)

```go
type Orchestrator struct {
    intentStore *intent.IntentStore
    ckg         *ckg.CKGService
    isolation   *isolation.Engine
    conflict    *conflict.Detector
    execution   *execution.Platform
    review      *review.Gate
    cert        *certification.Pipeline
    admission   *admission.Controller
    deployment  *deployment.Engine
    config      *config.PlatformConfig
}

func (o *Orchestrator) Run(ctx context.Context) error
// Polls intent-store every 5s for state changes
// Dispatches work to appropriate service based on current state
```

**Main loop:**
```
Every 5s:
  1. Fetch all intents in state: proposed, signed, certified, merged, deployed
  2. For each intent:
     proposed → check GS, collect approvals → transition to signed
     signed   → compute ICR, check conflicts → transition to executing (start agent pod)
     (executing is driven by K8s pod events, not polling)
     certified → call admission controller → transition to merged
     merged   → trigger deployment engine → transition to deployed
     deployed → validate intent metrics → transition to realized (or failed)
```

**Fault tolerance:**
- Orchestrator runs in active-passive pair (2 replicas, leader election via Redis SETNX)
- On leader failover: new leader re-scans intent-store and resumes from last known state
- All state stored in git (intent-store) and Redis (transient); orchestrator is stateless

---

## Phase 4 — Isolation Engine

**Goal:** Manage Redis locks for ICR regions; queue intents that conflict; recover dead agents.

### 4.1 Lock Management (`internal/isolation/engine.go`)

```go
type Engine struct {
    redis    *redis.Client
    ckg      *ckg.CKGService
}

// Acquire all locks for an ICR atomically
func (e *Engine) AcquireLocks(intentID string, icr *IsolatedChangeRegion, ttlSec int) (bool, string, error)
// Returns: granted, conflictingIntentID (if not granted), error

// Release all locks for an ICR
func (e *Engine) ReleaseLocks(intentID string, icr *IsolatedChangeRegion) error
```

**Lock key design:**
```
lock:icr:{sha256(sorted(exclusiveFiles))}
Value (JSON): { "intent_id": "...", "acquired_at": "ISO8601", "pod_id": "...", "ttl": 1800 }
Redis operation: SET key value NX EX 1800
```

**Acquire algorithm:**
```
ACQUIRE(intentID, icr):
  acquired = []
  for key in icr.LockKeys:
    result = REDIS.SET key {intentID, now, podID} NX EX 1800
    if result == nil (lock held):
      // Rollback
      for k in acquired: REDIS.DEL k
      return false, GET(key).intentID, nil
    acquired.append(key)
  return true, "", nil
```

**Priority preemption:**
- `critical` intents can preempt `low` priority locks: DEL the lock, requeue the low-priority intent
- `critical` CANNOT preempt `normal` or `high`

### 4.2 Priority Queue (`internal/isolation/queue.go`)

```go
// Redis sorted set: "queue:pending"
// Score: priorityWeight × (maxTimestamp - createdAt) — higher score = execute sooner
// priority weights: critical=1000, high=100, normal=10, low=1

func (q *Queue) Enqueue(intentID string, priority PriorityLevel) error
func (q *Queue) Dequeue() (string, error)     // pop highest-score item
func (q *Queue) Remove(intentID string) error
func (q *Queue) Position(intentID string) (int, error)
```

### 4.3 Heartbeat & Dead Agent Recovery (`internal/isolation/heartbeat.go`)

```go
// Agent pod sends heartbeat every 30s
// heartbeat:{podID} → {intentID, timestamp}, TTL 60s

func (h *HeartbeatMonitor) Run(ctx context.Context)
// Every 30s: SCAN lock:icr:* → check heartbeat:{lock.podID}
// If heartbeat missing: agent is dead → release locks → requeue intent
```

---

## Phase 5 — Conflict Detection (3 Layers)

**Goal:** Detect conflicts between concurrent intents before agent execution starts.

### Layer 1: Structural (`internal/conflict/structural.go`)

Pure set operations on ICR node sets. Called for every new ICR against all active ICRs.

```go
func CheckStructural(newICR, activeICR *IsolatedChangeRegion) *ConflictResult
// Checks:
//   FILE_OVERLAP (HIGH):   intersection(newICR.Exclusive files, activeICR.Exclusive files)
//   SYMBOL_OVERLAP (HIGH): intersection(newICR.Exclusive nodes, activeICR.Exclusive nodes)
//   READ_WRITE (MEDIUM):   newICR.Exclusive ∩ activeICR.SharedRead
//                          activeICR.Exclusive ∩ newICR.SharedRead
```

### Layer 2: Contract (`internal/conflict/contract.go`)

Uses Grove graph analysis. Invoked when Layer 1 finds no overlap.

```go
func CheckContract(newICR, activeICR *IsolatedChangeRegion, grove *grove.Client) *ConflictResult
// Checks:
//   API_CONFLICT (HIGH):   both modify symbols serving same API route (from Grove symbol annotations)
//   TYPE_CONFLICT (MEDIUM): both modify same shared type definition
```

### Layer 3: Semantic (`internal/conflict/semantic.go`)

LLM-powered. Invoked ONLY when: same domain AND shared boundary nodes AND no structural/contract conflict found.

```go
func CheckSemantic(intentA, intentB *Intent, sharedBoundary []string, llm LLMClient) *ConflictResult
```

**System prompt for LLM:**
```
You are analyzing two software intents for potential behavioral interference.
Both intents will execute concurrently on the same codebase.
They do NOT modify the same files, but they might still conflict semantically.

Determine if executing both simultaneously could produce an inconsistent system.
Consider:
1. Does one create something the other assumes doesn't exist?
2. Do both modify behavior that shares a system invariant?
3. Does one depend on state the other changes?

Respond with JSON only:
{
  "conflict_detected": boolean,
  "confidence": float (0.0–1.0),
  "explanation": "1-2 sentences",
  "resolution": "serialize" | "merge" | "partition" | "safe_parallel"
}
```

**Resolution actions:**
- `serialize`: queue the lower-priority intent until the first completes
- `merge`: both intents can proceed but their ChangeSets must be merged by Fuse before admission
- `partition`: intents must be scoped to separate domains (user action required)
- `safe_parallel`: no conflict, both can proceed

---

## Phase 6 — Agent Execution Platform

**Goal:** Run AI agents in isolated K8s pods that produce ChangeSets.

### 6.1 AgentExecution CRD (`manifests/crds/agentexecution.yaml`)

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: agentexecutions.platform.intent
spec:
  group: platform.intent
  versions:
    - name: v1
      served: true
      storage: true
  scope: Namespaced
  names:
    plural: agentexecutions
    singular: agentexecution
    kind: AgentExecution
```

### 6.2 K8s Operator (`internal/execution/operator.go`)

Watches `AgentExecution` CRDs. On new CRD:
1. Create ephemeral namespace: `exec-{intentID}`
2. Clone source repo into pod (read-only, except ICR exclusive paths)
3. Mount ICR boundaries as filesystem restrictions (Kubernetes RBAC + custom admission webhook)
4. Inject context: intent spec, ICR paths, Grove MCP endpoint, Prism endpoint
5. Start agent pod with time limit and cost budget
6. Watch pod for completion → collect ChangeSet from pod's output volume
7. Validate ChangeSet (has diff, has tests, within ICR paths)
8. Delete namespace on completion

### 6.3 Agent Pod Spec (`internal/execution/pod.go`)

```go
func BuildPodSpec(intent *Intent, icr *IsolatedChangeRegion, cfg *config.AgentConfig) *corev1.Pod {
    return &corev1.Pod{
        Spec: corev1.PodSpec{
            RestartPolicy: corev1.RestartPolicyNever,
            Containers: []corev1.Container{{
                Name:  "agent",
                Image: cfg.AgentImage,  // claude-code, codex, etc.
                Env: []corev1.EnvVar{
                    {Name: "INTENT_ID",          Value: intent.Metadata.ID},
                    {Name: "INTENT_SPEC",         Value: intentSpecJSON},
                    {Name: "ICR_EXCLUSIVE_PATHS", Value: strings.Join(icr.ExclusiveFiles, ":")},
                    {Name: "ICR_READABLE_PATHS",  Value: strings.Join(icr.ReadableFiles, ":")},
                    {Name: "GROVE_URL",           Value: cfg.GroveURL},
                    {Name: "PRISM_URL",           Value: cfg.PrismURL},
                    {Name: "LLM_BUDGET_DOLLARS",  Value: fmt.Sprintf("%.2f", cfg.CostBudget)},
                },
                Resources: corev1.ResourceRequirements{
                    Limits: corev1.ResourceList{
                        corev1.ResourceCPU:              resource.MustParse("4"),
                        corev1.ResourceMemory:           resource.MustParse("16Gi"),
                        corev1.ResourceEphemeralStorage: resource.MustParse("50Gi"),
                    },
                },
                // ActiveDeadlineSeconds enforced at pod level (30 min default)
            }},
        },
    }
}
```

### 6.4 ChangeSet Validation (`internal/execution/changeset.go`)

Before accepting a ChangeSet from an agent pod:
1. All modified files MUST be within `icr.ExclusiveFiles` (reject if outside)
2. ChangeSet MUST include at least one test file per modified source file
3. Diff must apply cleanly to current HEAD of source repo
4. ChangeSet cost must not exceed `LLM_BUDGET_DOLLARS`

---

## Phase 7 — Review Gate

**Goal:** Optional, team-configurable human review between agent output and certification.

### 7.1 Review Policy (`internal/review/policy.go`)

Read from platform-config repo: `policies/review.yaml`

```go
type ReviewPolicy struct {
    DefaultMode    ReviewMode    // "none" | "optional" | "mandatory"
    DefaultTimeout time.Duration // for "optional" mode
    MinReviewers   int
    Overrides      []PolicyOverride
}

type PolicyOverride struct {
    MatchDomains   []string
    MatchPriority  string
    Mode           ReviewMode
    Timeout        time.Duration
    MinReviewers   int
    RequiredRoles  []string
}
```

### 7.2 Review Gate Logic (`internal/review/gate.go`)

```
On review_pending state:
1. Resolve policy for this intent (domain + priority matching)
2. If mode == none:    → skip directly to certifying
3. If mode == optional:
   → notify reviewers (Slack/email/webhook)
   → start timeout timer
   → on approve: → certifying
   → on reject (intent wrong): → failed
   → on reject (implementation wrong): → re-execute agent with feedback
   → on timeout: → certifying (auto-advance, set auto_advanced=true)
4. If mode == mandatory:
   → notify reviewers
   → wait (no timeout)
   → same approve/reject logic, no timeout path
```

**Reviewer context (what they see):**
- Intent spec (title, description, acceptance criteria)
- Unified diff (ChangeSet.Diff)
- ICR boundaries (which files/symbols the agent was allowed to touch)
- Affected tests (CKG test plan)
- Granularity score
- Agent cost + execution time

---

## Phase 8 — Certification Engine

**Goal:** Multi-stage machine validation of ChangeSets. Replaces traditional CI.

### 8.1 Certification Pipeline (`internal/certification/pipeline.go`)

```go
type CertificationPipeline struct {
    grove   *grove.Client
    k8s     *kubernetes.Clientset
    config  *config.CertConfig
}

func (p *CertificationPipeline) Certify(cs *ChangeSet, intent *Intent) (*Certificate, error)
```

### 8.2 Stage 1 — Static

Runs in a certification pod (no deployment):

```
1a. BUILD VERIFICATION
    - Compile / type-check changed files
    - Resolve all dependencies
    - Expected: exit 0 within 5 min

1b. UNIT TESTS
    - Run: CKG-selected tests (from Phase 1 test selection) + ChangeSet declared unit tests
    - Coverage delta: new code must have >= 80% line coverage
    - Expected: all pass within 10 min

1c. CONTRACT TESTS
    - API schema validation (OpenAPI if applicable)
    - Event schema validation (Protobuf/Avro/JSON Schema if applicable)

1d. SAST (Semgrep)
    - Scan changed files with policy ruleset (OWASP, custom rules)
    - Fail on: HIGH/CRITICAL severity findings
    - Warn on: MEDIUM (block if policy requires)

1e. SEMANTIC VALIDATION
    - Verify public API contract preserved (compare exported signatures before/after)
    - Uses Grove grove_symbols + grove_impact to detect breaking changes
    - Check: no regression in blast radius confidence
```

### 8.3 Stage 2 — Integration (Ephemeral K8s Namespace)

```
2a. PROVISION
    - Create namespace: cert-{intentID}-{timestamp}
    - Deploy: changed service container + dependencies + DB (with migrations + seed data)
    - Health check: wait for all pods Ready

2b. INTEGRATION TESTS
    - Run ChangeSet.Tests.Integration against the live environment
    - Test real HTTP, DB, and event paths

2c. DAST (OWASP ZAP)
    - Baseline scan against running service
    - Fail on: HIGH severity findings

2d. TEARDOWN
    - Delete namespace unconditionally (always clean up)
```

### 8.4 Stage 3 — Staging (Conditional)

Runs when: intent has `estimated_scope.files >= 5` OR domain is `payments`/`auth`/`pii`.

```
3a. Deploy to persistent staging environment
3b. E2E tests derived from intent acceptance criteria
3c. Performance tests if intent is tagged latency-sensitive
```

### 8.5 Certificate Generation (`internal/certification/certificate.go`)

```go
type Certificate struct {
    ID        string
    IntentID  string
    Timestamp string
    Build     StageResult
    UnitTests StageResult
    IntegrationTests StageResult
    Security  SecurityResult
    Semantic  SemanticResult
    Signature string  // HMAC-SHA256 of all stage results, platform key
}
```

Certificate stored as YAML commit in intent-store repo:
`completed/{intentID}/certificate.yaml`

---

## Phase 9 — Admission Controller

**Goal:** Linear merge of certified ChangeSets to source repo main branch.

### 9.1 Admission Algorithm (`internal/admission/controller.go`)

```
On certified state for intent INT-XXXX:

1. REBASE
   - Fetch current HEAD of source repo main
   - git rebase HEAD -- ChangeSet.Diff
   - If rebase conflict: call Fuse merge driver on conflicted files
   - If Fuse still conflicts: fail intent, notify author

2. FINAL CONFLICT CHECK
   - Call CKG service: check new ChangeSet against any newly-admitted ChangeSets
     since agent started (race condition window)
   - If conflict: serialize (wait for conflicting intent to be admitted first)

3. COMMIT
   - Apply rebased diff as a single commit to source repo main
   - Commit message format:
     feat: {intent.Spec.Title}
     
     {intent.Spec.Description}
     
     Intent-ID: {intentID}
     Certificate: {cert.ID}
     Agent: {agent}:{podID}
     ICR-Hash: {sha256(icr.ExclusiveFiles)}
     Test-Plan: {unit}/{integration}/{e2e}
     Co-Authored-By: {agent} <noreply@example.com>
   - Push to main

4. TRANSITION intent state to merged
```

**Key constraint:** Only one admission at a time (admission controller is single-replica with Redis mutex). Serializes all merges to maintain linear history.

---

## Phase 10 — Deployment Engine

**Goal:** Progressive deployment with intent-metric validation.

### 10.1 Deployment Flow (`internal/deployment/engine.go`)

```
On merged state for intent INT-XXXX:

1. Build OCI container image from current source repo HEAD
2. Push to OCI registry: {registry}/service:{commit-sha}
3. Deploy via canary:
   - 1% traffic → wait gate (5 min)
   - 10% traffic → wait gate (10 min)
   - 25% traffic → wait gate (15 min)
   - 50% traffic → wait gate (15 min)
   - 100% traffic → done

4. At each gate: evaluate intent metrics
   - Collect: error rate, p99 latency, business metrics defined in intent spec
   - If metrics degrade beyond threshold: automatic rollback (redeploy previous SHA)
   - If metrics OK: proceed to next stage
```

### 10.2 Priority Deployment

`critical` intents: accelerated canary (1% → 100% in 2 stages, 2 min gates).  
`low` intents: extended gates (additional 30 min hold at 25% for observation).

### 10.3 Intent Metric Validation (`internal/deployment/validation.go`)

```
realized state = deployed AND intent metrics validated for >= 24h

Metrics validation:
- No regression in error rate vs baseline (p95 over last 7 days)
- No regression in p99 latency
- Acceptance criteria marked as verified in intent spec
```

---

## Phase 11 — Dashboard

**Goal:** Web UI for intent authoring, execution monitoring, and deployment status.

### 11.1 Backend API (`internal/dashboard/api.go`)

```
GET  /api/intents?state=all|draft|executing|...
GET  /api/intents/{id}
GET  /api/intents/{id}/changeset
GET  /api/intents/{id}/certificate
GET  /api/intents/{id}/deployment
GET  /api/queue                         # Priority queue state
GET  /api/locks                         # Active ICR locks
GET  /api/metrics                       # Platform-wide metrics
POST /api/intents                       # Create draft intent
POST /api/intents/{id}/approve
POST /api/intents/{id}/reject
POST /api/intents/{id}/review/approve
POST /api/intents/{id}/review/reject
```

### 11.2 Frontend SPA

Served from `cmd/dashboard/`. Stack: React (or plain HTML + htmx for simplicity). Views:
- Intent list with state pipeline visualization
- Intent detail: spec, approvals, execution log, review, certificate, deployment
- Queue view: active locks, waiting intents, priority breakdown
- Platform metrics: intents per day, average cycle time, certification pass rate

---

## Phase 12 — Intent CLI

### Commands (`internal/cli/commands.go`)

```
relay init                          Initialize relay in current repo (create platform-config)

# Intent lifecycle
relay intent create                 Interactive wizard to author an intent YAML
relay intent propose <intent-id>    Submit intent for approval
relay intent sign <intent-id>       Approve and sign an intent (requires role)
relay intent reject <intent-id>     Reject an intent with reason
relay intent cancel <intent-id>     Cancel an executing intent

# Monitoring
relay intent list [--state <state>] List intents, filter by state
relay intent show <intent-id>       Show full intent details + current state
relay intent log <intent-id>        Stream execution log for a running agent
relay intent diff <intent-id>       Show ChangeSet diff
relay intent cert <intent-id>       Show certification report
relay intent deploy <intent-id>     Show deployment status

# Review
relay review approve <intent-id> [--comment "..."]
relay review reject <intent-id> --reason "intent|implementation" --comment "..."

# Platform
relay queue                         Show priority queue state
relay locks                         Show active ICR locks
relay grove status                  Show Grove graph stats
relay config                        Show platform config

# Utilities
relay icr <intent-id>               Show computed ICR for an intent
relay score <intent-yaml>           Calculate granularity score for an intent
relay conflicts <intent-id>         Check for conflicts with active intents
```

---

## Three Git Repositories

| Repository          | Purpose                                                          |
|---------------------|------------------------------------------------------------------|
| `source-repo`       | Application source code. Linear main branch, no feature branches |
| `intent-store`      | Intent YAML files organized by state. Full audit trail in git log |
| `platform-config`   | Policies, domain definitions, agent configs, ReviewPolicy YAML  |

---

## Redis Key Namespace

| Key Pattern                    | Type         | TTL       | Purpose                         |
|--------------------------------|--------------|-----------|---------------------------------|
| `lock:icr:{sha256}`            | String (JSON)| 1800s     | ICR lock (SET NX EX)            |
| `queue:pending`                | Sorted Set   | —         | Priority queue (score = priority)|
| `queue:executing`              | Set          | —         | Currently executing intents     |
| `heartbeat:{podID}`            | String       | 60s       | Agent heartbeat (auto-expire)   |
| `graph:version`                | String       | —         | Source repo HEAD SHA (for cache)|
| `icr:{intentID}`               | String (JSON)| 86400s    | Cached ICR for active intent    |
| `leader:orchestrator`          | String       | 30s       | Leader election (auto-renew)    |

Zero persistence configured in Redis. All business state in git.

---

## Component Deployment (`manifests/deployments/`)

| Component                | Replicas | Type          | Notes                                |
|--------------------------|----------|---------------|--------------------------------------|
| Orchestrator             | 2        | Deployment    | Active-passive via Redis leader lock |
| CKG Service              | 2        | Deployment    | Both active, stateless               |
| Isolation Engine         | 1        | Deployment    | Single replica (Redis is source of truth) |
| Conflict Detector        | 2        | Deployment    | Stateless, both active               |
| Agent Execution Platform | 1        | K8s Operator  | Manages AgentExecution CRDs         |
| Certification Engine     | 1        | Deployment    | Spawns cert pods on demand           |
| Admission Controller     | 1        | Deployment    | Single replica (serializes merges)   |
| Deployment Engine        | 1        | Deployment    | Manages canary rollouts              |
| Dashboard                | 2        | Deployment    | Stateless, behind load balancer      |
| Grove                    | 2        | Deployment    | Shared Grove instance for all Relay services |

---

## Phased Delivery Schedule

| Phase | Deliverable                                                      | Depends on        |
|-------|------------------------------------------------------------------|-------------------|
| 1     | Grove client + CKG service (delegates ICR/tests to Grove)        | Grove ≥ Phase 5   |
| 2     | Intent schema, store, state machine, granularity enforcement     | —                 |
| 3     | Orchestrator (polling loop + state transitions)                  | Phase 1, 2        |
| 4     | Isolation engine (Redis locks, priority queue, heartbeat)        | Phase 1, 2        |
| 5     | Conflict detector (3 layers)                                     | Phase 1, 4        |
| 6     | Agent execution platform (K8s operator + pod spec)               | Phase 2, 4        |
| 7     | Review gate + policy evaluation                                  | Phase 2, 6        |
| 8     | Certification engine (3 stages)                                  | Phase 1, 6        |
| 9     | Admission controller (rebase + linear merge)                     | Phase 8           |
| 10    | Deployment engine (canary + metric validation)                   | Phase 9           |
| 11    | Dashboard (API + SPA)                                            | Phase 2, 3        |
| 12    | Intent CLI (all commands)                                        | Phase 2, 3, 7, 10 |
| 13    | Intent groups + atomic merge strategy                            | Phase 9           |
| 14    | Tests + benchmarks + e2e integration tests                       | All phases        |

---

## Key Design Constraints (Non-Negotiable)

1. **No branches** — source repo has a single `main` branch. Agents work on snapshots; diffs are rebased and committed by the admission controller. Feature branches, release branches, and hotfix branches are explicitly forbidden.
2. **Grove is required** — the CKG service has no fallback graph implementation. Grove must be running and reachable for Relay to function.
3. **Redis is transient only** — `appendonly no`, `save ""` in Redis config. Business state lives in git. Redis loss triggers a rebuild cycle (graph cache from Grove, locks expire naturally, queue rebuilt from intent-store).
4. **ChangeSets must include tests** — a ChangeSet without tests for each modified source file fails certification at Stage 1b. No exceptions.
5. **Linear admission** — the admission controller processes one ChangeSet at a time (Redis mutex). This is the mechanism that prevents merge conflicts instead of branches.
6. **Canary is always required** — even `critical` intents go through a 1% → 100% canary with at least one 2-minute gate. Direct 100% rollouts are not supported.
7. **Max 3 critical intents per 24h** — enforced by the isolation engine. A 4th `critical` intent in 24h is downgraded to `high` with a warning.
8. **Fuse integration for admission conflicts** — if the admission controller rebase fails, it calls Fuse (not the LLM directly) to resolve the conflict. This keeps the admission path deterministic.
