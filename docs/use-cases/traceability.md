---
title: Traceability
layout: default
parent: Use Cases
nav_order: 4
description: "The unbroken chain Provasign maintains from natural-language prompt to admitted commit."
permalink: /use-cases/traceability/
---

# Traceability

Traceability is the unbroken chain from *what was asked* to *what shipped*. For AI-generated code that chain normally breaks the moment the agent session ends — the prompt is gone, and the diff is all that's left. Provasign keeps the chain intact.

## The chain

```
User prompt
   │  captured verbatim
   ▼
Intent YAML  (.provasign/intents/INT-….yaml)
   │  prompt_hash · agent · model · acceptance criteria
   ▼
ChangeSet  (the diff the agent produced)
   │  Grove ICR: which symbols, which blast radius
   ▼
Certificate  (Ed25519-signed: changeset · config · toolchain · results)
   │
   ▼
Admitted commit
   trailers:  Intent-ID · Intent-Hash · Certificate-ID · ICR-Hash · Signed-By
```

Every link is addressable. From a commit you can reach its certificate and its intent; from an intent you can find the prompt that created it; from the certificate you can replay the gates.

## What each artifact pins down

| Artifact | Answers |
|---|---|
| Intent YAML | *What was asked?* verbatim prompt, who/what produced it, acceptance criteria |
| `Intent-Hash` | *Was the recorded prompt tampered with?* |
| ICR (from Grove) | *What did it actually change?* symbols + blast radius |
| Certificate | *What verified it?* gates, config, toolchain, results — signed |
| Commit trailers | *Where does it all hang together?* stable, parseable links on the commit |

## Identity granularity

The intent records the agent and model that produced the change (e.g. `claude-opus-4-8`). Recording precise, version-pinned model identity — distinguishing, say, two Opus versions or a different vendor's model — is what makes the chain useful for later analysis; capture it as `vendor/family-version` at intent-open time.

This is the artifact the EU AI Act expects for high-risk activities: a record of what the AI was asked to do, what checked the output, and a proof you can show an auditor.

## Related

- [Audit]({{ '/use-cases/audit/' | relative_url }}) · [Change management]({{ '/use-cases/change-management/' | relative_url }})
- [How It Works]({{ '/how-it-works/' | relative_url }})
