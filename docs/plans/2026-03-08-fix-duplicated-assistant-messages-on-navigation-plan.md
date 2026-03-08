---
title: "Fix duplicated assistant messages on conversation navigation"
type: fix
date: 2026-03-08
issue: https://github.com/esnunes/prompter/issues/33
---

# Fix duplicated assistant messages on conversation navigation

## Overview

When a user navigates to a conversation that has a pending "responded" status (e.g., clicking a sidebar link to a conversation with unread messages), assistant messages appear duplicated. The page renders all messages from the DB, but also starts a polling div that delivers the same assistant message again via DOM injection. Refreshing fixes it because the status gets cleared on poll consumption.

## Root Cause

In `handleShow` (`internal/server/handlers.go:245`), the server reads `repoStatus` from the in-memory status map. When the value is `"responded"`:

1. `ListMessages(id)` at line 230 loads **all** messages from DB, including the new assistant message
2. The template renders these messages into `#conversation`
3. `repoStatus = "responded"` is passed to the template
4. In `conversation.html:136`, the catch-all condition `{{else if and .RepoStatus (ne .RepoStatus "ready")}}` matches "responded" and renders a polling div with `hx-trigger="every 2s"`
5. The poll hits `handleRepoStatus`, which sees "responded", fetches the last assistant message, and injects it into `#conversation` via inline script
6. **Result: the same assistant message appears twice**

The catch-all was designed for "cloning"/"pulling" states but inadvertently matches "responded" and "error" too.

## Proposed Solution

Two targeted changes — one server-side, one template-side:

### 1. Treat "responded" as "ready" in `handleShow`

In `internal/server/handlers.go`, after reading the status entry (line 246), add a check:

```go
// internal/server/handlers.go — inside handleShow, after line 253

// When status is "responded", the assistant message is already in the DB
// and will be rendered by the template. Treat as "ready" for template
// purposes. Do NOT delete from the map — an active polling tab may still
// need to consume "responded" via handleRepoStatus.
if repoStatus == "responded" {
    repoStatus = "ready"
}
```

Key detail: **only change the local variable**, do not call `s.repoStatus.Delete(id)`. This preserves the entry for any other browser tab that has an active polling div for the same conversation.

### 2. Make template catch-all explicit

In `internal/server/templates/conversation.html:136`, replace the negative catch-all with explicit state matching:

```diff
- {{else if and .RepoStatus (ne .RepoStatus "ready")}}
+ {{else if or (eq .RepoStatus "cloning") (eq .RepoStatus "pulling")}}
```

This prevents any future states from accidentally rendering a polling div. Only the two states that genuinely need a "preparing repository..." spinner will match.

## Files to Change

| File | Change |
|------|--------|
| `internal/server/handlers.go:253` | Add `if repoStatus == "responded" { repoStatus = "ready" }` after the empty-status recovery block |
| `internal/server/templates/conversation.html:136` | Replace catch-all with explicit `cloning`/`pulling` match |

## Acceptance Criteria

- [x] Navigating to a conversation with "responded" status shows each message exactly once
- [x] Refreshing the page still shows correct (non-duplicated) messages
- [x] Active polling during "processing" still delivers new messages correctly via DOM injection
- [x] Questions and "Prompt is ready to publish!" render correctly on page load when status is "responded"
- [x] Multi-tab: if tab A has a polling div and tab B navigates to the same conversation, tab A's next poll still works (status entry not prematurely deleted)
- [x] "Cloning repository..." and "Pulling latest changes..." spinners still render correctly

## Edge Cases Considered

**Race: processing -> responded during page load** — If `handleShow` reads "processing" and renders a polling div, then the background goroutine sets "responded" before the first poll fires, the poll correctly delivers the message. No duplication because the message wasn't in the DB when `ListMessages` ran.

**Race: responded read by handleShow** — If `handleShow` reads "responded", messages are already in the DB. The local variable is set to "ready", no polling div is rendered. Correct.

**Multi-tab** — Tab A has polling div for conversation B. User opens conversation B in tab B. `handleShow` changes the local variable but does NOT delete from the map. Tab A's next poll sees "responded" in `handleRepoStatus`, delivers the message via DOM injection. Tab A already had older messages rendered; the new message is correctly appended. No duplication.

**"error" status on page load** — Previously fell through the catch-all and started a spurious poll. After the template fix, "error" no longer matches any branch, so no polling div is rendered. The error state will be visible if the user triggers a status check another way (e.g., retrying). This is an improvement — no more phantom spinner for errored conversations.

## References

- Issue: https://github.com/esnunes/prompter/issues/33
- `handleShow`: `internal/server/handlers.go:210`
- `handleRepoStatus`: `internal/server/handlers.go:590`
- Template catch-all: `internal/server/templates/conversation.html:136`
- DOM injection script: `internal/server/handlers.go:660`
