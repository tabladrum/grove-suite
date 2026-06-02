# Relay — agent system prompt fragment

Drop this fragment into the system prompt of any coding agent (Claude
Code, Copilot Workspace, Cursor agent mode, custom MCP client) that has
the Relay MCP server registered.

The fragment teaches the agent the Pre-Flight Autopilot loop: edit →
`relay_check` → read `next_action` per finding → fix → re-check →
`relay_submit`.

---

## System-prompt fragment (copy verbatim)

```
You have access to a Relay MCP server with these tools:

- relay_check     — run all certified-merge policy gates on a proposed diff
                    without admitting. Use this BEFORE you ask the user to
                    review code. Arguments: { diff (unified diff string)
                    OR diff_path, intent (id), brief (string), author,
                    model, repo (optional) }.
- relay_certify   — alias for relay_check. Returns a signed certificate
                    without admitting to git.
- relay_submit    — only call this when relay_check returns Allowed=true.
                    Creates a signed commit on the target branch.
- relay_policy    — list registered gates and their enabled/disabled
                    state. Use this to discover what relay_check will
                    actually evaluate against this repo.
- relay_explain   — get long-form guidance for a gate or rule key.
                    Arguments: { gate (required), rule (optional) }.

WORKFLOW (Pre-Flight Autopilot):
  1. Stage your edit as a unified diff against the working tree.
  2. Call relay_check with {diff, intent, brief}. Read the response:
       - If Allowed == true, call relay_submit with the SAME arguments.
       - If Allowed == false, iterate over result.Policies. For every
         entry with Verdict != "allow":
            a. Call relay_explain {gate: entry.Gate, rule: entry.Code}
               to fetch the recommended fix.
            b. Apply the fix to the diff (rotate the secret, remove the
               denied path, add a test, upgrade the dep, etc.).
       - Re-call relay_check. Loop until allowed or you have exhausted
         3 attempts; on the 3rd failure, surface the verdict to the
         human reviewer with the per-finding explanations.
  3. Never call relay_submit without first observing relay_check
     Allowed=true on the EXACT same diff. The certificate binds the
     verdict to the diff hash; mismatches will be rejected at admission.

DO NOT call relay_submit speculatively. Each call may produce a signed
commit on the target branch and a permanent audit-trail entry.
```

## Worked example

1. The user asks the agent to add a Slack webhook to the notifier.
2. The agent edits `internal/notify/slack.go` and stages a diff that
   includes the literal webhook URL.
3. The agent calls `relay_check {diff: <patch>, intent: "I-42",
   brief: "wire slack notifier"}`.
4. The response includes `Allowed=false` and a `secrets` finding with
   code `slack-webhook-url`.
5. The agent calls `relay_explain {gate: "secrets", rule: "slack-webhook-url"}`
   and learns to load the URL from an env var instead.
6. The agent rewrites the diff to read `os.Getenv("SLACK_WEBHOOK_URL")`
   and re-calls `relay_check`. This time `Allowed=true`.
7. The agent calls `relay_submit` with the same arguments. The
   certificate is persisted and a signed commit is admitted.

## Wiring

The MCP server runs over stdio. Register with your client by pointing
at the `relay` binary:

```jsonc
// Claude Desktop / Code (claude_desktop_config.json or similar)
{
  "mcpServers": {
    "relay": {
      "command": "relay",
      "args": ["mcp", "serve", "--repo", "/path/to/repo"]
    }
  }
}
```

If `--repo` is omitted the server uses the agent's current working
directory; each tool call may override with `repo` in the arguments.
