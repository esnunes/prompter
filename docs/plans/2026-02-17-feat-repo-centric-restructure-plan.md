---
title: "feat: Repository-Centric Restructure"
type: feat
date: 2026-02-17
brainstorm: docs/brainstorms/2026-02-17-repo-centric-restructure-brainstorm.md
---

# feat: Repository-Centric Restructure

## Overview

Restructure the app around repository-specific pages. Remove the CLI repository argument so `prompter` launches with no args. Add repo-scoped pages and URLs, async clone/pull with HTMX polling status feedback, and a dashboard URL input for discovering repos.

## Problem Statement / Motivation

Currently the app is single-repo: the CLI requires a `github.com/org/repo` argument, clones it synchronously (blocking startup), and scopes the entire session to that one repo. The DB and server already support multiple repos, but the UX does not. Users must restart the CLI to switch repos.

## Proposed Solution

- Remove the CLI repo argument. The app starts immediately and serves a multi-repo experience.
- Add repo pages at `/github.com/{org}/{repo}/prompt-requests` that list prompt requests for a specific repo and allow creating new ones.
- Move conversation views to repo-scoped URLs: `/github.com/{org}/{repo}/prompt-requests/{id}`.
- Clone/pull repos asynchronously when a prompt request is created, showing real-time status via HTMX polling.
- Allow the user to type and submit their first message during download; auto-process it when the repo is ready.

## Technical Approach

### Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Status feedback | HTMX polling (~2s) | Simple, consistent with existing patterns |
| Conversation URLs | Repo-scoped | Consistent URL structure |
| Repo verification | `gh` CLI | Authenticated, no rate limits |
| Clone/pull status storage | In-memory `sync.Map` | Transient state, no DB changes |
| Clone/pull trigger | Goroutine on create | Download starts ASAP |
| Message queuing | One message, auto-send | Simple, clear UX |
| Old URL handling | 404 (no redirects) | Simple, clean break |
| Dashboard entry point | URL input field | Discoverable, no external API needed |
| Error recovery | Error message + retry button | User-controlled recovery |

### Clone/Pull State Machine

```
                  ┌──────────┐
    create PR ──> │ cloning  │──── success ──> ready
                  │ pulling  │
                  └──────────┘
                       │
                     error
                       │
                       v
                  ┌──────────┐
                  │  error   │──── retry ──> cloning/pulling
                  └──────────┘
```

States: `cloning`, `pulling`, `ready`, `error`

On server restart with no `sync.Map` entry: check filesystem for `.git` dir. If exists, status is `ready`. If not, auto-start clone.

### Polling Endpoint Contract

**URL:** `GET /github.com/{org}/{repo}/prompt-requests/{id}/status`

**Response:** HTML fragment for HTMX swap into the status area.

**States and responses:**

- `cloning`: `<div id="repo-status" hx-get="..." hx-trigger="every 2s" hx-swap="outerHTML">Cloning repository...</div>`
- `pulling`: Same structure, "Pulling latest changes..."
- `ready` (no pending message): Returns HTTP 200 with stop-polling content (no `hx-trigger`): `<div id="repo-status">Repository ready!</div>`
- `ready` (pending message, Claude not started): Returns fragment that triggers Claude processing and swaps to "Processing your message..." with continued polling
- `ready` (Claude finished): Returns the Claude response fragment (same format as `message_fragment.html`), no more polling
- `error`: `<div id="repo-status"><span>Error: {message}</span> <button hx-post=".../{id}/retry">Retry</button></div>`

Polling stops when the response omits `hx-trigger="every 2s"` (HTMX's natural behavior).

### Concurrency

- **Per-repo mutex** (`sync.Map` keyed by repo URL) to serialize clone/pull operations for the same repo. Second goroutine waits for the first, then checks `.git` existence to decide if work is still needed.
- **Existing session mutex** continues to serialize Claude CLI calls per session.
- **Auto-send acquires session mutex** before invoking Claude, preventing races with manual sends.

### Architecture

```
                   Dashboard (/)
                   ┌─────────────────────┐
                   │ URL input field      │
                   │ [github.com/org/repo]│──> navigates to repo page
                   │                      │
                   │ Prompt Request Cards  │
                   │ [repo name] [title]  │──> repo name links to repo page
                   │                      │──> card links to conversation
                   └─────────────────────┘

         Repo Page (/github.com/{org}/{repo}/prompt-requests)
         ┌─────────────────────┐
         │ [< Dashboard]       │
         │ github.com/org/repo │
         │                     │
         │ [Create New]        │──> POST creates PR + async clone/pull
         │                     │    redirects to conversation
         │ PR list (filtered)  │
         └─────────────────────┘

         Conversation (/github.com/{org}/{repo}/prompt-requests/{id})
         ┌─────────────────────┐
         │ Chat area            │
         │ ┌───────────────┐   │
         │ │ #repo-status  │   │  <── HTMX polling during clone/pull
         │ │ "Cloning..."  │   │
         │ └───────────────┘   │
         │                     │
         │ [message input]     │  <── disabled after submit during download
         │ Revision sidebar    │
         └─────────────────────┘
```

### Implementation Phases

#### Phase 1: CLI Simplification + Database Query

Remove the CLI repo argument and add the missing DB query.

**Files changed:**

- `cmd/prompter/main.go` — Remove args parsing (lines 28-38), remove `repo.ValidateURL` (line 40), remove `repo.EnsureCloned` (lines 48-51), remove `queries.UpsertRepository` (lines 65-68). The CLI becomes: check deps → open DB → start server → open browser.
- `internal/db/queries.go` — Add `ListPromptRequestsByRepoURL(repoURL string)` method that filters prompt requests by joining with `repositories` on URL. Same query shape as `ListPromptRequests()` but with a `WHERE r.url = ?` clause.

```go
// cmd/prompter/main.go — simplified main
func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
    defer stop()

    if err := run(ctx); err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }
}

func run(ctx context.Context) error {
    if err := deps.Check(ctx); err != nil {
        return err
    }

    dbPath, err := paths.DBPath()
    // ... open DB, create queries, start server (no repo logic)
}
```

```go
// internal/db/queries.go — new query
func (q *Queries) ListPromptRequestsByRepoURL(repoURL string) ([]models.PromptRequest, error) {
    // Same as ListPromptRequests but with WHERE r.url = ?
}
```

#### Phase 2: Repo Page + Dashboard Entry Point

Add the repo page and update the dashboard.

**Files changed:**

- `internal/server/server.go` — Add routes: `GET /github.com/{org}/{repo}/prompt-requests` and remove `GET /new`. Add `repoMu sync.Map` field for per-repo clone/pull mutex.
- `internal/server/handlers.go` — Add `handleRepoPage` handler: extracts `{org}` and `{repo}` from path, constructs `github.com/org/repo`, verifies via `gh api repos/org/repo`, queries DB for repo and its prompt requests, renders page. Remove `handleNew`.
- `internal/github/github.go` — Add `VerifyRepo(ctx, org, repo string) error` that runs `gh api repos/{org}/{repo}` and returns nil on success, error on 404/failure.
- `internal/server/templates/repo.html` — New template. Shows repo name, "Back to dashboard" link, list of prompt requests (same card style as dashboard), "Create new prompt request" button. Error state for non-existent repos. Empty state when no prompt requests.
- `internal/server/templates/dashboard.html` — Add URL input field at top (form with text input + Go button, navigates to `/github.com/{input}/prompt-requests`). Make repo names on cards clickable links to repo pages.
- `internal/server/templates/new.html` — Delete this file.

```go
// internal/server/handlers.go — repo page handler
func (s *Server) handleRepoPage(w http.ResponseWriter, r *http.Request) {
    org := r.PathValue("org")
    repo := r.PathValue("repo")
    repoURL := fmt.Sprintf("github.com/%s/%s", org, repo)

    // Verify repo exists on GitHub
    if err := github.VerifyRepo(r.Context(), org, repo); err != nil {
        s.renderPage(w, "repo.html", repoData{
            RepoURL: repoURL,
            Error:   "This repository doesn't exist on GitHub.",
        })
        return
    }

    // List prompt requests for this repo
    prs, _ := s.queries.ListPromptRequestsByRepoURL(repoURL)
    s.renderPage(w, "repo.html", repoData{
        RepoURL:        repoURL,
        Org:            org,
        Repo:           repo,
        PromptRequests: prs,
    })
}
```

```html
<!-- internal/server/templates/dashboard.html — URL input addition -->
<form action="" method="GET" onsubmit="...navigate to repo page...">
  <input type="text" placeholder="github.com/owner/repo" />
  <button type="submit">Go</button>
</form>
```

#### Phase 3: Repo-Scoped Conversation URLs + Create Flow

Move conversation views to repo-scoped URLs and streamline creation.

**Files changed:**

- `internal/server/server.go` — Replace routes:
  - Remove: `POST /prompt-requests`, `GET /prompt-requests/{id}`, `POST /prompt-requests/{id}/messages`, `POST /prompt-requests/{id}/publish`, `DELETE /prompt-requests/{id}`
  - Add: `POST /github.com/{org}/{repo}/prompt-requests`, `GET /github.com/{org}/{repo}/prompt-requests/{id}`, `POST /github.com/{org}/{repo}/prompt-requests/{id}/messages`, `POST /github.com/{org}/{repo}/prompt-requests/{id}/publish`, `DELETE /github.com/{org}/{repo}/prompt-requests/{id}`
- `internal/server/handlers.go` — Modify `handleCreate`: no form parsing needed (repo from URL), upsert repo in DB, create prompt request, redirect to new URL. Modify `handleShow`: extract org/repo from path, pass to template for URL construction. Modify `handlePublish` redirect URL. Modify `handleDelete` to redirect to repo page instead of dashboard.
- `internal/server/templates/conversation.html` — Update all `hx-post` URLs to use repo-scoped paths. Add template data for org/repo.
- `internal/server/templates/message_fragment.html` — Update question form `hx-post` URLs.
- `internal/server/templates/dashboard.html` — Update card links to use repo-scoped URLs.
- `internal/server/templates/repo.html` — Add create button that POSTs to the repo-scoped create endpoint.

```go
// internal/server/handlers.go — streamlined create
func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
    org := r.PathValue("org")
    repo := r.PathValue("repo")
    repoURL := fmt.Sprintf("github.com/%s/%s", org, repo)

    // Upsert repo
    localPath, _ := repomod.LocalPath(repoURL)
    repoRecord, _ := s.queries.UpsertRepository(repoURL, localPath)

    // Create prompt request
    sessionID := uuid.New().String()
    pr, _ := s.queries.CreatePromptRequest(repoRecord.ID, sessionID)

    // Redirect to conversation (async clone/pull added in Phase 4)
    http.Redirect(w, r, fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d", org, repo, pr.ID), http.StatusSeeOther)
}
```

#### Phase 4: Async Clone/Pull with Polling Status

Add the async clone/pull goroutine, status tracking, and HTMX polling.

**Files changed:**

- `internal/server/server.go` — Add `repoStatus sync.Map` field (keyed by prompt request ID, stores status struct). Add `repoMu sync.Map` field (keyed by repo URL, stores `*sync.Mutex` for serializing clone/pull per repo).
- `internal/server/handlers.go`:
  - Modify `handleCreate` to launch async goroutine after creating the prompt request:
    ```go
    s.setRepoStatus(pr.ID, "cloning") // or "pulling"
    go s.asyncEnsureCloned(pr.ID, repoURL)
    ```
  - Add `asyncEnsureCloned(prID, repoURL)` method: acquires per-repo mutex, calls `repo.EnsureCloned`, updates status to `ready` or `error`.
  - Add `handleRepoStatus(w, r)` handler: reads status from `sync.Map`, returns HTML fragment. If no status entry, checks filesystem — if `.git` exists, returns `ready`; if not, starts clone goroutine and returns `cloning`.
  - Add `handleRetry(w, r)` handler: resets status to `cloning`/`pulling`, starts new goroutine.
  - Modify `handleSendMessage`: if repo status is not `ready`, save message to DB and return early (message will be auto-sent by polling endpoint when ready). If `ready`, process normally.
- `internal/server/server.go` — Add routes: `GET /github.com/{org}/{repo}/prompt-requests/{id}/status`, `POST /github.com/{org}/{repo}/prompt-requests/{id}/retry`
- `internal/server/templates/conversation.html` — Add polling div in assistant area:
  ```html
  <div id="repo-status"
       hx-get="/github.com/{{.Org}}/{{.Repo}}/prompt-requests/{{.PromptRequest.ID}}/status"
       hx-trigger="every 2s"
       hx-swap="outerHTML">
    Preparing repository...
  </div>
  ```
  Only render this div when status is not `ready`.
- `internal/server/templates/status_fragment.html` — New fragment template for polling responses. Renders different content based on status (cloning/pulling/ready/error).
- `internal/repo/repo.go` — Modify `clone` and `pull` to suppress stdout/stderr (capture to buffer or discard) since they now run from goroutines, not terminal.

```go
// internal/server/server.go — status tracking types
type repoStatusEntry struct {
    Status string // "cloning", "pulling", "ready", "error"
    Error  string // error message if status == "error"
}

// internal/server/handlers.go — async clone
func (s *Server) asyncEnsureCloned(prID int64, repoURL string) {
    // Acquire per-repo mutex
    v, _ := s.repoMu.LoadOrStore(repoURL, &sync.Mutex{})
    mu := v.(*sync.Mutex)
    mu.Lock()
    defer mu.Unlock()

    _, err := repo.EnsureCloned(context.Background(), repoURL)
    if err != nil {
        s.repoStatus.Store(prID, repoStatusEntry{Status: "error", Error: err.Error()})
        return
    }
    s.repoStatus.Store(prID, repoStatusEntry{Status: "ready"})
}
```

```go
// internal/server/handlers.go — status endpoint with auto-send
func (s *Server) handleRepoStatus(w http.ResponseWriter, r *http.Request) {
    prID := // extract from path
    entry := s.getRepoStatus(prID)

    if entry.Status == "ready" {
        // Check for pending user message
        lastMsg, _ := s.queries.GetLastMessage(prID)
        if lastMsg != nil && lastMsg.Role == "user" {
            // Auto-send: invoke Claude, return response fragment
            // (acquire session mutex, call claude.SendMessage, save response)
            s.renderFragment(w, "message_fragment.html", fragmentData)
            return
        }
        // No pending message — just show ready, stop polling
        s.renderFragment(w, "status_fragment.html", statusData{Status: "ready"})
        return
    }

    // Still in progress or error — return status with continued polling
    s.renderFragment(w, "status_fragment.html", statusData{
        Status: entry.Status,
        Error:  entry.Error,
        PollURL: fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d/status", org, repo, prID),
    })
}
```

#### Phase 5: Message Input Disable During Download

Disable the chat input after the user submits a message while the repo is downloading.

**Files changed:**

- `internal/server/templates/conversation.html` — When status is not `ready`, the message form submission should:
  1. POST the message to the messages endpoint (saves to DB)
  2. Disable the input and show a "Message queued" indicator
  3. The polling endpoint handles the auto-send

- `internal/server/handlers.go` — Modify `handleSendMessage`: if repo not ready, save message to DB and return a fragment that shows the user message bubble + disabled input state (no Claude call).

```html
<!-- Conversation template — form behavior during download -->
<form hx-post="/github.com/{{.Org}}/{{.Repo}}/prompt-requests/{{.PromptRequest.ID}}/messages"
      hx-target="#conversation"
      hx-swap="beforeend"
      hx-disabled-elt="find button, find textarea">
  <textarea name="message" placeholder="Type your message..."></textarea>
  <button type="submit">Send</button>
</form>
```

## Acceptance Criteria

### Functional Requirements

- [x] `prompter` starts with no arguments and launches the web server
- [x] CLI no longer accepts a repository argument
- [x] `/new` page is removed
- [x] Visiting `/github.com/{org}/{repo}/prompt-requests` shows a repo page with prompt request list
- [x] Non-existent repos (verified via `gh`) show an error message
- [x] Repos not yet in the app show empty state with "Create" button
- [x] Creating from repo page skips the form and goes straight to conversation
- [x] Conversation URLs are repo-scoped: `/github.com/{org}/{repo}/prompt-requests/{id}`
- [x] Dashboard shows clickable repo names linking to repo pages
- [x] Dashboard has a URL input field for navigating to any repo
- [x] Clone/pull runs asynchronously on prompt request creation
- [x] Status polling shows "Cloning..."/"Pulling..." in the assistant area
- [x] Clone/pull errors show error message with retry button
- [x] User can type and submit one message during download
- [x] Message auto-sends to Claude when repo is ready
- [x] Chat input is disabled after submitting during download
- [x] Concurrent creates for the same repo are serialized (per-repo mutex)
- [x] Server restart recovery: checks filesystem, auto-starts clone if needed

### Non-Functional Requirements

- [x] No new JS dependencies (HTMX polling is declarative)
- [x] No DB schema changes (in-memory status only)
- [x] Existing conversations remain accessible at new URLs
- [x] `git clone`/`pull` stdout/stderr no longer writes to terminal

## Dependencies & Risks

**Dependencies:**
- `gh` CLI must be authenticated (already checked at startup via `deps.Check`)
- Go 1.22+ ServeMux with multi-segment path parameters

**Risks:**
- Large repos (e.g., `torvalds/linux`) may take many minutes to clone. Mitigation: no hard timeout, polling shows ongoing status.
- Private repos may pass `gh api` verification but fail `git clone` if git credential helper isn't configured. Mitigation: error + retry catches this; error message will be descriptive.
- Path traversal via org/repo URL segments. Mitigation: existing `repo.ValidateURL` regex (`[\w.\-]+`) prevents `../`; apply validation before any path construction.

## References & Research

### Internal References

- Brainstorm: `docs/brainstorms/2026-02-17-repo-centric-restructure-brainstorm.md`
- CLI entry: `cmd/prompter/main.go:28-68` (code to remove)
- Route registration: `internal/server/server.go:52-67`
- Handler patterns: `internal/server/handlers.go`
- DB queries: `internal/db/queries.go:103` (`ListPromptRequests` — pattern for new query)
- Repo clone/pull: `internal/repo/repo.go:31-72`
- Session mutex pattern: `internal/server/server.go:159-165` (pattern for repo mutex)
- GitHub integration: `internal/github/github.go` (add `VerifyRepo`)
- Template parsing: `internal/server/server.go:73-110`

### Institutional Learnings

- Claude CLI structured output parsing: `docs/solutions/integration-issues/claude-cli-structured-output-parsing.md`
- Session resume flag: `docs/solutions/integration-issues/claude-cli-session-resume-flag.md`
- Route consolidation pattern: `docs/solutions/ui-bugs/published-prompt-request-404-continue-conversation.md`
