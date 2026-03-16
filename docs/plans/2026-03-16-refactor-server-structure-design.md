# Refactor: Server Structure (Pages + HX)

## Goal

Reorganize `internal/server/` so that handlers and templates are colocated by page, HTMX fragments live under `/hx/` URLs, and the codebase is easy for AI agents to navigate and extend.

## Architecture

Two kinds of HTTP endpoints:

1. **Pages** — full HTML with layout. Browser navigates to these.
2. **HTMX fragments** — bare HTML partials. Called by HTMX for partial updates.

All HTMX endpoints live under `/hx/`. Pages use normal URLs.

## File Structure

```
internal/server/
├── pages/
│   ├── base.html                            # shared layout (<html>, <head>, nav)
│   │
│   ├── dashboard/
│   │   ├── page.go                          # Page struct, GET /{$}
│   │   ├── page.html                        # full page template
│   │   ├── create_repository.go             # POST /hx/dashboard/create-repository
│   │   └── create_repository.html           # form fragment
│   │
│   ├── repo/
│   │   ├── page.go                          # Page struct, GET /github.com/{org}/{repo}/prompt-requests
│   │   └── page.html
│   │
│   ├── conversation/
│   │   ├── page.go                          # Page struct + shared deps
│   │   ├── page.html
│   │   ├── send_message.go                  # POST /hx/conversation/send-message
│   │   ├── send_message.html                # message fragment
│   │   ├── status.go                        # GET /hx/conversation/status
│   │   ├── status.html                      # status fragment
│   │   ├── publish.go                       # POST /hx/conversation/publish
│   │   ├── delete.go                        # POST /hx/conversation/delete
│   │   ├── archive.go                       # POST /hx/conversation/archive + unarchive
│   │   ├── archive_banner.html              # archive banner fragment
│   │   ├── cancel.go                        # POST /hx/conversation/cancel
│   │   ├── retry.go                         # POST /hx/conversation/retry
│   │   └── resend.go                        # POST /hx/conversation/resend
│
├── hx/
│   ├── sidebar.go                           # GET /hx/sidebar, BuildSidebar(), types, sort
│   ├── sidebar.html                         # shared sidebar fragment
│   ├── create_prompt_request.go             # POST /hx/create-prompt-request
│
├── static/                                  # CSS, JS (unchanged)
│
├── embed.go                                 # //go:embed pages/*.html pages/*/*.html hx/*.html static
├── server.go                                # Server struct, New(), Listen(), Serve(), state management
├── routes.go                                # all route registration
└── templates.go                             # template aggregation and loading
```

## Colocation Rules

- Page-specific fragments live in that page's folder.
- Fragments used across multiple pages live in `hx/`.
- When a page-specific fragment gets reused, move it to `hx/`.

## Template System

### Single embed at server root

```go
// embed.go
package server

import "embed"

//go:embed pages/*.html pages/*/*.html hx/*.html static
var contentFS embed.FS
```

### Template aggregation

`templates.go` walks the embedded FS and registers each `.html` file with its path as the template name:

- `pages/base.html`
- `pages/dashboard/page.html`
- `pages/dashboard/create_repository.html`
- `hx/sidebar.html`
- etc.

A single `*template.Template` is built at startup. All handlers share it.

### Rendering

- **Pages**: execute `pages/base.html` which references `{{template "title" .}}` and `{{template "content" .}}` blocks defined by each page template.
- **Fragments**: execute the fragment template directly (e.g. `hx/sidebar.html`), no layout wrapping.

Each Page struct holds `tmpl *template.Template` passed from `server.go`.

## Page Struct Pattern

Each page package defines a struct with its dependencies. All handlers (page + fragments) are methods on it.

```go
// pages/conversation/page.go
package conversation

type Page struct {
    tmpl          *template.Template
    queries       *db.Queries
    getRepoStatus func(int64) string
    setRepoStatus func(int64, string, string)
    // ... other deps
}
```

`server.go` constructs each Page struct and passes concrete method references.

## Routes

All routes registered in `routes.go`:

```
// Pages
GET  /{$}                                                    → dashboard.Page
GET  /github.com/{org}/{repo}/prompt-requests                → repo.Page
GET  /github.com/{org}/{repo}/prompt-requests/{id}           → conversation.Page

// Shared HTMX fragments
GET  /hx/sidebar                                             → hx.Sidebar
POST /hx/create-prompt-request                               → hx.CreatePromptRequest

// Dashboard-specific fragments
POST /hx/dashboard/create-repository                         → dashboard.CreateRepository

// Conversation-specific fragments
POST /hx/conversation/send-message                           → conversation.SendMessage
GET  /hx/conversation/status                                 → conversation.Status
POST /hx/conversation/publish                                → conversation.Publish
POST /hx/conversation/delete                                 → conversation.Delete
POST /hx/conversation/archive                                → conversation.Archive
POST /hx/conversation/unarchive                              → conversation.Unarchive
POST /hx/conversation/cancel                                 → conversation.Cancel
POST /hx/conversation/retry                                  → conversation.Retry
POST /hx/conversation/resend                                 → conversation.Resend
```

## URL Conventions

- HTMX endpoints use action-based naming (what they do), not REST resources.
- Page-specific fragments: `/hx/{page-name}/...`
- Shared fragments: `/hx/...`
- POST success with page transition: `HX-Redirect` header.
- POST validation failure: re-render same form with errors.

## Sidebar

`hx/sidebar.go` keeps:
- `BuildSidebar()` function (queries DB, computes unread flags, checks processing state)
- `SidebarData` and `SidebarItem` types
- `sortSidebarItems()`
- HTTP handler for `GET /hx/sidebar`

`hx/sidebar.html` changes:
- `hx-trigger` from `every 3s` to `sse:prompt-updated` (SSE-driven refresh, no polling)
- Keeps `hx-get="/hx/sidebar"` and `hx-swap="outerHTML"`
- Keeps `id="prompt-sidebar"` for SSE targeting

Two modes:
- Dashboard (scope="all"): shows repo name, queries across all repos
- Repo/Conversation (scope="repo"): filters by repo URL, hides repo name

## Dependency Direction

```
pages/*       ──depends on──> hx/        (pages use shared fragment types/data)
hx/           ──depends on──> nothing    (fragments are self-contained)
server.go     ──depends on──> pages/*, hx/  (wires everything together)
```

## What Gets Removed

- `internal/server/handlers.go` — split into page packages
- `internal/server/templates/` — entire directory, templates colocated with handlers
- `internal/server/pages/handler.go` — replaced by `pages/dashboard/page.go`
- Old `buildSidebarAny` / `getRepoStatusString` wrappers in `server.go`
- `parsePages()` function — replaced by `templates.go`

## What Gets Preserved

- `server.go` state management (repoStatus, cancelFuncs, sessionMu, repoMu sync.Maps)
- All existing handler logic (just relocated)
- All existing template HTML (just relocated and path-renamed in `{{template}}` calls)
- Static assets (unchanged)
- `FuncMap` (moved to `templates.go`)
