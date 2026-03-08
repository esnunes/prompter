---
title: "Fix blinking on polling-based status indicators"
type: fix
date: 2026-03-08
issue: https://github.com/esnunes/prompter/issues/34
---

# Fix Blinking on Polling-Based Status Indicators

## Overview

All polling-based status indicators (cloning, pulling, processing) visibly blink every poll cycle because HTMX's `hx-swap="outerHTML"` destroys and recreates the entire DOM element. This resets CSS animations (spinner), flickers text, and causes the elapsed timer to jump. The fix uses the HTMX idiomorph extension to diff/patch the DOM instead of replacing it wholesale.

## Problem Statement

Every 2-3 seconds, HTMX polls the server for status updates. When the response arrives, `outerHTML` swap replaces the entire element — even when the HTML content is identical. This causes:

1. **Spinner reset** — The CSS `@keyframes spin` animation restarts from 0deg on every swap
2. **Text flicker** — Status text briefly disappears and reappears during DOM replacement
3. **Timer jump** — The elapsed timer (managed by `setInterval` in `app.js`) shows a momentary visual glitch as the element is destroyed and recreated
4. **Sidebar flash** — The entire sidebar (`<aside>`) is replaced every 3s, resetting scroll position and hover states

## Proposed Solution

**Use the HTMX idiomorph extension** (`hx-swap="morph:outerHTML"`) on all polling-triggered swaps. Idiomorph performs a DOM diff and only patches what actually changed, preserving:

- CSS animations (spinner keeps rotating)
- Element identity (no destroy/create cycle)
- Scroll position (sidebar)
- JS state (timer interval continues uninterrupted)

When the status hasn't changed between polls, idiomorph detects no differences and leaves the DOM untouched — zero visual disruption.

When the status transitions (e.g., processing → responded), idiomorph morphs the DOM to the new structure, which is the correct and expected behavior.

### Why Not Alternatives?

| Alternative | Drawback |
|---|---|
| `innerHTML` swap with stable wrapper | Breaks state transitions that need to change the wrapper's attributes (`data-started-at`, `hx-get` removal) |
| `htmx:beforeSwap` content comparison | Requires custom JS string comparison; fragile, doesn't help when content changes slightly |
| Purely client-side spinner/timer | Would require significant JS refactoring and duplicates state management |

## Technical Approach

### Files to Modify

| File | Change |
|---|---|
| `internal/server/static/idiomorph-ext.min.js` | **New file** — Vendor the idiomorph HTMX extension |
| `internal/server/templates/layout.html` | Add `<script>` for idiomorph, add `hx-ext="morph"` to `<body>` |
| `internal/server/templates/status_fragment.html` | Change polling swaps to `hx-swap="morph:outerHTML"` |
| `internal/server/templates/conversation.html` | Change polling swaps to `hx-swap="morph:outerHTML"` |
| `internal/server/templates/sidebar.html` | Change swap to `hx-swap="morph:outerHTML"`, add `id` to list items |
| `internal/server/handlers.go` | Update inline HTML at line 377 to use `morph:outerHTML` |
| `internal/server/server.go` | Register `idiomorph-ext.min.js` in static file serving (already handled by `go:embed` on the `static` dir) |

### Implementation Steps

#### Step 1: Vendor the idiomorph extension

Download the official HTMX idiomorph extension (`idiomorph-ext.min.js`) compatible with HTMX 2.0.4 from the HTMX extensions repository. Place it in `internal/server/static/`.

#### Step 2: Load idiomorph in layout.html

Add the script tag after `htmx.min.js` and declare `hx-ext="morph"` on `<body>`:

```html
<script src="/static/htmx.min.js"></script>
<script src="/static/idiomorph-ext.min.js"></script>
<!-- ... -->
<body hx-ext="morph">
```

Placing `hx-ext="morph"` on `<body>` enables the morph swap mode globally. Individual elements opt in by specifying `hx-swap="morph:outerHTML"`.

#### Step 3: Update status_fragment.html

Change **only the polling-triggered** `hx-swap` attributes (lines 5, 12, 19):

```html
<!-- Before -->
hx-swap="outerHTML"

<!-- After -->
hx-swap="morph:outerHTML"
```

**Do NOT change** the Cancel/Retry/Resend form swaps (lines 28, 39, 55) — those are one-shot user actions that don't cause flicker.

#### Step 4: Update conversation.html

Change the polling swaps at lines 110 and 140:

```html
<!-- Before -->
hx-swap="outerHTML"

<!-- After -->
hx-swap="morph:outerHTML"
```

Same rule: only polling triggers, not action forms.

#### Step 5: Update sidebar.html

Change the sidebar swap (line 4):

```html
<!-- Before -->
hx-swap="outerHTML"

<!-- After -->
hx-swap="morph:outerHTML"
```

Add stable `id` attributes to each `<li>` for correct morph matching during list reordering:

```html
<!-- Before -->
<li class="prompt-list-item ...">

<!-- After -->
<li id="prompt-{{.ID}}" class="prompt-list-item ...">
```

Without IDs, idiomorph uses structural matching which can produce incorrect diffs when sidebar items reorder (e.g., a processing item floats to the top).

#### Step 6: Update inline HTML in handlers.go

Update line 377 in `handleSendMessage`:

```go
// Before
fmt.Fprintf(w, `... hx-swap="outerHTML" ...`)

// After
fmt.Fprintf(w, `... hx-swap="morph:outerHTML" ...`)
```

This is the most commonly exercised path — user sends a message and watches the "Thinking..." indicator.

### Edge Cases & Risks

#### The "responded" state DOM relocation (handlers.go:638-663)

When processing completes, the server returns a completely different DOM structure: a hidden `<div id="repo-status" style="display:none">` containing message fragments plus an inline `<script>` that moves children to `#conversation` and removes the div.

**Risk:** Idiomorph will morph the processing div into this hidden div. The inline script then runs and relocates content. This should work because idiomorph replaces the inner content and HTMX still executes inline scripts, but this transition needs manual testing.

**Mitigation:** If morph interferes with the script-based DOM relocation, this specific transition can fall back to regular `outerHTML` by having the handler set `HX-Reswap: outerHTML` response header, which overrides the element's `hx-swap` attribute.

#### htmx:afterSwap event compatibility

`app.js` listens for `htmx:afterSwap` to trigger `renderMarkdown()` and `updateElapsedTimers()`. The idiomorph extension for HTMX 2.x fires `htmx:afterSwap` after morph operations, so this should work without changes. Verify during testing.

#### fadeIn animation on initial render

The `.repo-status.htmx-added` animation fires when an element is first inserted. With morph, subsequent polls won't re-trigger `htmx-added` (because the element isn't being re-added), which is the desired behavior — entrance animation on first appearance, no flicker on subsequent polls.

## Acceptance Criteria

- [ ] Spinner rotates continuously without resetting during cloning/pulling/processing polls
- [ ] "Cloning repository...", "Pulling latest changes...", "Thinking..." text remains visually stable
- [ ] Elapsed timer updates smoothly every second without jumps
- [ ] Sidebar updates without visible flash or scroll position reset
- [ ] State transitions (cloning→ready, processing→responded, processing→cancelled) still work correctly
- [ ] Cancel button remains functional during processing
- [ ] Published messages appear correctly after processing→responded transition
- [ ] Sidebar active state and unread indicators update correctly
- [ ] No console errors from idiomorph extension
- [ ] fadeIn entrance animation still works on initial status element appearance

## References

- HTMX idiomorph extension: https://htmx.org/extensions/idiomorph/
- Current HTMX version: 2.0.4
- Status polling handler: `internal/server/handlers.go:590` (`handleRepoStatus`)
- Inline status HTML: `internal/server/handlers.go:377` (`handleSendMessage`)
- Status fragment template: `internal/server/templates/status_fragment.html`
- Sidebar template: `internal/server/templates/sidebar.html`
- Spinner CSS: `internal/server/static/style.css:949`
- Timer JS: `internal/server/static/app.js:15` (`updateElapsedTimers`)
- Related learning: CSS `htmx-added` class scoping prevents re-animation on existing elements
