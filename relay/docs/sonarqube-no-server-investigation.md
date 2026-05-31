# SonarQube Without a Remote Server — Investigation

How to run SonarQube-grade static analysis locally in Relay (laptop mode and team mode) without requiring a SonarQube Server or SonarCloud instance — modelled on what the VS Code "SonarQube for IDE" extension actually does, then translated to a path that fits Relay.

Status: investigation complete. Recommendation at [§5](#5-recommendation-for-relay-hybrid-path). Decision pending.

---

## 1. How SonarQube for IDE (formerly SonarLint) Works

The VS Code extension is the canonical "SonarQube without a server" implementation. Its architecture:

### 1.1 Three tiers

1. **VS Code extension host (TypeScript).** UI, settings, LSP client. This is what ships on the marketplace.
2. **Java Language Server (`sonarlint-ls.jar`).** Spawned as a child process. Speaks Language Server Protocol over stdio with custom `SonarLint/*` extensions. Coordinates multi-language analysis and rule evaluation.
3. **Analyzer plugins (JARs).** One Java JAR per language family, loaded by the language server. Each analyzer is versioned independently.

### 1.2 What's bundled vs fetched

The extension bundles a **JRE** (Java 21 minimum as of v5) for the three major platforms (Windows x86_64, Linux x86_64, macOS Intel + Apple Silicon). Other platforms require a manual JRE install.

The language server JAR (`sonarlint-ls.jar`) is bundled. Many analyzer JARs are also bundled. A few large analyzers — notably the CFamily (C/C++) and C# analyzers — are downloaded on demand on first use and cached at `~/.vscode/extensions/sonarsource.sonarlint_ondemand-analyzers/<analyzer>/<version>/`.

Languages with bundled analyzers include JS/TS/CSS (`sonar-javascript`), Java (`sonar-java`), Python (`sonar-python`), Go (`sonar-go`), PHP, IaC (Terraform / Kubernetes / Docker / CloudFormation / Azure RM), Secrets, HTML, XML.

### 1.3 Standalone vs connected mode

| Mode | Rules used | Server required | Profile sync |
|------|-----------|-----------------|--------------|
| **Standalone** | Built-in default ruleset, plus user overrides via `sonarlint.rules` VS Code setting | No | None |
| **Connected** | Project's active quality profile pulled from a SonarQube Server or SonarCloud | Yes (read-only) | Periodic |

Standalone mode is fully functional offline once the extension is installed (and any on-demand analyzers have been fetched).

### 1.4 What requires connected mode (cannot run standalone)

- COBOL, Apex, PL/SQL, T-SQL — commercial editions + connected mode only.
- **Injection vulnerabilities (taint analysis)** — requires SonarQube Server Enterprise Edition + connected mode. This is the single biggest "you must run a server" capability.

Everything else — code smells, bugs, security hotspots, quality measures — runs in standalone mode.

### 1.5 Per-language host requirements

The extension reuses host toolchains rather than bundling them:

| Language | Required on host |
|----------|-----------------|
| JS / TS / CSS | Node.js (20.12+, 22.11+, or 23/24) |
| Java | "Language Support for Java" VS Code extension (for classpath resolution) |
| C / C++ | `compile_commands.json` |
| C# | .NET SDK |
| Python, Go, IaC, Secrets, HTML, XML | None |

### 1.6 License

- VS Code extension and SonarLint Core library: **LGPL-3.0**. Open source, commercially redistributable with attribution.
- Bundled analyzer JARs: **LGPL-3.0** for the Community Edition analyzers shipped with SonarLint.
- The commercial-edition rules (taint analysis, COBOL, Apex, etc.) are not part of the redistributable bundle.

---

## 2. SonarLint Core — The Embeddable Library

[`SonarSource/sonarlint-core`](https://github.com/SonarSource/sonarlint-core) is the Java library that powers all the SonarLint IDE plugins (VS Code, IntelliJ, Eclipse, Visual Studio). It is open source under **LGPL-3.0** and is designed to be embedded.

Top-level modules:

- `backend/` — the analysis engine
- `client/` — client SDK implementations
- `rpc-protocol/` — LSP and custom-protocol support

The library exposes a `StandaloneSonarLintEngine` API: load analyzer plugins from a directory of JARs, configure the active rule set programmatically, submit a `ClientInputFile` set, receive `Issue` objects back. The IDE plugins are thin wrappers around this engine; the engine itself does not require any IDE.

**Key implication for Relay:** SonarLint Core is the right embedding target. The legacy [`sonarlint-cli`](https://github.com/SonarSource/sonarlint-cli) project was archived on January 11, 2018 ("This repository was archived by the owner on Jan 11, 2018. It is now read-only.") and is **not** suitable for new work. SonarSource's current direction is the embeddable Java library, not a standalone CLI.

---

## 3. Quality Profile XML Format

SonarQube Server, SonarQube Cloud, and SonarQube Community Build all support exporting a custom quality profile to an XML file via the UI ("Back up" action) or the `api/qualityprofiles/backup` HTTP endpoint. The exported XML lists the active rules and their parameter overrides for a single language.

The same XML can be re-imported into another SonarQube instance. It can also be loaded into SonarLint Core via `StandaloneSonarLintEngine.update(...)` with a `QualityProfileImport` — this is the API SonarLint's connected mode uses internally to sync from a server.

This means a Relay user with an existing SonarQube quality profile XML can keep using those exact rules locally, without ever running the server, by passing the XML to a SonarLint-Core-backed engine.

---

## 4. Three Real Paths for Relay

### Path A — Continue with semgrep + community rule packs (status quo)

Today's Relay design uses semgrep with bundled rulesets (`security-audit`, `owasp-top-ten`) and references a `p/sonarqube` semgrep pack. This is the lightest-weight path.

**What it gives:** Real SAST. Cross-language coverage. Zero JRE dependency. ~50MB total install footprint for the SAST tool surface.

**What it doesn't give:**
- Sonar-rule fidelity. Semgrep ships ~2,400 community rules; SonarQube ships ~6,500 rules across analyzers. Coverage overlap is partial.
- Quality profile XML import has no clean target — rules don't map one-to-one. The current Relay spec's "SQ key → semgrep rule ID" mapping table is necessarily incomplete and lossy.
- Specific Sonar-only signals (cognitive complexity, certain Java idioms, framework-aware checks) are not available.

**Verdict:** Acceptable as the default surface. Insufficient for teams who already standardize on SonarQube and want their existing profile to apply locally.

### Path B — Bundle SonarLint Core + analyzer JARs + JRE

Embed the same stack the VS Code extension uses. Install a bundled JRE (Eclipse Temurin, ~50MB compressed per platform), SonarLint Core + the standard analyzer JARs (~150–300MB depending on which languages), and invoke from Go via subprocess.

**Two sub-options for the invocation interface:**

- **B1 — speak LSP to `sonarlint-ls.jar`.** Reuse the existing language-server entry point exactly as VS Code does. Pro: zero custom Java. Con: LSP semantics are file-at-a-time / open-editor-driven; using them in a batch CLI flow is awkward.
- **B2 — write a thin Java wrapper around SonarLint Core's `StandaloneSonarLintEngine` API.** Ship `relay-sonar.jar` as a one-shot CLI: `java -jar relay-sonar.jar --rules profile.xml --files <list> --out findings.json`. Pro: clean batch semantics, simple Go-side parsing. Con: a small Java module to maintain.

B2 is the better long-term shape. The wrapper is small (a few hundred LOC) and lives in a sibling Java repo, released by the same Goreleaser pipeline.

**What this gives:**
- Real Sonar-rule analysis, locally, no server.
- Quality profile XML import works exactly as connected mode does in the IDE.
- Same rules a SonarQube Community Build server would apply.
- Path is fully open source under LGPL-3.0 (compatible with Relay's BSL license at the process boundary).

**What this costs:**
- ~200–400MB additional install footprint when SonarLint is enabled (JRE + Core + analyzers).
- Java 21 dependency. Bundled JRE handles three platforms (macOS Intel/ARM, Linux x86_64); ARM Linux and Windows require the user to install Java.
- Per-language host requirements inherited from SonarLint: Node.js for JS/TS, `compile_commands.json` for C/C++, .NET SDK for C#.
- Cannot do: taint analysis, COBOL/Apex/PL/SQL/T-SQL. These need a commercial SonarQube server. Document this clearly.

**Verdict:** This is the only path that delivers honest SonarQube parity without a server.

### Path C — Hybrid: lean default + opt-in SonarLint when needed

Default `relay tools install` ships path A (semgrep + gitleaks + lang linters; no JRE). When the user runs `relay import sonarqube-profile <profile.xml>` or explicitly opts in via `relay tools install --with-sonar`, Relay fetches the SonarLint Core stack + a portable JRE + the analyzer JARs for languages declared in `.relay/relay.yaml`, stores everything at `~/.relay/tools/sonar/`, and starts using the SonarLint engine in place of (or alongside) semgrep for the relevant languages.

**The toggle is recorded in the cert trailer:**

```
Sonar-Engine: sonarlint-core@10.x  # or "none" when path A is used
```

so the `Effective-Config-Hash` reflects which engine evaluated the code.

**Why this is the right shape:**

- Default install stays small (~50MB) and fast for the 80% case.
- Teams who need Sonar parity opt in with one command.
- No surprise JRE dependency for users who don't ask for it.
- The cert trailer makes the engine choice auditable.
- Path A and Path B are not mutually exclusive — semgrep can keep running for cross-language checks that SonarLint doesn't cover (e.g., `gitleaks` for secrets, custom organization rules in semgrep YAML).

---

## 5. Recommendation for Relay — Hybrid Path

Adopt **Path C (Hybrid)** for Phase 2A.

### 5.1 Default install (laptop and team mode)

`relay tools install` fetches:

- semgrep (with `p/security-audit`, `p/owasp-top-ten`, language-specific rule packs)
- gitleaks
- govulncheck / npm audit / pip-audit (deps)
- golangci-lint, eslint, ruff, checkstyle, pmd (lang linters)

No JRE. No SonarLint. Total ~50MB compressed.

### 5.2 Opt-in SonarLint engine

Triggered by either:

- `relay tools install --with-sonar` (explicit), or
- `relay import sonarqube-profile <profile.xml>` (implicit — importing a SQ profile auto-enables the engine for the languages it covers).

The tooling step fetches, by platform and active languages:

- Eclipse Temurin JRE 21 (~50MB compressed; bundled per platform).
- SonarLint Core + a thin `relay-sonar.jar` wrapper around `StandaloneSonarLintEngine` (~30MB).
- Analyzer JARs for languages declared in `.relay/relay.yaml` (selective; ~10–60MB per language).

Stored under `~/.relay/tools/sonar/<version>/` with the same version-pin discipline as other tools.

### 5.3 Invocation

For each analysis run, Relay's certification engine:

1. Builds the file list (changed files for `relay_check`; full project for `relay_certify`).
2. Calls `java -jar ~/.relay/tools/sonar/.../relay-sonar.jar --rules <profile.xml-or-default.xml> --files <list> --out <json>`.
3. Parses the JSON findings into `relay.findings/v1`.
4. Aggregates with semgrep findings; deduplicates where rules overlap.

### 5.4 Profile import

`relay import sonarqube-profile <profile.xml> [--output .relay/rulesets/<name>.yaml]`

The importer:

1. Validates the XML is a recognized SonarQube backup format.
2. Writes the profile to `.relay/rulesets/<name>.xml` (committed; travels with the repo).
3. Updates `.relay/relay.yaml` to reference the imported ruleset.
4. **Does not** attempt to map SQ rules to semgrep rule IDs (lossy). Instead, registers the profile to be loaded directly by the SonarLint engine.
5. If the SonarLint engine is not yet installed, prints `Run 'relay tools install --with-sonar' to enable.` and exits without error.

### 5.5 Honest limitations to document

| Capability | Available in Relay no-server mode? |
|------------|------------------------------------|
| Sonar Community Edition rules (code smells, bugs, hotspots) | **Yes** |
| Quality profile XML import | **Yes** |
| Taint / injection vulnerability analysis | **No** — requires SonarQube Server Enterprise Edition |
| COBOL, Apex, PL/SQL, T-SQL | **No** — commercial editions only |
| Pull request decoration / Sonar dashboards | **No** — that is a server-side product |
| Aggregated cross-project metrics | **No** — that is a server-side product |

These limitations are inherent to the LGPL Community-Edition rule set, not to Relay's design. Make the gap explicit in the Relay docs so enterprises aren't surprised.

### 5.6 License posture

- SonarLint Core: LGPL-3.0. Redistributable. Process-level isolation (Relay → `java` subprocess) avoids any concern around LGPL's dynamic-linking clauses.
- Bundled analyzer JARs: LGPL-3.0. Same redistribution terms.
- Bundled Eclipse Temurin JRE: GPL-2.0 with classpath exception. Bundleable; ship attribution per the OpenJDK assembly exception.
- Quality profile XML (user-supplied): owned by the user. Relay does not assert any rights.

Relay's own BSL license is unaffected because all SonarLint pieces run in their own JVM process.

### 5.7 Maintenance burden

- The `relay-sonar.jar` wrapper is roughly 300 lines of Java around `StandaloneSonarLintEngine` + a small JSON serializer. Maintained in `grove-suite/relay-sonar` (sibling repo), released by the same Goreleaser pipeline.
- Analyzer JAR versions are pinned per Relay release. Updates ride the Relay release train.
- SonarLint Core itself is actively maintained by SonarSource (the same team shipping the IDE plugins), so the upstream is durable.

---

## 6. Phasing

| Phase | Scope |
|-------|-------|
| **Phase 2A** | Path A default. `relay import sonarqube-profile` lands the importer + records the ruleset in `.relay/rulesets/`. **The SonarLint engine itself is fetched lazily on first import** — no Phase 2A scope to write the engine integration; the import surface alone is shipped. |
| **Phase 2A-late or 2B** | Ship the `relay-sonar.jar` wrapper, the JRE bundling logic, the `relay tools install --with-sonar` flow, and the runtime path that prefers SonarLint over semgrep where overlap exists. This is when "real Sonar parity" becomes a Relay capability. |
| **Phase 4 (enterprise)** | Connected-mode equivalent: Relay can sync a quality profile from a user's SonarQube/SonarCloud account on a schedule (via the official `api/qualityprofiles/backup` endpoint). Optional. Useful for orgs that still treat SonarQube Server as the canonical profile source. |

This phasing keeps Phase 2A's surface area honest — the import command lands now, the engine lands when there's an explicit user pulling for it.

---

## 7. What This Resolves on the Audit

Section [§7 of `laptop-mvp-audit.md`](laptop-mvp-audit.md#7-sonarqube-without-a-remote-server-investigation) referenced this investigation. The resolution:

- **No server required for real Sonar-grade analysis on a laptop.** Confirmed feasible via Path B/C.
- **Quality profile XML import is the right user-facing surface.** Confirmed; ships as `relay import sonarqube-profile`.
- **Recommended path: Hybrid (Path C).** Default install stays lean; SonarLint is opt-in via `--with-sonar` or implicit on profile import.
- **Limitations to communicate:** taint analysis, COBOL/Apex/PL/SQL/T-SQL are server-only. These are LGPL-edition limits, not Relay limits.

---

## Sources

- [SonarSource/sonarlint-core (GitHub)](https://github.com/SonarSource/sonarlint-core) — the embeddable Java library, LGPL-3.0.
- [SonarSource/sonarlint-vscode (GitHub)](https://github.com/SonarSource/sonarlint-vscode) — the VS Code extension; reference implementation of "no-server" Sonar analysis.
- [SonarSource/sonarlint-vscode | DeepWiki](https://deepwiki.com/SonarSource/sonarlint-vscode) — architectural breakdown of the extension's three-tier design.
- [SonarSource/sonarlint-cli (GitHub) — archived 2018-01-11](https://github.com/SonarSource/sonarlint-cli) — the deprecated CLI; do not use.
- [SonarQube for VS Code requirements (Sonar docs)](https://docs.sonarsource.com/sonarqube-for-vs-code/getting-started/requirements) — JRE 21 requirement, language-specific host requirements, which languages require connected mode.
- [SonarQube for IDE | Sonar (product page)](https://www.sonarsource.com/products/sonarqube/ide/) — current product framing and supported languages list.
- [Standalone vs Connected mode in SonarLint (Sonar docs)](https://docs.sonarsource.com/sonarqube-cloud/improving/connected-mode) — modes and what each enables.
- [Quality profile export/import — SonarQube Server docs](https://docs.sonarsource.com/sonarqube-server/quality-standards-administration/managing-quality-profiles/creating-a-quality-profile) — backup/restore via UI and API.
- [Semgrep Pro rules / Community Edition rules (Semgrep docs)](https://semgrep.dev/docs/semgrep-code/pro-rules) — semgrep's rule footprint, for the coverage comparison.
- [Semgrep vs SonarQube technical comparison (Konvu, 2026)](https://konvu.com/compare/semgrep-vs-sonarqube) — independent comparison; informs the "incomplete coverage" claim.
