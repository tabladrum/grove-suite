---
title: Change Management
layout: default
parent: Use Cases
nav_order: 3
description: "How Provasign turns every AI-generated change into a gated, signed, intent-linked admission."
permalink: /use-cases/change-management/
---

# Change Management

Change management asks: *was this change authorized, reviewed against policy, and recorded?* When most of the diff is written by agents at high volume, the human PR review that traditionally answered those questions can't keep up. Provasign encodes the policy into the admission step itself.

## Every admission is a controlled change

- **Authorized.** The change is linked to an approved intent — the captured prompt describing what was requested — via the `Intent-ID:` trailer. No intent, no link.
- **Policy-checked.** The `path`, `secrets`, `fileclass`, `deps`, `size`, and `coverage` gates run on every certification. A `deny` blocks admission; the agent gets a structured reason and self-corrects.
- **Recorded.** Admission produces a linear commit plus a signed certificate, so the change record is the commit history itself — not a separate ticket that drifts out of sync.

## Policy as configuration

Gates live in `.provasign/provasign.yaml` and merge with built-in defaults. Each gate can be set to `warn` (report, allow) or `enforce` (block), so you can roll out a policy gradually — start in `warn`, watch the findings, then flip to `enforce` once the noise is gone.

Compliance baselines ship as profiles (`provasign init --profile=soc2-baseline`, `pci-dss-baseline`, stack-strict variants), and the **effective config hash** is part of every certificate — so you can prove which policy was in force when a given change was admitted.

## Separation of duties

The policy is committed to the repo and versioned; changing it is itself a reviewable change. The agent that writes code cannot silently weaken the gates that certify it without that policy change showing up in history.

This maps to SOC 2 CC8.1 (change management).

## Related

- [Audit]({{ '/use-cases/audit/' | relative_url }}) · [Traceability]({{ '/use-cases/traceability/' | relative_url }})
- [Features → Policy gates]({{ '/features/#policy-gates' | relative_url }})
