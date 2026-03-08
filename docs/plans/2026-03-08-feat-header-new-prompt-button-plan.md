---
title: Add "New prompt request" button to the header bar
type: feat
date: 2026-03-08
issue: "#35"
---

# Add "New prompt request" button to the header bar

## Overview

Move the "New prompt request" button from the repo page body into the shared header bar, making it accessible from both repository pages and conversation pages. This eliminates the need to navigate back to the repo page to create a new prompt request.

## Proposed Solution

Template-only changes. Both `repoData` and `conversationData` already have `Org` and `Repo` fields, so no backend modifications are needed. The `layout.html` header already has a `{{block "header-actions" .}}` injection point used by each page.

### Changes by File

#### 1. `internal/server/templates/repo.html`

**Add button to `header-actions` block** (currently contains only the Dashboard link):

```html
{{define "header-actions"}}
<a href="/" class="btn btn-secondary btn-sm">&larr; Dashboard</a>
{{if not .Error}}
<form method="POST" action="/github.com/{{.Org}}/{{.Repo}}/prompt-requests">
  <button type="submit" class="btn btn-primary btn-sm">New prompt request</button>
</form>
{{end}}
{{end}}
```

**Remove the button from the page body** — delete the `<form>` block inside `dashboard-header` (lines 23-27 of current file):

```html
<!-- REMOVE this block -->
{{if not .ShowArchived}}
<form method="POST" action="/github.com/{{.Org}}/{{.Repo}}/prompt-requests">
  <button type="submit" class="btn btn-primary">New prompt request</button>
</form>
{{end}}
```

The `dashboard-header` div retains the repo heading and the "Show archived" toggle.

#### 2. `internal/server/templates/conversation.html`

**Add button to the existing `header-actions` block** (currently contains repo link, badge, and View Issue):

```html
{{define "header-actions"}}
<div style="display:flex;gap:var(--space-3);align-items:center;">
  <a href="/github.com/{{.Org}}/{{.Repo}}/prompt-requests" class="pr-repo">{{.PromptRequest.RepoURL}}</a>
  <span class="badge {{if eq .PromptRequest.Status "published"}}badge-published{{else}}badge-draft{{end}}">{{.PromptRequest.Status}}</span>
  {{if .PromptRequest.IssueURL}}
  <a href="{{deref .PromptRequest.IssueURL}}" target="_blank" class="btn btn-sm btn-secondary">View Issue</a>
  {{end}}
  <form method="POST" action="/github.com/{{.Org}}/{{.Repo}}/prompt-requests" style="margin:0;">
    <button type="submit" class="btn btn-primary btn-sm">New prompt request</button>
  </form>
</div>
{{end}}
```

#### 3. `internal/server/templates/dashboard.html`

**No changes.** Dashboard does not define `header-actions`, so the default empty block from `layout.html` applies. No button appears.

## Acceptance Criteria

- [x] "New prompt request" button appears in the header on the repo page (next to the Dashboard link)
- [x] "New prompt request" button appears in the header on conversation pages (after existing header elements)
- [x] Button does NOT appear on the dashboard page
- [x] Button does NOT appear on the repo page error state (invalid/inaccessible repo)
- [x] Clicking the button creates a new prompt request and redirects to the new conversation (same flow as today)
- [x] The old "New prompt request" button is removed from the repo page body
- [x] Button uses `btn-primary btn-sm` style to match header sizing while standing out as the primary action

## Design Decisions

1. **Hide on repo error state:** When `.Error` is set (repo not found/inaccessible), the button is hidden to prevent creating orphaned prompt requests that would immediately fail cloning.

2. **Always visible regardless of archived state:** The current body button hides when `ShowArchived` is true, but the header button should always be visible. Creating a new prompt request is always valid regardless of what you're browsing. The archived toggle filters the list view, not the creation capability.

3. **Show on archived conversations:** The button creates an independent new prompt request, not something related to the current archived conversation. The label "New prompt request" is clear enough.

4. **Button placement — rightmost:** On the conversation page, the button goes last (rightmost) after View Issue. This puts the primary creation action at the edge of the header, visually separated from the informational elements (repo link, badge).

5. **Style — `btn-primary btn-sm`:** Primary color (blue) for visual prominence as the main action, small size to fit the 52px header height and match existing header button sizing.

## References

- Issue: [#35](https://github.com/esnunes/prompter/issues/35)
- Layout template: `internal/server/templates/layout.html:15-20`
- Repo page template: `internal/server/templates/repo.html`
- Conversation template: `internal/server/templates/conversation.html`
- Handler data structs: `internal/server/handlers.go:74-82` (repoData), `internal/server/handlers.go:178-188` (conversationData)
- Institutional learning: Avoid nesting interactive elements (`<a>` inside `<a>`) — use `<form>` as a sibling, not inside card links
