---
title: "Dashboard: Show repository list instead of prompt requests"
type: feat
date: 2026-03-08
issue: https://github.com/esnunes/prompter/issues/42
---

# Dashboard: Show repository list instead of prompt requests

## Overview

Replace the dashboard main content area with a list of repositories ordered by most recent activity, instead of the current flat grid of prompt request cards. Users see all their repos at a glance and navigate into the one they want, while the sidebar retains quick access to individual prompts.

## Acceptance Criteria

- [x] Dashboard main content shows repositories, not prompt request cards
- [x] Each repository entry displays its URL (e.g. `github.com/owner/repo`)
- [x] Each repository entry shows the count of active (non-archived) prompt requests and last activity date
- [x] Repositories ordered by most recent `prompt_requests.updated_at` (DESC)
- [x] Clicking a repository navigates to `/github.com/{org}/{repo}/prompt-requests`
- [x] "Go to repository" URL input form remains as-is
- [x] "Show archived" toggle removed from dashboard
- [x] Left sidebar unchanged (shows all prompts across all repos)
- [x] Empty state updated: "No repositories yet. Enter a repository URL above to get started."
- [x] Only repositories with at least one non-deleted prompt request appear

## Technical Approach

### Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| "Activity" metric | `MAX(prompt_requests.updated_at)` | Simpler than joining through messages; `updated_at` is touched on title, status, and issue changes |
| Include archived in activity calc | Yes | Archiving is a recent action; excluding it could hide recently-active repos |
| Exclude deleted PRs | Yes | Deleted PRs are logically gone |
| Show repos with zero non-deleted PRs | No | Avoids ghost entries; repos only exist in DB when a PR is created |
| Repo entry metadata | URL + active PR count + last activity date | Provides enough scannability without clutter |
| Clickable element | Reuse `.card` / `.card-link` pattern | Consistent with existing UI; avoids nested `<a>` issues (see learnings) |

### Gotcha: Nested Anchor Tags

Per `docs/solutions/ui-bugs/nested-anchor-tags-duplicate-rendering.md`, avoid nesting `<a>` elements inside clickable cards. The new repo cards should use a single `<a>` wrapping the entire card with no inner anchors.

## MVP

### 1. New model: `RepositorySummary` (`internal/models/models.go`)

Add a lightweight struct for the dashboard query result:

```go
type RepositorySummary struct {
    ID            int64
    URL           string
    ActivePRCount int
    LastActivity  time.Time
}
```

### 2. New query: `ListRepositorySummaries` (`internal/db/queries.go`)

```sql
SELECT r.id, r.url,
       COUNT(CASE WHEN pr.archived = 0 THEN 1 END) as active_pr_count,
       MAX(pr.updated_at) as last_activity
FROM repositories r
JOIN prompt_requests pr ON pr.repository_id = r.id
WHERE pr.status != 'deleted'
GROUP BY r.id
ORDER BY last_activity DESC
```

Returns `[]models.RepositorySummary`. The `JOIN` ensures repos with zero non-deleted PRs are excluded. The `COUNT(CASE ...)` counts only active (non-archived) PRs for the display count.

### 3. Update handler: `handleDashboard` (`internal/server/handlers.go:53`)

- Replace `dashboardData` struct:
  - Remove `PromptRequests []models.PromptRequest` and `ShowArchived bool`
  - Add `Repositories []models.RepositorySummary`
- Remove `showArchived` query param logic
- Call `ListRepositorySummaries()` instead of `ListPromptRequests()`
- Simplify sidebar fetch: always call `ListPromptRequests(false)` (remove the conditional branch)

### 4. Update template: `dashboard.html` (`internal/server/templates/dashboard.html`)

- Remove the archive toggle (lines 6-10)
- Remove the `{{range .PromptRequests}}` card loop and archive action markup
- Add a `{{range .Repositories}}` loop rendering repo cards:
  - `<a href="/{{.URL}}/prompt-requests" class="card card-link">`
  - Repo URL as card title
  - Meta line: `{{.ActivePRCount}} prompt requests` + `Last activity: {{.LastActivity.Format "Jan 2, 2006"}}`
- Update empty state text to "No repositories yet"
- Keep "Go to repository" form untouched

### 5. Clean up dead code

- Remove `ShowArchived` from `dashboardData` struct
- The `?archived=1` query param is no longer handled on the dashboard route

## File Change Summary

| File | Change |
|------|--------|
| `internal/models/models.go` | Add `RepositorySummary` struct |
| `internal/db/queries.go` | Add `ListRepositorySummaries()` query function |
| `internal/server/handlers.go` | Update `dashboardData` struct and `handleDashboard()` |
| `internal/server/templates/dashboard.html` | Replace card loop with repo list, remove archive toggle |

## References

- Issue: https://github.com/esnunes/prompter/issues/42
- Nested anchor gotcha: `docs/solutions/ui-bugs/nested-anchor-tags-duplicate-rendering.md`
- Existing repo page pattern: `internal/server/templates/repo.html`
- Original repo-centric design: `docs/brainstorms/2026-02-17-repo-centric-restructure-brainstorm.md`
