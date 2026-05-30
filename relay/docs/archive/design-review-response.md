# Relay Design Review Response

## Executive Verdict

Relay is aiming at a real and important problem: AI-generated change volume has already outgrown human PR review as the default safety mechanism. The thesis that humans should review intent and machines should certify implementation is strong.

The weak part is the current claim that ICR isolation can replace branches. ICR should be treated as a scheduling and risk-reduction mechanism, not a correctness proof. It can reduce conflicts, choose safe parallelism, and drive certification scope, but it cannot by itself guarantee semantic independence.

Recommended framing for Phase 2:

1. Build Relay as an intent-to-certified-commit system first.
2. Keep main linear.
3. Use ICR to schedule, lock, and explain risk.
4. Treat Fuse and certification as the actual safety boundary.
5. Defer broad parallelism until the system has observed enough real intent/change pairs to calibrate ICR accuracy.

I would not ship the message “branches serve no purpose” yet. I would ship “Relay makes branches unnecessary for approved classes of agent-delivered work.” That is more defensible and more adoptable.

## Highest-Risk Assumptions

### 1. ICR isolation is not strong enough to be the only concurrency guard

Symbol-level isolation misses several real-world conflict classes:

1. File-level append conflicts: two agents add different functions, imports, routes, migrations, or tests to the same file.
2. Non-symbol assets: JSON/YAML config, SQL migrations, generated code, lockfiles, markdown docs, snapshots, OpenAPI specs.
3. Semantic coupling not visible in the graph: conventions, runtime reflection, dependency injection, string-based routing, env vars, feature flags.
4. Low-confidence edges: a missing or uncertain calls/uses-type edge can cause unsafe parallelism.
5. Temporal coupling: one intent changes behavior that another intent assumes, with no direct symbol overlap.

ICR can be good enough for scheduling only if every lock decision includes confidence, file-level fallbacks, and category-specific rules.

### 2. Certification staleness is the real admission problem

The system does not just need to prove “this ChangeSet passed once.” It needs to prove “this ChangeSet still passes against current HEAD.”

For any non-trivial queue, admission must rebase, re-run a minimum certification slice, and invalidate certificates when relevant dependencies changed.

### 3. Agent ambiguity needs to be first-class

If the agent is forced to choose architectural defaults, Relay will eventually deliver wrong but test-passing code. That is dangerous because it creates false confidence.

Relay needs an explicit ambiguity policy with typed outcomes:

1. proceed_with_default for reversible, low-risk defaults
2. needs_clarification for design-affecting choices
3. split_intent for oversized/multi-domain work
4. blocked_by_policy for unsafe or unsupported work

### 4. Three repos plus Postgres introduces consistency risk

The hybrid model is probably correct, but only if there is a formal consistency pattern. Without an outbox/reconciler, Postgres and git snapshots will drift.

Recommended rule: Postgres is source of truth for mutable operational state; git is append-only audit/certificate artifact. Every state transition writes an event to Postgres and an outbox row. A reconciler writes immutable snapshots to git and marks the outbox row complete.

## Severity-Ranked Defects and Risks

### Critical

1. No clear trust boundary for generated code admission

If a compromised or misaligned agent can produce a ChangeSet that passes tests, Relay may commit it directly to main. Certification must include policy checks beyond tests: restricted file paths, dependency changes, secret scanning, risky API detection, migration policies, and human gate triggers.

2. ICR confidence is not part of the concurrency decision

The design mentions edge confidence but does not define how confidence gates locks. Low-confidence graph regions should force serialization or human review. Without this, Relay can parallelize precisely where the graph is least trustworthy.

3. Redis lock expiry can allow overlapping writes after slow execution

SET NX EX is not enough unless agents renew leases and admission verifies lock ownership with fencing tokens. Otherwise an expired lock can be reacquired by another intent while the first pod is still running.

### High

1. Migration/config/generated-file handling is under-specified

These files are often conflict-prone and not symbol-addressable. They need path-class policies. Examples: migrations always serialize; lockfiles are regenerated during admission; generated files are either forbidden or regenerated in certification.

2. Certificate invalidation is under-specified

Certificates should include base commit, ICR hash, dependency hash, test selection hash, toolchain image digest, policy version, and environment class. Any relevant change should invalidate or downgrade the certificate.

3. Intent groups are deferred, but admission model already depends on them

The design asks about C+D+E stale certification, but Phase 2 says intent groups are deferred. Keep groups out of Phase 2 entirely or implement the full invalidation model. Partial groups will create complex failure modes early.

4. Human review of intent may be too weak without examples and templates

Most users will write vague intents unless the product guides them aggressively. Intent authoring needs linting, examples, and auto-generated clarification questions before approval.

### Medium

1. Claude-only is fine for Phase 2, but model identity must be part of certification

The exact model, prompt template version, tool versions, and context pack hash should be recorded. Otherwise results are not reproducible enough for audit.

2. Git intent store is poor for operational querying

The hybrid Postgres + git snapshot model is right, but it must be explicit. Pure git should not be used as the operational queue.

3. Canary deployment is too much for Phase 2

Canary requires service mesh/runtime metrics/domain-specific SLO configuration. It will distract from proving intent-to-certified-commit. Phase 2 should stop at admitted commit plus optional deployment hook.

4. Decomposition is likely harder than single-agent execution

Union-find over graph components is a useful heuristic, but component boundaries will be wrong early. Build observability around proposed decompositions before letting multiple pods mutate code in parallel.

## Answers to Open Questions

### Q1: Is ICR Isolation Strong Enough to Replace Branches?

No, not as stated.

ICR isolation is strong enough to reduce conflict probability, but not strong enough to eliminate branches or other isolation mechanisms as a universal claim. It should be positioned as a concurrency control heuristic backed by Fuse, rebase, certification, and policy gates.

Failure modes:

1. Same-file structural conflicts outside symbols.
2. Shared imports/routes/registries.
3. Migration and generated files.
4. Dynamic language features and reflection.
5. Missing low-confidence edges.
6. Tests that pass while behavior is semantically wrong.

Recommended design:

1. Lock exclusive symbols.
2. Also lock special file classes.
3. Use file-level locks for files with low parser confidence.
4. Treat low-confidence graph edges as shared/exclusive rather than ignoring them.
5. Always certify against current HEAD before admission.

### Q2: The Agent Gets Stuck — What Is the Right Escalation Path?

Use a policy-driven middle ground.

The agent should classify ambiguity:

1. Low-risk reversible decision: choose a default and record it.
2. Product/design decision: fail with questions.
3. Scope mismatch: split intent or request decomposition.
4. Security/compliance ambiguity: stop and require human approval.

Add an Agent Decision Record to each ChangeSet:

1. decisions made
2. alternatives considered
3. assumptions
4. confidence
5. whether human confirmation was required

This gives speed for routine work while preventing silent architectural drift.

### Q3: Where Does Design Conversation Go?

Relay should not become a chat/ADR product in Phase 2, but intents should be able to reference design records.

Recommended minimal addition:

1. Add related_artifacts to intent YAML.
2. Allow links to ADRs, issues, docs, Slack threads, Claude transcripts, or design docs.
3. Prism should include referenced artifacts in the context pack.

Do not build a full ADR system yet. Make Relay consume design context, not own it.

### Q4: The Admission Ordering Problem

Your current thinking is mostly right, but needs stricter certificate invalidation.

Recommended admission rules:

1. Every certificate is tied to base commit and dependency hash.
2. On admission, rebase onto HEAD.
3. If rebase conflicts, re-execute from HEAD.
4. If rebase is clean but affected dependency hash changed, rerun selected certification stages.
5. If only unrelated files changed, fast certification is enough.
6. For intent groups, admit atomically only if all group members are certified against compatible base/dependency hashes.

For Phase 2, defer intent groups. They multiply complexity.

### Q5: What Is the Right Scope for Phase 2?

Do not build full decomposition + parallel agents yet.

Minimum viable Phase 2:

1. One queued intent executes end-to-end.
2. Grove computes ICR and risk classification.
3. Relay acquires ICR locks, even if concurrency is initially disabled.
4. One pod executes one intent.
5. Prism provides context.
6. ChangeSet is generated.
7. Certification runs.
8. Admission commits to linear main.
9. Audit/certificate is written.

Basic ICR locking should be included, but parallel execution can remain off by default. Record whether two intents would have been safe to run in parallel. Use that data to validate the model before enabling parallelism.

### Q6: Is Git the Right Intent Store?

Pure git is not the right operational intent store. The hybrid is the right call.

Use:

1. Postgres for mutable/queryable workflow state.
2. Git for immutable snapshots, certificates, and audit export.
3. Outbox pattern to prevent drift.
4. Periodic reconciliation job to compare Postgres state and git audit snapshots.

This avoids git query limitations while preserving the audit benefits.

## Answers to Specific Reviewer Questions

### 1. Are there fundamental flaws in the ICR isolation model?

Yes: the model conflates symbol independence with semantic independence. It is useful, but not sufficient. Add file-class locks, confidence thresholds, dependency hashes, and mandatory certification against current HEAD.

### 2. Is the three-repo model operationally sound?

It can be, but it is heavy. The biggest risks are drift and cross-repo transaction boundaries.

Recommendation:

1. Keep source-repo separate.
2. Keep platform-config separate only if multiple source repos share it.
3. Treat intent-store as an audit export, not the live queue, while Postgres remains operational state.

### 3. Is Claude only the right call for Phase 2?

Yes. Multi-provider support would add routing, prompt compatibility, result comparability, and audit complexity too early.

What would change this:

1. A project requires a model unavailable in Claude.
2. Empirical evidence shows another model is significantly better for a task class.
3. Customer procurement requires provider choice.

### 4. What is missing from the intent YAML schema?

Add:

1. risk_level
2. rollback_plan
3. verification_plan
4. related_artifacts
5. affected_interfaces
6. data_migration
7. feature_flag
8. observability_expectations
9. security_considerations
10. owner/reviewer policy
11. allowed_paths and forbidden_paths
12. ambiguity_policy

### 5. Biggest unidentified Phase 2 risk

The biggest hidden risk is not merge conflict. It is wrong successful change.

An agent can satisfy tests and acceptance criteria while violating product intent, architectural direction, security posture, or operational expectations. Relay needs explicit policy gates and richer intent schemas to catch that.

### 6. Infrastructure changes in Phase 2 or code-only?

Mostly code-only for Phase 2.

Allow tightly constrained infrastructure-adjacent files only if policy is strict:

1. non-production config
2. feature flags
3. CI test fixtures
4. local dev manifests

Defer Terraform, production K8s, IAM, networking, and database migrations unless the whole Phase 2 goal is infrastructure automation. These are high-blast-radius domains and require specialized certification.

### 7. Is there a simpler design?

Yes. Simpler Phase 2 design:

1. Intent intake and approval.
2. Grove ICR risk estimate.
3. Serialized agent execution.
4. Prism context delivery.
5. Certification.
6. Linear admission.
7. Audit certificate.

No decomposition, no parallel agents, no Fuse in the critical path initially, no canary, no intent groups.

This still proves the core product thesis: humans review intent; machines certify diff; Relay admits to main.

## Recommended Phase 2 Scope

Ship this:

1. Single-intent execution loop.
2. ICR computation and lock acquisition.
3. ICR confidence/risk report.
4. One agent pod per intent.
5. Prism context pack.
6. ChangeSet metadata with assumptions and decisions.
7. Certification tied to base commit.
8. Rebase and fast re-certification before commit.
9. Linear commit with trailers.
10. Postgres operational state plus git audit snapshots.

Do not ship yet:

1. Parallel decomposition.
2. Intent groups.
3. Canary deployment.
4. Production infrastructure mutation.
5. Multi-provider model routing.

## Suggested Revised Product Claim

Current claim:

> If merge conflicts are solved structurally, branches serve no purpose.

Recommended claim:

> Relay uses code-graph isolation, semantic merge, and machine certification to make branches unnecessary for approved classes of agent-delivered work, while preserving a linear, auditable main history.

This is more credible and avoids overpromising.

## Final Assessment

Relay is a strong and original design, but Phase 2 should be narrower and more evidence-driven. The core insight is valuable: intent should be reviewed by humans and implementation should be certified by machines. The dangerous part is assuming graph isolation is equivalent to semantic safety.

If Phase 2 proves reliable single-intent execution with strong certification and auditability, Relay will have a credible foundation. Parallel agents, Fuse-heavy merging, and branchless-by-default delivery should come after real data shows ICR predictions are trustworthy.
