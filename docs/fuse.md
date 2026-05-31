---
title: Fuse
layout: default
nav_order: 5
description: "Fuse — symbol-aware git merge driver. Auto-resolves the conflicts that shouldn't exist."
permalink: /fuse/
---

# Fuse

**Symbol-aware Git merge driver. Auto-resolves the conflicts that shouldn't exist.**
{: .fs-5 .fw-300 }

[Install Fuse](/installation){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View source](https://github.com/tabladrum/grove-suite/tree/main/fuse){: .btn .fs-5 .mb-4 .mb-md-0 }

---

You're running multiple AI agents in parallel. Or an agent and your human developers. They all commit to the same files.

Git sees lines. It doesn't know your agent changed `Login()` while your developer changed `validatePassword()` in the same file. They're different functions — structurally independent — but they happen to occupy adjacent lines. Git declares a conflict. A developer stops, opens a merge tool, manually resolves something that was never actually conflicting, and goes back to work.

Multiply that by a hundred agent PRs a week.

Fuse replaces git's line-level merge with a symbol-aware one. It parses all three versions of a file with Tree-sitter, extracts symbols, queries [Grove](/grove) for cross-file blast radius, and merges at symbol granularity. Two changes to different symbols never conflict, regardless of where they appear in the file. The ones that *are* genuinely ambiguous get conflict markers plus an AI-ready handoff prompt — all the context an agent needs to resolve them in one pass.

---

## How It Works

```
git merge <branch>
     │  .gitattributes: *.go merge=fuse
     ▼
fuse merge %O %A %B %P
     │
     ├──► Grove: /impact  (blast radius of changed symbols)
     ├──► Grove: /deps    (cross-file dependencies)
     │
     ▼
┌────────────────────────────────────────────────────────┐
│  IntelliMerge — 7-phase pipeline                       │
│  1. Context building        (Grove API calls)          │
│  2. Symbol extraction       (Tree-sitter, in-memory)   │
│  3. Recency analysis        (git log weighting)        │
│  4. Project graph context   (cross-file edges)         │
│  5. Breaking change detect  (signature diff)           │
│  6. Conflict classification (5 categories)             │
│  7. Strategy selection      (5 strategies)             │
└─────────────────────────┬──────────────────────────────┘
                          │
              ┌───────────┴────────────┐
              ▼                        ▼
      Auto-resolved             Unresolvable
      Write merged file   Conflict markers +
      Exit 0              .git/fuse/conflict-<sha>.md
                          (structured AI handoff)
                          Exit 1
```

---

## Conflict Classification

Fuse classifies every conflict before choosing a resolution strategy:

| Class | Description | Typical auto-resolution |
|-------|------------|------------------------|
| `INCREMENTAL` | Additive changes to different parts of a symbol | ~85% |
| `STRUCTURAL` | One branch renamed or moved a symbol | ~60% |
| `CONFIGURATIONAL` | Changes to config/dependency files | ~80% |
| `ARCHITECTURAL` | Cross-file interface or API change | Handoff |
| `COMPLEX` | Interleaved logic changes | Handoff |

---

## Merge Strategies

| Strategy | Confidence | When used |
|----------|-----------|-----------|
| Symbol | ≥ 85% | Distinct symbol changes |
| Import | ≥ 90% | Import statement differences |
| Config | ≥ 80% | JSON / YAML / TOML structure merge |
| Line | 60–70% | Structural changes (LCS fallback) |
| Handoff | < 30% | Complex / architectural conflicts |

---

## AI Handoff (when Fuse can't auto-resolve)

When Fuse cannot resolve a conflict confidently, it writes a structured prompt to `.git/fuse/conflict-<hash>.md`. The prompt includes:

- All three versions of the conflicting region (base, ours, theirs)
- Symbol signatures from all three versions
- Grove's blast radius — what other symbols reference the changed symbol
- Grove's breaking-change analysis — what callers would break under each version
- A suggested resolution approach based on conflict classification

Feed this file to any AI agent to resolve the conflict with full context.

---

## Fuse vs AI-only merge resolvers

AI-only merge tools (Continue's resolve-conflict, Cody's smart merge, various agent CLIs) send the conflict region to an LLM and use the response as the resolution. This works — sometimes — but has tradeoffs:

| Concern | AI-only merge | Fuse |
|---------|--------------|------|
| Deterministic | No | Yes |
| Reviewable | Hard (LLM judgment) | Easy (named strategies + confidence) |
| Auditable | Difficult | Yes (`.git/fuse/audit.json`) |
| Cost | API tokens per conflict | Zero per conflict |
| Wrong resolutions | LLM can confidently produce broken code | Fuse falls back to conflict markers |
| Speed | Slow (network roundtrip) | Local, sub-second |
| Cross-file blast radius | Depends on context window | Always, via Grove |

**Fuse uses a deterministic algorithm first, escalates to AI only when ambiguity is real.**

[Full comparison →](/comparisons#fuse-vs-other-merge-tools)

---

## Languages Supported

Same languages as [Grove's parser](/grove#languages-supported):

Go · TypeScript · TSX · JavaScript · Python · Java · Rust · C · C++ · C# · PHP

Config files (JSON, YAML, TOML) use a structural diff instead of Tree-sitter.

---

## Quick Start

```bash
# Install Grove first, then Fuse
cd grove-suite/grove && make install && cd ..
cd grove-suite/fuse  && make install && cd ..

# Register fuse as the git merge driver globally
fuse install

# Per-repository — tell git which file types to merge with fuse
cd /your/repo
cat >> .gitattributes <<EOF
*.go   merge=fuse
*.ts   merge=fuse
*.tsx  merge=fuse
*.py   merge=fuse
*.java merge=fuse
*.rs   merge=fuse
*.cs   merge=fuse
EOF
git add .gitattributes && git commit -m "Add Fuse merge driver"
```

That's it. Your next `git merge` will use Fuse for any file matching the `.gitattributes` patterns. False conflicts disappear.

[Full installation guide →](/installation)

---

## Audit Log

Every merge decision is appended to `.git/fuse/audit.json`:

```json
{
  "timestamp": "2026-05-28T14:23:01Z",
  "file": "internal/auth/login.go",
  "class": "INCREMENTAL",
  "strategy": "Symbol",
  "confidence": 0.92,
  "resolved": true,
  "symbols_merged": ["Login", "validatePassword"]
}
```

This audit log is local-only; nothing leaves your machine.

---

## Performance

Per-file merge target: **< 1 second**.

Phases 1–2 (Grove API + Tree-sitter parse) typically take 100–200 ms; phases 3–7 (classification + strategy selection) add 50–100 ms; the remainder is symbol-level diff and write. On a 10,000-line file, end-to-end is around 500 ms.

---

## Read More

- [How Fuse compares to git merge, IntelliMerge, Plastic SCM, AI-only resolvers](/comparisons#fuse-vs-other-merge-tools)
- [Why Grove Suite exists](/why)
- [Full reference on GitHub](https://github.com/tabladrum/grove-suite/tree/main/fuse)
- [Troubleshooting](/troubleshooting#fuse)
