---
status: pending
priority: p2
issue_id: "002"
tags: [code-review, performance, javascript]
dependencies: []
---

# pendingLoading Map Not Cleared on WebSocket Disconnect

## Problem Statement

The `pendingLoading` object in `client.js` maps ref strings to DOM element references. If the server never responds (network drop, crash, handler panic) or the connection closes before responses arrive, entries are never cleaned up. Each entry holds a strong reference to a DOM element, preventing garbage collection. Over a long session with intermittent connectivity, this map grows without bound. Additionally, buttons remain in a permanently disabled state after disconnect.

## Findings

- **Source**: performance-oracle agent
- **File**: `gotk/client.js`, lines 8 and 71-76
- **Evidence**: `pendingLoading` is populated on command send but only cleaned on response receipt; no cleanup on `ws.onclose`

## Proposed Solutions

### Option A: Clear pendingLoading on disconnect (Recommended)
In `ws.onclose`, iterate `pendingLoading`, restore `originalText`, re-enable elements, and clear the map.
```javascript
ws.onclose = function() {
    Object.keys(pendingLoading).forEach(function(ref) {
        var info = pendingLoading[ref];
        if (info.el) {
            info.el.textContent = info.originalText;
            info.el.disabled = false;
        }
    });
    pendingLoading = {};
    // ... existing reconnect logic
};
```
- **Pros**: Simple, prevents memory leak, restores UI state
- **Cons**: None
- **Effort**: Small
- **Risk**: Low

## Acceptance Criteria
- [ ] pendingLoading is cleared when WebSocket disconnects
- [ ] Loading buttons are restored to their original text/state on disconnect
- [ ] No memory growth over repeated reconnections

## Work Log
| Date | Action | Notes |
|------|--------|-------|
| 2026-03-12 | Created | From code review |
