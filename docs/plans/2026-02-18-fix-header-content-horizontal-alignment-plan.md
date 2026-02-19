---
title: "fix: Header and content horizontal alignment"
type: fix
date: 2026-02-18
---

# fix: Header and content horizontal alignment

Fix horizontal alignment so that the header content and main page content share the same left and right edges on all pages.

## Problem Statement

Three issues cause the header and content to be horizontally misaligned:

1. **Padding mismatch on narrow viewports** — `.header` uses `var(--space-6)` (1.5rem) horizontal padding while `.container` uses `var(--space-4)` (1rem). When the viewport is narrower than `max-width`, the header content is inset 0.5rem more than the page content.

2. **No width expansion on conversation page** — `.container:has(.conversation-wrapper)` expands to `calc(var(--size-container) + var(--size-sidebar) + var(--space-4))` (65rem), but `.header-inner` stays at `var(--size-container)` (48rem). The header is 17rem narrower than the content below it.

3. **No mechanism for page-aware header width** — The header has no way to know which page is active and adjust accordingly.

## Proposed Solution

Move horizontal padding from `.header` to `.header-inner` so both `.header-inner` and `.container` use the same horizontal padding (`var(--space-4)`). Then use `body:has(.conversation-wrapper)` to expand `.header-inner` on the conversation page, mirroring the existing container expansion pattern.

### Why this approach

- Mirrors the existing `:has()` pattern already used for `.container`
- Minimal changes (3 CSS rules modified, 1 added)
- No HTML template changes needed
- `.header` border-bottom still spans full viewport width (border is on the parent, unaffected)

## Acceptance Criteria

- [x] On dashboard and repo pages, "Prompter" title left-aligns with content left edge
- [x] On dashboard and repo pages, header action buttons right-align with content right edge
- [x] On conversation page, header expands to match the wider content area
- [x] On mobile (<=768px), header reverts to standard width when sidebar collapses
- [x] Header border-bottom still spans full viewport width

## MVP

### `internal/server/static/style.css`

**Step 1: Move horizontal padding from `.header` to `.header-inner`**

```css
/* Before */
.header {
  background: var(--color-background);
  border-bottom: var(--border-width) solid var(--color-border);
  padding: var(--space-4) var(--space-6);
}

.header-inner {
  max-width: var(--size-container);
  margin: 0 auto;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

/* After */
.header {
  background: var(--color-background);
  border-bottom: var(--border-width) solid var(--color-border);
  padding: var(--space-4) 0;
}

.header-inner {
  max-width: var(--size-container);
  margin: 0 auto;
  padding: 0 var(--space-4);
  display: flex;
  align-items: center;
  justify-content: space-between;
}
```

**Step 2: Expand header on conversation page**

Add this rule near the existing `.container:has(.conversation-wrapper)` rule (around line 244):

```css
body:has(.conversation-wrapper) .header-inner {
  max-width: calc(var(--size-container) + var(--size-sidebar) + var(--space-4));
}
```

**Step 3: Revert header width on mobile**

Add inside the existing `@media (max-width: 768px)` block (around line 374):

```css
body:has(.conversation-wrapper) .header-inner {
  max-width: var(--size-container);
}
```

## References

- `.header` and `.header-inner`: `internal/server/static/style.css:42-54`
- `.container`: `internal/server/static/style.css:35-39`
- Conversation container expansion: `internal/server/static/style.css:244-246`
- Mobile breakpoint: `internal/server/static/style.css:374-407`
- Layout template: `internal/server/templates/layout.html:15-23`
- Design tokens: `internal/server/static/tokens.css:63-64`
