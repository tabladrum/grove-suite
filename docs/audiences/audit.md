---
title: Audit Brief
layout: default
nav_order: 4
description: "How Grove Suite supports auditability, evidence collection, and replayable commit history."
permalink: /audiences/audit/
---

# Audit Brief

Grove Suite is designed to make AI-generated changes auditable after the fact, not just reviewable at the pull request stage.

## What You Get

- The original user prompt captured as a committed YAML intent
- An Ed25519-signed certificate for each admitted commit
- Replayable evidence for build, test, coverage, secrets, SAST, and dependency checks
- A commit trailer that links the code change to the intent and certificate

## Why It Matters

For audit and compliance work, the key question is not just "did the code merge?" It is "what exactly was asked, what ran, and what passed?" Relay preserves that chain so teams can reconstruct the evidence months later.

## Related Pages

- [Security brief]({{ '/audiences/security/' | relative_url }})
- [FAQ]({{ '/faq/' | relative_url }})
- [Relay README]({{ '/relay/' | relative_url }})