---
title: Claude CLI Session Not Resuming — --session-id vs --resume Flag
date: 2026-02-17
category: integration-issues
tags:
  - claude-cli
  - session-management
  - conversation-continuity
severity: high
component: claude-integration
symptoms: |
  Multi-turn conversations lost context between messages. Each Claude CLI
  call started a fresh session instead of continuing the existing one,
  so the AI had no memory of prior exchanges.
root_cause: >
  Claude CLI's --session-id flag creates a NEW session with that ID.
  To continue an existing session, you must use --resume instead.
  The code was using --session-id for every message.
---

# Claude CLI Session Not Resuming — --session-id vs --resume Flag

## Problem

After the first message in a conversation, subsequent messages were not continuing the session. The AI responded as if each message was the start of a new conversation — no memory of prior questions or answers.

## Investigation

The session ID was correctly stored in the database and reused across messages. The Claude CLI was being invoked with the same `--session-id` value each time, which seemed correct.

## Root Cause

The Claude CLI has two distinct flags for session handling:

- **`--session-id <id>`** — Creates a **new** session with the given ID
- **`--resume <id>`** — Continues an **existing** session by its ID

The code was always using `--session-id`, which creates a new session every time rather than resuming the existing one.

**Before (broken):**

```go
cmd := exec.CommandContext(ctx, "claude",
    "-p",
    "--session-id", sessionID,  // Always creates new — wrong for turn 2+
    "--output-format", "json",
    // ...
    userMessage,
)
```

## Solution

Added a `resume` parameter to `SendMessage()` that switches between the two flags.

**`internal/claude/claude.go`:**

```go
func SendMessage(ctx context.Context, sessionID, repoDir, userMessage string, resume bool) (*Response, string, error) {
    args := []string{"-p"}
    if resume {
        // Continue an existing session.
        args = append(args, "--resume", sessionID)
    } else {
        // First message — create a new session with this ID.
        args = append(args, "--session-id", sessionID)
    }
    args = append(args,
        "--output-format", "json",
        "--json-schema", jsonSchema,
        // ...
        userMessage,
    )
    cmd := exec.CommandContext(ctx, "claude", args...)
```

**`internal/server/handlers.go`:**

```go
// Check if this session already has messages (resume vs. new)
existingMsgs, err := s.queries.ListMessages(id)
if err != nil {
    // ...
}
resume := len(existingMsgs) > 0

resp, rawJSON, err := claude.SendMessage(r.Context(), pr.SessionID, pr.RepoLocalPath, userMessage, resume)
```

The handler queries the database for existing messages. If any exist, it's a resume; if none, it's a new session.

## Prevention

- **Test CLI flags with real invocations.** The `--session-id` and `--resume` distinction is not obvious from `--help` output alone. Always run a multi-turn test before building a parser.
- **Name parameters after intent.** The explicit `resume bool` parameter makes the flag choice visible at every call site, preventing silent misuse.
- **Test multi-turn flows end-to-end.** A single-message test wouldn't catch this — the bug only manifests on the second message and beyond.

## Related

- [Claude CLI Structured Output Parsing](./claude-cli-structured-output-parsing.md) — another Claude CLI integration issue with `--output-format json`
- [Plan: Prompter MVP](../../plans/2026-02-16-feat-prompter-mvp-plan.md) — original Claude CLI integration design
- [Brainstorm: Prompter MVP](../../brainstorms/2026-02-16-prompter-mvp-brainstorm.md) — per-message CLI execution rationale
