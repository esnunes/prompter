---
status: pending
priority: p2
issue_id: "003"
tags: [code-review, performance, golang]
dependencies: []
---

# conn.writeJSON Uses context.Background() — Write Can Block Indefinitely

## Problem Statement

`conn.writeJSON` uses `context.Background()` for WebSocket writes. If the remote client disconnects but the TCP connection is half-open, `Write` can block for the full TCP timeout (potentially minutes). During this time the goroutine holds `c.mu`, so any concurrent `Push` call also blocks indefinitely.

## Findings

- **Source**: performance-oracle agent
- **File**: `gotk/conn.go`, line 47
- **Evidence**: `c.ws.Write(context.Background(), websocket.MessageText, data)` — no timeout, no cancellation

## Proposed Solutions

### Option A: Write Deadline Context (Recommended)
Use a context with a write deadline:
```go
func (c *Conn) writeJSON(v any) error {
    data, err := json.Marshal(v)
    if err != nil { return err }
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    return c.ws.Write(ctx, websocket.MessageText, data)
}
```
- **Pros**: Simple, prevents indefinite blocking
- **Cons**: Hard timeout may not suit all scenarios
- **Effort**: Small
- **Risk**: Low

### Option B: Store Request Context in Conn
Pass the HTTP request context into `Conn` during construction and derive write contexts from it.
- **Pros**: Lifecycle-aware
- **Cons**: More plumbing
- **Effort**: Medium
- **Risk**: Low

## Acceptance Criteria
- [ ] WebSocket writes have a bounded timeout
- [ ] Half-open connections don't block indefinitely
- [ ] Tests still pass

## Work Log
| Date | Action | Notes |
|------|--------|-------|
| 2026-03-12 | Created | From code review |
