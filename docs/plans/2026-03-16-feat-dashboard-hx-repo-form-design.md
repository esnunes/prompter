# Dashboard HX Repository Form

Extract the dashboard page into `internal/server/pages/` and the repository form into `internal/server/hx/`. The form validates, sanitizes, and upserts repositories via HTMX, re-rendering the fragment on error or redirecting on success.

## File Structure

```
internal/server/
  hx/
    handler.go
    repo_form.html
  pages/
    handler.go
    dashboard.html
```

`.html` and `.go` files live in the same directory — no `templates/` subfolder.

## hx/repo_form.html

The form fragment extracted from `dashboard.html`. Renders a `<form>` with an optional `.Error` string and a pre-filled `.RepoURL` value. The form posts to `/hx/repositories` with `hx-swap="outerHTML"` so the fragment replaces itself on validation error.

## hx/handler.go

- Embeds `repo_form.html` via `go:embed`.
- Exposes `var TemplateFS embed.FS` so pages can compose the fragment into their template set.
- `Handler` struct holds `*db.Queries` and the parsed fragment template.
- `New(queries) → *Handler`.
- `Register(mux)` wires `POST /hx/repositories`.

POST handler logic:
1. Parse `repo_url` from form.
2. Sanitize: trim whitespace, strip `https://`, `http://`, trailing `/` and `.git`.
3. Validate with `repo.ValidateURL()` — re-render fragment with error if invalid.
4. Compute local path with `repo.LocalPath()`.
5. `queries.UpsertRepository(repoURL, localPath)` — re-render fragment with error on failure.
6. On success: set `HX-Redirect` header to `/github.com/{org}/{repo}/prompt-requests`.

## pages/dashboard.html

Uses `{{template "repo_form.html" .Form}}` to include the fragment. The rest of the dashboard (repository list, empty state) stays the same.

## pages/handler.go

- Embeds `dashboard.html` via `go:embed`.
- Imports `hx.TemplateFS` to include the fragment template at parse time.
- Also needs the shared layout and sidebar templates from the parent `server` package.
- `Handler` struct holds `*db.Queries` and the parsed page template.
- `New(queries) → *Handler`.
- `Register(mux)` wires `GET /{$}`.
- Dashboard handler: queries repos, builds sidebar, renders page.

## Template Data

```go
// hx package
type RepoFormData struct {
    RepoURL string // pre-filled value on error
    Error   string // validation error message
}

// pages package
type DashboardData struct {
    basePageData
    Form         hx.RepoFormData
    Repositories []models.RepositorySummary
}
```

## server.go Changes

- Remove `handleDashboard` and `dashboardData` from `handlers.go`.
- Remove `"dashboard.html"` from `parsePages()` template list.
- Create `hx.New(queries)` and `pages.New(queries)` in `New()`.
- Call `hxHandler.Register(mux)` and `pagesHandler.Register(mux)`.

Pages need access to the shared layout, sidebar, and funcMap from the server package. The server exposes these via exported vars or a helper that pages call during template parsing.

## What Stays Unchanged

- All other handlers remain in `server/handlers.go`.
- Layout, sidebar, and shared template parsing stay in `server/server.go`.
- `repo.ValidateURL()` is reused as-is.
