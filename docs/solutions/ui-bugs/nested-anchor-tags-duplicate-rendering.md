---
title: "Nested Anchor Tags Causing Duplicate Card Rendering on Dashboard"
date: 2026-02-18
category: ui-bugs
tags:
  - html
  - semantic-html
  - browser-rendering
  - go-templates
  - htmx
  - dashboard
component: internal/server/templates/dashboard.html
severity: high
symptoms:
  - "Duplicate card elements appearing on dashboard"
  - "Each prompt request renders two card-shaped boxes instead of one"
  - "Second card appears empty with repo name and metadata below it"
---

# Nested Anchor Tags Causing Duplicate Card Rendering

## Problem

The dashboard showed two card-shaped boxes for each prompt request — one containing the title/badge and one empty — followed by the repo name and metadata outside both cards.

## Root Cause

The dashboard template had nested `<a>` tags: an outer `<a class="card card-link">` wrapping the entire card, with an inner `<a>` for the clickable repository name.

```html
<!-- BROKEN: nested anchors -->
<a href="/repo/prompt-requests/1" class="card card-link">
  <div class="pr-title">Title <span class="badge">DRAFT</span></div>
  <div class="pr-repo">
    <a href="/repo/prompt-requests" onclick="event.stopPropagation();">
      github.com/org/repo
    </a>
  </div>
  <div class="pr-meta">0 messages  Feb 18, 2026</div>
</a>
```

The HTML spec forbids nested `<a>` elements. When browsers encounter this, they auto-close the outer `<a>` before starting the inner one, splitting the card into two separate elements. The `.card` CSS styling (background, border, padding, shadow) applied to both fragments, producing the duplicate appearance.

What the browser actually parsed:

```html
<!-- Browser's corrected DOM -->
<a href="/repo/prompt-requests/1" class="card card-link">
  <div class="pr-title">Title <span class="badge">DRAFT</span></div>
</a>
<div class="pr-repo">
  <a href="/repo/prompt-requests">github.com/org/repo</a>
</div>
<div class="pr-meta">0 messages  Feb 18, 2026</div>
```

## Solution

Replace the inner `<a>` with a `<span>` that uses JavaScript for navigation:

```html
<!-- FIXED: no nested anchors -->
<a href="/repo/prompt-requests/1" class="card card-link">
  <div class="pr-title">Title <span class="badge">DRAFT</span></div>
  <div class="pr-repo">
    <span class="pr-repo-link"
          onclick="event.preventDefault(); event.stopPropagation(); window.location.href='/repo/prompt-requests';">
      github.com/org/repo
    </span>
  </div>
  <div class="pr-meta">0 messages  Feb 18, 2026</div>
</a>
```

With CSS to make the span look and behave like a link:

```css
.pr-repo-link {
  color: var(--color-primary);
  cursor: pointer;
}

.pr-repo-link:hover {
  text-decoration: underline;
}
```

## Why the Symptom Was Confusing

- The duplicate cards appeared styled identically to real cards (background, border, shadow) because `.card + .card` margin rules applied to both fragments
- No console errors — the browser silently "fixed" the invalid HTML
- Both fragments were partially clickable, masking the structural issue
- The visual symptom (duplicate cards) didn't obviously point to an HTML nesting issue

## Prevention

**General rule:** Never nest interactive HTML elements inside each other:
- `<a>` inside `<a>`
- `<button>` inside `<a>`
- `<a>` inside `<button>`

**When you need a clickable card with an inner clickable element:**
1. Use `<span>` with `onclick` + CSS (as in this fix)
2. Use `<div>` with `onclick` as the outer container instead of `<a>`
3. Use CSS `::after` pseudo-element overlay for the card click area

**Code review checklist for Go templates:**
- [ ] Verify no `<a>` tags nested inside other `<a>` tags
- [ ] Check that clickable cards (`card-link`) don't contain inner `<a>` or `<button>` elements
- [ ] Inspect actual DOM in browser DevTools to confirm structure matches template intent

## Related

- [Unified conversation view solution](./published-prompt-request-404-continue-conversation.md) — covers HTMX template patterns in this codebase
