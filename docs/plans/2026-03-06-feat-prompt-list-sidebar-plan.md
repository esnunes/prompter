---
title: "feat: Add prompt list sidebar for parallel prompt management"
type: feat
date: 2026-03-06
issue: https://github.com/esnunes/prompter/issues/21
---

# feat: Add Prompt List Sidebar for Parallel Prompt Management

## Overview

Add a persistent left sidebar across all pages that lists prompt requests, allowing users to monitor and switch between multiple prompts in parallel. The sidebar shows status badges, processing indicators, and unread markers so users can efficiently manage concurrent conversations with Claude.

## Problem Statement / Motivation

Currently, working on multiple prompts requires navigating back and forth between the dashboard or repo page and individual conversations. When creating several prompts in parallel -- especially while waiting for Claude to respond on one -- there's no quick way to see which prompts need attention, which have new responses, or to switch between them. This slows down the workflow significantly when managing multiple feature requests at once.

## Proposed Solution

A persistent left sidebar rendered at the layout level, appearing on every page. The sidebar lists prompt requests with title, status badge, time info, and unread indicators. Clicking a sidebar item navigates to that conversation.

**Sidebar scope by page:**
- **Dashboard (`/`):** All prompt requests across all repos, flat list. Each item shows repo name as secondary text.
- **Repo page (`/{org}/{repo}/prompt-requests`):** Only prompts belonging to that repo.
- **Conversation page (`/{org}/{repo}/prompt-requests/{id}`):** Only prompts belonging to that repo. Current prompt is visually highlighted.

**Sorting:**
1. Processing prompts (actively waiting for Claude) -- top
2. Non-processing draft prompts -- middle
3. Published prompts -- bottom
4. Within each group: `updated_at DESC` (most recently active first)

**Real-time updates:** HTMX polling on a sidebar-specific fragment endpoint, consistent with existing polling patterns.

**Unread tracking:** A `last_viewed_at` column on `prompt_requests`, updated when the user views a conversation. Compared against the latest assistant message timestamp.

## Technical Approach

### Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Sidebar data injection | Embed `SidebarData` in all page data structs | Follows existing template composition pattern |
| Real-time updates | HTMX polling (~3s) on sidebar fragment endpoint | Consistent with existing `status_fragment.html` polling pattern |
| Unread persistence | `last_viewed_at` column on `prompt_requests` | Survives server restarts; simple to implement |
| Processing state | Merge in-memory `repoStatus` with DB results in handler | No new DB state; processing is transient |
| Sidebar width | `--size-sidebar` (16rem), same as revision sidebar | Visual consistency |
| Navigation | Standard `<a href>` links (full page navigation) | Simple, consistent with existing patterns |
| Responsive behavior | Hide sidebar below 768px | Desktop-focused tool; no collapse toggle |
| Time display | Absolute format matching existing patterns | Simple, no JS needed |

### Architecture

```
  ┌──────────────────────────────────────────────────────────────────┐
  │ Header                                                           │
  ├──────────┬───────────────────────────────────────┬───────────────┤
  │          │                                       │               │
  │ Prompt   │         Main Content                  │  Revision     │
  │ List     │    (Dashboard / Repo / Conversation)  │  Sidebar      │
  │ Sidebar  │                                       │  (conv only)  │
  │ (16rem)  │         (flex: 1)                     │  (16rem)      │
  │          │                                       │               │
  └──────────┴───────────────────────────────────────┴───────────────┘
              ↑                                       ↑
              Always present on all pages             Only on conversation page
```

**Container width changes:**
- Dashboard/Repo pages: `max-width: calc(var(--size-container) + var(--size-sidebar) + var(--space-4))` (~65rem)
- Conversation page: `max-width: calc(var(--size-container) + 2 * var(--size-sidebar) + 2 * var(--space-4))` (~82rem)

### Data Flow

```
  Page Load:
  ┌─────────┐    ┌──────────┐    ┌──────────────┐    ┌──────────┐
  │ Handler │───>│ DB Query │───>│ Merge w/      │───>│ Template │
  │         │    │ (sidebar │    │ repoStatus    │    │ Render   │
  │         │    │  items)  │    │ (processing)  │    │          │
  └─────────┘    └──────────┘    └──────────────┘    └──────────┘

  HTMX Polling (sidebar updates):
  ┌──────────┐    ┌───────────────┐    ┌──────────────┐    ┌──────────┐
  │ Browser  │───>│ GET /sidebar  │───>│ Merge w/      │───>│ Fragment │
  │ hx-get   │    │ ?scope=...    │    │ repoStatus    │    │ Response │
  │ every 3s │    │ &repo=...     │    │ & last_viewed │    │          │
  └──────────┘    └───────────────┘    └──────────────┘    └──────────┘
```

### DB Schema Change

```sql
-- Migration (idempotent, follows existing ALTER TABLE pattern in db.go:80)
ALTER TABLE prompt_requests ADD COLUMN last_viewed_at TEXT;
```

The `last_viewed_at` column is nullable. When NULL, the prompt has never been viewed (all assistant responses are "unread"). It is updated to `datetime('now')` in the `handleShow` handler.

**Unread logic:** A prompt is "unread" when:
- It has at least one assistant message
- The latest assistant message's `created_at` > `prompt_request.last_viewed_at` (or `last_viewed_at` IS NULL)
- The user is NOT currently viewing this prompt (i.e., it's not the current conversation page)

### Sidebar Data Struct

```go
// internal/server/handlers.go

type sidebarItem struct {
    ID         int64
    Title      string
    Status     string // "draft", "published"
    Processing bool   // true if repoStatus shows cloning/pulling/processing
    Unread     bool   // true if new assistant response since last_viewed_at
    RepoURL    string // shown only on dashboard
    UpdatedAt  time.Time
    Org        string // for URL construction
    Repo       string // for URL construction
}

type sidebarData struct {
    Items     []sidebarItem
    Scope     string // "all" (dashboard) or "repo"
    CurrentID int64  // highlighted item (0 if not on conversation page)
}
```

### Base Page Data

```go
// Embed in all page data structs
type basePageData struct {
    Sidebar sidebarData
}

type dashboardData struct {
    basePageData
    PromptRequests []models.PromptRequest
}

type repoData struct {
    basePageData
    RepoURL        string
    Org            string
    Repo           string
    Error          string
    PromptRequests []models.PromptRequest
}

type conversationData struct {
    basePageData
    PromptRequest *models.PromptRequest
    Org           string
    Repo          string
    // ... existing fields
}
```

### Sidebar Template

```html
<!-- internal/server/templates/sidebar.html (new partial) -->
<aside class="prompt-sidebar" id="prompt-sidebar"
       hx-get="{{.Sidebar.PollURL}}"
       hx-trigger="every 3s"
       hx-swap="outerHTML">
  <h3 class="sidebar-heading">Prompts</h3>
  {{if .Sidebar.Items}}
  <ul class="prompt-list">
    {{range .Sidebar.Items}}
    <li class="prompt-list-item{{if eq .ID $.Sidebar.CurrentID}} prompt-list-item-active{{end}}{{if .Unread}} prompt-list-item-unread{{end}}">
      <a href="/github.com/{{.Org}}/{{.Repo}}/prompt-requests/{{.ID}}" class="prompt-list-link">
        <div class="prompt-list-title">
          {{if .Title}}{{.Title}}{{else}}Untitled{{end}}
        </div>
        <div class="prompt-list-meta">
          <span class="badge {{if .Processing}}badge-processing{{else if eq .Status "published"}}badge-published{{else}}badge-draft{{end}}">
            {{if .Processing}}processing{{else}}{{.Status}}{{end}}
          </span>
          <time class="text-sm text-secondary">{{.UpdatedAt.Format "Jan 2, 2006"}}</time>
        </div>
        {{if $.Sidebar.Scope | eq "all"}}
        <div class="prompt-list-repo text-sm text-secondary">{{.RepoURL}}</div>
        {{end}}
      </a>
    </li>
    {{end}}
  </ul>
  {{else}}
  <p class="text-secondary text-sm">No prompt requests yet</p>
  {{end}}
</aside>
```

### Sidebar Polling Endpoint

```go
// New route: GET /api/sidebar
// Query params: scope=all|repo, repo_url=github.com/org/repo, current_id=123

func (s *Server) handleSidebarFragment(w http.ResponseWriter, r *http.Request) {
    scope := r.URL.Query().Get("scope")
    repoURL := r.URL.Query().Get("repo_url")
    currentID, _ := strconv.ParseInt(r.URL.Query().Get("current_id"), 10, 64)

    var prs []models.PromptRequest
    if scope == "repo" && repoURL != "" {
        prs, _ = s.queries.ListPromptRequestsByRepoURL(repoURL)
    } else {
        prs, _ = s.queries.ListPromptRequests()
    }

    items := s.buildSidebarItems(prs)
    data := sidebarData{
        Items:     items,
        Scope:     scope,
        CurrentID: currentID,
    }
    s.renderFragment(w, "sidebar.html", data)
}
```

### Layout Changes

The sidebar must be rendered at the layout level. Currently, `layout.html` wraps content in `<main class="container">`. The new structure:

```html
<!-- layout.html -->
<body>
  <header class="header">...</header>
  <div class="app-layout">
    {{block "sidebar" .}}{{end}}
    <main class="container">
      {{block "content" .}}{{end}}
    </main>
  </div>
</body>
```

Each page template defines the `sidebar` block using the shared sidebar partial, passing the sidebar data from the embedded `basePageData`.

### Unread Update in handleShow

```go
func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
    // ... existing code to get prompt request ...

    // Update last_viewed_at for unread tracking
    s.queries.UpdateLastViewedAt(id)

    // ... rest of existing code ...
}
```

```go
// internal/db/queries.go
func (q *Queries) UpdateLastViewedAt(id int64) error {
    _, err := q.db.Exec(
        `UPDATE prompt_requests SET last_viewed_at = datetime('now') WHERE id = ?`, id,
    )
    return err
}
```

### Updated Listing Queries

The existing `ListPromptRequests` and `ListPromptRequestsByRepoURL` need to additionally return `last_viewed_at` and the latest assistant message timestamp for unread calculation. Add to the `PromptRequest` model:

```go
// internal/models/models.go — add to PromptRequest
LastViewedAt       *time.Time
LatestAssistantAt  *time.Time
```

Update the queries to join with the latest assistant message:

```sql
SELECT pr.*, r.url,
       (SELECT COUNT(*) FROM messages WHERE prompt_request_id = pr.id) as message_count,
       (SELECT COUNT(*) FROM revisions WHERE prompt_request_id = pr.id) as revision_count,
       pr.last_viewed_at,
       (SELECT MAX(created_at) FROM messages
        WHERE prompt_request_id = pr.id AND role = 'assistant') as latest_assistant_at
FROM prompt_requests pr
JOIN repositories r ON r.id = pr.repository_id
WHERE pr.status != 'deleted'
ORDER BY
    CASE WHEN pr.status = 'draft' THEN 0 ELSE 1 END ASC,
    pr.updated_at DESC
```

### CSS Changes

```css
/* internal/server/static/style.css — new styles */

/* App layout wrapper */
.app-layout {
  display: flex;
  max-width: calc(var(--size-container) + var(--size-sidebar) + var(--space-4));
  margin: 0 auto;
}

/* Override for conversation page (3 columns) */
body:has(.conversation-wrapper) .app-layout {
  max-width: calc(var(--size-container) + 2 * var(--size-sidebar) + 2 * var(--space-4));
}

/* Prompt list sidebar (left) */
.prompt-sidebar {
  width: var(--size-sidebar);
  flex-shrink: 0;
  border-right: 1px solid var(--color-border);
  padding: var(--space-4);
  overflow-y: auto;
  height: calc(100vh - 73px); /* below header */
  position: sticky;
  top: 73px;
}

.prompt-list {
  list-style: none;
  padding: 0;
  margin: 0;
}

.prompt-list-item {
  border-bottom: 1px solid var(--color-border);
}

.prompt-list-item:last-child {
  border-bottom: none;
}

.prompt-list-link {
  display: block;
  padding: var(--space-3) var(--space-2);
  text-decoration: none;
  color: inherit;
  border-radius: var(--radius-md);
  transition: background var(--transition-fast);
}

.prompt-list-link:hover {
  background: var(--color-muted);
  text-decoration: none;
}

.prompt-list-item-active .prompt-list-link {
  background: rgba(37, 99, 235, 0.08);
  border-left: 3px solid var(--color-primary);
}

.prompt-list-item-unread .prompt-list-link::before {
  content: "";
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-primary);
  margin-right: var(--space-2);
  vertical-align: middle;
}

.prompt-list-title {
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-medium);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.prompt-list-meta {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-top: var(--space-1);
}

.prompt-list-repo {
  margin-top: var(--space-1);
  font-family: var(--font-mono);
  font-size: var(--font-size-xs);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Processing badge */
.badge-processing {
  background: rgba(37, 99, 235, 0.1);
  color: var(--color-primary);
  animation: pulse-badge 2s ease-in-out infinite;
}

@keyframes pulse-badge {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.6; }
}

/* Header alignment with sidebar */
.header-inner {
  max-width: calc(var(--size-container) + var(--size-sidebar) + var(--space-4));
}

body:has(.conversation-wrapper) .header-inner {
  max-width: calc(var(--size-container) + 2 * var(--size-sidebar) + 2 * var(--space-4));
}

/* Responsive: hide sidebar on narrow screens */
@media (max-width: 768px) {
  .prompt-sidebar {
    display: none;
  }

  .app-layout {
    max-width: var(--size-container);
  }
}
```

### Template Parsing Changes

The sidebar is a shared partial rendered within the layout. Update `parsePages()` in `server.go` to include the sidebar template as a shared partial that can be invoked from layout.html.

```go
// internal/server/server.go — update parsePages()
// After parsing layout, also parse sidebar.html into each page template.
// Layout calls {{template "sidebar.html" .}} to render the sidebar.
```

### Implementation Phases

#### Phase 1: Layout Restructure + Sidebar Shell

Create the layout wrapper, base page data, and a static sidebar that renders on page load (no polling, no unread).

**Files changed:**
- `internal/server/templates/layout.html` — Add `.app-layout` wrapper, add `sidebar` block
- `internal/server/templates/sidebar.html` — **New file.** Sidebar template partial
- `internal/server/static/style.css` — Add `.app-layout`, `.prompt-sidebar`, `.prompt-list-*` styles
- `internal/server/handlers.go` — Add `sidebarItem`, `sidebarData`, `basePageData` types. Update `dashboardData`, `repoData`, `conversationData` to embed `basePageData`. Update `handleDashboard`, `handleRepoPage`, `handleShow` to populate sidebar data.
- `internal/server/server.go` — Update `parsePages()` to include `sidebar.html` as a shared partial. Add helper method `buildSidebarItems()`.

**Acceptance criteria:**
- [x] Sidebar appears on all three page types (dashboard, repo, conversation)
- [x] Dashboard sidebar shows all prompts with repo name as secondary text
- [x] Repo/conversation sidebar shows only that repo's prompts
- [x] Conversation page highlights current prompt in sidebar
- [x] Sidebar items sorted: drafts first (by updated_at DESC), then published (by updated_at DESC)
- [x] Items display title (or "Untitled"), status badge, and time
- [x] Clicking a sidebar item navigates to the conversation page
- [x] Layout is a clean three-column on conversation page, two-column on others
- [x] Sidebar hidden on screens narrower than 768px

#### Phase 2: Processing State in Sidebar

Merge the in-memory `repoStatus` map with sidebar items to show processing indicators.

**Files changed:**
- `internal/server/handlers.go` — Update `buildSidebarItems()` to check `s.repoStatus` for each prompt request and set `Processing: true` when the status is `cloning`, `pulling`, or `processing`.
- `internal/server/static/style.css` — Add `.badge-processing` style with pulse animation.

**Acceptance criteria:**
- [x] Prompts with active Claude processing show a "processing" badge instead of "draft"
- [x] Processing badge has a subtle pulse animation to distinguish from static badges
- [x] Processing prompts sort above non-processing drafts in the sidebar

#### Phase 3: Unread Indicator

Add `last_viewed_at` DB column and unread indicator logic.

**Files changed:**
- `internal/db/db.go` — Add migration: `ALTER TABLE prompt_requests ADD COLUMN last_viewed_at TEXT`
- `internal/db/queries.go` — Add `UpdateLastViewedAt(id)` method. Update `ListPromptRequests` and `ListPromptRequestsByRepoURL` to return `last_viewed_at` and latest assistant message timestamp.
- `internal/models/models.go` — Add `LastViewedAt *time.Time` and `LatestAssistantAt *time.Time` to `PromptRequest`.
- `internal/server/handlers.go` — Call `UpdateLastViewedAt` at the start of `handleShow`. Update `buildSidebarItems()` to compute `Unread` flag.
- `internal/server/templates/sidebar.html` — Add unread CSS class conditionally.
- `internal/server/static/style.css` — Add `.prompt-list-item-unread` style with blue dot indicator.

**Acceptance criteria:**
- [x] Blue unread dot appears on sidebar items when Claude has responded but user hasn't visited
- [x] Navigating to a conversation clears the unread indicator for that prompt
- [x] Prompts the user is currently viewing never show as unread
- [x] Unread state survives page refresh (persisted in DB)

#### Phase 4: HTMX Polling for Real-Time Updates

Add a sidebar fragment endpoint and HTMX polling so the sidebar updates without full page reload.

**Files changed:**
- `internal/server/server.go` — Add route: `GET /api/sidebar`
- `internal/server/handlers.go` — Add `handleSidebarFragment` handler that returns sidebar HTML fragment.
- `internal/server/templates/sidebar.html` — Add `hx-get`, `hx-trigger="every 3s"`, `hx-swap="outerHTML"` attributes to the sidebar `<aside>`.
- `internal/server/server.go` — Register `sidebar.html` in `parsePages()` for fragment rendering.

**Acceptance criteria:**
- [x] Sidebar updates every 3 seconds without full page reload
- [x] Processing indicators appear/disappear in real-time as Claude starts/finishes
- [x] Unread dots appear when Claude responds to a different prompt
- [x] New prompt requests appear in sidebar after creation (from another tab or after navigation)
- [x] Sidebar polling does not interfere with existing repo status polling on conversation page

## Acceptance Criteria

### Functional Requirements

- [x] Left sidebar is visible on dashboard, repo, and conversation pages
- [x] Sidebar is not collapsible (no toggle button)
- [x] Dashboard sidebar shows all non-deleted prompt requests in a flat list with repo names
- [x] Repo and conversation page sidebars show only prompts for that repo
- [x] Each sidebar item displays: title (or "Untitled"), status badge, time, unread dot
- [x] Sidebar items sorted: processing > draft > published, then by updated_at DESC within group
- [x] Clicking a sidebar item navigates to the conversation page
- [x] Current conversation is highlighted in sidebar with active state
- [x] Processing prompts show animated "processing" badge
- [x] Unread indicator appears when Claude responds to a non-viewed prompt
- [x] Viewing a conversation clears its unread indicator
- [x] Sidebar updates in real-time via HTMX polling (~3s)
- [x] Existing revision sidebar on the right is unchanged
- [x] No "create new prompt" button in the sidebar

### Non-Functional Requirements

- [x] No new JS dependencies (HTMX polling is declarative)
- [x] Sidebar hidden on screens below 768px width
- [x] DB migration is idempotent (safe to run multiple times)
- [x] Sidebar query is lightweight (single query with subqueries, no N+1)

## Dependencies & Risks

**Dependencies:**
- Existing `repoStatus sync.Map` must remain accessible to sidebar handler
- Existing page handler data structures must be updated (breaking change to template data)

**Risks:**
- **Three-column layout width:** At ~82rem, the conversation page may feel cramped on smaller laptop screens (1280px). Mitigation: CSS ensures main content area has `min-width` and flexes appropriately; sidebar auto-hides below 768px.
- **Polling overhead:** A sidebar poll every 3s adds one DB query per 3 seconds per open tab. Mitigation: The query is lightweight (same as existing listing queries); SQLite WAL mode handles concurrent reads well. Can increase interval to 5s if needed.
- **Template refactoring scope:** Updating all page data structs and the layout is a cross-cutting change. Mitigation: Phase 1 focuses solely on this refactoring before adding dynamic features.

## References & Research

### Internal References

- Layout template: `internal/server/templates/layout.html`
- Page templates: `internal/server/templates/dashboard.html`, `repo.html`, `conversation.html`
- Handler data structs: `internal/server/handlers.go:20-153`
- Template parsing: `internal/server/server.go:83-121`
- Existing sidebar CSS: `internal/server/static/style.css:268-324`
- CSS tokens: `internal/server/static/tokens.css`
- DB schema: `internal/db/db.go:14-55`
- DB migrations pattern: `internal/db/db.go:80`
- Listing queries: `internal/db/queries.go:103-167`
- Models: `internal/models/models.go`
- In-memory status tracking: `internal/server/server.go:24-37`
- Status polling pattern: `internal/server/templates/status_fragment.html`
- Repo status handler (pattern for sidebar polling): `internal/server/handlers.go:514-594`

### Related Issues

- Issue #21: https://github.com/esnunes/prompter/issues/21
