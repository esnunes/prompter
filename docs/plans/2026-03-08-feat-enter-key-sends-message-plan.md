---
title: "feat: Enter key sends message, Shift+Enter adds new line"
type: feat
date: 2026-03-08
issue: "#39"
---

# feat: Enter key sends message, Shift+Enter adds new line

## Overview

Change the chat textarea keyboard behavior so that pressing Enter submits the message (like modern chat apps), while Shift+Enter inserts a new line. This aligns with user expectations from Slack, ChatGPT, Discord, etc.

## Acceptance Criteria

- [x] **Enter** (no modifiers) submits the message via HTMX (same as clicking Send)
- [x] **Shift+Enter** inserts a new line (default textarea behavior)
- [x] **Empty/whitespace-only** messages are blocked on Enter (no submission)
- [x] **Send button** continues to work unchanged
- [x] **IME composition** (CJK input): Enter during composition confirms characters, does NOT submit
- [x] **Server-side** also rejects whitespace-only messages (defense in depth)
- [x] **Placeholder hint** updated to indicate keyboard shortcuts

## MVP

### 1. JavaScript keydown handler — `internal/server/static/app.js`

Add a delegated `keydown` listener inside the existing IIFE. Use event delegation on `document` so it survives HTMX morphing/swaps.

```javascript
// Enter-to-send: submit chat form on Enter, newline on Shift+Enter
document.addEventListener("keydown", function (e) {
  // Only handle Enter on the chat textarea
  if (e.key !== "Enter") return;
  var textarea = e.target;
  if (!textarea.matches(".chat-form textarea")) return;

  // Allow Shift+Enter to insert newline (default behavior)
  if (e.shiftKey) return;

  // Don't submit during IME composition (CJK input)
  if (e.isComposing || e.keyCode === 229) return;

  e.preventDefault();

  // Don't submit empty/whitespace-only messages
  if (textarea.value.trim() === "") return;

  // Don't submit if textarea is disabled (processing in progress)
  if (textarea.disabled) return;

  // Submit via the form's submit button to keep HTMX pipeline intact
  var form = textarea.closest("form");
  if (form) {
    var btn = form.querySelector('button[type="submit"]');
    if (btn && !btn.disabled) btn.click();
  }
});
```

**Key decisions:**
- **Event delegation on `document`** — survives HTMX DOM swaps without re-attaching listeners
- **`btn.click()`** as submission mechanism — triggers the same HTMX flow as a user click, including `hx-disabled-elt`, `hx-on::after-request`, and the loading indicator. Simpler and more reliable than `form.requestSubmit()` or `htmx.trigger()`
- **IME guard** via `e.isComposing || e.keyCode === 229` — standard approach used by Slack, Discord, VS Code
- **Disabled check** — prevents double-submission during processing
- **Ctrl+Enter / Cmd+Enter** — fall through to default (browser default inserts newline in textarea). Not worth overriding for this feature scope

### 2. Placeholder hint — `internal/server/templates/conversation.html`

Update the textarea placeholder to hint at keyboard shortcuts:

```html
<!-- Before -->
<textarea name="message" placeholder="Describe the feature you'd like..." rows="2" required></textarea>

<!-- After -->
<textarea name="message" placeholder="Describe the feature you'd like... (Enter to send, Shift+Enter for new line)" rows="2" required></textarea>
```

### 3. Server-side whitespace validation — `internal/server/handlers.go`

Trim the message before the empty check (defense in depth):

```go
// Before (line 328)
userMessage := r.FormValue("message")

// After
userMessage := strings.TrimSpace(r.FormValue("message"))
```

This ensures whitespace-only messages are rejected even if client-side validation is bypassed.

## Out of Scope

- **Textarea auto-resize** as lines are added (follow-up improvement)
- **Mobile-specific behavior** — virtual keyboards fire standard `keydown` events; the IME guard and Shift check work on mobile. Mobile users can still use the Send button. No touch-device detection needed
- **Ctrl+Enter / Cmd+Enter** as submit alternatives — minimal gain, Enter alone is sufficient

## References

- Issue: #39
- Chat form: `internal/server/templates/conversation.html:150-161`
- Client JS: `internal/server/static/app.js`
- Server handler: `internal/server/handlers.go:328`
