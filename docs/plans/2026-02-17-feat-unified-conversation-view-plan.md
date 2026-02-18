---
title: "feat: Unified conversation view with revision sidebar"
type: feat
date: 2026-02-17
---

# Unified Conversation View with Revision Sidebar

## Overview

Replace the separate `conversation.html` and `published.html` templates with a single unified conversation view. A sidebar panel shows revision history ("Not published yet" for drafts, timestamped revision list for published). Inline submission markers appear in the chat timeline at the point each publish occurred. The chat input is always available, enabling users to continue conversations after publishing.

## Problem Statement / Motivation

After publishing a prompt request to GitHub, clicking "Continue conversation" redirects to `/prompt-requests/{id}/continue` which returns 404 — no route or handler exists. The separate `published.html` template is a dead-end that prevents iterating on prompts after publishing.

The two-template approach (`conversation.html` vs `published.html`) also causes code duplication (message rendering appears in 3 places) and creates a jarring UX where publishing fundamentally changes the page layout.

## Proposed Solution

A unified template that always renders the full conversation with a two-column layout:

```
┌──────────────────────────────────────┬──────────────────┐
│  Header (title, repo, status badge)  │                  │
├──────────────────────────────────────┤   Sidebar        │
│  Chat messages                       │   - Issue link   │
│  ├── User message                    │   - Revisions    │
│  ├── Assistant message               │     - Rev 1 ts   │
│  ├── [Published — Revision 1]        │     - Rev 2 ts   │
│  ├── User message                    │   or             │
│  ├── Assistant message               │   "Not published │
│  └── [Publish button / Question]     │    yet"          │
│                                      │                  │
│  ┌────────────────────────────────┐  │                  │
│  │  Chat input textarea           │  │                  │
│  └────────────────────────────────┘  │                  │
└──────────────────────────────────────┴──────────────────┘
```

On screens narrower than 768px, the sidebar collapses to a compact summary bar above the chat.

## Technical Considerations

### Schema change: `after_message_id` on revisions

To position inline markers accurately, each revision records the ID of the last message at the time of publishing. This is more reliable than timestamp comparison (SQLite `datetime('now')` has only second-level precision).

- Add `after_message_id INTEGER REFERENCES messages(id)` as a **nullable** column
- Existing revisions get `NULL` — template treats NULL as "place marker at the end"
- Migration via `ALTER TABLE` since the project uses `CREATE TABLE IF NOT EXISTS`

### Question/prompt_ready extraction for all statuses

Currently `handleShow` only extracts `LastQuestions` and `PromptReady` for draft requests (line 117 branches on status). The unified view must extract these regardless of status, so that returning to a published request with pending questions shows the question form, and `prompt_ready` after re-chatting displays the publish button.

### Timeline interleaving

Messages and revision markers are merged into a single timeline in the handler:

```go
type timelineItem struct {
    Type     string           // "message" or "revision-marker"
    Message  *models.Message
    Revision *models.Revision
}
```

The handler iterates messages in order and inserts revision markers after the message whose ID matches `revision.AfterMessageID`. Revisions with `NULL` `after_message_id` are appended at the end.

### HTMX behavior preserved

- Message sending: `hx-post` targeting `#conversation` with `beforeend` swap — unchanged
- Publishing: `hx-target="body" hx-swap="innerHTML"` triggers full page reload via 303 redirect — works with unified template since the entire page re-renders with fresh data including new inline marker and updated sidebar
- Add `hx-disabled-elt="find button"` to publish form to prevent double-publish

### Sidebar content

Compact list showing revision number, timestamp, and link to GitHub issue. Clicking a sidebar revision item scrolls the chat to the corresponding inline marker.

## Acceptance Criteria

- [x] `GET /prompt-requests/{id}` renders the unified conversation view regardless of status
- [x] Sidebar shows "Not published yet" for draft requests with no revisions
- [x] Sidebar shows revision list (number + timestamp) for published requests
- [x] Sidebar shows "View Issue" link when `issue_url` is set
- [x] Inline submission markers appear in the chat at the correct position for each revision
- [x] Chat input is always available (for both draft and published requests)
- [x] Question forms appear when the last assistant response has questions (regardless of status)
- [x] Publish button appears when `prompt_ready` is true (regardless of status)
- [x] Re-publishing updates the existing GitHub issue, creates a new revision, and shows a new inline marker
- [x] `published.html` template is removed
- [x] No `/continue` route needed
- [x] Sidebar collapses to a compact bar on screens < 768px

## Dependencies & Risks

- **Risk:** The `ALTER TABLE` migration adds a nullable column — safe for SQLite, no data loss
- **Risk:** Removing `published.html` is a breaking change if any external links point to it — mitigated because the URL (`/prompt-requests/{id}`) doesn't change, only the rendered template does
- **Dependency:** Existing HTMX fragment pattern (`message_fragment.html`) remains unchanged — new messages are always appended at the end

## MVP

### Phase 1: Schema & Data Model

#### `internal/db/db.go`

Add migration after existing `CREATE TABLE` statements:

```go
// Migration: add after_message_id to revisions
_, err = db.Exec(`ALTER TABLE revisions ADD COLUMN after_message_id INTEGER REFERENCES messages(id)`)
// Ignore error if column already exists (SQLite doesn't have IF NOT EXISTS for ALTER TABLE)
```

#### `internal/models/models.go`

Add `AfterMessageID` to `Revision`:

```go
type Revision struct {
    ID              int64
    PromptRequestID int64
    Content         string
    AfterMessageID  *int64   // nullable — NULL for legacy revisions
    PublishedAt     time.Time
}
```

#### `internal/db/queries.go`

Update `CreateRevision` to accept and store `afterMessageID`:

```go
func (q *Queries) CreateRevision(promptRequestID int64, content string, afterMessageID *int64) (*models.Revision, error)
```

Update `ListRevisions` to include `after_message_id` in the SELECT and scan.

### Phase 2: Handler Changes

#### `internal/server/handlers.go`

**Unified data struct** — replace both `conversationData` and `publishedData`:

```go
type conversationData struct {
    PromptRequest *models.PromptRequest
    Timeline      []timelineItem
    LastQuestions  []questionData
    PromptReady   bool
    Revisions     []models.Revision
}

type timelineItem struct {
    Type     string           // "message" or "revision-marker"
    Message  *models.Message
    Revision *models.Revision
}
```

**Update `handleShow`** (lines 97-152):
- Remove the `if pr.Status == "published"` branch
- Always load messages AND revisions
- Always extract `LastQuestions` and `PromptReady` from the last assistant message
- Build the timeline by interleaving messages and revision markers
- Render the unified `conversation.html`

```go
func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
    // ... parse id, get PR, get messages (unchanged)

    revisions, _ := s.queries.ListRevisions(id)

    // Build timeline
    timeline := buildTimeline(messages, revisions)

    // Extract questions/promptReady from last assistant message (regardless of status)
    var lastQuestions []questionData
    var promptReady bool
    if len(messages) > 0 {
        last := messages[len(messages)-1]
        if last.Role == "assistant" && last.RawResponse != nil {
            lastQuestions, promptReady = extractQuestionsFromRaw(*last.RawResponse)
        }
    }

    data := conversationData{
        PromptRequest: pr,
        Timeline:      timeline,
        LastQuestions:  lastQuestions,
        PromptReady:   promptReady,
        Revisions:     revisions,
    }
    s.renderPage(w, "conversation.html", data)
}
```

**Add `buildTimeline` helper:**

```go
func buildTimeline(messages []models.Message, revisions []models.Revision) []timelineItem {
    // Create a map of afterMessageID -> revision for O(1) lookup
    revByMsg := map[int64][]models.Revision{}
    var orphanRevs []models.Revision
    for _, rev := range revisions {
        if rev.AfterMessageID != nil {
            revByMsg[*rev.AfterMessageID] = append(revByMsg[*rev.AfterMessageID], rev)
        } else {
            orphanRevs = append(orphanRevs, rev)
        }
    }

    var items []timelineItem
    for _, msg := range messages {
        m := msg
        items = append(items, timelineItem{Type: "message", Message: &m})
        if revs, ok := revByMsg[msg.ID]; ok {
            for _, rev := range revs {
                r := rev
                items = append(items, timelineItem{Type: "revision-marker", Revision: &r})
            }
        }
    }
    // Append orphan revisions (legacy data with NULL after_message_id)
    for _, rev := range orphanRevs {
        r := rev
        items = append(items, timelineItem{Type: "revision-marker", Revision: &r})
    }
    return items
}
```

**Update `handlePublish`** (lines 286-358):
- Before creating the revision, get the ID of the last message
- Pass `afterMessageID` to `CreateRevision`

```go
// Get the last message ID for the revision marker
lastMsg, _ := s.queries.GetLastMessage(promptRequestID)
var afterMsgID *int64
if lastMsg != nil {
    afterMsgID = &lastMsg.ID
}
revision, err := s.queries.CreateRevision(id, body, afterMsgID)
```

**Remove `publishedData` struct** (lines 154-158) — no longer needed.

### Phase 3: Unified Template

#### `internal/server/templates/conversation.html`

Replace current content with unified layout:

```html
{{define "title"}}{{.PromptRequest.Title}}{{end}}

{{define "header-actions"}}
  <span class="text-secondary text-sm">{{.PromptRequest.RepoURL}}</span>
  {{if eq .PromptRequest.Status "published"}}
    <span class="badge badge-published">published</span>
  {{end}}
  {{if .PromptRequest.IssueURL}}
    <a href="{{deref .PromptRequest.IssueURL}}" target="_blank" class="btn btn-sm">View Issue</a>
  {{end}}
{{end}}

{{define "content"}}
<div class="conversation-wrapper">
  {{/* Main chat area */}}
  <div class="conversation-main">
    <div class="chat-container">
      <div class="chat-messages" id="conversation">
        {{range .Timeline}}
          {{if eq .Type "message"}}
            <div class="message message-{{.Message.Role}}">
              <div class="message-bubble">{{.Message.Content}}</div>
            </div>
          {{else if eq .Type "revision-marker"}}
            <div class="submission-marker" id="revision-{{.Revision.ID}}">
              <span class="submission-marker-text">
                Published to GitHub — Revision {{.Revision.ID}}
                <time>{{.Revision.PublishedAt.Format "Jan 2, 2006 3:04 PM"}}</time>
              </span>
            </div>
          {{end}}
        {{end}}

        {{/* Question form (if questions pending) */}}
        {{if .LastQuestions}}
          {{/* ... existing question-block markup ... */}}
        {{end}}

        {{/* Publish button (if prompt ready) */}}
        {{if .PromptReady}}
          {{/* ... existing prompt-ready markup with hx-disabled-elt added ... */}}
        {{end}}
      </div>

      {{/* Chat input */}}
      <div class="chat-input" id="message-form" {{if .LastQuestions}}style="display:none"{{end}}>
        {{/* ... existing message form markup ... */}}
      </div>
    </div>
  </div>

  {{/* Sidebar */}}
  <aside class="revision-sidebar">
    <h3 class="sidebar-heading">Revisions</h3>
    {{if .Revisions}}
      <ul class="revision-list">
        {{range .Revisions}}
          <li class="revision-list-item">
            <a href="#revision-{{.ID}}" class="revision-link">
              <span class="revision-number">Revision {{.ID}}</span>
              <time class="revision-time text-sm text-secondary">
                {{.PublishedAt.Format "Jan 2, 2006 3:04 PM"}}
              </time>
            </a>
          </li>
        {{end}}
      </ul>
      {{if .PromptRequest.IssueURL}}
        <a href="{{deref .PromptRequest.IssueURL}}" target="_blank" class="sidebar-issue-link">
          View GitHub Issue
        </a>
      {{end}}
    {{else}}
      <p class="text-secondary text-sm">Not published yet</p>
    {{end}}
  </aside>
</div>
{{end}}
```

#### Remove `internal/server/templates/published.html`

Delete this file entirely.

#### Update `internal/server/server.go`

Remove `"published.html"` from the `pageNames` slice in `parsePages`.

### Phase 4: CSS

#### `internal/server/static/style.css`

Add two-column layout and sidebar styles:

```css
/* Two-column conversation layout */
.conversation-wrapper {
  display: flex;
  gap: var(--space-4);
  height: calc(100vh - 180px);
}

.conversation-main {
  flex: 1;
  min-width: 0; /* prevent flex overflow */
}

.conversation-main .chat-container {
  height: 100%;
}

/* Sidebar */
.revision-sidebar {
  width: var(--size-sidebar);
  flex-shrink: 0;
  border-left: 1px solid var(--color-border);
  padding: var(--space-4);
  overflow-y: auto;
}

.sidebar-heading {
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-bold);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--color-text-secondary);
  margin-bottom: var(--space-3);
}

.revision-list {
  list-style: none;
  padding: 0;
  margin: 0;
}

.revision-list-item {
  padding: var(--space-2) 0;
  border-bottom: 1px solid var(--color-border);
}

.revision-list-item:last-child {
  border-bottom: none;
}

.revision-link {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  text-decoration: none;
  color: inherit;
}

.revision-link:hover {
  color: var(--color-primary);
}

.revision-number {
  font-weight: var(--font-weight-medium);
}

.sidebar-issue-link {
  display: block;
  margin-top: var(--space-4);
  padding-top: var(--space-4);
  border-top: 1px solid var(--color-border);
  font-size: var(--font-size-sm);
}

/* Inline submission markers */
.submission-marker {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-3) 0;
  margin: var(--space-3) 0;
}

.submission-marker-text {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  background: var(--color-surface);
  padding: var(--space-1) var(--space-3);
  border-radius: var(--radius-full);
  border: 1px solid var(--color-border);
}

.submission-marker-text time {
  margin-left: var(--space-2);
  opacity: 0.7;
}

/* Scroll highlight for sidebar links */
.submission-marker:target {
  animation: highlight-flash 1.5s ease-out;
}

@keyframes highlight-flash {
  0% { background-color: var(--color-primary-light, rgba(59, 130, 246, 0.1)); }
  100% { background-color: transparent; }
}

/* Responsive: collapse sidebar on narrow screens */
@media (max-width: 768px) {
  .conversation-wrapper {
    flex-direction: column;
    height: auto;
  }

  .revision-sidebar {
    width: 100%;
    border-left: none;
    border-bottom: 1px solid var(--color-border);
    padding: var(--space-2) var(--space-4);
    overflow-y: visible;
    order: -1; /* sidebar above chat on mobile */
  }

  .revision-list {
    display: flex;
    gap: var(--space-2);
    overflow-x: auto;
  }

  .revision-list-item {
    border-bottom: none;
    white-space: nowrap;
  }

  .conversation-main .chat-container {
    height: calc(100vh - 240px);
  }
}
```

### Phase 5: Cleanup

- [x] Delete `internal/server/templates/published.html`
- [x] Remove `"published.html"` from `pageNames` in `server.go`
- [x] Remove `publishedData` struct from `handlers.go`
- [x] Remove the `if pr.Status == "published"` branch from `handleShow`
- [x] Add `hx-disabled-elt="find button"` to the publish form in both `conversation.html` and `message_fragment.html`

## References & Research

### Internal References
- Brainstorm: `docs/brainstorms/2026-02-17-continue-conversation-brainstorm.md`
- Current templates: `internal/server/templates/conversation.html`, `internal/server/templates/published.html`
- Handler routing: `internal/server/handlers.go:97-152` (handleShow), `internal/server/handlers.go:286-358` (handlePublish)
- DB schema: `internal/db/db.go:14-55`
- Revision queries: `internal/db/queries.go:289-331`
- CSS tokens: `internal/server/static/tokens.css` (note: `--size-sidebar: 16rem` already exists at line 65)
- MVP plan: `docs/plans/2026-02-16-feat-prompter-mvp-plan.md`
