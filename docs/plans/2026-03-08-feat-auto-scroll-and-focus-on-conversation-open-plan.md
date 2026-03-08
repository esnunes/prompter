---
title: "feat: Auto-scroll and auto-focus on conversation open"
type: feat
date: 2026-03-08
issue: https://github.com/esnunes/prompter/issues/45
---

# feat: Auto-scroll and auto-focus on conversation open

## Overview

When a user navigates to a conversation page, reduce friction by automatically scrolling to the most recent content and focusing the textarea so they can start typing immediately.

## Problem Statement

Currently, opening a conversation with a long message history requires the user to manually scroll down and click into the textarea before interacting. This creates unnecessary friction on every conversation open.

## Proposed Solution

Add two behaviors to the existing `DOMContentLoaded` handler in `app.js`:

1. **Scroll to bottom** — reuse the existing `scrollConversation()` function, which already handles the questionnaire exception (scrolls to `#question-form` instead of bottom when present).
2. **Focus the textarea** — focus `.chat-form textarea` unless a questionnaire is displayed (textarea is hidden when `#question-form` exists).

Both behaviors fire only once on initial page load via `DOMContentLoaded`, which does not re-fire on HTMX swaps.

## Technical Considerations

### Edge Cases (from SpecFlow analysis)

| Case | Scroll | Focus | Notes |
|---|---|---|---|
| Normal conversation | Bottom | Yes | Happy path |
| Questionnaire present | To `#question-form` | No | Textarea is `display:none` |
| Empty/new conversation | No-op (already at top) | Yes | `scrollTop = scrollHeight` is harmless |
| Processing state | Bottom | Yes | Textarea is not server-disabled on full load |
| URL hash (`#revision-N`) | **Skip** | **Skip** | Preserve native anchor scroll |
| Mobile (< 769px) | Bottom | **Skip** | Virtual keyboard disrupts scroll position |

### Ordering

The correct sequence in `DOMContentLoaded`:

1. `renderMarkdown()` — already present, changes element heights
2. `scrollConversation()` — needs correct heights from step 1
3. Focus textarea — last, so scroll position is settled

### Scroll Behavior

The `.chat-messages` container may have `scroll-behavior: smooth` via CSS. On initial page load, use instant scroll to avoid a visible animation through all messages. Temporarily override or use `scrollTo({ behavior: 'instant' })`.

## Acceptance Criteria

- [x] Opening a conversation scrolls to the bottom showing the most recent messages
- [x] The textarea receives focus automatically so the user can type immediately
- [x] When the last assistant message has a questionnaire, scroll to the questionnaire instead and do NOT focus the textarea
- [x] URL fragment navigation (e.g., `#revision-3` from sidebar) is not overridden by auto-scroll
- [x] Auto-focus is skipped on mobile viewports (< 769px) to avoid keyboard disruption
- [x] These behaviors only fire on initial page load, not after sending messages or HTMX swaps

## MVP

### `internal/server/static/app.js`

Modify the existing `DOMContentLoaded` handler (currently lines 88-90):

```javascript
document.addEventListener("DOMContentLoaded", function () {
  renderMarkdown();

  // Auto-scroll and focus on initial conversation page load.
  // Skip if URL has a hash fragment (e.g., #revision-3) to preserve
  // native anchor scroll from revision sidebar links.
  if (!window.location.hash && document.getElementById("conversation")) {
    // Scroll instantly (no animation) to avoid visual jank on load.
    var c = document.getElementById("conversation");
    var q = document.getElementById("question-form");
    if (q) {
      var target = q.previousElementSibling || q;
      target.scrollIntoView({ behavior: "instant", block: "start" });
    } else if (c) {
      c.scrollTo({ top: c.scrollHeight, behavior: "instant" });
    }

    // Focus textarea unless questionnaire is showing (textarea hidden)
    // or on mobile where keyboard would disrupt scroll position.
    if (!q && window.innerWidth >= 769) {
      var textarea = document.querySelector(".chat-form textarea");
      if (textarea && !textarea.disabled) {
        textarea.focus();
      }
    }
  }
});
```

**Key decisions:**
- Inline the scroll logic (don't reuse `scrollConversation()`) because initial load needs `behavior: "instant"` while post-message scroll uses the default (smooth via CSS). This avoids adding a parameter to the shared function.
- Guard with `window.innerWidth >= 769` matching the existing responsive breakpoint at 769px.
- Guard with `!window.location.hash` to preserve revision anchor navigation.
- Check `!textarea.disabled` for robustness, though textarea is not disabled on server render.

**No other files need modification.** No server-side changes required.

## References

- Issue: https://github.com/esnunes/prompter/issues/45
- Existing scroll logic: `internal/server/static/app.js:1-13` (`scrollConversation()`)
- DOMContentLoaded handler: `internal/server/static/app.js:88-90`
- Conversation template: `internal/server/templates/conversation.html`
- Questionnaire block: `conversation.html:52-94` (`#question-form`)
- Message form: `conversation.html:150-161` (`#message-form`)
- Responsive breakpoint: `internal/server/static/style.css` (769px)
