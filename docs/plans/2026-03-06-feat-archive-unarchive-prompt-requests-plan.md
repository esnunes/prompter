---
title: "feat: Archive and unarchive prompt requests"
type: feat
date: 2026-03-06
issue: https://github.com/esnunes/prompter/issues/27
brainstorm: docs/brainstorms/2026-03-06-archive-unarchive-brainstorm.md
---

# feat: Archive and Unarchive Prompt Requests

## Overview

Add the ability to archive and unarchive prompt requests so users can declutter their prompt list without permanently deleting anything. Archived prompts disappear from the default view but remain accessible through a toggle on list pages. The left sidebar always shows active prompts only. Archiving from the conversation view keeps the user on the page with a banner.

## Problem Statement / Motivation

Over time the prompt list becomes cluttered with old or abandoned requests. There's no way to clean up without losing prompts entirely. Users need to hide prompts they no longer actively need while keeping them accessible for later revisit.

## Proposed Solution

A boolean `archived` column on `prompt_requests` (preserving the existing `status` field for seamless restore). Archive/unarchive actions on list page rows and in the conversation's revision sidebar. A "Show archived" toggle switch on dashboard and repo list pages, driven by a `?archived=1` URL query parameter.

## Technical Approach

### Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Schema | Boolean `archived` column (INTEGER DEFAULT 0) | Preserves status (draft/published) for restore on unarchive |
| Confirmation | Native `confirm()` | YAGNI — simple, accessible, no custom modal |
| List page filter | Toggle switch via `?archived=1` query param | Survives navigation, linkable, no client-side state |
| Sidebar behavior | Active prompts only, always | Sidebar is for quick nav, not archive management |
| Conversation archive location | Revision sidebar (right panel) | Natural home for prompt-level actions |
| Post-archive (conversation) | Stay on page, show archived banner | User may want to unarchive immediately |
| Archive on list pages | Icon button per row (always visible) | Direct one-click access |
| Unarchive confirmation | None | Low-risk, easily reversible |
| Archive handler response (list) | Full page redirect | Consistent with `handleDelete` pattern |
| `updated_at` on archive/unarchive | Do NOT update | Prompt returns to natural chronological position |
| Archive while processing | Processing continues | Response is saved; user sees it on revisit or unarchive |

### Architecture

```
  Archive from list page:
  ┌─────────┐    ┌──────────┐    ┌─────────────┐    ┌──────────┐
  │ onclick  │───>│ confirm()│───>│ POST        │───>│ Redirect │
  │ on span  │    │ dialog   │    │ .../archive │    │ back to  │
  │          │    │          │    │ (set col=1) │    │ list page│
  └─────────┘    └──────────┘    └─────────────┘    └──────────┘

  Archive from conversation:
  ┌─────────┐    ┌──────────┐    ┌─────────────┐    ┌─────────────────┐
  │ onclick  │───>│ confirm()│───>│ POST        │───>│ HTMX swap:      │
  │ in rev   │    │ dialog   │    │ .../archive │    │ show banner +   │
  │ sidebar  │    │          │    │             │    │ update sidebar  │
  └─────────┘    └──────────┘    └─────────────┘    └─────────────────┘

  Toggle archived on list page:
  ┌──────────┐    ┌───────────────────────┐
  │ checkbox │───>│ Navigate to           │
  │ onchange │    │ ?archived=1 or base   │
  └──────────┘    └───────────────────────┘
```

### DB Schema Change

```sql
-- Migration (idempotent, follows existing ALTER TABLE pattern in db.go:83)
ALTER TABLE prompt_requests ADD COLUMN archived INTEGER NOT NULL DEFAULT 0;
```

Add to `internal/db/db.go` after the existing `last_viewed_at` migration (line 83):

```go
// Migration: add archived flag for archiving prompt requests.
db.Exec(`ALTER TABLE prompt_requests ADD COLUMN archived INTEGER NOT NULL DEFAULT 0`)
```

### Model Change

```go
// internal/models/models.go — add to PromptRequest struct
type PromptRequest struct {
    // ... existing fields ...
    Archived bool // new field
}
```

### Query Changes

**Update `listPromptRequestsQuery`** to include `archived` in SELECT and accept filtering:

```go
// internal/db/queries.go

const listPromptRequestsQuery = `SELECT pr.id, pr.repository_id, pr.title, pr.status, pr.session_id,
                pr.issue_number, pr.issue_url, pr.created_at, pr.updated_at,
                r.url,
                (SELECT COUNT(*) FROM messages WHERE prompt_request_id = pr.id) as message_count,
                (SELECT COUNT(*) FROM revisions WHERE prompt_request_id = pr.id) as revision_count,
                pr.last_viewed_at,
                (SELECT MAX(created_at) FROM messages WHERE prompt_request_id = pr.id AND role = 'assistant') as latest_assistant_at,
                pr.archived
         FROM prompt_requests pr
         JOIN repositories r ON r.id = pr.repository_id
         WHERE pr.status != 'deleted'`
```

**Update `scanPromptRequest`** to scan the new column:

```go
func scanPromptRequest(rows *sql.Rows) (models.PromptRequest, error) {
    var pr models.PromptRequest
    var createdAt, updatedAt string
    var lastViewedAt, latestAssistantAt *string
    var archived int
    if err := rows.Scan(&pr.ID, &pr.RepositoryID, &pr.Title, &pr.Status, &pr.SessionID,
        &pr.IssueNumber, &pr.IssueURL, &createdAt, &updatedAt, &pr.RepoURL,
        &pr.MessageCount, &pr.RevisionCount, &lastViewedAt, &latestAssistantAt,
        &archived); err != nil {
        return pr, err
    }
    pr.Archived = archived != 0
    // ... existing time parsing ...
    return pr, nil
}
```

**Update `ListPromptRequests` and `ListPromptRequestsByRepoURL`** to accept an `archived` filter parameter:

```go
func (q *Queries) ListPromptRequests(archivedOnly bool) ([]models.PromptRequest, error) {
    archivedVal := 0
    if archivedOnly {
        archivedVal = 1
    }
    rows, err := q.db.Query(
        listPromptRequestsQuery+` AND pr.archived = ? ORDER BY
            CASE WHEN pr.status = 'draft' THEN 0 ELSE 1 END ASC,
            pr.updated_at DESC`,
        archivedVal,
    )
    // ... existing scan loop ...
}

func (q *Queries) ListPromptRequestsByRepoURL(repoURL string, archivedOnly bool) ([]models.PromptRequest, error) {
    archivedVal := 0
    if archivedOnly {
        archivedVal = 1
    }
    rows, err := q.db.Query(
        listPromptRequestsQuery+` AND r.url = ? AND pr.archived = ? ORDER BY
            CASE WHEN pr.status = 'draft' THEN 0 ELSE 1 END ASC,
            pr.updated_at DESC`,
        repoURL, archivedVal,
    )
    // ... existing scan loop ...
}
```

**Add archive/unarchive query methods:**

```go
func (q *Queries) ArchivePromptRequest(id int64) error {
    _, err := q.db.Exec(
        `UPDATE prompt_requests SET archived = 1 WHERE id = ?`, id,
    )
    return err
}

func (q *Queries) UnarchivePromptRequest(id int64) error {
    _, err := q.db.Exec(
        `UPDATE prompt_requests SET archived = 0 WHERE id = ?`, id,
    )
    return err
}
```

**Update `GetPromptRequest`** to also scan the `archived` column:

```go
// The existing GetPromptRequest query needs `pr.archived` added to its SELECT.
// It uses a different query string from listPromptRequestsQuery, so update it directly.
```

### Handler Changes

**New route handlers:**

```go
// internal/server/handlers.go

func (s *Server) handleArchive(w http.ResponseWriter, r *http.Request) {
    org := r.PathValue("org")
    repoName := r.PathValue("repo")
    id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
    if err != nil {
        http.Error(w, "invalid id", http.StatusBadRequest)
        return
    }

    if err := s.queries.ArchivePromptRequest(id); err != nil {
        http.Error(w, "failed to archive", http.StatusInternalServerError)
        return
    }

    // If HTMX request (from conversation page), return the archived banner fragment
    if r.Header.Get("HX-Request") == "true" {
        pr, _ := s.queries.GetPromptRequest(id)
        s.renderFragment(w, "archive_banner_fragment.html", archiveBannerData{
            Org:           org,
            Repo:          repoName,
            PromptRequest: pr,
        })
        return
    }

    // Otherwise (from list page), redirect back
    referer := r.Header.Get("Referer")
    if referer == "" {
        referer = fmt.Sprintf("/github.com/%s/%s/prompt-requests", org, repoName)
    }
    http.Redirect(w, r, referer, http.StatusSeeOther)
}

func (s *Server) handleUnarchive(w http.ResponseWriter, r *http.Request) {
    org := r.PathValue("org")
    repoName := r.PathValue("repo")
    id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
    if err != nil {
        http.Error(w, "invalid id", http.StatusBadRequest)
        return
    }

    if err := s.queries.UnarchivePromptRequest(id); err != nil {
        http.Error(w, "failed to unarchive", http.StatusInternalServerError)
        return
    }

    // If HTMX request (from conversation page), return empty banner (removes it)
    if r.Header.Get("HX-Request") == "true" {
        // Return an empty div that replaces the banner
        w.Header().Set("Content-Type", "text/html")
        w.Write([]byte(`<div id="archive-banner"></div>`))
        return
    }

    // Otherwise (from list page), redirect back
    referer := r.Header.Get("Referer")
    if referer == "" {
        referer = fmt.Sprintf("/github.com/%s/%s/prompt-requests", org, repoName)
    }
    http.Redirect(w, r, referer, http.StatusSeeOther)
}
```

**Register routes** in `internal/server/server.go`:

```go
mux.HandleFunc("POST /github.com/{org}/{repo}/prompt-requests/{id}/archive", s.handleArchive)
mux.HandleFunc("POST /github.com/{org}/{repo}/prompt-requests/{id}/unarchive", s.handleUnarchive)
```

**Update `handleDashboard`** to read `?archived=1` query param:

```go
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
    showArchived := r.URL.Query().Get("archived") == "1"
    prs, err := s.queries.ListPromptRequests(showArchived)
    // ...
    // Sidebar always gets active prompts
    sidebarPRs := prs
    if showArchived {
        sidebarPRs, _ = s.queries.ListPromptRequests(false)
    }
    sidebar := s.buildSidebar(sidebarPRs, "all", 0)
    s.renderPage(w, "dashboard.html", dashboardData{
        basePageData:   basePageData{Sidebar: sidebar},
        PromptRequests: prs,
        ShowArchived:   showArchived,
    })
}
```

**Update `handleRepoPage`** similarly:

```go
func (s *Server) handleRepoPage(w http.ResponseWriter, r *http.Request) {
    // ...
    showArchived := r.URL.Query().Get("archived") == "1"
    prs, err := s.queries.ListPromptRequestsByRepoURL(repoURL, showArchived)
    // ...
    sidebarPRs := prs
    if showArchived {
        sidebarPRs, _ = s.queries.ListPromptRequestsByRepoURL(repoURL, false)
    }
    sidebar := s.buildSidebar(sidebarPRs, "repo", 0)
    // ...
    s.renderPage(w, "repo.html", repoData{
        // ...
        ShowArchived: showArchived,
    })
}
```

**Update `handleShow`** to pass archived state to template:

```go
// No changes to handleShow's logic — it already fetches by ID regardless of archived state.
// The template will read pr.Archived to conditionally show the banner.
// The sidebar query already filters by active (false) since handleShow calls
// ListPromptRequestsByRepoURL which now requires the archived param — pass false.
```

**Update `handleSidebarFragment`** to always pass `false` for archived:

```go
// In handleSidebarFragment, update calls:
prs, _ = s.queries.ListPromptRequestsByRepoURL(repoURL, false)
// and
prs, _ = s.queries.ListPromptRequests(false)
```

**Update data structs:**

```go
type dashboardData struct {
    basePageData
    PromptRequests []models.PromptRequest
    ShowArchived   bool
}

type repoData struct {
    basePageData
    RepoURL        string
    Org            string
    Repo           string
    Error          string
    PromptRequests []models.PromptRequest
    ShowArchived   bool
}
```

### Template Changes

**Dashboard archive toggle and row actions** (`dashboard.html`):

```html
<div class="dashboard-header">
  <h2>Dashboard</h2>
  <label class="archive-toggle">
    <input type="checkbox" {{if .ShowArchived}}checked{{end}}
           onchange="window.location.href = this.checked ? '/?archived=1' : '/'">
    Show archived
  </label>
</div>

{{range .PromptRequests}}
<a href="/{{.RepoURL}}/prompt-requests/{{.ID}}" class="card card-link">
  <div class="pr-title">
    {{if .Title}}{{.Title}}{{else}}Untitled{{end}}
    <span class="badge {{if eq .Status "published"}}badge-published{{else}}badge-draft{{end}}">{{.Status}}</span>
    {{if .Archived}}<span class="badge badge-archived">archived</span>{{end}}
  </div>
  <div class="pr-repo">...</div>
  <div class="pr-meta">
    <span>{{.MessageCount}} messages</span>
    {{if gt .RevisionCount 0}}<span>{{.RevisionCount}} revisions</span>{{end}}
    <span>{{.CreatedAt.Format "Jan 2, 2006"}}</span>
  </div>
  {{if $.ShowArchived}}
  <span class="card-action card-action-unarchive" role="button" tabindex="0"
        aria-label="Unarchive prompt"
        onclick="event.preventDefault(); event.stopPropagation(); fetch('/{{.RepoURL}}/prompt-requests/{{.ID}}/unarchive', {method:'POST'}).then(function(){location.reload()});"
        onkeydown="if(event.key==='Enter'||event.key===' '){this.click();}">
    <!-- unarchive SVG icon (box with up arrow) -->
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
      <path d="M2 5h12v8a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V5z"/>
      <path d="M8 11V7"/>
      <path d="M6 9l2-2 2 2"/>
    </svg>
  </span>
  {{else}}
  <span class="card-action card-action-archive" role="button" tabindex="0"
        aria-label="Archive prompt"
        onclick="event.preventDefault(); event.stopPropagation(); var msg='Archive this prompt request?'; {{if .IssueURL}}msg+=' The linked GitHub issue will remain open.';{{end}} if(confirm(msg)){fetch('/{{.RepoURL}}/prompt-requests/{{.ID}}/archive', {method:'POST'}).then(function(){location.reload()});}"
        onkeydown="if(event.key==='Enter'||event.key===' '){this.click();}">
    <!-- archive SVG icon (box with down arrow) -->
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
      <path d="M2 5h12v8a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V5z"/>
      <path d="M8 7v4"/>
      <path d="M6 9l2 2 2-2"/>
    </svg>
  </span>
  {{end}}
</a>
{{end}}
```

**Note:** The `<span>` with `role="button"` and `tabindex="0"` follows the learnings from `docs/solutions/ui-bugs/nested-anchor-tags-duplicate-rendering.md` — never nest `<a>` or `<button>` inside `<a>`. The `onkeydown` handler ensures keyboard accessibility.

**Repo page** (`repo.html`): Same pattern as dashboard but with repo-scoped URLs:

```html
<div class="dashboard-header">
  <h2>{{.RepoURL}}</h2>
  <div style="display:flex;gap:var(--space-3);align-items:center;">
    <label class="archive-toggle">
      <input type="checkbox" {{if .ShowArchived}}checked{{end}}
             onchange="window.location.href = this.checked ? '?archived=1' : window.location.pathname">
      Show archived
    </label>
    {{if not .ShowArchived}}
    <form method="POST" action="/github.com/{{.Org}}/{{.Repo}}/prompt-requests">
      <button type="submit" class="btn btn-primary">New prompt request</button>
    </form>
    {{end}}
  </div>
</div>
```

**Empty state messages** for both dashboard and repo pages:

```html
{{if .PromptRequests}}
  <!-- render cards -->
{{else}}
  {{if .ShowArchived}}
  <p class="text-secondary text-center">No archived prompt requests.</p>
  {{else}}
  <p class="text-secondary text-center">No active prompt requests.</p>
  {{end}}
{{end}}
```

**Conversation page archived banner** (`conversation.html`):

Add an archive banner above `#conversation` and an archive/unarchive action in the revision sidebar:

```html
{{define "content"}}
<div class="conversation-wrapper">
  <div class="conversation-main">
    {{if .PromptRequest.Archived}}
    <div class="archive-banner" id="archive-banner">
      <span>This prompt request is archived.</span>
      <form hx-post="/github.com/{{.Org}}/{{.Repo}}/prompt-requests/{{.PromptRequest.ID}}/unarchive"
            hx-target="#archive-banner"
            hx-swap="outerHTML"
            style="display:inline;">
        <button type="submit" class="btn btn-sm btn-secondary">Unarchive</button>
      </form>
    </div>
    {{else}}
    <div id="archive-banner"></div>
    {{end}}

    <div class="chat-container">
      <!-- existing chat content -->
    </div>
  </div>

  <aside class="revision-sidebar">
    <h3 class="sidebar-heading">Revisions</h3>
    <!-- existing revision list -->

    {{if $.PromptRequest.IssueURL}}
    <a href="{{deref $.PromptRequest.IssueURL}}" target="_blank" class="sidebar-issue-link">View GitHub Issue</a>
    {{end}}

    <div class="sidebar-archive-action">
      {{if .PromptRequest.Archived}}
      <form hx-post="/github.com/{{.Org}}/{{.Repo}}/prompt-requests/{{.PromptRequest.ID}}/unarchive"
            hx-target="#archive-banner"
            hx-swap="outerHTML">
        <button type="submit" class="btn btn-sm btn-secondary btn-block">Unarchive</button>
      </form>
      {{else}}
      <button type="button" class="btn btn-sm btn-secondary btn-block"
              onclick="var msg='Archive this prompt request?'; {{if .PromptRequest.IssueURL}}msg+=' The linked GitHub issue will remain open.';{{end}} if(confirm(msg)){htmx.ajax('POST', '/github.com/{{.Org}}/{{.Repo}}/prompt-requests/{{.PromptRequest.ID}}/archive', {target:'#archive-banner', swap:'outerHTML'});}">
        Archive
      </button>
      {{end}}
    </div>
  </aside>
</div>
{{end}}
```

**Archive banner fragment** (`archive_banner_fragment.html` — new file):

```html
<div class="archive-banner" id="archive-banner">
  <span>This prompt request is archived.</span>
  <form hx-post="/github.com/{{.Org}}/{{.Repo}}/prompt-requests/{{.PromptRequest.ID}}/unarchive"
        hx-target="#archive-banner"
        hx-swap="outerHTML"
        style="display:inline;">
    <button type="submit" class="btn btn-sm btn-secondary">Unarchive</button>
  </form>
</div>
```

### CSS Changes

```css
/* internal/server/static/style.css — new styles */

/* Archive toggle switch */
.archive-toggle {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  cursor: pointer;
  user-select: none;
}

.archive-toggle input[type="checkbox"] {
  width: 16px;
  height: 16px;
  cursor: pointer;
}

/* Archived badge */
.badge-archived {
  background: var(--color-muted);
  color: var(--color-text-secondary);
}

/* Card action icons */
.card-action {
  position: absolute;
  top: var(--space-3);
  right: var(--space-3);
  padding: var(--space-1);
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: color var(--transition-fast), background var(--transition-fast);
}

.card-action:hover {
  color: var(--color-text);
  background: var(--color-muted);
}

.card-action:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 2px;
}

/* Card needs position relative for absolute action */
.card {
  position: relative;
}

/* Archive banner in conversation view */
.archive-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-3) var(--space-4);
  background: var(--color-warning-bg);
  color: var(--color-warning);
  border-radius: var(--radius-md);
  margin-bottom: var(--space-4);
  font-size: var(--font-size-sm);
}

/* Sidebar archive action */
.sidebar-archive-action {
  margin-top: var(--space-4);
  padding-top: var(--space-4);
  border-top: 1px solid var(--color-border);
}

.btn-block {
  display: block;
  width: 100%;
}
```

### Implementation Phases

#### Phase 1: Database + Model + Queries

Add the `archived` column, update the model, and update all queries.

**Files changed:**
- `internal/db/db.go` — Add `ALTER TABLE` migration for `archived` column
- `internal/models/models.go` — Add `Archived bool` to `PromptRequest`
- `internal/db/queries.go` — Add `archived` to SELECT in `listPromptRequestsQuery`, update `scanPromptRequest`, add `archivedOnly` param to `ListPromptRequests` and `ListPromptRequestsByRepoURL`, add `ArchivePromptRequest` and `UnarchivePromptRequest` methods, update `GetPromptRequest` to scan `archived`

**Acceptance criteria:**
- [x] Migration is idempotent (safe to run multiple times)
- [x] All existing queries continue to work with the new column
- [x] New `ArchivePromptRequest` and `UnarchivePromptRequest` update only the `archived` column (not `updated_at` or `status`)
- [x] `ListPromptRequests(false)` returns only active prompts
- [x] `ListPromptRequests(true)` returns only archived prompts

#### Phase 2: Route Handlers

Add archive/unarchive HTTP handlers and routes. Update existing handlers to support the `?archived=1` query param.

**Files changed:**
- `internal/server/server.go` — Register `POST .../archive` and `POST .../unarchive` routes
- `internal/server/handlers.go` — Add `handleArchive` and `handleUnarchive`, update `handleDashboard` and `handleRepoPage` to read `?archived` param and pass `ShowArchived` to templates, update `handleSidebarFragment` to always pass `false` for archived, add `ShowArchived` to `dashboardData` and `repoData`

**Acceptance criteria:**
- [x] `POST .../archive` sets archived=1 and redirects back for non-HTMX requests
- [x] `POST .../archive` returns banner HTML fragment for HTMX requests
- [x] `POST .../unarchive` sets archived=0 and redirects back for non-HTMX requests
- [x] `POST .../unarchive` returns empty `#archive-banner` div for HTMX requests
- [x] Dashboard and repo pages filter by archived state based on query param
- [x] Sidebar always shows only active prompts regardless of page's archive toggle

#### Phase 3: List Page UI

Add the archive toggle switch and per-row archive/unarchive icons to dashboard and repo templates.

**Files changed:**
- `internal/server/templates/dashboard.html` — Add toggle switch in header, add archive/unarchive `<span>` icons per card, update empty state messages
- `internal/server/templates/repo.html` — Same changes as dashboard
- `internal/server/static/style.css` — Add `.archive-toggle`, `.card-action`, `.badge-archived`, `.card { position: relative }` styles

**Acceptance criteria:**
- [x] Toggle switch appears on both dashboard and repo pages
- [x] Toggling navigates to `?archived=1` or removes the param
- [x] Archive icon visible on each active prompt row
- [x] Unarchive icon visible on each archived prompt row
- [x] `<span>` icons have `role="button"`, `tabindex="0"`, `aria-label`, and `onkeydown` for keyboard accessibility
- [x] Icons use `event.stopPropagation()` and `event.preventDefault()` to avoid triggering card navigation
- [x] No nested `<a>` or `<button>` inside card `<a>` elements
- [x] Confirm dialog appears before archiving, with issue warning for published prompts
- [x] No confirmation for unarchive
- [x] Empty state shows "No archived prompt requests." when viewing archived with none
- [x] Empty state shows "No active prompt requests." when all are archived

#### Phase 4: Conversation Page UI

Add the archived banner and archive/unarchive actions in the revision sidebar.

**Files changed:**
- `internal/server/templates/conversation.html` — Add `#archive-banner` div (conditional on `Archived`), add archive/unarchive button in revision sidebar
- `internal/server/templates/archive_banner_fragment.html` — **New file.** HTMX fragment for the archived banner
- `internal/server/server.go` — Register `archive_banner_fragment.html` in template parsing
- `internal/server/static/style.css` — Add `.archive-banner`, `.sidebar-archive-action`, `.btn-block` styles

**Acceptance criteria:**
- [x] Archived banner appears at top of conversation main area when prompt is archived
- [x] Banner has "Unarchive" button that removes the banner via HTMX swap
- [x] Revision sidebar shows "Archive" button for active prompts
- [x] Revision sidebar shows "Unarchive" button for archived prompts
- [x] Archive button triggers confirm dialog, with issue warning if published
- [x] After archiving from conversation, banner appears without page reload (HTMX swap)
- [x] After unarchiving from conversation, banner disappears without page reload
- [x] Chat input and all other interactions still work on archived prompts
- [x] Direct URL access to an archived prompt shows the banner

## Acceptance Criteria

### Functional Requirements

- [x] Archive action available on dashboard, repo, and conversation pages
- [x] Archiving shows native `confirm()` dialog
- [x] Published prompts get extra warning about GitHub issue staying open
- [x] Archived prompts disappear from default list view and sidebar
- [x] "Show archived" toggle on dashboard and repo pages
- [x] Unarchive restores prompt to previous status (draft/published)
- [x] No confirmation needed for unarchive
- [x] Archiving from conversation stays on page with banner
- [x] Sidebar always shows only active prompts
- [x] Archive/unarchive does not update `updated_at`

### Non-Functional Requirements

- [x] DB migration is idempotent
- [x] No new JS dependencies
- [x] Archive icons are keyboard accessible (role, tabindex, onkeydown)
- [x] No nested interactive elements inside card links
- [x] Toggle state preserved via URL query parameter (survives navigation)

### Edge Cases

- [x] Archiving a processing prompt: processing continues, response saved
- [x] All prompts archived: empty state message directs user to toggle
- [x] No archived prompts: toggle shows appropriate empty state
- [x] Direct URL to archived prompt: conversation loads with banner

## Files to Modify

| File | Change |
|------|--------|
| `internal/db/db.go` | Add `archived` column migration |
| `internal/models/models.go` | Add `Archived bool` field |
| `internal/db/queries.go` | Update listing query + scan, add archive/unarchive methods, add filter param |
| `internal/server/server.go` | Register archive/unarchive routes, register new template |
| `internal/server/handlers.go` | Add handlers, update dashboard/repo/sidebar handlers, update data structs |
| `internal/server/templates/dashboard.html` | Add toggle, archive icons, empty states |
| `internal/server/templates/repo.html` | Add toggle, archive icons, empty states |
| `internal/server/templates/conversation.html` | Add banner, sidebar archive action |
| `internal/server/templates/archive_banner_fragment.html` | **New** — HTMX fragment for banner |
| `internal/server/static/style.css` | Add toggle, card-action, banner, sidebar-action styles |

## References

### Internal References

- Brainstorm: `docs/brainstorms/2026-03-06-archive-unarchive-brainstorm.md`
- DB schema + migrations: `internal/db/db.go:14-83`
- Listing queries: `internal/db/queries.go:103-167`
- Scan function: `internal/db/queries.go:114-134`
- Models: `internal/models/models.go:13-32`
- Dashboard handler: `internal/server/handlers.go:52-64`
- Repo page handler: `internal/server/handlers.go:75-118`
- Conversation handler: `internal/server/handlers.go:194-278`
- Sidebar fragment handler: `internal/server/handlers.go:1019-1039`
- Delete handler (redirect pattern): `internal/server/handlers.go:458-475`
- Publish handler (HTMX redirect pattern): `internal/server/handlers.go:370-456`
- Route registration: `internal/server/server.go:71-82`
- Dashboard template: `internal/server/templates/dashboard.html`
- Repo template: `internal/server/templates/repo.html`
- Conversation template: `internal/server/templates/conversation.html`
- CSS styles: `internal/server/static/style.css`

### Institutional Learnings

- Nested interactive elements: `docs/solutions/ui-bugs/nested-anchor-tags-duplicate-rendering.md` — Never nest `<a>`/`<button>` inside `<a>`. Use `<span>` with `onclick` + `event.stopPropagation()`.
- State management: `docs/solutions/ui-bugs/published-prompt-request-404-continue-conversation.md` — Re-fetch all data from DB after state changes, don't rely on previous request state.

### Related Issues

- Issue #27: https://github.com/esnunes/prompter/issues/27
