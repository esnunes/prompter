---
title: "Replace timeout with cancel button and elapsed timer"
type: feat
date: 2026-03-06
issue: https://github.com/esnunes/prompter/issues/22
---

# Replace Timeout with Cancel Button and Elapsed Timer

## Overview

Remove the hard-coded 2-minute timeout on Claude CLI requests. Instead, show a real-time elapsed timer ("Thinking... (1m 23s)") and a cancel button so users have visibility and control over long-running AI requests. When cancelled, show a "Cancelled" message with a retry button.

## Problem Statement

When the AI explores a repository — especially on the first message — it often exceeds the 2-minute timeout. This produces "AI is taking too long, please try again" errors that force manual retries, losing time and breaking flow. Users have no visibility into request duration and no way to cancel mid-request.

## Proposed Solution

### Architecture Decision: Unify All Claude Calls to Async

The current codebase has two paths for calling Claude:
- **Path A (sync):** `handleSendMessage` blocks the HTTP request while Claude runs
- **Path B (async):** `backgroundSendMessage` runs Claude in a goroutine, client polls `/status`

Path A cannot support cancel or elapsed timer because the HTTP connection is held open and there's no way to send a cancel signal from a separate request.

**Decision:** Convert `handleSendMessage` to always use the async pattern. When a user sends a message:
1. Save the user message to DB
2. Set status to `processing` and launch `backgroundSendMessage` in a goroutine
3. Return immediately with the user message bubble + processing indicator (spinner + timer + cancel button)
4. Client polls `/status` every 2s to get the response

This unifies both paths under a single cancellable, observable flow.

### Cancel Mechanism

- Extend `repoStatusEntry` with `StartedAt time.Time` and `CancelFunc context.CancelFunc`
- When launching `backgroundSendMessage`, create a cancellable context and store the `CancelFunc` in the status entry
- New endpoint `POST .../cancel` calls the stored `CancelFunc`, which sends SIGTERM to Claude CLI
- The existing `cmd.WaitDelay = 5 * time.Second` ensures SIGKILL follows if SIGTERM is ignored

### DB State After Cancel

When Claude is cancelled:
- The user message is already saved (before Claude was called)
- Save a synthetic assistant message: `"Request cancelled by user."` with no `raw_response`
- This breaks the auto-retry loop in `handleRepoStatus` (which re-sends when last message is from "user")
- Set status to `cancelled` in `sync.Map`

### Retry After Cancel

- The `cancelled` status fragment includes a retry button
- Retry calls a new `POST .../resend` endpoint that:
  1. Deletes the synthetic "cancelled" assistant message
  2. Sets status to `processing`
  3. Launches `backgroundSendMessage` again with the original user message
- This avoids the existing `/retry` endpoint which re-clones/re-pulls the repo unnecessarily

### Elapsed Timer

- Pure client-side JavaScript: `setInterval` updates display every second
- Server includes `StartedAt` as a Unix timestamp in the status fragment HTML (as a `data-started-at` attribute)
- On each poll response, the client reads `data-started-at` and computes elapsed — survives page refresh
- Timer only appears during `processing` state (not during `cloning`/`pulling`)

## Technical Approach

### Phase 1: Backend — Remove Timeout and Add Cancel Infrastructure

#### 1.1 Remove timeout from `claude.go`

**File:** `internal/claude/claude.go`

- Delete `const timeout = 120 * time.Second`
- Remove the `context.WithTimeout` wrapper (line 127-128)
- Keep the existing `ctx` parameter — caller now controls cancellation
- Update error handling: detect `context.Canceled` (not just `DeadlineExceeded`)

```go
// Before
func SendMessage(ctx context.Context, ...) (*Response, string, error) {
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    ...
    if ctx.Err() == context.DeadlineExceeded {
        return nil, "", fmt.Errorf("AI is taking too long, please try again")
    }

// After
func SendMessage(ctx context.Context, ...) (*Response, string, error) {
    // No timeout — caller controls cancellation
    ...
    if ctx.Err() == context.Canceled {
        return nil, "", fmt.Errorf("request cancelled")
    }
```

#### 1.2 Extend `repoStatusEntry` in `server.go`

**File:** `internal/server/server.go`

```go
type repoStatusEntry struct {
    Status    string              // "cloning", "pulling", "ready", "processing", "responded", "cancelled", "error"
    Error     string
    StartedAt time.Time           // when processing started (zero for non-processing states)
    CancelFunc context.CancelFunc // cancel function for in-flight Claude call (nil when not processing)
}
```

Note: Since `sync.Map` stores values, and `CancelFunc` is a reference type, this works correctly with `CompareAndSwap` if we avoid it for entries with func fields. Use `Store` directly instead.

#### 1.3 Convert `handleSendMessage` to async

**File:** `internal/server/handlers.go`

Change `handleSendMessage` to:
1. Save user message to DB (existing logic)
2. Set status to `processing` with `StartedAt: time.Now()` and a new `CancelFunc`
3. Launch `backgroundSendMessage` in a goroutine
4. Return the user message bubble + a `#repo-status` div that triggers polling

The returned HTML fragment:
- Shows the user message bubble
- Includes a `#repo-status` div with `hx-get=".../status"` and `hx-trigger="every 2s"` to start polling
- Hides the message form (existing pattern)

#### 1.4 Update `backgroundSendMessage` to use cancellable context

**File:** `internal/server/handlers.go`

- Accept a `context.Context` parameter instead of using `context.Background()`
- Pass it to `claude.SendMessage`
- On `context.Canceled` error: save synthetic "Request cancelled by user." message, set status to `cancelled`
- On success: existing behavior (save message, set status to `responded`)

#### 1.5 Add cancel endpoint

**File:** `internal/server/server.go` (route registration)
**File:** `internal/server/handlers.go` (handler)

```
POST /github.com/{org}/{repo}/prompt-requests/{id}/cancel
```

Handler logic:
1. Load status entry from `sync.Map`
2. If status is `processing` and `CancelFunc` is not nil, call it
3. Return a status fragment showing "Cancelling..." (the background goroutine will transition to `cancelled`)
4. If status is not `processing`, return current status (no-op)

#### 1.6 Add resend endpoint

**File:** `internal/server/server.go` (route registration)
**File:** `internal/server/handlers.go` (handler)

```
POST /github.com/{org}/{repo}/prompt-requests/{id}/resend
```

Handler logic:
1. Get last message — verify it's an assistant "cancelled" message
2. Delete the synthetic cancelled message from DB
3. Create cancellable context, set status to `processing`
4. Launch `backgroundSendMessage`
5. Return processing status fragment with timer + cancel button

#### 1.7 Update `handleRepoStatus` to handle `cancelled` state

**File:** `internal/server/handlers.go`

- Do NOT trigger `backgroundSendMessage` when status is `cancelled`
- When status is `cancelled`, render the cancelled fragment (spinner gone, show "Cancelled" + retry button)

#### 1.8 Fix `CompareAndSwap` for processing transition

The current `CompareAndSwap` at line 550 compares struct values. With the new fields (`StartedAt`, `CancelFunc`), direct struct comparison won't work for `CancelFunc` (functions are not comparable). Replace with a status-check-then-store pattern protected by the session lock, or use a separate `sync.Map` for cancel functions.

**Recommended:** Store `CancelFunc` in a separate `sync.Map` (`cancelFuncs sync.Map // prID → context.CancelFunc`) to keep `repoStatusEntry` comparable.

### Phase 2: Frontend — Timer, Cancel Button, and Cancelled State

#### 2.1 Update `status_fragment.html`

**File:** `internal/server/templates/status_fragment.html`

Add to the `statusFragmentData` struct: `StartedAt int64` (Unix timestamp) and `CancelURL string`.

Update the `processing` block:

```html
{{if eq .Status "processing"}}
<div id="repo-status" class="repo-status"
     hx-get="{{.PollURL}}"
     hx-trigger="every 2s"
     hx-swap="outerHTML"
     data-started-at="{{.StartedAt}}">
  <div class="processing-indicator">
    <div class="spinner"></div>
    <span class="processing-text">Thinking...</span>
    <span class="elapsed-timer"></span>
  </div>
  <form hx-post="{{.CancelURL}}"
        hx-target="#repo-status"
        hx-swap="outerHTML"
        hx-disabled-elt="find button"
        style="display:inline;">
    <button type="submit" class="btn btn-sm btn-secondary">Cancel</button>
  </form>
</div>
```

Add a new `cancelled` block:

```html
{{else if eq .Status "cancelled"}}
<div id="repo-status" class="repo-status repo-status-cancelled">
  <span>Request cancelled.</span>
  <form hx-post="{{.ResendURL}}"
        hx-target="#repo-status"
        hx-swap="outerHTML"
        hx-disabled-elt="find button"
        style="display:inline;">
    <button type="submit" class="btn btn-sm btn-primary">Retry</button>
  </form>
</div>
```

#### 2.2 Update `statusFragmentData` struct

**File:** `internal/server/handlers.go`

```go
type statusFragmentData struct {
    Status    string
    Error     string
    PollURL   string
    RetryURL  string
    CancelURL string
    ResendURL string
    StartedAt int64  // Unix timestamp, 0 if not processing
}
```

#### 2.3 Add elapsed timer JavaScript

**File:** `internal/server/static/app.js`

```javascript
// Start/update elapsed timer for processing indicators
function updateElapsedTimers() {
  var els = document.querySelectorAll('[data-started-at]');
  els.forEach(function(el) {
    var startedAt = parseInt(el.getAttribute('data-started-at'), 10);
    if (!startedAt) return;
    var timer = el.querySelector('.elapsed-timer');
    if (!timer) return;
    var elapsed = Math.floor(Date.now() / 1000) - startedAt;
    if (elapsed < 0) elapsed = 0;
    var mins = Math.floor(elapsed / 60);
    var secs = elapsed % 60;
    timer.textContent = mins > 0
      ? '(' + mins + 'm ' + secs + 's)'
      : '(' + secs + 's)';
  });
}
```

Add a `setInterval(updateElapsedTimers, 1000)` call and trigger on `htmx:afterSwap`.

#### 2.4 Update `handleSendMessage` response fragment

When `handleSendMessage` returns the user message + processing indicator, include a `#repo-status` div that will be picked up by the polling mechanism. This replaces the current inline spinner from `htmx-indicator`.

#### 2.5 Add CSS for new states

**File:** `internal/server/static/style.css`

- `.processing-indicator` — flex row with spinner, text, timer
- `.elapsed-timer` — secondary text color, monospace font
- `.repo-status-cancelled` — styling for cancelled state

### Phase 3: Cleanup and Edge Cases

#### 3.1 Handle page refresh during processing

When `handleShow` renders the conversation page and status is `processing`:
- The `#repo-status` div already renders with polling (existing behavior)
- Include `data-started-at` from the `repoStatusEntry.StartedAt` so the timer shows correct elapsed time
- Cancel button appears immediately

#### 3.2 Handle navigate-away during processing

With the async pattern, navigating away does NOT cancel Claude — the goroutine continues with its own context. The cancel button is the only way to stop it. When the user returns, the status will be `responded` (if Claude finished) or still `processing` (if still running).

#### 3.3 Handle server restart during processing

Existing behavior: `sync.Map` state is lost, DB has dangling user message, auto-retry kicks in on next visit. This is unchanged — it's actually correct behavior for server restarts.

#### 3.4 Prevent double-cancel

The cancel endpoint is idempotent: if `CancelFunc` is nil or status is not `processing`, it's a no-op. Return current status fragment.

#### 3.5 Remove `htmx-indicator` spinner from message form

Since all requests are now async, the inline `htmx-indicator` in the message form and question form should be removed or repurposed. The `#repo-status` div handles all progress display.

Actually — keep the `htmx-indicator` for the brief moment between form submit and server response (the HTTP round-trip to save the message and start processing). The timer + cancel button appear only after the `#repo-status` div starts polling.

## State Machine

```
                   ┌──────────┐
                   │ cloning  │
                   └────┬─────┘
                        │
                   ┌────▼─────┐
                   │ pulling  │
                   └────┬─────┘
                        │
                   ┌────▼─────┐
       ┌──────────►│  ready   │◄──────────┐
       │           └────┬─────┘           │
       │                │ user sends msg  │
       │           ┌────▼──────┐          │
       │           │processing │──────────┤
       │           └──┬─────┬──┘          │
       │              │     │             │
       │    cancel    │     │ success     │
       │         ┌────▼──┐  │        ┌────▼────┐
       │         │cancelled│ └──────►│responded│
       │         └────┬──┘          └─────────┘
       │              │ resend
       └──────────────┘
```

## Acceptance Criteria

- [x] No hard-coded timeout — Claude can run as long as needed
- [x] Elapsed timer appears during `processing` state, counting up every second
- [x] Timer format: "(Xs)" for under 1 minute, "(Xm Ys)" for 1 minute and above
- [x] Timer survives page refresh (server provides start time)
- [x] Cancel button appears next to the timer during `processing`
- [x] Clicking cancel sends SIGTERM to Claude CLI subprocess
- [x] After cancel, "Request cancelled." message + Retry button shown
- [x] Clicking Retry resends the same user message to Claude
- [x] Cancelled exchange is saved to DB (user message + synthetic cancelled assistant message)
- [x] Auto-retry in `handleRepoStatus` does NOT trigger when status is `cancelled`
- [x] Cancel is idempotent — clicking twice is harmless
- [x] Session lock is released promptly after cancel (within 5s WaitDelay)
- [x] Question-form submissions also show timer + cancel button
- [x] Existing clone/pull status polling continues to work unchanged

## Files to Modify

| File | Changes |
|---|---|
| `internal/claude/claude.go` | Remove timeout, detect `context.Canceled` |
| `internal/server/server.go` | Extend `repoStatusEntry`, add `cancelFuncs sync.Map`, register new routes |
| `internal/server/handlers.go` | Convert `handleSendMessage` to async, add `handleCancel`, add `handleResend`, update `handleRepoStatus`, update `backgroundSendMessage` signature |
| `internal/server/templates/status_fragment.html` | Add `processing` timer/cancel, add `cancelled` state |
| `internal/server/templates/conversation.html` | Add `#repo-status` for processing state after message send |
| `internal/server/templates/message_fragment.html` | Include `#repo-status` div for async processing |
| `internal/server/static/app.js` | Add elapsed timer logic |
| `internal/server/static/style.css` | Add processing indicator and cancelled state styles |

## References

- Issue: https://github.com/esnunes/prompter/issues/22
- Existing async pattern: `internal/server/handlers.go:598` (`backgroundSendMessage`)
- Status polling: `internal/server/handlers.go:514` (`handleRepoStatus`)
- Claude CLI subprocess: `internal/claude/claude.go:126` (`SendMessage`)
- Past solution — structured output parsing: `docs/solutions/integration-issues/claude-cli-structured-output-parsing.md`
- Past solution — session resume: `docs/solutions/integration-issues/claude-cli-session-resume-flag.md`
