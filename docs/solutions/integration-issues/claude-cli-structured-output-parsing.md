---
title: Empty Claude Responses in Web UI Despite Valid SQLite Data
date: 2026-02-16
category: integration-issues
tags:
  - claude-cli
  - json-parsing
  - structured-output
severity: high
component: claude-integration
symptoms: |
  Web UI displayed empty Claude responses. Valid response data existed in
  SQLite (messages.raw_response column) but the parsed message content was
  empty, causing blank chat bubbles.
root_cause: >
  Claude CLI --output-format json puts --json-schema results in a
  structured_output object field, not the result string field.
---

# Empty Claude Responses in Web UI Despite Valid SQLite Data

## Problem

After sending a message in the conversation UI, the assistant response appeared empty — no message text, no structured question, nothing. However, inspecting the SQLite database showed a full, valid response stored in `messages.raw_response`.

## Investigation

Checked the `messages` table directly:

```sql
SELECT id, role, content, raw_response FROM messages ORDER BY id DESC LIMIT 1;
```

The `content` column was empty, but `raw_response` contained a large JSON blob with valid data including a message and a structured question with options.

## Root Cause

The Claude CLI `--output-format json` response format was different from what we assumed during planning.

**What we expected:**

```json
{
  "result": "{\"message\":\"Hello\",\"question\":{...}}"
}
```

A `result` string field containing JSON-encoded schema output (requiring double unmarshal).

**What Claude CLI actually returns:**

```json
{
  "type": "result",
  "structured_output": {
    "message": "Thanks for the feature request! ...",
    "question": {
      "text": "How would you like to add new repositories?",
      "options": [...]
    },
    "prompt_ready": false
  },
  "result": "",
  "session_id": "...",
  "usage": {...}
}
```

The schema output is in `structured_output` as a **parsed object**, and `result` is an **empty string**.

Our parser checked `result` first, found it empty, fell through to direct parsing (which also failed because the top-level JSON doesn't match our schema), and returned an empty message.

## Solution

Updated all three parse sites to check `structured_output` first:

**`internal/claude/claude.go` — live response parsing:**

```go
var wrapper struct {
    StructuredOutput *Response `json:"structured_output"`
    Result           string    `json:"result"`
}
if err := json.Unmarshal(output, &wrapper); err == nil {
    if wrapper.StructuredOutput != nil {
        return wrapper.StructuredOutput, nil
    }
    // fall back to Result string if structured_output is absent
}
```

**`internal/server/handlers.go` — re-parsing stored raw JSON on page reload:**

Extracted a shared `parseRawResponse` helper with the same priority: `structured_output` > `result` string > direct parse.

**`internal/db/queries.go` — extracting generated prompt for publishing:**

Same pattern applied to `extractGeneratedPrompt` which scans raw responses for the `generated_prompt` field.

## Prevention

- **Test against real CLI output.** The Claude CLI `--help` documents the flags but not the exact JSON envelope format. Always verify with a real invocation before building a parser.
- **Log raw responses on parse failure.** The fix was fast because `raw_response` was stored in the DB. Without that column, debugging would have required reproducing the issue.
- **Parse defensively with a fallback chain.** The fixed code tries three strategies: `structured_output` object, `result` string, direct parse. This protects against future format changes.

## Related

- [Brainstorm: Prompter MVP](../../brainstorms/2026-02-16-prompter-mvp-brainstorm.md) — original JSON schema design
- [Plan: Prompter MVP](../../plans/2026-02-16-feat-prompter-mvp-plan.md) — Claude CLI integration section
- Commit: `ab843df` — the fix
