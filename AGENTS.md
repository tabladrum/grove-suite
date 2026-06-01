
## Relay — certified merge gate (ALWAYS use these tools)

This project uses [Relay](https://github.com/tabladrum/grove-suite) for
certified code admission. Relay MCP tools are registered. Follow this
workflow on EVERY coding task:

### Pre-Flight Autopilot

1. **Open an intent BEFORE making code changes** — call relay_intent_open with
   {title: <short summary>, description: <verbatim user request>}.
   Save the returned intent_id.

2. **Before asking the user to review** — call relay_check with the unified
   diff plus {intent: <intent_id>, brief: <one-liner>}.

3. **If Allowed=false** — for each policy entry with Verdict != "allow":
   - Call relay_explain {gate, rule} for the recommended fix.
   - Apply the fix, re-diff, and re-call relay_check.
   - Loop up to 3 times; on the 3rd failure surface the verdict to the user.

4. **Only call relay_submit when relay_check returns Allowed=true** on the
   EXACT same diff. Never call relay_submit speculatively.

5. **Close the intent when done** — call relay_intent_close {intent_id}.
   Pass the returned trailer_block to relay_submit so the commit is linked
   to the intent YAML.

### Tool quick-reference

| Tool                 | When                                          |
|----------------------|-----------------------------------------------|
| relay_intent_open    | First — capture the user request as an intent |
| relay_check          | Before every review request                   |
| relay_explain        | On any Verdict != allow                       |
| relay_submit         | Only after relay_check Allowed=true           |
| relay_policy         | Discover which gates are active               |
| relay_intent_close   | When the task is complete                     |
