---
title: How It Works
layout: default
nav_order: 3
description: "The Provasign loop: capture the intent, gate in the agent's loop, certify with Ed25519, replay the evidence forever."
permalink: /how-it-works/
---

# How It Works
{: .no_toc }

Provasign moves the quality gate **into the agent's loop** instead of leaving it at the end of the pipeline, and it turns every admitted change into a cryptographically verifiable record.

1. TOC
{:toc}

---

## The loop that wastes everyone's time

Here is AI-assisted development as it works today:

1. Developer prompts the agent
2. Agent codes, writes tests, opens a PR
3. CI runs — catches a security finding, a coverage gap, a lint violation
4. Developer relays the CI output back to the agent
5. Agent fixes it, updates the PR
6. CI runs again — a different finding
7. Repeat until green
8. A human reviews the diff — with no idea what the original prompt was

The gates live at the **end** of the pipeline; the agent is at the **beginning**. By the time the agent sees a finding, it has lost the context that produced the code.

---

## The Provasign loop

```
Prompt
  │
  ▼
① Intent captured as YAML  ──────────────►  .provasign/intents/INT-….yaml
  │                                          (the verbatim prompt + agent/model identity)
  ▼
② Agent writes code
  │
  ▼
③ provasign_check   (in-loop, sub-10s)
  │   ├─ SAST on changed files
  │   ├─ Grove-selected affected unit tests
  │   └─ structured findings → agent self-corrects
  │
  │  (agent satisfied)
  ▼
④ provasign_certify  (full suite at commit time)
  │   ├─ Stage 1: build + tests + coverage-of-changed-symbols
  │   ├─ Stage 2: secrets · SAST · deps · linters
  │   ├─ policy gates: path·secrets·fileclass·deps·size·coverage
  │   └─ risk heatmap (versioned)
  │
  ▼
⑤ Ed25519-signed admission  ─────────────►  linear commit + certificate
  │                                          trailers: Intent-ID · Certificate-ID
  │                                          ICR-Hash · Signed-By · Signature
  ▼
⑥ provasign cert replay <id>   (any time later)  →  byte_reproducible / tool_drift / config_drift
```

---

## ① Capture the intent

Before any code changes, the prompt that produced the code is captured as a YAML intent. In a draft it lives at `.provasign/.cache/intents/` (gitignored); when the task closes it is **promoted** to `.provasign/intents/` and committed, so it travels with the repo.

The intent records the verbatim user prompt, a `prompt_hash` for tamper-detection, and the agent and model identity that produced it. The resulting commit carries an `Intent-ID:` and `Intent-Hash:` trailer linking the code back to exactly what was asked.

## ②–③ Gate inside the loop

`provasign_check` runs the fast gates *while the agent is still working*: SAST on the changed files plus the unit tests Grove identifies as affected by the change. It returns structured findings — file, line, rule, severity, fix-hint — in under ten seconds, so the agent self-corrects before it ever opens a PR.

## ④ Certify

When the code is ready, `provasign_certify` runs the full suite in a git-worktree-isolated build:

- **Stage 1** — build + full test run + coverage of the *changed* symbols (measured against Grove's `tests` edges).
- **Stage 2** — secrets (gitleaks + inline), SAST (semgrep, OWASP + your `.provasign/rulesets/`), dependency audit (govulncheck / npm audit / pip-audit), language linters.
- **Policy gates** — `path`, `secrets`, `fileclass`, `deps`, `size`, `coverage` each return `allow` / `warn` / `deny`.
- **Risk heatmap** — a versioned score combining ICR, Stage 2 severity, coverage delta, and touch intensity.

## ⑤ Sign and admit

The certificate is an Ed25519 signature over the canonical bytes of the changeset's signed fields: changeset id, intent id, base SHA, ICR, policy results, effective config hash, policy version, toolchain, signer key id, and timestamp. The **admitted commit SHA is deliberately excluded** from the signed bytes — the certificate is signed *before* admission produces the commit, and the (commit → certificate) mapping is stored separately. The change is then admitted as a linear commit carrying the provenance trailers.

```bash
provasign cert show HEAD
# Certificate-ID: provasign-cert-abc123
# Intent-ID:      INT-2026-06-01-…
# Agent / Model:  claude-code / claude-opus-4-8
# Stage1:         PASS (42 tests, 87.3% coverage)
# Stage2:         PASS (0 HIGH, 1 MEDIUM suppressed by policy)
# Risk:           0.12 (low) — model v1
# Signed-By:      local-ed25519:9f3a…
```

## ⑥ Replay the evidence

`provasign cert replay <id>` re-runs the gates against the recorded changeset and classifies any divergence:

| Verdict | Meaning |
|---|---|
| `byte_reproducible` | Same config hash, same gate verdicts — the certificate still holds |
| `tool_drift` | Same config, but an analyzer changed its verdict — your tooling moved |
| `config_drift` | The effective `.provasign/` config hash differs from the certificate's |
| `unrecoverable` | The recorded changeset/state can't be reconstructed |

The audit trail is **cryptographic, not narrative**: what was asked, what ran, what passed — verifiable months later.

> Replay and signature verification run where the certificate store and key live. In laptop mode that's the originating machine; verifying on a *different* machine (auditor, CI, reviewer) is a [server-mode]({{ '/architecture/#deployment-modes' | relative_url }}) capability (roadmap).

---

## Read more

- [Architecture]({{ '/architecture/' | relative_url }}) — the embedded Grove engine that powers impact and test selection
- [Features]({{ '/features/' | relative_url }}) — the full capability and gate reference
- [Use cases]({{ '/use-cases/' | relative_url }}) — security, audit, change management, traceability
