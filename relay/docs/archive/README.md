# Relay Archive

Historical documents from earlier iterations of the project, preserved for context. These do not reflect current direction — see [`../product-proposal.md`](../product-proposal.md), [`../architecture.md`](../architecture.md), and [`../implementation-plan.md`](../implementation-plan.md) for the current plan.

| File | Origin | Status |
|------|--------|--------|
| `original-vision.md` | Original "Next-gen CI/CD" master prompt | Superseded by `product-proposal.md` — many design decisions changed (Postgres + git hybrid storage, deferred parallelism, agent-agnostic wedge product, etc.) |
| `original-implementation-plan.md` | Pre-Grove implementation plan with internal CKG service | Superseded — Relay now uses Grove as the code-graph foundation; no internal CKG service |
| `original-roadmap.md` | Pre-Grove roadmap | Superseded by `product-proposal.md` §10 |
| `design-review.md` | Original design question set sent for external review | Superseded — all findings incorporated into `design.md`, `architecture.md`, and `product-proposal.md` |
| `design-review-response.md` | Codex/Claude design review response | Superseded — key recommendations (defer parallelism, ICR as scheduling, intent ambiguity first-class) fully incorporated |
| `design-review-critique.md` | Gemini design review critique | Superseded — key recommendations (same as Codex; additional ICR confidence framing) fully incorporated |

These files were originally hosted at `github.com/tabladrum/relay` as a standalone planning repository. That repo has been merged into this directory; the standalone repo is archived.
