## Provasign — certified merge gate (ALWAYS use these tools)

This project uses [Provasign](https://github.com/provasign/provasign) for
certified code admission. Provasign MCP tools are registered. Follow this
workflow on EVERY coding task:

### Pre-Flight Autopilot

1. **Open an intent BEFORE making code changes** — call provasign_intent_open with
   {title: <short summary>, description: <verbatim user request>}.
   Save the returned intent_id.

2. **Before asking the user to review** — call provasign_check with the unified
   diff plus {intent: <intent_id>, brief: <one-liner>}.

3. **If Allowed=false** — for each policy entry with Verdict != "allow":
   - Call relay_explain {gate, rule} for the recommended fix.
   - Apply the fix, re-diff, and re-call provasign_check.
   - Loop up to 3 times; on the 3rd failure surface the verdict to the user.

4. **Only call provasign_submit when provasign_check returns Allowed=true** on the
   EXACT same diff. Never call provasign_submit speculatively.

5. **Close the intent when done** — call relay_intent_close {intent_id}.
   Pass the returned trailer_block to provasign_submit so the commit is linked
   to the intent YAML.

### Tool quick-reference

| Tool                 | When                                          |
|----------------------|-----------------------------------------------|
| provasign_intent_open    | First — capture the user request as an intent |
| provasign_check          | Before every review request                   |
| relay_explain        | On any Verdict != allow                       |
| provasign_submit         | Only after provasign_check Allowed=true           |
| relay_policy         | Discover which gates are active               |
| relay_intent_close   | When the task is complete                     |


## Prism — context delivery (ALWAYS use these tools)

This project uses [Prism](https://github.com/provasign/provasign/prism) for token-optimized context delivery.
Prism tools are registered via MCP. Follow these rules in every task:

### Rules
1. **Start every task with prism_query** — call it with the task description before
   reading any files. It returns pre-ranked, compressed context covering targets,
   dependencies, and tests within the token budget.

2. **Use prism_read instead of read_file / cat** — it applies session-aware compression:
   full text on first read, signatures on second, references on third+.
   This saves 35–92% tokens and prevents context window overflow.

3. **Use prism_search instead of grep/ripgrep** — when you need to find a symbol
   by name, use prism_search. Follow up with prism_lookup for the full source.

4. **Call prism_index once at session start** (or after significant file changes).
   Do not re-index on every step — delta indexing is automatic.

5. **Call prism_compact when the context window is near capacity** — it summarizes
   older turns while preserving recent ones.

6. **If a Prism tool returns empty results, do not immediately fall back to grep/read.**
	First run prism_index for the current workspace root and retry the same Prism tool.
	Only use non-Prism fallback if the second Prism attempt is still empty.

### Tool priority order
| Instead of...          | Use...         |
|------------------------|----------------|
| read_file / open file  | prism_read     |
| grep / ripgrep / find  | prism_search   |
| manual context gather  | prism_query    |
| symbol definition      | prism_lookup   |
