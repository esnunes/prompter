# Dashboard HX Repository Form Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extract dashboard into `pages/` and repository form into `hx/`, with POST validation, sanitization, upsert, and HTMX fragment re-rendering.

**Architecture:** `hx/` package owns the form fragment template + POST handler. `pages/` package owns the dashboard page template and composes the fragment via `{{template}}`. Both are self-contained packages with `go:embed`, wired into the existing server mux.

**Tech Stack:** Go 1.25, html/template, net/http, embed, HTMX

---

### Task 1: Create `hx/` package — fragment template + handler

**Files:**
- Create: `internal/server/hx/repo_form.html`
- Create: `internal/server/hx/handler.go`

**Step 1: Create the fragment template**

Create `internal/server/hx/repo_form.html`:

```html
<form hx-post="/hx/repositories" hx-swap="outerHTML">
  <label for="repo_url">Go to repository</label>
  {{if .Error}}
  <div class="form-error">{{.Error}}</div>
  {{end}}
  <div style="display:flex;gap:var(--space-3);margin-top:var(--space-2);">
    <input type="text" name="repo_url" id="repo_url" placeholder="github.com/owner/repo" value="{{.RepoURL}}" style="flex:1;">
    <button type="submit" class="btn btn-primary">Go</button>
  </div>
</form>
```

**Step 2: Create the handler**

Create `internal/server/hx/handler.go`:

```go
package hx

import (
	"embed"
	"html/template"
	"net/http"
	"strings"

	"github.com/esnunes/prompter/internal/db"
	"github.com/esnunes/prompter/internal/repo"
)

//go:embed repo_form.html
var TemplateFS embed.FS

type RepoFormData struct {
	RepoURL string
	Error   string
}

type Handler struct {
	queries  *db.Queries
	formTmpl *template.Template
}

func New(queries *db.Queries) (*Handler, error) {
	tmpl, err := template.ParseFS(TemplateFS, "repo_form.html")
	if err != nil {
		return nil, err
	}
	return &Handler{queries: queries, formTmpl: tmpl}, nil
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /hx/repositories", h.handleCreateRepository)
}

func (h *Handler) renderForm(w http.ResponseWriter, data RepoFormData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.formTmpl.ExecuteTemplate(w, "repo_form.html", data)
}

// sanitizeRepoURL normalizes user input into the github.com/owner/repo format.
func sanitizeRepoURL(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	return s
}

func (h *Handler) handleCreateRepository(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderForm(w, RepoFormData{Error: "Invalid form submission."})
		return
	}

	repoURL := sanitizeRepoURL(r.FormValue("repo_url"))

	if err := repo.ValidateURL(repoURL); err != nil {
		h.renderForm(w, RepoFormData{
			RepoURL: r.FormValue("repo_url"),
			Error:   "Invalid repository URL. Expected format: github.com/owner/repo",
		})
		return
	}

	localPath, err := repo.LocalPath(repoURL)
	if err != nil {
		h.renderForm(w, RepoFormData{
			RepoURL: repoURL,
			Error:   "Failed to compute local path.",
		})
		return
	}

	if _, err := h.queries.UpsertRepository(repoURL, localPath); err != nil {
		h.renderForm(w, RepoFormData{
			RepoURL: repoURL,
			Error:   "Failed to create repository.",
		})
		return
	}

	// Extract org/repo from validated URL (github.com/org/repo)
	parts := strings.SplitN(repoURL, "/", 3)
	redirectURL := "/" + parts[0] + "/" + parts[1] + "/" + parts[2] + "/prompt-requests"
	w.Header().Set("HX-Redirect", redirectURL)
	w.WriteHeader(http.StatusOK)
}
```

**Step 3: Verify it compiles**

Run: `go build ./internal/server/hx/`
Expected: success, no errors.

**Step 4: Commit**

```bash
git add internal/server/hx/
git commit -m "feat: add hx package with repository form fragment and POST handler"
```

---

### Task 2: Create `pages/` package — dashboard page

**Files:**
- Create: `internal/server/pages/dashboard.html`
- Create: `internal/server/pages/handler.go`

**Step 1: Create the dashboard template**

Create `internal/server/pages/dashboard.html`. This composites the hx fragment via `{{template "repo_form.html" .Form}}`:

```html
{{define "title"}}Prompter — Dashboard{{end}}

{{define "content"}}
<div class="dashboard-header">
  <h2>Dashboard</h2>
</div>

<div class="card mb-4">
  {{template "repo_form.html" .Form}}
</div>

{{if .Repositories}}
<h3 class="mb-4">Your repositories</h3>
{{range .Repositories}}
<a href="/{{.URL}}/prompt-requests" class="card card-link">
  <div class="pr-title">{{.URL}}</div>
  <div class="pr-meta">
    <span>{{.ActivePRCount}} prompt requests</span>
    <span>Last activity: {{.LastActivity.Format "Jan 2, 2006"}}</span>
  </div>
</a>
{{end}}
{{else}}
<div class="empty-state">
  <h2>No repositories yet</h2>
  <p>Enter a repository URL above to get started.</p>
</div>
{{end}}
{{end}}
```

**Step 2: Create the handler**

Create `internal/server/pages/handler.go`. This needs the shared layout, sidebar, and funcMap from the server package. The server package must export these for pages to compose templates.

```go
package pages

import (
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"

	"github.com/esnunes/prompter/internal/db"
	"github.com/esnunes/prompter/internal/models"
	"github.com/esnunes/prompter/internal/server/hx"
)

//go:embed dashboard.html
var templateFS embed.FS

type Handler struct {
	queries       *db.Queries
	dashboardTmpl *template.Template
	buildSidebar  func(prs []models.PromptRequest, scope string, currentID int64) any
}

type DashboardData struct {
	Sidebar      any
	Form         hx.RepoFormData
	Repositories []models.RepositorySummary
}

func New(
	queries *db.Queries,
	layoutFS fs.FS,
	funcMap template.FuncMap,
	buildSidebar func(prs []models.PromptRequest, scope string, currentID int64) any,
) (*Handler, error) {
	// Parse layout + sidebar from shared FS
	layoutBytes, err := fs.ReadFile(layoutFS, "layout.html")
	if err != nil {
		return nil, err
	}
	sidebarBytes, err := fs.ReadFile(layoutFS, "sidebar.html")
	if err != nil {
		return nil, err
	}

	// Parse dashboard page template
	dashboardBytes, err := fs.ReadFile(templateFS, "dashboard.html")
	if err != nil {
		return nil, err
	}

	// Parse hx fragment template
	repoFormBytes, err := fs.ReadFile(hx.TemplateFS, "repo_form.html")
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New("layout.html").Funcs(funcMap).Parse(string(layoutBytes))
	if err != nil {
		return nil, err
	}
	if _, err := tmpl.New("sidebar.html").Parse(string(sidebarBytes)); err != nil {
		return nil, err
	}
	if _, err := tmpl.New("repo_form.html").Parse(string(repoFormBytes)); err != nil {
		return nil, err
	}
	if _, err := tmpl.New("dashboard.html").Parse(string(dashboardBytes)); err != nil {
		return nil, err
	}

	return &Handler{
		queries:       queries,
		dashboardTmpl: tmpl,
		buildSidebar:  buildSidebar,
	}, nil
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.handleDashboard)
}

func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	repos, err := h.queries.ListRepositorySummaries()
	if err != nil {
		log.Printf("listing repository summaries: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	sidebarPRs, _ := h.queries.ListPromptRequests(false)
	sidebar := h.buildSidebar(sidebarPRs, "all", 0)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.dashboardTmpl.ExecuteTemplate(w, "layout.html", DashboardData{
		Sidebar:      sidebar,
		Form:         hx.RepoFormData{},
		Repositories: repos,
	}); err != nil {
		log.Printf("render error (dashboard): %v", err)
	}
}
```

**Step 3: Verify it compiles**

Run: `go build ./internal/server/pages/`
Expected: This will fail because server doesn't export `TemplatesFS` and `FuncMap` yet. That's Task 3.

**Step 4: Commit (template only for now)**

```bash
git add internal/server/pages/dashboard.html internal/server/pages/handler.go
git commit -m "feat: add pages package with dashboard page handler"
```

---

### Task 3: Export shared resources from server package

**Files:**
- Modify: `internal/server/server.go`

**Step 1: Export the templates FS, funcMap, and buildSidebar**

In `internal/server/server.go`, make these accessible to sub-packages:

1. Export `TemplatesFS` (rename `templatesFS` → `TemplatesFS`) or add a `TemplatesSubFS()` helper.
2. Export `FuncMap` (rename `funcMap` → `FuncMap`).
3. Add a `BuildSidebar` method on `Server` that wraps the existing `buildSidebar` and returns `any` (so pages don't need to know about `sidebarData`).

Since `pages` can't import `server` (circular), the cleanest approach is to pass these as arguments when constructing `pages.New()` — which the design already shows. The server reads its own `templatesFS`, creates a sub-FS, and passes it to `pages.New()`.

Changes to `server.go`:
- Export `FuncMap` (capitalize).
- Add a helper `TemplatesSubFS() fs.FS` that returns `fs.Sub(templatesFS, "templates")`.
- In `New()`: create `hx.New(queries)` and `pages.New(queries, tmplSubFS, FuncMap, s.buildSidebarAny)`.
- Add `buildSidebarAny` method that wraps `buildSidebar` returning `any`.
- Remove `"dashboard.html"` from `parsePages()` list.
- Remove `handleDashboard` from `handlers.go`.
- Remove `dashboardData` type from `handlers.go`.
- Replace the `GET /{$}` route with `pagesHandler.Register(mux)`.

**Step 2: Verify it compiles**

Run: `go build ./...`
Expected: success.

**Step 3: Run the server manually and test**

Run: `go run ./cmd/prompter/ serve`
1. Visit `http://localhost:PORT/` — dashboard renders with the form.
2. Submit empty form — re-renders with error.
3. Submit `https://github.com/esnunes/prompter` — redirects to `/github.com/esnunes/prompter/prompt-requests`.
4. Submit `invalid-url` — re-renders with error.

**Step 4: Commit**

```bash
git add internal/server/server.go internal/server/handlers.go
git commit -m "feat: wire hx and pages packages into server, remove old dashboard handler"
```

---

### Task 4: Delete old dashboard template

**Files:**
- Delete: `internal/server/templates/dashboard.html`

**Step 1: Delete the file**

```bash
rm internal/server/templates/dashboard.html
```

**Step 2: Verify it compiles and runs**

Run: `go build ./...`
Expected: success (no code references this file anymore).

**Step 3: Commit**

```bash
git add internal/server/templates/dashboard.html
git commit -m "chore: remove old dashboard template"
```

---

### Task 5: Add form-error CSS class

**Files:**
- Modify: `internal/server/static/style.css`

**Step 1: Check if `.form-error` already exists**

Search `style.css` for `form-error`. If it doesn't exist, add it:

```css
.form-error {
  color: var(--color-red);
  font-size: var(--font-size-sm);
  margin-top: var(--space-1);
  margin-bottom: var(--space-1);
}
```

**Step 2: Verify visually**

Run the server, submit an invalid URL, confirm the error message is styled.

**Step 3: Commit**

```bash
git add internal/server/static/style.css
git commit -m "feat: add form-error CSS class for validation messages"
```
