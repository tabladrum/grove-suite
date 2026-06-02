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

### 7.6 Stage 2: Static Analysis Suite (Hybrid — Lean Default + Opt-In SonarLint)

Stage 2 is fully self-contained — no external server required at runtime. All tools run as subprocesses against the ChangeSet's changed files only. Relay owns the quality gate decision: it aggregates findings from all tools, applies per-severity thresholds from project policy, and produces a single pass/fail verdict with a structured finding list.

The "new code only" scoping is natural here — Relay already knows the changed file set from the ChangeSet diff and passes it explicitly to each tool (via `--include` flags or an explicit file list).

#### 7.6.1 Lean default tool surface (Phase 2A, ~50MB no JRE)

**Security analysis tools:**

| Tool | Purpose | Invocation |
|------|---------|------------|
| `semgrep` | SAST — vulnerability and bug patterns | `semgrep scan --json --config=<rulesets> <changed-files>` |
| `gitleaks` | Secret detection | `gitleaks detect --source=. --report-format=json` |
| `govulncheck` | Go dependency CVEs | `govulncheck -json ./...` |
| `npm audit` | Node dependency CVEs | `npm audit --json` |
| `pip-audit` | Python dependency CVEs | `pip-audit --format=json` |

Default semgrep rulesets: `[p/security-audit, p/owasp-top-ten]`, plus language-specific packs.

**Code quality tools (language-specific):**

| Language | Tool | What it catches |
|----------|------|----------------|
| Go | `golangci-lint` | bugs, complexity, unused code, style |
| TypeScript/JS | `eslint` | bugs, best practices, complexity |
| Python | `ruff` | bugs, style, complexity, import hygiene |
| Java | `checkstyle` + `pmd` | style, design issues, duplicate code |

Each tool runs only against files touched by the ChangeSet. Results are merged into a unified findings list with normalized severity: `error | warning | info`.

Tool versions are pinned in the Relay binary release's tool-set manifest. `relay tools install` fetches the manifest's versions. Deviations are recorded in the certificate trailer as `Toolchain-Image-Drift: true`.

#### 7.6.2 Opt-in SonarLint Core engine (Phase 2B, ~200–400MB +JRE)

For teams that want actual SonarQube-rule fidelity locally — not semgrep's reimplementations — `relay tools install --with-sonar` adds the SonarLint Core stack:

- Eclipse Temurin JRE 21 (bundled per platform: macOS Intel/ARM, Linux x86_64/ARM64)
- SonarLint Core library (LGPL-3.0) + analyzer JARs for the project's declared languages (also LGPL-3.0)
- `relay-sonar.jar` — a thin Java wrapper around `StandaloneSonarLintEngine`, released by Relay's Goreleaser pipeline (sibling repo `grove-suite/relay-sonar`, LGPL-3.0)

Invocation flow at analyze time:

```
relay engine
   ├─ semgrep scan ... → findings.semgrep.json
   └─ java -jar relay-sonar.jar \
          --profile <.relay/rulesets/acme-java.xml> \
          --files <changed-files> \
          --out findings.sonar.json
relay engine aggregates + de-duplicates overlapping rules
```

Certificate trailer records `Sonar-Engine: sonarlint-core@<version>` or `none` (Phase 2A, semgrep only). The presence of the engine is auditable.

**Known limitations** (LGPL Community Edition, not Relay-imposed; documented in `getting-started.md`):
- No taint / injection vulnerability analysis (requires SonarQube Server Enterprise Edition).
- No COBOL, Apex, PL/SQL, T-SQL (commercial editions only).
- No cross-project dashboards / PR decoration (those are server-side SonarQube products, not analyzers).

Per-language host requirements inherited from SonarLint: Node.js for JS/TS, `compile_commands.json` for C/C++, .NET SDK for C#.

Rationale and architecture: [`sonarqube-no-server-investigation.md`](sonarqube-no-server-investigation.md).

#### 7.6.3 Quality gate logic (Relay-owned, engine-agnostic)

```
deny if: count(findings where severity >= deny_severity) > deny_on_findings_above
```

Defaults: `deny_severity: error`, `deny_on_findings_above: 0`. Zero new `error`-level findings in changed code is the bar. The gate decision is the same regardless of which engine produced the findings.

#### 7.6.4 Optional: enrichment via existing SonarQube server

Enterprises that already run SonarQube can configure Relay to submit analysis results to their server for historical tracking and the SonarQube dashboard. This is enrichment only — the gate decision always belongs to Relay, and the analysis still runs locally (no round-trip to the server for the verdict).

### 7.7 SonarQube Profile Import (Hybrid — Surface in 2A, Engine in 2B)

Teams with customized SonarQube quality profiles (tuned rules, severity overrides, custom quality gates) can import their profile so Relay enforces the same rules during local certification. This eliminates divergence between Relay admission and any downstream SonarQube check.

**Phase 2A ships the importer surface**: lands the profile XML into `.relay/rulesets/` and registers it in `.relay/relay.yaml`. **Phase 2B ships the engine** that evaluates the imported profile via SonarLint Core + bundled JRE + analyzer JARs (see §7.6.2 and [`sonarqube-no-server-investigation.md`](sonarqube-no-server-investigation.md)).

The earlier draft proposed an SQ-key → semgrep-rule-ID mapping table. **That approach is abandoned** — it is lossy (semgrep covers ~2,400 community rules vs SonarQube's ~6,500) and brittle. The new approach preserves the SonarLint-native XML verbatim for the Phase 2B engine to consume directly.

**CLI command (Phase 2A):**

```bash
relay import sonarqube-profile ./acme-java-profile.xml \
  [--output .relay/rulesets/acme-java.xml] \
  [--name acme-java]
```

Export the profile XML from SonarQube via:
`GET /api/qualityprofiles/export?language=java&qualityProfile=Acme+Java`
or via the UI's "Back up" action on the quality profile.

**Import process (Phase 2A — surface only):**

1. Validate the XML is a recognized SonarQube backup format (top-level `<profile>` element, language attribute, rule list).
2. Write the XML **verbatim** to `.relay/rulesets/<name>.xml`. The repo now owns it; PR review applies. The file travels with the code.
3. Update `.relay/relay.yaml` to reference the imported ruleset:
   ```yaml
   sonar:
     profile: rulesets/acme-java.xml
     # engine_pin: sonarlint-core@10.x   # filled in by `relay tools install --with-sonar`
   ```
4. Print one of two messages:
   - If `--with-sonar` is already installed (Phase 2B available locally): "Profile imported. Will be applied on next `relay check` / `relay certify`."
   - If `--with-sonar` is not installed: "Profile imported. Run `relay tools install --with-sonar` to enable evaluation."

The `.relay/rulesets/acme-java.xml` file is the canonical home for imported profiles — committed to the repo, identical on laptop and team server, replayable in the audit trail via `Effective-Config-Hash`.

**Phase 2B activation:** once `relay tools install --with-sonar` is run, the engine path picks up the profile from `.relay/rulesets/<name>.xml` and SonarLint Core evaluates it at analyze time. No further import step is required.

Re-run `relay import sonarqube-profile` when the upstream profile changes on the SonarQube server — Relay overwrites the local `.xml` and updates the file's `Effective-Config-Hash` contribution.

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
- **Mode-defaulted behavior** (per the laptop-MVP audit): on laptop, the coverage gate defaults to `mode: warn` because Grove's `tests`-edge inference may be sparse on a freshly-indexed TypeScript or Python repo and risks false-positive denies. On team mode, the gate defaults to `mode: enforce`. Both modes are overridden by an explicit `.relay/policies/coverage.yaml`.

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
      coverage:    { mode: warn, threshold: 1.00, allow_no_test_required_annotation: true }   # mode default: warn on laptop, enforce on team

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
      sonar:
        # profile: rulesets/acme-java.xml   # set by `relay import sonarqube-profile` (Phase 2A surface)
        # engine_pin: sonarlint-core@10.x   # set by `relay tools install --with-sonar` (Phase 2B)

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

### 12.5.1 Laptop mode (full Relay server, single user)

```yaml
# ~/.relay/config.yaml
mode: laptop
daemon:
  socket: ~/.relay/daemon.sock     # Unix domain socket; auto-started, persists as launchd / systemd --user
  daemon_state: ~/.relay/daemon.sqlite
store:
  type: sqlite
  per_repo_path: .relay/.cache/state.sqlite   # gitignored
intent_store:
  type: local_git
  path: ~/.relay/intent-store
grove:
  url: http://localhost:7777        # auto-started if unreachable
tools:
  manifest_pin: ${RELAY_VERSION}    # tool-set manifest pinned to the binary release
  with_sonar: false                 # Phase 2B: set to true via `relay tools install --with-sonar`
signer:
  type: local
  key_path: ~/.relay/keys/admission.ed25519   # mode 0600
admission:
  default_target: current_branch    # team mode default: relay-main
telemetry:
  otel_enabled: false               # laptop default: off; opt-in by setting an endpoint
  phone_home: false                 # never on laptop
```

**The laptop binary is the full Relay server, running for a single user.** Pipeline (ingest → ICR → policy → certification → admission → signing → intent-store → audit) is identical to team mode. The substitutions:

- Embedded SQLite (`modernc.org/sqlite`, pure Go — no CGO conflict with Grove's tree-sitter). Per-repo operational state at `.relay/.cache/state.sqlite` (gitignored); daemon-level coordination at `~/.relay/daemon.sqlite`.
- Long-running `relay daemon` owns SQLite, the local Grove client, the intent-store git repo, and the signer key. MCP stdio shims (per IDE), CLI calls, and the git pre-push hook all proxy to the daemon over the Unix socket — single writer guarantee under concurrent invocations.
- No Redis. The daemon is the only writer; ICR locks are irrelevant in the single-agent case.
- Intent-store is a local bare git repo at `~/.relay/intent-store/`. Certificates committed there.
- Cert signing uses a locally generated Ed25519 key at `~/.relay/keys/admission.ed25519` (mode 0600). `relay keys gen` is invoked by `relay init`. `relay keys export` / `relay keys import` for cross-machine portability. Trust model: the developer trusts their own key (TOFU); when they upgrade to team mode, the team key registry replaces TOFU.
- Default admission target = current branch (not `relay-main`). A solo dev's `relay certify` commits to the branch they're on.
- Bundled tool binaries (lean default): shipped in the install tarball or fetched on first run from a content-addressed mirror at `~/.relay/tools/<version>/`. Tool-set manifest pinned to the Relay release.
- `relay tools install --with-sonar` (Phase 2B) additionally fetches Eclipse Temurin JRE 21 + SonarLint Core + analyzer JARs + the `relay-sonar.jar` wrapper to `~/.relay/tools/sonar/<version>/`.
- Telemetry off by default. No phone-home. Documented in `getting-started.md`.
- Cross-platform support in Phase 2A: macOS Intel + ARM, Linux x86_64 + ARM64. macOS binary is Developer-ID signed and notarized. Windows = WSL.

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

- **Laptop mode:** stdio shim per IDE → Unix domain socket → long-running `relay daemon` (single writer). Zero network setup. MCP client auto-registration via `relay mcp install-for {claude-code,cursor,continue,windsurf}`.
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

#### Auto-Intent Capture tools (Phase 2A, new) — `relay_intent_open` / `_update` / `_close` / `_list`

These four tools turn the user's natural-language prompt into a first-class artifact committed alongside the code. See §17 (Auto-Intent Capture) for the full flow.

```jsonc
{
  "name": "relay_intent_open",
  "description": "Draft an Intent from the user's prompt BEFORE making code changes. Stores a draft YAML at .relay/.cache/intents/INT-{id}.draft.yaml. Returns the intent ID. The recommended agent system prompt instructs calling this tool first whenever a user requests a code change.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "title":        { "type": "string", "description": "Short title from the user's request (≤80 chars)." },
      "description":  { "type": "string", "description": "The verbatim user prompt. The most important field." },
      "originated_from": {
        "type": "object",
        "properties": {
          "agent":            { "type": "string", "description": "e.g., claude-code:1.4.2" },
          "model":            { "type": "string", "description": "e.g., claude-sonnet-4-6:2026-04-15" },
          "conversation_ts":  { "type": "string", "format": "date-time" }
        }
      },
      "allowed_paths_hint": { "type": "array", "items": {"type": "string"} },
      "acceptance_criteria_hint": { "type": "array", "items": {"type": "string"} }
    },
    "required": ["title", "description"]
  }
}
```

**Result:** `{ "intent_id": "INT-2026-05-30-rate-limiting", "draft_path": ".relay/.cache/intents/INT-2026-05-30-rate-limiting.draft.yaml" }`

```jsonc
{
  "name": "relay_intent_update",
  "description": "Refine title / description / acceptance criteria mid-session on an open intent draft.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "intent_id": { "type": "string" },
      "patch":     { "type": "object", "description": "Partial Intent YAML fields to merge into the draft." }
    },
    "required": ["intent_id", "patch"]
  }
}
```

**Result:** updated draft path.

```jsonc
{
  "name": "relay_intent_close",
  "description": "Promote the intent draft to .relay/intents/{id}.yaml (committed). Called when the agent reports complete. Emits the commit-trailer block for the eventual relay_certify commit.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "intent_id": { "type": "string" }
    },
    "required": ["intent_id"]
  }
}
```

**Result:** `{ "committed_path": ".relay/intents/INT-2026-05-30-rate-limiting.yaml", "intent_hash": "sha256:...", "trailer_block": "Intent-ID: INT-2026-05-30-rate-limiting\nIntent-Hash: sha256:..." }`

```jsonc
{
  "name": "relay_intent_list",
  "description": "List open and committed intents for this repo.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "status": { "type": "string", "enum": ["draft", "committed", "all"], "default": "all" }
    }
  }
}
```

**Result:** `[{ "id": "...", "title": "...", "status": "draft|committed", "created_at": "...", "file_path": "..." }, ...]`

### 13.3 Recommended Agent System-Prompt Pattern

Bundled with the Relay install (`docs/agent-prompt.md`) is a system-prompt fragment for agents:

```text
This project is governed by Relay.

When the user asks for a code change:

1. BEFORE writing any code, call relay_intent_open with:
     - title: a short version of the user's request
     - description: the verbatim user prompt
     - originated_from: { agent, model, conversation_ts }
   Save the returned intent_id.

2. Call relay_policy once at session start to load the active rules.

3. Make code changes, then call relay_check.

4. If relay_check returns findings:
     - If finding.class == "infrastructure_error": surface to the user.
       Do NOT auto-fix (it is a Relay/Grove/tool problem, not a code problem).
     - Otherwise, fix the findings and re-call relay_check. Loop until clean.
       Do not report success while error-severity findings remain.

5. When the implementation is complete:
     - Call relay_intent_close with the intent_id (promotes the draft for commit).
     - Call relay_certify --intent <intent_id> (laptop) or
       relay_submit (team/company) to produce the signed certificate.
```

This is what makes Relay an agent *companion* rather than an agent *gate*. The agent self-corrects in-loop; the user's original prompt becomes a committed artifact; humans only see code that has already passed the engine.

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
relay init [--stack=go-microservice|node-api|python-service|java-spring]
           [--profile=<marketplace-profile>]
           [--mode laptop|team|company]      # writes ~/.relay/config.yaml, scaffolds .relay/,
                                             # generates Ed25519 key (laptop), starts daemon

# Daemon control (laptop)
relay daemon start | stop | restart | status
                                             # long-running local daemon; auto-started by `relay init`
                                             # and by first MCP/CLI/hook invocation; persists as
                                             # launchd (macOS) / systemd --user (Linux)

# MCP client registration (laptop)
relay mcp install-for {claude-code|cursor|continue|windsurf}     # idempotent
relay mcp install-for {claude-code|cursor|continue|windsurf} --uninstall
relay mcp serve [--stdio|--http]             # advanced: start MCP server manually

# Tenant / project lifecycle (team / company mode)
relay tenant init <name>
relay repo add <project> --url <git-url>
relay project add <name> --repo <repo> --path <subdir>

# Engine commands (same engine as MCP tools)
relay check [--diff <file>] [--base <sha>] [--gates <list>]   # = relay_check (fast in-loop)
relay certify [--intent <id>]                                  # = relay_certify (full pipeline)
relay submit                                                   # extract diff + submit (= relay_submit)
relay submit --diff <file> --base <sha>
relay policy show [--project <name>]                           # = relay_policy
relay explain --rule <rule-id>                                 # = relay_explain

# Auto-Intent Capture (mirrors the MCP tools for human/scripting use)
relay intent open --title <t> --description <d>                # = relay_intent_open
relay intent update <id> --patch <yaml-fragment>               # = relay_intent_update
relay intent close <id>                                        # = relay_intent_close (promote draft → committed)
relay intent get <id>
relay intent list [--status draft|committed|all]               # = relay_intent_list

# Cert + audit
relay cert show <cert-id-or-ref>                               # human-readable AI Code Passport
relay cert verify <cert-id>                                    # offline verify against trusted key bundle
relay cert replay <cert-id> [--out <report.json>]              # byte-reproducible audit replay
relay audit query --since <ts> --agent <name>

# Key management (laptop)
relay keys gen                                                  # generate ~/.relay/keys/admission.ed25519 (mode 0600)
relay keys export [--out <bundle.tgz>] [--passphrase]           # portable export for cross-machine use
relay keys import <bundle.tgz> [--passphrase]
relay keys fingerprint                                          # print fingerprint for trust establishment

# Tooling + profile import
relay tools install                                             # lean default: semgrep, gitleaks, etc. (~50MB, no JRE)
relay tools install --with-sonar                                # Phase 2B: JRE 21 + SonarLint Core + analyzer JARs (~+300MB)
relay tools status                                              # show installed versions + drift from manifest
relay import sonarqube-profile <profile.xml>                    # writes .relay/rulesets/<name>.xml (verbatim);
                                                                # auto-suggests --with-sonar if engine not installed
                                                                # [--output .relay/rulesets/<name>.xml] [--name <name>]

# Git hook installation
relay hook install [--pre-push]                                 # writes .git/hooks/pre-push (proxies to daemon on laptop)
relay hook uninstall

# State migration
relay migrate sqlite-to-postgres --dsn <postgres-dsn>           # laptop → team upgrade
                                                                # `.relay/` is unchanged across the upgrade

# Signature capabilities
relay revert --intent <intent-id> [--dry-run]                   # symbol-scoped surgical revert (Phase 2B)
relay scorecard --agent <name> [--since <ts>] [--format csv]    # agent/model performance report (Phase 2B+)
relay marketplace search <query>                                # community .relay/ profile lookup
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

---

## 17. Auto-Intent Capture (Phase 2A)

The most consequential new capability falling out of the laptop-MVP audit ([`laptop-mvp-audit.md`](laptop-mvp-audit.md)). Today, every coding agent treats the user's natural-language prompt as ephemeral — it dies with the agent session, never reaches the PR, never reaches the audit trail. Reviewers and auditors see only the resulting diff and have to reverse-engineer the intent.

Relay captures the prompt as a YAML committed alongside the code. The user types nothing extra; the agent does the bookkeeping via four MCP tools.

### 17.1 End-to-end flow

```
1. User → Claude Code: "Add rate limiting to /api/auth/* endpoints, 100 req/min per IP"

2. Claude Code (with the Relay system-prompt fragment from §13.3):
   relay_intent_open(
     title:        "Add rate limiting to /api/auth/* endpoints",
     description:  <verbatim user prompt>,
     originated_from: {
       agent:           "claude-code:1.4.2",
       model:           "claude-sonnet-4-6:2026-04-15",
       conversation_ts: "2026-05-30T14:33:09Z"
     }
   )
   → intent_id: "INT-2026-05-30-rate-limiting"
   → draft written to .relay/.cache/intents/INT-*.draft.yaml (gitignored)

3. Claude Code writes the code. Each pass calls relay_check (fast in-loop):
   - If findings: fix and re-check.
   - If `class: infrastructure_error`: surface to user, do NOT auto-fix.
   - Loop until clean.

4. Claude Code calls relay_intent_close(intent_id).
   → draft promoted to .relay/intents/INT-2026-05-30-rate-limiting.yaml (committed)
   → returns commit-trailer block: "Intent-ID: ...", "Intent-Hash: sha256:..."

5. relay_certify --intent INT-... runs the full pipeline and writes the commit
   with the trailer set (incl. Intent-ID and Intent-Hash).

6. On push to GitHub: PR shows the .relay/intents/INT-*.yaml file in the diff.
   Reviewers see what the agent was ASKED to do, alongside what it DID.

7. Server-side admission (team mode) re-validates the diff against the intent's
   allowed_paths and acceptance_criteria before signing the team-mode cert.
```

### 17.2 Intent YAML v2 schema (Auto-Intent additions)

```yaml
schema: relay.intent/v2
id: INT-2026-05-30-rate-limiting          # globally unique; date-sortable slug
status: committed                          # draft | committed
title: "Add rate limiting to /api/auth/* endpoints"
description: |
  <verbatim user prompt>

originated_from:                           # NEW in v2 (Auto-Intent Capture)
  agent: claude-code:1.4.2
  model: claude-sonnet-4-6:2026-04-15
  conversation_ts: "2026-05-30T14:33:09Z"
  prompt_hash: sha256:abc...               # hash of `description` for integrity

domain: auth
capability: rate_limiting

allowed_paths:   ["internal/auth/**", "tests/auth/**"]
forbidden_paths: ["services/payments/**", "migrations/**"]
acceptance_criteria:
  - "POST /api/auth/login returns 429 after 100 requests from the same IP in a 60-second window"
  - "Rate-limit state survives single-pod restart"

verification_plan:
  must_pass_tests: ["auth_test.go::TestRateLimiter*"]
  must_run_sast: true
  must_check_secrets: true

ambiguity_policy: fail_with_questions      # or proceed_with_default
affected_interfaces: ["POST /api/auth/login", "POST /api/auth/refresh"]
rollback_plan: "revert single commit; rate limiting is feature-flagged off by default"
feature_flag: "rate_limiting_enabled"
observability_expectations:
  - "rate_limit.allowed counter increments"
  - "rate_limit.denied counter increments on 429"
security_considerations: "must not log raw IPs in production"
risk_level: low                             # low|medium|high|critical
related_artifacts:
  - type: adr
    url: "platform-config/adrs/0042-rate-limiting.md"

# Filled in by relay_intent_close:
committed_at: "2026-05-30T14:51:22Z"
committed_by_agent: claude-code:1.4.2
```

### 17.3 Storage discipline

| Stage | Path | Git status |
|-------|------|-----------|
| Draft (during agent session) | `.relay/.cache/intents/INT-*.draft.yaml` | **Gitignored.** Machine-local. |
| Committed (after `relay_intent_close`) | `.relay/intents/INT-*.yaml` | **Committed.** Travels with the repo. PR-reviewable. |

The promotion from draft to committed is atomic: `relay_intent_close` writes the final YAML, computes its SHA-256 (`Intent-Hash`), removes the draft, and returns the commit-trailer block for the eventual `relay_certify` invocation.

### 17.4 Cross-validation against the ChangeSet

At `relay_certify` / `relay_submit` time, the engine cross-validates:

| Check | Pass condition |
|-------|---------------|
| `allowed_paths` | Every file in the diff matches at least one `allowed_paths` glob, or `ambiguity_policy: proceed_with_default` is set. |
| `forbidden_paths` | No file in the diff matches any `forbidden_paths` glob. Hard fail; not overridable by ambiguity policy. |
| `acceptance_criteria` | If `verification_plan.must_pass_tests` is set, the TestRun includes those tests and they all pass. |
| `intent_hash` | The committed YAML's SHA-256 matches `Intent-Hash` in the trailer. Replay-safe. |

If any check fails, `relay_certify` returns a structured finding `class: intent_violation` rather than producing a certificate. The agent's loop then sees the violation and can adjust the diff or call `relay_intent_update` to refine the criteria (with the user's awareness, since this is no longer "do what the prompt said").

### 17.5 Why this is a signature capability

| Existing artifact | What gets persisted about the change |
|-------------------|--------------------------------------|
| Git commit message | What the dev decides to summarize |
| GitHub PR description | Same, manual, often empty |
| Claude Code / Cursor session | Lost on session close |
| Conventional Commits | A type prefix, no semantic content |
| Devin session replay | Vendor-locked URL on Cognition's servers |
| **Relay Intent (`.relay/intents/INT-*.yaml`)** | **The user's actual prompt + agent identity + model + acceptance criteria + originating conversation timestamp — committed in the repo as YAML, PR-reviewable, replayable, auditable forever** |

This is the capability that turns Relay from "another linter / admission gate" into "the canonical record of why each AI-generated commit exists." It is the *prompt-as-artifact* tier in a market that has so far thrown the prompt away.
