# Server Structure Refactoring Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Reorganize `internal/server/` so handlers and templates are colocated by page, HTMX fragments live under `/hx/` URLs, and the codebase is easy for AI agents to navigate and extend.

**Architecture:** Pages are full HTML (layout-wrapped). HTMX fragments are bare partials under `/hx/`. Each page is a Go package with a `Page` struct holding dependencies. A single `embed.go` at `internal/server/` embeds all templates. A central `templates.go` aggregates them into one `*template.Template`. A central `routes.go` registers all routes.

**Tech Stack:** Go 1.25.5, html/template, HTMX, SSE (htmx-ext-sse)

**Design doc:** `docs/plans/2026-03-16-refactor-server-structure-design.md`

---

### Task 1: Create templates.go and embed.go foundation

Create the new template loading infrastructure that will replace `parsePages()`.

**Files:**
- Create: `internal/server/embed.go`
- Create: `internal/server/templates.go`

**Step 1: Create embed.go**

```go
// internal/server/embed.go
package server

import "embed"

//go:embed pages/*.html pages/*/*.html hx/*.html static
var contentFS embed.FS
```

**Step 2: Create templates.go**

```go
// internal/server/templates.go
package server

import (
	"html/template"
	"io/fs"
	"path/filepath"
)

var funcMap = template.FuncMap{
	"deref": func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	},
}

func loadTemplates() (*template.Template, error) {
	t := template.New("").Funcs(funcMap)

	dirs := []string{"pages", "pages/dashboard", "pages/repo", "pages/conversation", "hx"}
	for _, dir := range dirs {
		err := fs.WalkDir(contentFS, dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(path) != ".html" {
				return err
			}
			data, readErr := fs.ReadFile(contentFS, path)
			if readErr != nil {
				return readErr
			}
			template.Must(t.New(path).Parse(string(data)))
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return t, nil
}
```

Note: this doesn't compile yet — the directories don't exist. It will compile after Task 2.

**Step 3: Commit**

```
git add internal/server/embed.go internal/server/templates.go
git commit -m "refactor: add templates.go and embed.go foundation"
```

---

### Task 2: Create pages/base.html and move layout

Move `templates/layout.html` to `pages/base.html`. Update the template to use path-based `{{template}}` names.

**Files:**
- Create: `internal/server/pages/base.html` (copy from `templates/layout.html`)
- Modify: template references inside `base.html`

**Step 1: Copy layout.html to pages/base.html**

Copy `internal/server/templates/layout.html` to `internal/server/pages/base.html`.

In `base.html`, change the sidebar template reference:
```html
<!-- old -->
{{template "sidebar.html" .Sidebar}}
<!-- new -->
{{template "hx/sidebar.html" .Sidebar}}
```

**Step 2: Commit**

```
git add internal/server/pages/base.html
git commit -m "refactor: copy layout to pages/base.html with path-based template names"
```

---

### Task 3: Move sidebar to use SSE trigger

Update `hx/sidebar.html` to replace polling with SSE trigger. Remove polling attributes and add SSE-based refresh.

**Files:**
- Modify: `internal/server/hx/sidebar.html`

**Step 1: Update sidebar.html**

Change the `<aside>` element:
```html
<!-- old -->
<aside class="prompt-sidebar" id="prompt-sidebar"
       hx-get="{{.PollURL}}"
       hx-trigger="every 3s"
       hx-swap="outerHTML">
<!-- new -->
<aside class="prompt-sidebar" id="prompt-sidebar"
       hx-get="/hx/sidebar?scope={{.Scope}}{{if .RepoURL}}&repo_url={{.RepoURL}}{{end}}{{if .CurrentID}}&current_id={{.CurrentID}}{{end}}"
       hx-trigger="sse:prompt-updated"
       hx-swap="outerHTML">
```

Update `SidebarData` struct in `hx/sidebar.go` to add `RepoURL` field (replacing `PollURL`). Remove `PollURL` from `BuildSidebar`. Update template to use the new fields.

**Step 2: Commit**

```
git commit -am "refactor: replace sidebar polling with SSE trigger"
```

---

### Task 4: Create pages/dashboard package

Move dashboard from `pages/handler.go` + `pages/dashboard.html` into `pages/dashboard/page.go` + `pages/dashboard/page.html`. Move create_repository from `hx/` into `pages/dashboard/`.

**Files:**
- Create: `internal/server/pages/dashboard/page.go`
- Move: `internal/server/pages/dashboard.html` → `internal/server/pages/dashboard/page.html`
- Move: `internal/server/hx/create_repository.go` → `internal/server/pages/dashboard/create_repository.go`
- Move: `internal/server/hx/create_repository.html` → `internal/server/pages/dashboard/create_repository.html`
- Delete: `internal/server/pages/handler.go`
- Delete: `internal/server/pages/dashboard.html`
- Delete: `internal/server/hx/create_repository.go`
- Delete: `internal/server/hx/create_repository.html`

**Step 1: Create pages/dashboard/page.go**

Define the `Page` struct with dependencies and handlers:

```go
package dashboard

import (
	"html/template"
	"log"
	"net/http"

	"github.com/esnunes/prompter/internal/db"
	"github.com/esnunes/prompter/internal/models"
)

type Page struct {
	Tmpl         *template.Template
	Queries      *db.Queries
	BuildSidebar func(prs []models.PromptRequest, scope string, currentID int64) any
}

type pageData struct {
	Sidebar      any
	Form         CreateRepositoryData
	Repositories []models.RepositorySummary
}

func (p *Page) HandlePage(w http.ResponseWriter, r *http.Request) {
	repos, err := p.Queries.ListRepositorySummaries()
	if err != nil {
		log.Printf("listing repository summaries: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	sidebarPRs, _ := p.Queries.ListPromptRequests(false)
	sidebar := p.BuildSidebar(sidebarPRs, "all", 0)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.Tmpl.ExecuteTemplate(w, "pages/base.html", pageData{
		Sidebar:      sidebar,
		Form:         CreateRepositoryData{},
		Repositories: repos,
	}); err != nil {
		log.Printf("render error (dashboard): %v", err)
	}
}
```

**Step 2: Move and rename template files**

- Move `pages/dashboard.html` → `pages/dashboard/page.html`
- Update `{{template "create_repository.html" .Form}}` → `{{template "pages/dashboard/create_repository.html" .Form}}`
- Move `hx/create_repository.html` → `pages/dashboard/create_repository.html`
- Update `hx-post` URL in create_repository.html: `/hx/create-repository` → `/hx/dashboard/create-repository`

**Step 3: Create pages/dashboard/create_repository.go**

Move handler from `hx/create_repository.go`, change package to `dashboard`, update to use `Page` struct receiver. The `CreateRepositoryData` type and `sanitizeRepoURL` function also move here.

```go
package dashboard

import (
	"net/http"
	"strings"

	"github.com/esnunes/prompter/internal/repo"
)

type CreateRepositoryData struct {
	RepoURL string
	Error   string
}

func (p *Page) HandleCreateRepository(w http.ResponseWriter, r *http.Request) {
	// ... same logic as hx/create_repository.go handleCreateRepository
	// but uses p.Tmpl.ExecuteTemplate(w, "pages/dashboard/create_repository.html", data)
	// and p.Queries instead of h.queries
}
```

**Step 4: Delete old files**

- Delete `internal/server/pages/handler.go`
- Delete `internal/server/hx/create_repository.go`
- Delete `internal/server/hx/create_repository.html`

**Step 5: Build and verify**

Run: `go build ./...`
Expected: PASS

**Step 6: Commit**

```
git commit -am "refactor: create pages/dashboard package with colocated create_repository"
```

---

### Task 5: Create pages/repo package

Move the repo page handler from `handlers.go` into its own package.

**Files:**
- Create: `internal/server/pages/repo/page.go`
- Move: `internal/server/templates/repo.html` → `internal/server/pages/repo/page.html`
- Modify: `internal/server/handlers.go` (remove `handleRepoPage`, `repoData`)

**Step 1: Create pages/repo/page.go**

```go
package repo

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/esnunes/prompter/internal/db"
	"github.com/esnunes/prompter/internal/github"
	"github.com/esnunes/prompter/internal/models"
	repoLib "github.com/esnunes/prompter/internal/repo"
)

type Page struct {
	Tmpl         *template.Template
	Queries      *db.Queries
	BuildSidebar func(prs []models.PromptRequest, scope string, currentID int64) any
}

type pageData struct {
	Sidebar        any
	RepoURL        string
	Org            string
	Repo           string
	Error          string
	PromptRequests []models.PromptRequest
	ShowArchived   bool
}

func (p *Page) HandlePage(w http.ResponseWriter, r *http.Request) {
	// ... same logic as handlers.go handleRepoPage
	// uses p.Tmpl.ExecuteTemplate(w, "pages/base.html", data)
}
```

**Step 2: Move template**

Move `templates/repo.html` → `pages/repo/page.html`. No URL changes needed in the template — the repo page doesn't have HTMX fragment URLs that need updating (links to conversations use full page navigation).

**Step 3: Remove from handlers.go**

Remove `repoData` type and `handleRepoPage` method from `handlers.go`.

**Step 4: Build and verify**

Run: `go build ./...`

**Step 5: Commit**

```
git commit -am "refactor: create pages/repo package"
```

---

### Task 6: Create pages/conversation package

This is the largest task. Move all conversation-related handlers from `handlers.go` into `pages/conversation/`.

**Files:**
- Create: `internal/server/pages/conversation/page.go` — Page struct, HandlePage (show), types, helper functions
- Create: `internal/server/pages/conversation/send_message.go` — HandleSendMessage
- Create: `internal/server/pages/conversation/status.go` — HandleStatus
- Create: `internal/server/pages/conversation/publish.go` — HandlePublish
- Create: `internal/server/pages/conversation/delete.go` — HandleDelete
- Create: `internal/server/pages/conversation/archive.go` — HandleArchive, HandleUnarchive
- Create: `internal/server/pages/conversation/cancel.go` — HandleCancel
- Create: `internal/server/pages/conversation/retry.go` — HandleRetry
- Create: `internal/server/pages/conversation/resend.go` — HandleResend
- Move: `internal/server/templates/conversation.html` → `internal/server/pages/conversation/page.html`
- Move: `internal/server/templates/message_fragment.html` → `internal/server/pages/conversation/send_message.html`
- Move: `internal/server/templates/status_fragment.html` → `internal/server/pages/conversation/status.html`
- Move: `internal/server/templates/archive_banner_fragment.html` → `internal/server/pages/conversation/archive_banner.html`
- Delete: `internal/server/handlers.go` (should be empty after this)

**Step 1: Create pages/conversation/page.go**

The Page struct needs all the server state that conversation handlers depend on:

```go
package conversation

import (
	"context"
	"html/template"
	"sync"
	"time"

	"github.com/esnunes/prompter/internal/db"
	"github.com/esnunes/prompter/internal/models"
)

type Page struct {
	Tmpl                   *template.Template
	Queries                *db.Queries
	BuildSidebar           func(prs []models.PromptRequest, scope string, currentID int64) any
	GetRepoStatus          func(int64) repoStatusEntry
	SetRepoStatus          func(int64, string, string)
	SetRepoStatusProcessing func(int64, context.CancelFunc)
	ClearCancelFunc        func(int64)
	RepoStatus             *sync.Map   // direct access for CompareAndSwap
	CancelFuncs            *sync.Map   // direct access for cancel lookup
	LockSession            func(string) *sync.Mutex
	LockRepo               func(string) *sync.Mutex
}

// repoStatusEntry mirrors the server package's type.
type repoStatusEntry struct {
	Status    string
	Error     string
	StartedAt time.Time
}
```

This file also contains:
- `HandlePage` (from `handleShow`)
- All shared types: `conversationData`, `timelineItem`, `questionData`, `optionData`, `messageFragmentData`, `archiveBannerData`, `statusFragmentData`
- All shared helper functions: `buildTimeline`, `extractQuestionsFromRaw`, `extractLegacyQuestion`, `assembleQuestionAnswers`, `parseRawResponse`

**Step 2: Create individual handler files**

Each handler file contains one handler method on `*Page`:

- `send_message.go`: `HandleSendMessage` (from `handleSendMessage`) + `backgroundSendMessage`
- `status.go`: `HandleStatus` (from `handleRepoStatus`) + `asyncEnsureCloned`
- `publish.go`: `HandlePublish` (from `handlePublish`)
- `delete.go`: `HandleDelete` (from `handleDelete`)
- `archive.go`: `HandleArchive` + `HandleUnarchive` (from `handleArchive`, `handleUnarchive`)
- `cancel.go`: `HandleCancel` (from `handleCancel`)
- `retry.go`: `HandleRetry` (from `handleRetry`)
- `resend.go`: `HandleResend` (from `handleResend`)

**Key changes in each handler:**
- Replace `s.renderPage(w, "conversation.html", data)` → `p.Tmpl.ExecuteTemplate(w, "pages/base.html", data)`
- Replace `s.renderFragment(w, "status_fragment.html", data)` → `p.Tmpl.ExecuteTemplate(w, "pages/conversation/status.html", data)`
- Replace `s.pages["message_fragment.html"].ExecuteTemplate(...)` → `p.Tmpl.ExecuteTemplate(w, "pages/conversation/send_message.html", data)`
- Replace `s.renderFragment(w, "archive_banner_fragment.html", data)` → `p.Tmpl.ExecuteTemplate(w, "pages/conversation/archive_banner.html", data)`
- Replace `s.queries` → `p.Queries`
- Replace `s.getRepoStatus(id)` → `p.GetRepoStatus(id)`
- Replace `s.setRepoStatus(...)` → `p.SetRepoStatus(...)`
- etc.

**Step 3: Move and rename templates**

Move each template to its new location. Update `{{template}}` references inside templates to use path-based names. Update `hx-get`, `hx-post` URLs in templates to use `/hx/conversation/...` pattern:

In `page.html` (conversation.html):
- `hx-get="/github.com/{{.Org}}/{{.Repo}}/prompt-requests/{{.PromptRequest.ID}}/status"` → `hx-get="/hx/conversation/status?id={{.PromptRequest.ID}}"`
- `hx-post="/github.com/{{.Org}}/{{.Repo}}/prompt-requests/{{.PromptRequest.ID}}/messages"` → `hx-post="/hx/conversation/send-message"`
- `hx-post="/github.com/{{.Org}}/{{.Repo}}/prompt-requests/{{.PromptRequest.ID}}/cancel"` → `hx-post="/hx/conversation/cancel"`
- `hx-post="/github.com/{{.Org}}/{{.Repo}}/prompt-requests/{{.PromptRequest.ID}}/resend"` → `hx-post="/hx/conversation/resend"`
- `hx-post="/github.com/{{.Org}}/{{.Repo}}/prompt-requests/{{.PromptRequest.ID}}/publish"` → `hx-post="/hx/conversation/publish"`
- `hx-post="/github.com/{{.Org}}/{{.Repo}}/prompt-requests/{{.PromptRequest.ID}}/unarchive"` → `hx-post="/hx/conversation/unarchive"`
- Archive fetch URL: `/github.com/{{.Org}}/{{.Repo}}/prompt-requests/{{.PromptRequest.ID}}/archive` → `/hx/conversation/archive`

Add hidden `<input type="hidden" name="id" value="{{.PromptRequest.ID}}">` to all forms that need the prompt request ID (since the ID is no longer in the URL path).

Similarly update URLs in `status.html` and `send_message.html` for poll/cancel/retry/resend URLs.

In `send_message.html` (message_fragment.html):
- Update form action URLs to `/hx/conversation/send-message`, `/hx/conversation/publish`
- Add hidden ID input

In `status.html` (status_fragment.html):
- `{{.PollURL}}` → `/hx/conversation/status?id={{.PromptRequestID}}`
- `{{.RetryURL}}` → keep using data fields but now with `/hx/conversation/retry` etc.
- Add hidden ID input to forms

**Step 4: Update handler URL parsing**

All conversation handlers currently extract `org`, `repo`, and `id` from path values. With `/hx/conversation/...` URLs, the ID comes from form values or query params instead:

```go
// old (path-based)
id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

// new (for POST handlers)
id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)

// new (for GET handlers like status)
id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
```

The `org` and `repo` are derived from the prompt request's `RepoURL` field in the DB:
```go
pr, err := p.Queries.GetPromptRequest(id)
parts := strings.SplitN(pr.RepoURL, "/", 3) // "github.com/org/repo"
org, repoName := parts[1], parts[2]
```

**Step 5: Delete handlers.go**

After moving everything out, `handlers.go` should be empty. Delete it.

**Step 6: Build and verify**

Run: `go build ./...`

**Step 7: Commit**

```
git commit -am "refactor: create pages/conversation package with all conversation handlers"
```

---

### Task 7: Create hx/create_prompt_request.go

Move `handleCreate` (POST to create a new prompt request) from the old handlers into `hx/` since it's used from both the repo page and conversation page.

**Files:**
- Create: `internal/server/hx/create_prompt_request.go`

**Step 1: Create the handler**

```go
package hx

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/esnunes/prompter/internal/db"
	"github.com/esnunes/prompter/internal/repo"
	"github.com/google/uuid"
)

type CreatePromptRequestHandler struct {
	Queries        *db.Queries
	SetRepoStatus  func(int64, string, string)
	AsyncEnsureCloned func(int64, string)
}

func (h *CreatePromptRequestHandler) Handle(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")
	repoURL := fmt.Sprintf("github.com/%s/%s", org, repoName)

	localPath, err := repo.LocalPath(repoURL)
	if err != nil {
		log.Printf("computing local path: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	repoRecord, err := h.Queries.UpsertRepository(repoURL, localPath)
	if err != nil {
		log.Printf("upserting repository: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	sessionID := uuid.New().String()
	pr, err := h.Queries.CreatePromptRequest(repoRecord.ID, sessionID)
	if err != nil {
		log.Printf("creating prompt request: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	cloned, _ := repo.IsCloned(repoURL)
	if cloned {
		h.SetRepoStatus(pr.ID, "pulling", "")
	} else {
		h.SetRepoStatus(pr.ID, "cloning", "")
	}

	go h.AsyncEnsureCloned(pr.ID, repoURL)

	http.Redirect(w, r, fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d", org, repoName, pr.ID), http.StatusSeeOther)
}
```

Note: this handler keeps the existing URL pattern `POST /github.com/{org}/{repo}/prompt-requests` since it's a form action that does a full redirect. Alternatively, it could use `POST /hx/create-prompt-request` with `HX-Redirect`. Choose whichever is consistent with the rest of the app.

**Step 2: Build and verify**

Run: `go build ./...`

**Step 3: Commit**

```
git commit -am "refactor: move create prompt request handler to hx package"
```

---

### Task 8: Update hx/handler.go to remove create_repository references

Now that create_repository moved to dashboard, clean up `hx/handler.go`. The `Handler` struct should only manage the sidebar.

**Files:**
- Modify: `internal/server/hx/handler.go`

**Step 1: Simplify hx/handler.go**

Remove `createRepoTmpl` field. The handler struct now only needs sidebar template and queries:

```go
package hx

import (
	"html/template"
	"net/http"

	"github.com/esnunes/prompter/internal/db"
)

type Handler struct {
	tmpl          *template.Template
	queries       *db.Queries
	getRepoStatus func(int64) string
}

func New(tmpl *template.Template, queries *db.Queries, getRepoStatus func(int64) string) *Handler {
	return &Handler{tmpl: tmpl, queries: queries, getRepoStatus: getRepoStatus}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /hx/sidebar", h.handleSidebarFragment)
}
```

Update `handleSidebarFragment` in `sidebar.go` to use `h.tmpl.ExecuteTemplate(w, "hx/sidebar.html", sidebar)`.

Remove the `TemplateFS` export and the `//go:embed` directive from `hx/handler.go` since templates are now embedded at the server level.

**Step 2: Build and verify**

Run: `go build ./...`

**Step 3: Commit**

```
git commit -am "refactor: simplify hx package, remove embed and create_repository references"
```

---

### Task 9: Create routes.go and update server.go

Create a central `routes.go` with all route registration. Update `server.go` to use `loadTemplates()` instead of `parsePages()`.

**Files:**
- Create: `internal/server/routes.go`
- Modify: `internal/server/server.go`

**Step 1: Create routes.go**

```go
package server

import (
	"io/fs"
	"net/http"

	"github.com/esnunes/prompter/internal/server/hx"
	"github.com/esnunes/prompter/internal/server/pages/conversation"
	"github.com/esnunes/prompter/internal/server/pages/dashboard"
	"github.com/esnunes/prompter/internal/server/pages/repo"
)

func (s *Server) registerRoutes(mux *http.ServeMux) error {
	// Static files
	staticSub, err := fs.Sub(contentFS, "static")
	if err != nil {
		return err
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// --- Pages ---
	dashboardPage := &dashboard.Page{
		Tmpl:         s.tmpl,
		Queries:      s.queries,
		BuildSidebar: s.buildSidebarAny,
	}
	mux.HandleFunc("GET /{$}", dashboardPage.HandlePage)
	mux.HandleFunc("POST /hx/dashboard/create-repository", dashboardPage.HandleCreateRepository)

	repoPage := &repo.Page{
		Tmpl:         s.tmpl,
		Queries:      s.queries,
		BuildSidebar: s.buildSidebarAny,
	}
	mux.HandleFunc("GET /github.com/{org}/{repo}/prompt-requests", repoPage.HandlePage)

	convPage := &conversation.Page{
		Tmpl:                    s.tmpl,
		Queries:                 s.queries,
		BuildSidebar:            s.buildSidebarAny,
		GetRepoStatus:           s.getRepoStatusForConv,
		SetRepoStatus:           s.setRepoStatus,
		SetRepoStatusProcessing: s.setRepoStatusProcessing,
		ClearCancelFunc:         s.clearCancelFunc,
		RepoStatus:              &s.repoStatus,
		CancelFuncs:             &s.cancelFuncs,
		LockSession:             s.lockSession,
		LockRepo:                s.lockRepo,
	}
	mux.HandleFunc("GET /github.com/{org}/{repo}/prompt-requests/{id}", convPage.HandlePage)
	mux.HandleFunc("POST /hx/conversation/send-message", convPage.HandleSendMessage)
	mux.HandleFunc("GET /hx/conversation/status", convPage.HandleStatus)
	mux.HandleFunc("POST /hx/conversation/publish", convPage.HandlePublish)
	mux.HandleFunc("POST /hx/conversation/delete", convPage.HandleDelete)
	mux.HandleFunc("POST /hx/conversation/archive", convPage.HandleArchive)
	mux.HandleFunc("POST /hx/conversation/unarchive", convPage.HandleUnarchive)
	mux.HandleFunc("POST /hx/conversation/cancel", convPage.HandleCancel)
	mux.HandleFunc("POST /hx/conversation/retry", convPage.HandleRetry)
	mux.HandleFunc("POST /hx/conversation/resend", convPage.HandleResend)

	// Create prompt request (shared — used from repo + conversation pages)
	createPR := &hx.CreatePromptRequestHandler{
		Queries:           s.queries,
		SetRepoStatus:     s.setRepoStatus,
		AsyncEnsureCloned: convPage.AsyncEnsureCloned,
	}
	mux.HandleFunc("POST /github.com/{org}/{repo}/prompt-requests", createPR.Handle)

	// Shared HX fragments
	hxHandler := hx.New(s.tmpl, s.queries, s.getRepoStatusString)
	mux.HandleFunc("GET /hx/sidebar", hxHandler.HandleSidebar)

	return nil
}
```

**Step 2: Update server.go**

Replace `parsePages()` with `loadTemplates()`. Remove old fields and methods. The `Server` struct changes:

```go
type Server struct {
	queries     *db.Queries
	tmpl        *template.Template  // single template set (replaces pages map)
	httpSrv     *http.Server
	ln          net.Listener
	addr        string
	sessionMu   sync.Map
	repoStatus  sync.Map
	cancelFuncs sync.Map
	repoMu      sync.Map
}
```

`New()` becomes:
```go
func New(queries *db.Queries) (*Server, error) {
	tmpl, err := loadTemplates()
	if err != nil {
		return nil, err
	}

	s := &Server{
		queries: queries,
		tmpl:    tmpl,
	}

	mux := http.NewServeMux()
	if err := s.registerRoutes(mux); err != nil {
		return nil, err
	}
	s.httpSrv = &http.Server{Handler: mux}
	return s, nil
}
```

Remove:
- `parsePages()` function
- `renderPage()` and `renderFragment()` methods
- `FuncMap` var (now `funcMap` in `templates.go`)
- Old embed directives (`//go:embed templates`, `//go:embed static`) — replaced by `embed.go`
- `pages` field from Server struct
- `buildSidebarAny` stays (delegates to `hx.BuildSidebar`)
- `getRepoStatusString` stays
- State management methods stay (`setRepoStatus`, `getRepoStatus`, `lockSession`, etc.)

Add `getRepoStatusForConv` that returns the conversation package's `repoStatusEntry`:
```go
func (s *Server) getRepoStatusForConv(prID int64) conversation.RepoStatusEntry {
	e := s.getRepoStatus(prID)
	return conversation.RepoStatusEntry{Status: e.Status, Error: e.Error, StartedAt: e.StartedAt}
}
```

**Step 3: Build and verify**

Run: `go build ./...`

**Step 4: Commit**

```
git commit -am "refactor: create routes.go, update server.go to use loadTemplates"
```

---

### Task 10: Delete old templates directory

Remove `internal/server/templates/` — all templates have been moved to their new locations.

**Files:**
- Delete: `internal/server/templates/` (entire directory)

**Step 1: Verify no references remain**

```bash
grep -r "templates/" internal/server/ --include="*.go"
```

Should show nothing (or only `embed.go` which was already updated).

**Step 2: Delete and commit**

```bash
rm -rf internal/server/templates/
git add -A internal/server/templates/
git commit -m "refactor: delete old templates directory"
```

---

### Task 11: Final verification

**Step 1: Build**

```bash
go build ./...
```

**Step 2: Run the app and manually verify**

```bash
go run ./cmd/prompter serve
```

Test each page:
- Dashboard loads at `/`
- Create repository form submits and redirects
- Repo page loads at `/github.com/org/repo/prompt-requests`
- Conversation page loads
- Status polling works
- Send message works
- Cancel/retry/resend work
- Publish works
- Archive/unarchive work
- Sidebar renders on all pages (no polling, ready for SSE)

**Step 3: Commit any fixes**

```
git commit -am "refactor: final fixes after server structure refactoring"
```
