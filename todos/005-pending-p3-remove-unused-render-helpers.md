---
status: pending
priority: p3
issue_id: "005"
tags: [code-review, quality, yagni]
dependencies: []
---

# render.go Contains Unused/Duplicate Functions

## Problem Statement

`gotk/render.go` has three issues:
1. `RenderPage` and `RenderFragment` have identical implementations
2. `RenderString` uses a custom `stringWriter` that duplicates `bytes.Buffer`
3. None of these functions are called from the integration code

This adds ~90 LOC of untested-in-production code.

## Findings

- **Source**: code-simplicity-reviewer agent
- **File**: `gotk/render.go` (51 LOC), `gotk/render_test.go` (39 LOC)
- **Evidence**: No calls to RenderPage/RenderFragment/RenderString from handlers.go or server.go

## Proposed Solutions

### Option A: Keep for Spec Compliance
The gotk UI spec calls for these helpers. Keep them for when other commands are migrated.
- **Pros**: Ready for future use
- **Cons**: YAGNI
- **Effort**: None

### Option B: Remove and Re-Add When Needed
Delete render.go and render_test.go, reducing package surface.
- **Pros**: Simpler package, no dead code
- **Cons**: Must re-create later
- **Effort**: Small

### Option C: Merge Duplicates (Compromise)
Remove `RenderFragment` (alias for `RenderPage`), keep `RenderPage` and `RenderString`.
- **Pros**: Reduces duplication while keeping useful helpers
- **Cons**: Still has unused code
- **Effort**: Small

## Acceptance Criteria
- [ ] No duplicate function implementations
- [ ] Tests still pass

## Work Log
| Date | Action | Notes |
|------|--------|-------|
| 2026-03-12 | Created | From code review |
