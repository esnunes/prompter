---
title: "fix: Independent container scrolling for dashboard and repo pages"
type: fix
date: 2026-03-07
issue: "#31"
---

# Independent Container Scrolling for Dashboard and Repo Pages

On the dashboard and repo pages, the main content area uses page-level scrolling while the sidebar scrolls independently. This creates an inconsistent experience — scrolling the main content moves the entire page. Each panel (sidebar and main content) should scroll independently within its own container.

## Proposed Solution

Add `overflow-y: auto` and `min-height: 0` to `.container` on desktop viewports, scoped to exclude the conversation page which already handles its own scrolling.

**Why `min-height: 0`:** In CSS flexbox, a flex item's `min-height` defaults to `auto`, meaning it cannot shrink below its content size. Without `min-height: 0`, the container may grow beyond its flex parent's height instead of activating the scrollbar. The codebase already applies the horizontal equivalent (`min-width: 0`) on `.container`.

**Why exclude conversation page:** The conversation page has its own scroll container (`.chat-messages` with `overflow-y: auto`). Adding a second scrollable ancestor would create double scrollbars and intercept scroll events at container boundaries. The codebase already uses `.container:has(.conversation-wrapper)` for conversation-specific overrides (style.css:393), so this pattern is established.

## Acceptance Criteria

- [x] Header remains fixed/sticky at the top of the viewport
- [x] Left sidebar (prompt list) continues to scroll independently
- [x] Main content on dashboard and repo pages scrolls within its own container
- [x] Scrolling one panel does not affect the other
- [x] On mobile (sidebar hidden), main content uses normal page-level scrolling
- [x] Conversation page is not affected by this change
- [x] No double scrollbars appear on any page
- [x] Empty state pages (few or no items) show no unnecessary scrollbar

## Implementation

### `internal/server/static/style.css`

Add a desktop-only media query at line ~75 (after the existing `.container` rule):

```css
/* Desktop: independent container scrolling for dashboard/repo pages */
@media (min-width: 769px) {
  .container {
    overflow-y: auto;
    min-height: 0;
  }

  /* Conversation page already handles its own scrolling */
  .container:has(.conversation-wrapper) {
    overflow-y: hidden;
  }
}
```

**That's it.** One CSS rule block. No template, handler, or JS changes needed.

### Why this works

1. `.app-layout` already constrains height: `height: calc(100vh - var(--header-height))`
2. `.container` is a flex child with `flex: 1` — its height is bounded by the parent
3. `min-height: 0` allows the container to shrink below its content size (enabling overflow)
4. `overflow-y: auto` creates the scroll container — scrollbar appears only when content overflows
5. The `@media (min-width: 769px)` matches the existing mobile breakpoint (`max-width: 768px` hides sidebar)
6. `:has(.conversation-wrapper)` resets overflow for the conversation page using an established pattern

## Test Plan

- [ ] Dashboard with many prompt request cards — verify container scrolls, page does not
- [ ] Dashboard with few/no cards (empty state) — verify no unnecessary scrollbar
- [ ] Repo page with many cards — same as dashboard
- [ ] Conversation page — verify NO change: chat scrolls internally, no double scroll
- [ ] Scroll sidebar while main content is scrolled — verify independent scroll
- [ ] Scroll main content while sidebar is scrolled — verify independent scroll
- [ ] Resize browser from desktop to mobile — verify scroll behavior transitions correctly
- [ ] Narrow desktop (769px-900px) — verify no horizontal overflow issues

## References

- Issue: [#31](https://github.com/esnunes/prompter/issues/31)
- Existing container styles: `internal/server/static/style.css:66-75`
- Existing conversation override: `internal/server/static/style.css:393-397`
- Mobile breakpoint: `internal/server/static/style.css:629`
