---
title: "fix: Remove gap between left sidebar and chat on conversation page"
type: fix
date: 2026-03-08
issue: https://github.com/esnunes/prompter/issues/41
---

# fix: Remove gap between left sidebar and chat on conversation page

On the conversation page, there is an unwanted visual gap between the left sidebar (prompt list) and the chat content area. The chat content should sit flush against the left sidebar border.

## Root Cause

In `internal/server/static/style.css`, the `.container` class applies `padding: var(--space-6) var(--space-6)` to all pages (line 72). The conversation page override at line 406 removes top, bottom, and right padding — but **omits `padding-left`**:

```css
/* Current (line 406-410) */
.container:has(.conversation-wrapper) {
  padding-top: 0;
  padding-bottom: 0;
  padding-right: 0;
  /* padding-left is missing — this causes the gap */
}
```

## Fix

Add `padding-left: 0` to the existing conversation container rule:

```css
/* internal/server/static/style.css:406 */
.container:has(.conversation-wrapper) {
  padding-top: 0;
  padding-bottom: 0;
  padding-right: 0;
  padding-left: 0;
}
```

Or simplify to:

```css
.container:has(.conversation-wrapper) {
  padding: 0;
}
```

## Acceptance Criteria

- [x] Chat content sits flush against the left sidebar border on the conversation page
- [x] Dashboard page keeps its current padding (`var(--space-6)`)
- [x] Repo page keeps its current padding (`var(--space-6)`)
- [x] Right side (chat to revision sidebar) remains unchanged
- [x] Mobile layout (<=768px) is unaffected (sidebar is hidden)

## Why This Is Safe

- **Scoped selector**: `.container:has(.conversation-wrapper)` only matches the conversation page
- **Internal padding preserved**: `.chat-messages` has its own `padding: var(--space-6) var(--space-4)`, so message readability is unaffected
- **Chat input preserved**: `.chat-input` has its own `padding: var(--space-4) var(--space-4)`
- **Archive banner preserved**: Inside `.conversation-main`, unaffected by container padding
- **Mobile unaffected**: `.prompt-sidebar` is `display: none` at <=768px

## References

- Issue: [#41](https://github.com/esnunes/prompter/issues/41)
- File: `internal/server/static/style.css:406-410`
