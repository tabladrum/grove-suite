# Relay — Detailed Design

Companion to `architecture.md`. Specifies data models, state machines, schemas, and algorithms for the Phase 2A wedge with forward-compatible hooks for later phases.

---

## 1. Domain Model

```
Tenant 1 ── * Project 1 ── 1 SourceRepo
   │
   ├── 1 IntentStore (git)
   ├── 1 PolicyConfig (platform-config @ ref)
   ├── * Intent 1 ── * ChangeSet 1 ── 1 Certificate
   │                       │
   │                       └── 1 ICR
   └── * AgentIdentity
```

- **Tenant**: isolation unit. Owns Postgres schema, Redis namespace, intent-store, K8s namespace.
- **Project**: a logical product mapped to one `SourceRepo` and a path/sub-tree.
- **Intent**: the work request. May originate from a human (YAML) or from an inferred description on a direct ChangeSet submission.
- **ChangeSet**: a unified diff + metadata + base commit. The unit of admission.
- **ICR**: the symbol-level Isolated Change Region computed by Grove + Relay.
- **Certificate**: the signed admission artifact.

---

## 2. Intent State Machine

```
            ┌────────────┐
            │  drafted   │  (CLI/UI created, not yet submitted)
            └─────┬──────┘
                  │ submit
                  ▼
            ┌────────────┐
            │ ingested   │
            └─────┬──────┘
                  ▼
            ┌────────────┐
            │ analyzing  │  (Grove impact/deps/tests + ICR)
            └─────┬──────┘
                  ├── policy_violation ──► rejected
                  ▼
            ┌────────────┐
            │  policy    │
            └─────┬──────┘
                  ├── low_confidence ──► awaiting_review
                  ▼
            ┌────────────┐
            │  locking   │  (Redis SET NX EX + fencing token)
            └─────┬──────┘
                  ├── lock_busy ──► queued ──► locking
                  ▼
            ┌────────────┐
            │ certifying │  (Stage 1, 2, optional 3)
            └─────┬──────┘
                  ├── cert_failed ──► rejected
                  ▼
            ┌────────────┐
            │  admitting │  (rebase + Fuse + re-cert slice + commit)
            └─────┬──────┘
                  ├── rebase_conflict_unresolvable ──► rejected
                  ▼
            ┌────────────┐
            │  admitted  │
            └────────────┘
```

Terminal states: `admitted`, `rejected`, `failed`, `cancelled`. `awaiting_review` is non-terminal; resolves to `locking` or `rejected`.

---

## 3. Postgres Schema (Phase 2A)

All tables in per-tenant schema `relay_<tenant>`.

```sql
-- intents: durable record of every intent
CREATE TABLE intents (
  id              TEXT PRIMARY KEY,                -- INT-YYYY-NNNN
  tenant_id       TEXT NOT NULL,
  project_id      TEXT NOT NULL,
  parent_id       TEXT REFERENCES intents(id),
  spec            JSONB NOT NULL,                  -- full Intent YAML
  state           TEXT NOT NULL,
  granularity     NUMERIC(3,2),
  icr_confidence  NUMERIC(3,2),
  created_by      TEXT NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON intents (tenant_id, state, updated_at);

-- changesets: one or more per intent (latest is current)
CREATE TABLE changesets (
  id              TEXT PRIMARY KEY,
  intent_id       TEXT NOT NULL REFERENCES intents(id),
  base_commit     TEXT NOT NULL,
  diff_oid        TEXT NOT NULL,                   -- content-addressed in object store
  agent_identity  TEXT NOT NULL,
  model_identity  TEXT NOT NULL,
  prompt_hash     TEXT,
  context_hash    TEXT,
  submitted_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- icrs: computed per changeset
CREATE TABLE icrs (
  changeset_id    TEXT PRIMARY KEY REFERENCES changesets(id),
  exclusive       JSONB NOT NULL,                  -- ["pkg::Symbol", ...]
  shared_read     JSONB NOT NULL,
  boundary        JSONB NOT NULL,
  confidence      NUMERIC(3,2) NOT NULL,
  hash            TEXT NOT NULL                    -- sha256 over canonical encoding
);

-- certificates: one per admitted commit
CREATE TABLE certificates (
  id              TEXT PRIMARY KEY,                -- cert-YYYY-MM-DD-xxxxxx
  intent_id       TEXT NOT NULL REFERENCES intents(id),
  changeset_id    TEXT NOT NULL REFERENCES changesets(id),
  commit_sha      TEXT NOT NULL,
  policy_version  TEXT NOT NULL,
  toolchain_image TEXT NOT NULL,
  signer_key_id   TEXT NOT NULL,
  signature       BYTEA NOT NULL,
  payload         JSONB NOT NULL,                  -- full cert body
  issued_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- locks: mirror of Redis (debug + recovery)
CREATE TABLE locks (
  symbol_id       TEXT NOT NULL,
  changeset_id    TEXT NOT NULL,
  fencing_token   BIGINT NOT NULL,
  acquired_at     TIMESTAMPTZ NOT NULL,
  expires_at      TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (symbol_id, fencing_token)
);

-- outbox: durable hand-off to git audit
CREATE TABLE outbox (
  id              BIGSERIAL PRIMARY KEY,
  kind            TEXT NOT NULL,                   -- intent_snapshot|certificate|state_event
  ref_id          TEXT NOT NULL,
  payload         JSONB NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  written_at      TIMESTAMPTZ
);
CREATE INDEX ON outbox (written_at) WHERE written_at IS NULL;

-- events: append-only audit of state transitions
CREATE TABLE events (
  id              BIGSERIAL PRIMARY KEY,
  intent_id       TEXT NOT NULL,
  ts              TIMESTAMPTZ NOT NULL DEFAULT now(),
  kind            TEXT NOT NULL,
  from_state      TEXT,
  to_state        TEXT,
  detail          JSONB
);
```

**Fencing tokens** are produced by a per-tenant Postgres sequence `lock_fence_seq`. The Admission Controller passes the token in the git push pre-receive payload; admission rejects if a higher token exists for any overlapping symbol.

---

## 4. ChangeSet Schema

```json
{
  "schema": "relay.changeset/v1",
  "intent_id": "INT-2026-042",
  "tenant": "acme",
  "project": "api",
  "base_commit": "9f2c1a4...",
  "agent": {
    "identity": "claude-code:1.4.2",
    "model": "claude-sonnet-4-6:2026-04-15",
    "prompt_template_hash": "sha256:b1e9...",
    "context_pack_hash": "sha256:7c2d..."
  },
  "diff": "<unified diff, utf-8>",
  "decision_record": {
    "summary": "...",
    "assumptions": ["..."],
    "abandoned_alternatives": ["..."],
    "unresolved_questions": []
  },
  "self_report": {
    "tests_run_locally": ["..."],
    "estimated_blast_radius": ["pkg/auth::RateLimiter"]
  }
}
```

`decision_record` is the Agent Decision Record (Codex review §3); required from Phase 2B.

---

## 5. ICR Computation Algorithm

Input: a ChangeSet diff + Grove client.

```
1. Parse diff → modified files, modified line ranges per file.
2. For each (file, line range): call Grove /symbols-at to map to symbol IDs.
3. exclusive = symbols whose body is modified.
4. shared_read = symbols referenced by exclusive set (via /deps), not modified.
5. boundary = symbols whose signature is touched (return type, params, generics).
6. For each edge contributing to shared_read/boundary, fetch confidence from Grove.
7. ICR confidence =
       min(0.99,
           geometric_mean(edge_confidences)
         * file_class_weight(exclusive)
         * decay(stale_index_age))
   where file_class_weight ∈ {1.0 normal, 0.5 config/migration, 0.0 generated}.
8. Canonicalize and hash {exclusive, shared_read, boundary} → icr_hash.
```

If `exclusive ∩ forbidden_paths` or any file in `exclusive` matches a serialize-class pattern, ICR is replaced by a coarse file-level lock and confidence is set from a separate table.

---

## 6. Lock Protocol

Redis keys, all namespaced `relay:<tenant>:lock:<symbol_id>`:

```
SET relay:<tenant>:lock:<sym> "<changeset_id>:<fencing_token>"
    NX EX <ttl_seconds>
```

- TTL = certification budget + admission buffer (default 20 min).
- Per-changeset lock acquisition is **all-or-nothing**: acquire all `exclusive` symbols in a Lua script; on failure, release any acquired and enqueue.
- `shared_read` symbols take a separate shared counter key with TTL; admission only checks they are not held exclusively.
- Heartbeat: orchestrator extends TTL every `ttl/3` while certifying.
- Admission verifies `fencing_token == latest token for symbol set` via Postgres before committing.

---

## 7. Certification Pipeline Detail

Each stage runs in an isolated container; results stored in `certifications` table (schema in §7.5).

| Stage | Inputs | Tools | Output |
|-------|--------|-------|--------|
| 1 — Build & Test | rebased worktree, test strategy | language toolchain image | `build_ok`, `test_run` (see §7.4), `coverage_of_changed_symbols` |
| 2 — Static Analysis | worktree, dependency manifest, quality profile | semgrep, gitleaks, govulncheck/npm audit, language linters | `sast_findings`, `secret_findings`, `dep_findings`, `quality_findings` |
| 3 — Integration | ephemeral env spec | helm/compose, smoke tests | `integration_ok`, traces |

Failure of any required stage → `cert_failed`. Stage outputs are inputs to the certificate payload.

### 7.1 Test Strategy

**Default: run the full unit + integration test suite.** This matches how teams already work and avoids the false-confidence failure mode of clever selection. Most suites run in < 10 minutes; the value of intelligent selection is small relative to the cost of getting it wrong.

Configuration is per-project:

```yaml
# in platform-config/policies/<tenant>.yaml under projects.<name>.certification
tests:
  strategy: full           # full | selective
  runners:
    go:         { cmd: "gotestsum --junitfile=results.xml -- -race ./..." }
    python:     { cmd: "pytest --junitxml=results.xml" }
    typescript: { cmd: "jest --reporters=jest-junit" }
    java:       { cmd: "mvn test" }
  always_run:              # ordered list of suites that always run regardless of strategy
    - "smoke"
  timeout_minutes: 15
  flake_retries: 0         # >0 only for tests marked @flaky
```

**`strategy: selective`** is opt-in for projects whose full suite exceeds the timeout. When selected:
- Direct tests: Grove `/tests` for changed symbols (depth 1)
- Transitive: walk `calls` + `uses-type` backward (depth 2–3), then `/tests` for those callers
- Always: any test in `always_run` + any test file present in the ChangeSet diff
- Fallback: if ICR confidence < 0.50 *or* any selected test framework can't be addressed at symbol level → fall back to full run for that framework

**Nightly full-suite guard:** when `selective` is enabled, the full suite runs nightly on `relay-main` head; failures open a high-priority `verification_drift` incident and freeze admission until resolved.

**E2E / functional tests:** out of scope for Phase 2A. The intent's `verification_plan.external_attestations` may reference E2E runs in external CI by ID; the certificate records the attestation reference but does not execute the suite.

### 7.2 Coverage of Changed Symbols (the Real Test Value-Add)

After tests pass, the engine verifies every changed symbol has a covering test. For each `exclusive` symbol in the ICR:

1. Look up inbound `tests` edges in Grove.
2. Intersect with the set of tests that PASSED in this run (not just selected — actually ran and passed).
3. Tests newly added by the ChangeSet count as covering symbols they reference.
4. Symbol annotated `// relay:no-test-required <reason>` (or language equivalent) is exempt; the reason is recorded in the certificate.

```
coverage = |covered_changed_symbols| / |changed_symbols|
```

Project policy: `coverage_threshold` (default `1.00`). If `coverage < threshold` and no annotation exempts the gap, certification fails with the list of uncovered symbols.

### 7.3 Blast Radius (Audit Metadata, Not a Gate)

The certificate records, but does not gate on:
- `changed_files`, `changed_symbols`, `changed_loc`
- `impact_files`, `impact_symbols` (BFS depth 2 over `calls` + `uses-type`)
- Reviewer dashboard renders this as the "what else might break" view.

### 7.4 TestRun Schema

```json
{
  "schema": "relay.testrun/v1",
  "framework": "go",
  "strategy": "full",
  "selection_hash": "sha256:7a3c...",
  "command": "gotestsum --junitfile=results.xml -- -race ./...",
  "started_at": "2026-05-30T14:33:09Z",
  "duration_ms": 84210,
  "total": 1247, "passed": 1247, "failed": 0, "skipped": 12, "errored": 0,
  "junit_xml_oid": "sha256:c2f8...",
  "failed_tests": [],
  "flaky_retries": [],
  "coverage_of_changed_symbols": {
    "threshold": 1.0,
    "actual": 0.92,
    "uncovered": ["pkg/auth::refreshToken"],
    "exempt": ["pkg/auth::initLogger // relay:no-test-required logger has no behavior to test"]
  }
}
```

Full JUnit XML is stored in object storage (or git LFS for self-hosted); the `junit_xml_oid` references it. The summary lives in Postgres.

### 7.5 Certifications Table

```sql
CREATE TABLE certifications (
  id               TEXT PRIMARY KEY,                  -- run-id
  changeset_id     TEXT NOT NULL REFERENCES changesets(id),
  stage            TEXT NOT NULL,                     -- build_test | static_analysis | integration
  status           TEXT NOT NULL,                     -- pass | fail | error
  started_at       TIMESTAMPTZ NOT NULL,
  finished_at      TIMESTAMPTZ,
  test_run         JSONB,                             -- TestRun schema §7.4 (Stage build_test only)
  findings         JSONB,                             -- all findings from Stage static_analysis
  detail           JSONB                              -- stage-specific extras
);
CREATE INDEX ON certifications (changeset_id, stage);
```

### 7.6 Stage 2: Static Analysis Suite

Stage 2 is fully self-contained — no external server required at runtime. All tools run as subprocesses against the ChangeSet's changed files only. Relay owns the quality gate decision: it aggregates findings from all tools, applies per-severity thresholds from project policy, and produces a single pass/fail verdict with a structured finding list.

The "new code only" scoping is natural here — Relay already knows the changed file set from the ChangeSet diff and passes it explicitly to each tool (via `--include` flags or an explicit file list).

**Security analysis tools:**

| Tool | Purpose | Invocation |
|------|---------|------------|
| `semgrep` | SAST — vulnerability and bug patterns | `semgrep scan --json --config=<rulesets> <changed-files>` |
| `gitleaks` | Secret detection | `gitleaks detect --source=. --report-format=json` |
| `govulncheck` | Go dependency CVEs | `govulncheck -json ./...` |
| `npm audit` | Node dependency CVEs | `npm audit --json` |
| `pip-audit` | Python dependency CVEs | `pip-audit --format=json` |

Default semgrep rulesets: `[p/security-audit, p/owasp-top-ten]`. The `p/sonarqube` ruleset is available when a SonarQube profile is imported (see §7.7).

**Code quality tools (language-specific):**

| Language | Tool | What it catches |
|----------|------|----------------|
| Go | `golangci-lint` | bugs, complexity, unused code, style |
| TypeScript/JS | `eslint` | bugs, best practices, complexity |
| Python | `ruff` | bugs, style, complexity, import hygiene |
| Java | `checkstyle` + `pmd` | style, design issues, duplicate code |

Each tool runs only against files touched by the ChangeSet. Results from all tools are merged into a unified findings list with normalized severity: `error | warning | info`.

**Quality gate logic (Relay-owned):**

```
deny if: count(findings where severity >= deny_severity) > deny_on_findings_above
```

Defaults: `deny_severity: error`, `deny_on_findings_above: 0`. Zero new `error`-level findings in changed code is the bar.

**SonarQube server as optional enrichment:** enterprises that already run SonarQube can configure Relay to submit analysis results to it for historical tracking and the SonarQube dashboard. This is enrichment only — the gate decision always belongs to Relay.

### 7.7 SonarQube Profile Import

Teams with customized SonarQube quality profiles (tuned rules, severity overrides, custom quality gates) can import their profile so Relay enforces the same rules during certification. This ensures the shift-left gate matches what the SonarQube server would evaluate, eliminating divergence between Relay admission and a downstream SonarQube check.

**CLI command:**

```bash
relay import sonarqube-profile ./acme-java-profile.xml \
  --output platform-config/policies/rulesets/acme-java.yaml
```

Export the profile XML from SonarQube via:
`GET /api/qualityprofiles/export?language=java&qualityProfile=Acme+Java`

**Import process:**

1. Parse the quality profile XML — extract enabled rules (keys like `java:S1764`), severity overrides, and rule parameters.
2. Walk a bundled SQ-key → semgrep-rule-ID mapping table (covers the `p/sonarqube` ruleset).
3. For matched rules: generate a semgrep rules file containing only those rules with correct severities applied.
4. For unmatched rules: emit a coverage gap list — rule keys with no semgrep equivalent, flagged for manual review.
5. Import quality gate conditions (e.g., "0 blocker issues on new code") as Relay gate thresholds.
6. Write the generated Relay ruleset YAML + gap report to `--output`.

**Output format:**

```yaml
# platform-config/policies/rulesets/acme-java.yaml
# Generated by: relay import sonarqube-profile acme-java-profile.xml
# Source profile: Acme Java / sonarqube.acme.com / exported 2026-05-30
# Rule coverage: 142/168 rules mapped (84.5%)
schema: relay.ruleset/v1
source:
  type: sonarqube_profile
  profile_name: "Acme Java"
  language: java
  exported_at: "2026-05-30T09:00:00Z"
semgrep_rules:
  - id: java.lang.security.audit.cbc-padding-oracle
    severity: error   # maps SQ rule java:S5542
  # ... 141 more
quality_gate:
  deny_severity: error
  max_warnings: 0
coverage_gaps:
  - sq_rule: java:S6437    # no semgrep equivalent; consider manual review
  - sq_rule: java:S4830
  # ... 24 more
```

The generated file lives in `platform-config` (version-controlled alongside policies). Re-run the import command when the quality profile changes on the SonarQube server. The coverage gap list is how teams track which SonarQube rules have no Relay equivalent and decide whether those gaps are acceptable.

---

## 8. Admission Algorithm

```
1. lock_owned = verify_fencing_tokens(changeset.icr.exclusive)
   if not lock_owned: requeue.
2. ours = changeset.diff applied to base_commit
   theirs = current HEAD of target branch (relay-main)
   if base_commit == theirs: skip rebase.
   else:
       try git rebase ours onto theirs.
       on conflict: invoke fuse for each conflicting path.
       if fuse cannot resolve any path: reject(rebase_conflict_unresolvable).
3. If rebase changed any tracked file: re-run Stage 1 slice on rebased state.
4. Build certificate payload from stored stage results + ICR + identity.
5. Sign payload with KMS key → signature.
6. git commit-tree with trailers = certificate fields (one trailer per field).
7. git push relay-main --atomic.
8. On push success:
       a. Insert certificates row + outbox(kind=certificate).
       b. Mark intent admitted; emit event.
       c. Release Redis locks.
       d. Trigger Grove /index-delta on the new commit.
9. On push failure (fencing race):
       reset state to locking, requeue with new token.
```

---

## 9. Outbox Reconciler

- Polls `outbox WHERE written_at IS NULL ORDER BY id LIMIT N`.
- For each row: open per-tenant intent-store working copy, write file at the canonical path, commit with author = `relay-outbox`, push.
- Update `written_at`.
- Backoff on push conflicts; the reconciler is the only writer to intent-store.

Canonical paths:
```
intents/<YYYY>/<MM>/<intent-id>.yaml
changesets/<YYYY>/<MM>/<changeset-id>.json
certificates/<YYYY>/<MM>/<cert-id>.json
events/<YYYY>/<MM>/<DD>/<intent-id>.jsonl
```

---

## 10. Policy Engine

Policy gates run in declared order BEFORE certification. First `Deny` short-circuits the pipeline. `ReviewRequired` continues but flags the intent for human approval before admission. `Warn` continues; recorded only.

The Test Coverage gate (§7.2) is a special case: it runs INSIDE certification, after tests pass — gates have access to test results.

### 10.1 Gate Interface

```go
package policy

type Decision string
const (
    Allow          Decision = "allow"
    Deny           Decision = "deny"
    Warn           Decision = "warn"
    ReviewRequired Decision = "review_required"
)

type Gate interface {
    Name() string
    Evaluate(ctx context.Context, cs *core.ChangeSet, cfg Config) (*Result, error)
}

type Result struct {
    Gate     string
    Decision Decision
    Reason   string                   // human-readable
    Details  map[string]interface{}   // structured, included in certificate
}

type Engine struct {
    gates []Gate
}

func (e *Engine) Evaluate(ctx context.Context, cs *core.ChangeSet, cfg Config) (*Evaluation, error) {
    out := &Evaluation{}
    for _, g := range e.gates {
        r, err := g.Evaluate(ctx, cs, cfg)
        if err != nil { return nil, fmt.Errorf("%s: %w", g.Name(), err) }
        out.Results = append(out.Results, r)
        switch r.Decision {
        case Deny:
            out.Final = Deny
            return out, nil
        case ReviewRequired:
            out.RequiresReview = true
        }
    }
    out.Final = Allow
    return out, nil
}
```

Each gate lives under `internal/policy/<gate>/`. The Engine is constructed once per tenant from config; gates are stateless.

### 10.2 Phase 2A Gates

| # | Gate | Pkg | Default decision on hit |
|---|------|-----|-------------------------|
| 1 | Path Policy | `internal/policy/path` | Deny |
| 2 | Secret Scanner | `internal/policy/secrets` | Deny |
| 3 | Special File Class | `internal/policy/fileclass` | Varies (deny / regenerate / serialize) |
| 4 | Dependency Change | `internal/policy/deps` | ReviewRequired |
| 5 | Size Limit | `internal/policy/size` | Deny (hard) / Warn (soft) |
| 6 | Test Coverage* | `internal/policy/coverage` | Deny |

\* Runs inside certification, not pre-certification.

#### Gate 1 — Path Policy
- `forbidden_paths` (glob) ∩ touched paths → Deny
- `allowed_paths` set and any touched path not matched → Deny
- `allowed_paths` empty → fall through to project defaults

#### Gate 2 — Secret Scanner
- Scans only added/modified lines.
- Ships with ~20 high-confidence patterns (AWS keys `AKIA[0-9A-Z]{16}`, GitHub PATs `gh[ps]_[A-Za-z0-9]{36,}`, GCP service-account JSON, private-key headers, Stripe keys, Slack tokens, etc.).
- Allowlist supports `path` and `value` matchers; per-project additional patterns allowed.
- `severity_block: [high, critical]` controls threshold.

#### Gate 3 — Special File Class
- `migration` (path patterns): serialize at admission via file-level lock; ChangeSet must touch only one migration file; lockfile decision recorded.
- `lockfile`: agent's version is discarded; admission regenerates by running the language tool (`go mod tidy`, `npm install --package-lock-only`, etc.) post-rebase.
- `generated` (path patterns or `// Code generated by` / `@generated` headers): Deny unless intent has `allow_generated: true` (then regenerate via tool, ignoring agent's output).
- `config` (path patterns): file-level lock + ReviewRequired.

#### Gate 4 — Dependency Change
- Detects modifications to `go.mod`, `go.sum`, `package.json`, `package-lock.json`, `requirements.txt`, `Pipfile.lock`, `Cargo.toml`, `Cargo.lock`, `pom.xml`, `build.gradle`.
- Default action: `ReviewRequired`.
- Phase 2A: passes through to certification's `dep_findings` (govulncheck/npm audit). Phase 2B: cross-checks against an allowlist.

#### Gate 5 — Size Limit
- `max_added_lines` (default 1000) → Deny
- `max_files` (default 50) → Deny
- `max_lines_single_file` (default 500) → Warn

#### Gate 6 — Test Coverage (intra-certification)
- See §7.2. Inputs are the changed symbol set and the TestRun's passed-tests list.
- Project policy controls `coverage_threshold` and whether `// relay:no-test-required` exemption is allowed.

### 10.3 Project Policy Config

Per-project policy lives in the repo's `.relay/` directory (see §12.6). Org-wide locked baselines (enterprise mode only) live in `platform-config`. The example below shows the merged effective config that the engine sees at runtime; the same content can come entirely from `.relay/relay.yaml` (laptop/team) or from a merge of `platform-config` baseline + `.relay/relay.yaml` (company).

```yaml
# Effective configuration (laptop/team: just .relay/relay.yaml + .relay/policies/*.yaml;
#                         company: org baseline + .relay/ merged)
schema: relay.config/v1
relay_version: ">=0.5 <0.6"
projects:
  api:
    gates:
      path:        { enforce: strict, allowed: ["internal/**", "pkg/**", "tests/**"], forbidden: ["services/payments/**"] }
      secrets:     { severity_block: [high, critical] }
      fileclass:
        migration:  { paths: ["**/migrations/*.sql"] }
        lockfile:   { paths: ["go.sum", "**/package-lock.json"], regenerate: true }
        generated:  { paths: ["**/*.pb.go", "**/*_generated.*"], allow_with_intent: false }
        config:     { paths: ["config/**.yaml"] }
      deps:        { action: review_required }
      size:        { max_added_lines: 1000, max_files: 50, max_lines_single_file: 500 }
      coverage:    { threshold: 1.00, allow_no_test_required_annotation: true }

    certification:
      stages: [build_test, static_analysis]
      tests:   # see §7.1
        strategy: full
        timeout_minutes: 15
      sast:
        tools: [semgrep]
        rulesets: [p/security-audit, p/owasp-top-ten]
        severity_block: [error, warning]
      secrets:
        tools: [gitleaks]
      deps:
        tools: [govulncheck, npm_audit]
        severity_block: [high, critical]
      quality:
        tools: [golangci_lint, eslint, ruff]
        deny_on_findings_above: 0    # zero new issues in changed code
        deny_severity: error
        # ruleset: platform-config/policies/rulesets/acme-java.yaml  # sonarqube profile import (optional)

    icr:
      min_confidence_auto: 0.85
      min_confidence_review: 0.50
```

Policy version is captured in every certificate so the exact policy snapshot is reproducible from audit data. Two values are recorded: `Policy-Version` (git SHA of `platform-config` if present, else the repo's commit SHA at config-load time) and `Effective-Config-Hash` (sha256 of the canonicalized merged config from all layers — §12.6.6).

---

## 11. Concurrency Model

- Each Orchestrator worker picks one intent at a time and runs the state machine to a terminal or yielding state (`queued`, `awaiting_review`).
- Per-tenant max-in-flight is enforced via Postgres advisory lock keyed on `(tenant_id, project_id)` and a counting semaphore in Redis.
- Stages run as detached jobs in K8s with per-stage timeouts; the worker resumes on completion event.

---

## 12. Failure Modes and Recovery

| Failure | Detection | Recovery |
|---------|-----------|----------|
| Worker crash mid-cert | Heartbeat lost → lease expires | New worker resumes intent in `certifying` or `locking` |
| Redis loss | Reconnect; locks gone | All in-flight intents revert to `queued`; replays from Postgres |
| Postgres failover | App reconnect | Outbox replays unsent rows; no audit gap |
| Git push race on admission | Pre-receive rejects | Increment fencing token, requeue |
| Grove index stale | `index_age > threshold` in confidence calc | Confidence decayed → may flip to `awaiting_review` |
| Cert signing key rotation | KMS rotation event | New certs use new key; old certs remain verifiable via key history |

---

## 12.5 Deployment Modes

Relay ships as a single binary that runs in three modes. Mode is selected by configuration, not by build.

### 12.5.1 Laptop mode

```yaml
# ~/.relay/config.yaml
mode: laptop
store:
  type: sqlite
  path: ~/.relay/relay.db
intent_store:
  type: local_git
  path: ~/.relay/intent-store
grove:
  url: http://localhost:7777    # auto-started if unreachable
tools:
  fetch_on_demand: true          # download missing tools on first use
signer:
  type: local
  key_path: ~/.relay/signer.ed25519
```

- Operational state in embedded SQLite (`modernc.org/sqlite`, pure Go — no CGO conflict with Grove's tree-sitter).
- Intent-store is a local bare git repo at `~/.relay/intent-store/`. Certificates committed there.
- No Redis (single-agent, ICR locks not needed in the critical path).
- Cert signing uses a locally generated Ed25519 key. Trust model: the developer trusts their own key.
- Bundled tool binaries: shipped in the install tarball or fetched on first run from a content-addressed mirror.

### 12.5.2 Team mode

```yaml
mode: team
store:
  type: postgres
  dsn: postgres://relay:***@db.internal/relay
intent_store:
  type: git
  url: git@gitserver:relay/intent-store.git
redis:
  url: redis://redis.internal:6379
grove:
  url: http://grove.internal:7777
signer:
  type: kms_or_local
  key_id: relay-team-signer-2026
```

- Postgres replaces SQLite. Same schema (`internal/store/migrations/` works against both — SQLite-compatible SQL).
- Redis enables ICR locks (multi-agent coordination becomes meaningful).
- Shared intent-store git repo.
- Dashboard at `:9000`.

### 12.5.3 Company mode

Adds: multi-tenancy (per-tenant Postgres schemas, Redis namespaces, intent-stores, K8s namespaces), SSO/OIDC, RBAC, regional control planes, Grove Federation, Audit Aggregator. See product-proposal §9.

### 12.5.4 Engine isomorphism

The same Go packages (`internal/policy`, `internal/icr`, `internal/cert`, `internal/admission`, `internal/signer`) execute in every mode. Only the storage adapter, signer backend, and transport differ. The certificate format is identical across modes; a certificate signed in laptop mode verifies the same way as one signed in company mode (against the trusted key bundle the verifier holds).

---

## 12.6 Configuration in the Repo (`.relay/`)

The repo's Relay configuration is source-controlled in `.relay/` alongside the code it governs. This is what makes the engine truly mode-portable: the same `.relay/` works on a laptop, a team server, and an enterprise control plane.

### 12.6.1 Resolved Config Layering

```
built-in defaults
    ⨁ org baseline (platform-config, enterprise only; can lock fields)
        ⨁ repo config (.relay/ in source repo; primary surface)
            ⨁ user/host config (~/.relay/config.yaml; credentials only, no policy)
                = effective config (hash recorded in certificate)
```

Merge semantics:
- **Scalars** in the lower layer override those in the higher layer, *unless* the higher layer marked them locked.
- **Lists** default to *replace*. The org baseline can opt a list into *union* mode (e.g., `forbidden_paths: { mode: union }`) so per-repo additions stack on top.
- **Locked fields** in the org baseline reject any change from lower layers; the loader errors out at config load time with a clear message naming the offending file and field.

### 12.6.2 Repo Layout

```
.relay/
├── relay.yaml              # entry point
├── policies/
│   ├── path.yaml
│   ├── secrets.yaml
│   ├── fileclass.yaml
│   ├── deps.yaml
│   ├── size.yaml
│   └── coverage.yaml
├── rulesets/
│   └── acme-java.yaml      # imported SonarQube profile or custom semgrep bundle
├── intents/                # source-controlled intents (optional)
│   └── INT-2026-042.yaml
└── templates/              # intent templates
    └── feature.yaml
```

### 12.6.3 `relay.yaml` schema

```yaml
schema: relay.config/v1
relay_version: ">=0.5 <0.6"     # binary version pin; load fails if not satisfied

project:
  name: api
  path: /                        # path within repo (monorepo support)
  default_branch: main
  language_primary: go
  languages: [go, sql]

gates:
  path:      { config: policies/path.yaml }
  secrets:   { config: policies/secrets.yaml }
  fileclass: { config: policies/fileclass.yaml }
  deps:      { config: policies/deps.yaml }
  size:      { config: policies/size.yaml }
  coverage:  { config: policies/coverage.yaml }

certification:
  stages: [build_test, static_analysis]
  tests:
    strategy: full
    runners:
      go: { cmd: "gotestsum --junitfile=results.xml -- -race ./..." }
    timeout_minutes: 15
  static_analysis:
    sast:     { tools: [semgrep], rulesets: [p/security-audit, p/owasp-top-ten, rulesets/acme-java.yaml] }
    secrets:  { tools: [gitleaks] }
    deps:     { tools: [govulncheck] }
    quality:  { tools: [golangci_lint], deny_severity: error, deny_on_findings_above: 0 }

icr:
  min_confidence_auto: 0.85
  min_confidence_review: 0.50

# Optional pointers for external systems
external:
  source_repo:   "git@github.com:acme/api.git"
  intent_store:  "git@github.com:acme/intent-store.git"   # team/company mode
```

### 12.6.4 Discovery

`relay_check`, `relay check`, the git pre-push hook, and the MCP server all walk upward from `cwd` looking for the nearest `.relay/relay.yaml`. The first one found defines the project. In monorepos, deepest match wins; nested `.relay/` overrides specific gates from the parent.

```go
// internal/config/discovery.go (sketch)
func DiscoverProject(cwd string) (*ProjectConfig, error) {
    for dir := cwd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
        if cfg, err := loadIfExists(filepath.Join(dir, ".relay", "relay.yaml")); err == nil {
            return cfg, nil
        }
    }
    return nil, ErrNoRelayConfig
}
```

### 12.6.5 What goes where — definitive split

| `.relay/` (repo, committed) | `platform-config` (enterprise) | `~/.relay/config.yaml` (host) |
|-----------------------------|--------------------------------|-------------------------------|
| Gate enable/disable, thresholds | Org-wide baselines + locked fields | Postgres DSN, Redis URL |
| Path policies, file classes | Mandatory cert stages | KMS key ID / local signer path |
| Test runners + commands | Minimum coverage / ICR thresholds | MCP transport (stdio/http) + port |
| Custom + imported rulesets | Approved ruleset whitelist | OIDC token / tenant token |
| Intent templates | Org-wide intent templates | Intent-store git URL (team mode) |
| ICR confidence thresholds (within bounds) | Lock-bounded thresholds | Bundled tool path overrides |

**Rule of thumb:** *what the project must satisfy* → `.relay/`. *Organization-wide minimum standards* → `platform-config`. *Where this Relay binary connects and authenticates* → `~/.relay/config.yaml`.

### 12.6.6 Config hash → Certificate

The effective merged config is canonicalized and hashed (sha256). The hash appears in every certificate as `Effective-Config-Hash` (and the `platform-config` git SHA is in `Policy-Version`). Two certificates with the same hash were evaluated by byte-identical rules. This is the foundation of reproducible audit: a regulator can replay verification against the historical config hash.

### 12.6.7 Bootstrap

```bash
cd my-repo
relay init --stack=go-microservice    # writes .relay/ with stack-appropriate defaults
git add .relay/ && git commit -m "Add Relay configuration"
```

After this, every Relay invocation against this repo — laptop, team, or company; MCP, CLI, or git hook — sees the same configuration. No "create a project in Relay first" step; the repo defines itself.

### 12.6.8 SonarQube profile import target

The `relay import sonarqube-profile` command writes its output into the repo, not into platform-config:

```bash
relay import sonarqube-profile ./acme-java-profile.xml \
  --output .relay/rulesets/acme-java.yaml
```

The imported ruleset is then referenced from `.relay/relay.yaml`. It travels with the repo, version-controlled like everything else.

---

## 13. MCP Tool Surface

Relay exposes the certification engine as an MCP server. This is the **primary integration point for agents** — the surface a Claude Code, Cursor, Continue, or Windsurf user invokes Relay through.

### 13.1 Transport

- **Laptop mode:** stdio (Relay runs as a child process of the IDE/agent). Zero network setup.
- **Team / company mode:** HTTP+SSE on the shared server. Multiple agents from multiple developers connect concurrently.

### 13.2 Tool Definitions

#### `relay_check`

```jsonc
{
  "name": "relay_check",
  "description": "Run the certification pipeline on uncommitted changes (or a provided diff). Returns structured findings the agent can act on before committing. Idempotent and side-effect-free.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "diff":         { "type": "string", "description": "Unified diff. Optional; defaults to current worktree diff vs HEAD." },
      "base_commit":  { "type": "string", "description": "Base commit SHA. Optional; defaults to HEAD." },
      "project":      { "type": "string", "description": "Project name (matches platform-config). Optional in laptop mode." },
      "gates":        { "type": "array",  "items": {"type": "string"}, "description": "Subset of gates to run. Default: all." }
    }
  }
}
```

**Result:**

```jsonc
{
  "verdict": "pass" | "fail" | "review_required",
  "findings": [
    {
      "gate":        "secrets",
      "file":        "internal/auth/login.go",
      "line":        42,
      "rule":        "gitleaks:aws-access-key",
      "severity":    "error",
      "message":     "AWS access key detected",
      "fix_hint":    "Move to environment variable or KMS secret"
    }
  ],
  "test_run":     { "...TestRun schema (§7.4)..." },
  "icr": {
    "exclusive":   ["pkg/auth::Login", "pkg/auth::validateToken"],
    "confidence":  0.92
  }
}
```

#### `relay_certify`

Runs the full certification pipeline AND emits a signed certificate. Side-effecting (writes to operational store). Used when the agent is ready to commit.

```jsonc
{
  "name": "relay_certify",
  "inputSchema": {
    "type": "object",
    "properties": {
      "diff":         { "type": "string" },
      "base_commit":  { "type": "string" },
      "intent_id":    { "type": "string", "description": "Required. Use relay_submit first if no intent exists." },
      "project":      { "type": "string" }
    },
    "required": ["intent_id"]
  }
}
```

**Result:** certificate payload, signature, full commit trailer block ready to attach to a git commit, plus the verdict + findings (same shape as `relay_check`).

#### `relay_submit`

```jsonc
{
  "name": "relay_submit",
  "description": "Submit a ChangeSet to the admission queue. Team/company mode only. In laptop mode, this is an alias for relay_certify followed by a local commit.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "intent": {
        "type": "object",
        "description": "Intent metadata. If intent_id is provided, references an existing intent; otherwise creates one."
      },
      "diff":         { "type": "string" },
      "base_commit":  { "type": "string" }
    },
    "required": ["diff", "base_commit"]
  }
}
```

**Result:** `{ "intent_id": "...", "state": "queued|certifying|admitting", "queue_position": 3, "estimated_admission_seconds": 47 }`

#### `relay_policy`

```jsonc
{
  "name": "relay_policy",
  "description": "Fetch the active policy for the current project so the agent knows the rules upfront. Recommended at session start.",
  "inputSchema": {
    "type": "object",
    "properties": { "project": { "type": "string" } }
  }
}
```

**Result:** policy YAML (the same one in `platform-config/policies/<tenant>.yaml`) plus its content-addressed version.

#### `relay_explain`

```jsonc
{
  "name": "relay_explain",
  "description": "Return a human-readable explanation of a specific finding, with rule documentation and remediation steps.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "finding_id": { "type": "string" },
      "rule":       { "type": "string" }
    }
  }
}
```

**Result:** markdown explanation, rule documentation link, suggested remediation.

### 13.3 Recommended Agent System-Prompt Pattern

Bundled with the Relay install is a system-prompt fragment for agents:

```text
This project is governed by Relay. Before reporting code changes complete:

1. Call relay_policy at session start to load the active rules.
2. After making changes, call relay_check.
3. If findings are returned, fix them and re-check. Do not report success while
   relay_check returns error-severity findings.
4. When ready to commit, call relay_certify (laptop) or relay_submit (team/company).
```

This is what makes Relay an agent *companion* rather than an agent *gate*. The agent self-corrects in-loop; humans only see code that has already passed the engine.

### 13.4 Engine Isomorphism

The MCP tools, CLI commands, and git pre-push hook all invoke the same internal Go packages:

```
internal/api/mcp/    ← MCP transport binding
internal/cli/        ← CLI transport binding
internal/githook/    ← Pre-push hook binding
        │
        └──→ internal/engine/  (shared certification engine)
                ├── internal/policy/    (gates)
                ├── internal/icr/       (ICR + Grove)
                ├── internal/cert/      (cert pipeline)
                ├── internal/signer/    (signing)
                └── internal/admission/ (rebase + commit)
```

Adding a gate to `internal/engine` adds it to all three surfaces simultaneously. No surface owns its own logic.

---

## 14. CLI Surface (Phase 2A)

```
# Mode bootstrap
relay init [--mode laptop|team|company]   # writes ~/.relay/config.yaml
relay mcp serve [--stdio|--http]          # start MCP server (default stdio in laptop)

# Tenant / project lifecycle (team / company mode)
relay tenant init <name>
relay repo add <project> --url <git-url>
relay project add <name> --repo <repo> --path <subdir>

# Engine commands (same engine as MCP tools)
relay check [--diff <file>] [--base <sha>] [--gates <list>]   # = relay_check
relay certify --intent <id>                                    # = relay_certify
relay submit                                                   # extract diff + submit (= relay_submit)
relay submit --diff <file> --base <sha>
relay policy show [--project <name>]                           # = relay_policy
relay explain --rule <rule-id>                                 # = relay_explain

# Intent + cert browsing
relay intent get <id>
relay intent list --state <state>
relay cert show <cert-id>
relay cert verify <cert-id>                                    # offline verify against trusted key bundle
relay audit query --since <ts> --agent <name>

# Tooling + profile import
relay tools install                                             # download bundled toolset
relay tools status                                              # show installed versions
relay import sonarqube-profile <profile.xml> [--output <rulesets/path.yaml>]

# Git hook installation
relay hook install [--pre-push]                                 # writes .git/hooks/pre-push
relay hook uninstall

# State migration
relay migrate sqlite-to-postgres --dsn <postgres-dsn>           # laptop → team upgrade

# Signature capabilities
relay cert show <ref>                                           # human-readable Passport for any commit/cert
relay cert replay <cert-id> [--out <report.json>]               # byte-reproducible audit replay
relay revert --intent <intent-id> [--dry-run]                   # symbol-scoped surgical revert (Phase 2B)
relay scorecard --agent <name> [--since <ts>] [--format csv]    # agent/model performance report (Phase 2B+)
relay marketplace search <query>                                # community .relay/ profile lookup
relay init --profile=<profile-name>                             # scaffold from marketplace profile
```

---

## 15. Extension Points (forward-compat for Phase 3+)

- `Decomposer` interface: `Decompose(intent) -> []intent`; invoked between `ingested` and `analyzing` when `intent.execution.decomposition = auto`.
- `GroupAdmissionController`: wraps Admission Controller to commit N intents atomically (multi-parent merge commit with N-1 fast-forwards).
- `CanaryGate`: interceptor between `admitting` and `admitted`; promotes on metric SLO.
- `ModelRouter`: chooses model from intent metadata + GS + risk; consumed by agent pod runtime.

All four are no-op by default in Phase 2A; switching them on does not change any persisted schema.

---

## 16. Signature Capabilities — Data Contracts

These are the runtime data shapes for the capabilities described in [product-proposal.md §7B](product-proposal.md#7b-signature-capabilities-the-things-people-remember) and [architecture.md §3.11](architecture.md#311-signature-capability-services). Each is a derived view over already-persisted state — no new tables in Phase 2A.

### 16.1 `relay_check` findings schema (Phase 2A — feeds Pre-Flight Autopilot)

The tight, agent-friendly findings format that the recommended system prompt relies on:

```json
{
  "schema": "relay.findings/v1",
  "pass": false,
  "gates": [
    { "name": "secret_scan", "decision": "deny", "blocking": true },
    { "name": "sast",        "decision": "warn", "blocking": false },
    { "name": "coverage",    "decision": "deny", "blocking": true }
  ],
  "findings": [
    {
      "id": "gitleaks:aws-access-key",
      "severity": "error",
      "file": "internal/auth/secrets.go",
      "line": 42,
      "symbol": "internal/auth::loadSecrets",
      "rule_url": "https://...",
      "message": "AWS access key committed in source",
      "fix_hint": "Move to environment variable; rotate the leaked key.",
      "auto_fixable": false
    }
  ],
  "next_action": "fix_and_recheck"
}
```

`next_action` is the contract the agent loop reads: one of `fix_and_recheck`, `request_human`, `proceed_to_certify`. Agents that respect `next_action` get the Pre-Flight Autopilot behavior for free.

### 16.2 AI Code Passport payload (Phase 2A)

A human + machine view over the existing certificate:

```json
{
  "schema": "relay.passport/v1",
  "cert_id": "cert-2026-05-30-a3f9b2",
  "commit": "9f2c1a4...",
  "intent": { "id": "INT-2026-042", "title": "Add rate limiting to /api/auth/*" },
  "agent":  { "identity": "claude-code:1.4.2", "model": "claude-sonnet-4-6:2026-04-15" },
  "policy": { "version": "platform-config@4f8a...", "config_hash": "sha256:..." },
  "tests":  { "selected": 23, "passed": 23, "coverage_of_changed_symbols": 1.0 },
  "scans":  { "sast_passed": true, "secrets_passed": true, "deps_passed": true },
  "risk":   { "icr_confidence": 0.91, "boundaries": [], "blast_radius": 7 },
  "signature": { "algo": "ed25519", "key_id": "0xABCD1234", "value": "..." }
}
```

Surfaces: `relay cert show <ref>`, PR/MR bot comment, dashboard card, JSON-LD export for SBOM/SLSA toolchains.

### 16.3 Risk Heatmap (Phase 2A)

A per-symbol score computed at admission time and stored alongside the certificate. No new table — added to the `certificates.payload` JSON under `risk_heatmap`.

```json
"risk_heatmap": [
  {
    "symbol": "internal/auth::RateLimiter.Allow",
    "score": 0.62,
    "factors": {
      "icr_confidence": 0.91,
      "boundary_flags": ["auth-touched"],
      "coverage_delta": 0.0,
      "downstream_callers": 14,
      "historical_defect_density": 0.18
    }
  }
]
```

Score formula is documented and versioned (`risk_model_version` in the certificate); changing it requires bumping the version so historical scores are not silently revised.

### 16.4 Surgical Revert ChangeSet (Phase 2B)

`relay revert --intent <id>` synthesizes a ChangeSet whose `decision_record.summary` is `"surgical revert of INT-2026-042"` and whose diff is the symbol-scoped inverse:

```json
{
  "schema": "relay.changeset/v1",
  "intent_id": "INT-2026-181",
  "tenant": "acme",
  "project": "api",
  "base_commit": "<current HEAD>",
  "agent": { "identity": "relay-revert:0.1", "model": null },
  "diff": "<unified diff scoped to ICR symbols of the reverted intent>",
  "decision_record": {
    "summary": "surgical revert of INT-2026-042",
    "reverts": "INT-2026-042",
    "reverted_cert_id": "cert-2026-05-30-a3f9b2",
    "method": "icr-scoped-inverse",
    "assumptions": ["adjacent symbols changed after admission are preserved via Fuse"]
  }
}
```

The revert ChangeSet goes through the **same** admission pipeline as any other ChangeSet. The resulting certificate carries a `Reverts:` trailer pointing at the original cert; intent-store records the linkage.

If the ICR-scoped inverse cannot apply cleanly (later changes have replaced the reverted symbols entirely), the command fails with a clear diagnostic and points the operator at `git revert <merge-sha>` as the wider-blast fallback. **Relay never silently widens the revert scope.**

### 16.5 Evidence Replay report (Phase 2A-late / Phase 2B)

`relay cert replay <cert-id>` produces a deterministic comparison report:

```json
{
  "schema": "relay.replay/v1",
  "cert_id": "cert-2026-05-30-a3f9b2",
  "replayed_at": "2026-08-15T10:00:00Z",
  "inputs": {
    "repo_config_sha": "<sha>",
    "effective_config_hash": "sha256:...",
    "toolchain_image": "<oci-digest>"
  },
  "stage_results": [
    {
      "stage": "secret_scan",
      "original": { "passed": true, "findings_count": 0 },
      "replayed": { "passed": true, "findings_count": 0 },
      "match": true
    }
  ],
  "verdict": "byte_reproducible",
  "drift": []
}
```

Verdict values: `byte_reproducible` (all stages match), `tool_drift` (a pinned tool produced different output — investigate toolchain image integrity), `config_drift` (resolved config hash does not match the certificate's — refuse to verify), `unrecoverable` (the original ChangeSet, repo, or toolchain image is no longer fetchable — explicit failure, not a silent pass).

### 16.6 Agent Scorecard report (Phase 2B → Phase 3)

Aggregated read over `certifications` + `events` + `intent_costs` (Phase 2B):

```json
{
  "schema": "relay.scorecard/v1",
  "scope": { "tenant": "acme", "since": "2026-04-01", "until": "2026-06-30" },
  "by_agent": [
    {
      "agent": "claude-code", "model": "claude-sonnet-4-6",
      "intents_attempted": 1840,
      "intents_admitted": 1494,
      "first_pass_certification_rate": 0.62,
      "avg_check_cycles_to_pass": 1.7,
      "failure_breakdown": { "sast": 0.31, "coverage": 0.27, "secrets": 0.02, "size": 0.18, "policy": 0.22 },
      "cost_per_admitted_intent_usd": 0.84,
      "post_admission_defect_rate": 0.013
    }
  ]
}
```

Exposed as a dashboard view, a Prometheus exporter, and `relay scorecard --format csv` for procurement. **Post-admission defect rate** is the most important number and is derived by joining `certificates` against incidents tagged with intent IDs — a closed loop only meaningful in a deployment with a real incident-tracking integration (Phase 2B+).

### 16.7 Policy Marketplace profile schema (Phase 2A → 2B)

A marketplace profile is a versioned, signed bundle:

```yaml
# profiles/soc2-go-api/profile.yaml
schema: relay.profile/v1
id: soc2-go-api
version: 1.4.0
description: "SOC 2-aligned baseline for Go HTTP APIs"
authors: ["acme-platform", "community"]
signature: "ed25519:..."           # signed by marketplace maintainers
applies_to:
  stacks: ["go-microservice"]
includes:
  - ".relay/policies/path-policy.yaml"
  - ".relay/policies/secrets.yaml"
  - ".relay/policies/coverage.yaml"
  - ".relay/rulesets/owasp-api-top-ten.yaml"
locks:
  - "policies.secrets"             # repos using this profile cannot disable secret scanning
```

`relay init --profile=soc2-go-api` fetches the profile, verifies the signature, lays the files into `.relay/`, and records the profile + version in `.relay/relay.yaml` under `profiles: [...]`. Subsequent `relay update profiles` upgrades pinned profiles to the latest compatible version with a diff preview.

The marketplace itself is just a git repo (initially `grove-suite/relay-profiles`). No new service.
