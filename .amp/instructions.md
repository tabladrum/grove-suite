
## Provasign — certified merge gate (ALWAYS use these tools)

This project uses [Provasign](https://github.com/tabladrum/grove-suite) for
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
