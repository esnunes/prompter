---
status: pending
priority: p1
issue_id: "001"
tags: [code-review, security, websocket]
dependencies: []
---

# WebSocket Origin Validation Disabled (CSWSH)

## Problem Statement

`InsecureSkipVerify: true` in `gotk/websocket.go` disables Origin header checks on WebSocket upgrade. Any website open in the user's browser can connect to `ws://localhost:<port>/ws` and issue commands — a Cross-Site WebSocket Hijacking (CSWSH) vulnerability. Even for a localhost app, this is exploitable because any page in any browser tab can reach localhost.

An attacker-controlled webpage could connect, send `send-message` commands with arbitrary payloads, trigger Claude CLI executions, and read back responses.

## Findings

- **Source**: security-sentinel agent
- **File**: `gotk/websocket.go`, line 29
- **Evidence**: `InsecureSkipVerify: true` in `websocket.AcceptOptions`

## Proposed Solutions

### Option A: Explicit Origin Allowlist (Recommended)
Replace `InsecureSkipVerify` with `OriginPatterns`:
```go
ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
    OriginPatterns: []string{"localhost:*", "127.0.0.1:*", "[::1]:*"},
})
```
- **Pros**: Simple, secure, still works for local development
- **Cons**: None significant
- **Effort**: Small
- **Risk**: Low

### Option B: Configurable Origin Check
Add an `InsecureOrigin bool` option to Mux that defaults to false, only set true in tests.
- **Pros**: Flexible
- **Cons**: More API surface
- **Effort**: Small
- **Risk**: Low

## Acceptance Criteria
- [ ] WebSocket connections from non-localhost origins are rejected
- [ ] WebSocket connections from localhost still work
- [ ] Tests still pass

## Work Log
| Date | Action | Notes |
|------|--------|-------|
| 2026-03-12 | Created | From code review |
