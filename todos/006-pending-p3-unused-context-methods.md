---
status: pending
priority: p3
issue_id: "006"
tags: [code-review, quality, yagni]
dependencies: []
---

# 8 of 13 Context Methods Are Unused in Real Handlers

## Problem Statement

The send-message handler uses 6 Context methods: `HTML`, `Remove`, `SetValue`, `AttrSet`, `Exec`, `Error`. The remaining 8 methods (`Template`, `Populate`, `Navigate`, `AttrRemove`, `Dispatch`, `Focus`, `Async`, `Render`) have no callers in the integration code. Similarly, `Payload.Int`, `Payload.Float`, and `Payload.Bool` are unused.

These are tested but represent speculative functionality built for hypothetical future commands.

## Findings

- **Source**: code-simplicity-reviewer agent
- **Files**: `gotk/context.go`, `gotk/payload.go`
- **Evidence**: grep shows no calls from `internal/server/` to these methods

## Proposed Solutions

### Option A: Keep for Spec Compliance (Recommended)
The gotk spec defines all these instructions. They will be needed when migrating other interactions (question forms, status polling, navigation).
- **Pros**: Complete spec implementation, ready for next migration steps
- **Cons**: YAGNI for now

### Option B: Remove Unused Methods
Strip to only what send-message needs.
- **Pros**: Minimal code
- **Cons**: Must re-add for each new command migration

## Acceptance Criteria
- [ ] Decision documented on approach

## Work Log
| Date | Action | Notes |
|------|--------|-------|
| 2026-03-12 | Created | From code review |
