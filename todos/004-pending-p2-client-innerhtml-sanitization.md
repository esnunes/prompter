---
status: pending
priority: p2
issue_id: "004"
tags: [code-review, security, javascript]
dependencies: []
---

# innerHTML Used Without Client-Side Sanitization

## Problem Statement

The gotk client's `applyHTML` function sets `innerHTML` directly from server-provided `ins.html` strings without any client-side sanitization (DOMPurify). The security model relies entirely on the server producing safe HTML. While the server currently escapes user content via `template.HTMLEscapeString`, this is a defense-in-depth gap — if any server-side path ever passes unsanitized content (e.g., Claude AI responses, GitHub data), it would result in XSS.

The existing HTMX code path already uses `DOMPurify.sanitize(marked.parse(...))` for assistant message rendering, but gotk bypasses this entirely.

## Findings

- **Source**: security-sentinel agent
- **File**: `gotk/client.js`, lines 153, 165, 205
- **Evidence**: `target.innerHTML = ins.html;` with no sanitization

## Proposed Solutions

### Option A: Sanitize in applyHTML (Recommended)
```javascript
function applyHTML(ins) {
    var target = document.querySelector(ins.target);
    if (!target) return;
    var safeHTML = typeof DOMPurify !== "undefined"
        ? DOMPurify.sanitize(ins.html)
        : ins.html;
    // ... apply safeHTML instead of ins.html
}
```
- **Pros**: Defense in depth, uses already-loaded DOMPurify
- **Cons**: Small perf overhead; may strip intentional HTML attributes
- **Effort**: Small
- **Risk**: Low — DOMPurify is already on the page

### Option B: Server-side only (Status Quo)
Trust the server completely, document the trust model.
- **Pros**: Zero overhead
- **Cons**: No defense in depth
- **Effort**: None
- **Risk**: Medium — any future server bug = XSS

## Acceptance Criteria
- [ ] HTML instructions are sanitized before DOM insertion
- [ ] Existing functionality (processing indicators, messages) still works
- [ ] No double-encoding issues

## Work Log
| Date | Action | Notes |
|------|--------|-------|
| 2026-03-12 | Created | From code review |
