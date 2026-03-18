package dashboard

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/esnunes/prompter/internal/db"
	"github.com/esnunes/prompter/internal/models"

	_ "modernc.org/sqlite"
)

// testPage creates a Page with an in-memory SQLite DB and minimal templates.
func testPage(t *testing.T) *Page {
	t.Helper()

	sqlDB, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("opening in-memory db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	queries := db.NewQueries(sqlDB)

	tmpl := template.New("")
	tmpl.Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
	})
	// Minimal base layout
	template.Must(tmpl.New("pages/base.html").Parse(
		`{{block "title" .}}{{end}}{{block "sidebar" .}}{{end}}{{block "content" .}}{{end}}`))
	// Sidebar stub
	template.Must(tmpl.New("hx/sidebar.html").Parse(`<aside>sidebar</aside>`))
	// Dashboard page template
	template.Must(tmpl.New("pages/dashboard/page.html").Parse(
		`{{define "title"}}Dashboard{{end}}` +
			`{{define "sidebar"}}{{template "hx/sidebar.html" .Sidebar}}{{end}}` +
			`{{define "content"}}` +
			`{{template "pages/dashboard/create_repository.html" .Form}}` +
			`{{if .Repositories}}{{range .Repositories}}<a href="/{{.URL}}">{{.URL}}</a>{{end}}` +
			`{{else}}<div class="empty-state">No repositories yet</div>{{end}}` +
			`{{end}}`))
	// Create repository form template
	template.Must(tmpl.New("pages/dashboard/create_repository.html").Parse(
		`<form>{{if .Error}}<div class="form-error">{{.Error}}</div>{{end}}` +
			`<input name="repo_url" value="{{.RepoURL}}"></form>`))

	return &Page{
		Tmpl:    tmpl,
		Queries: queries,
		BuildSidebar: func(prs []models.PromptRequest, scope string, currentID int64) any {
			return nil
		},
		VerifyRepo: func(ctx context.Context, org, repoName string) error {
			return nil // default: always pass
		},
	}
}

// --- sanitizeRepoURL ---

func TestSanitizeRepoURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain url", "github.com/owner/repo", "github.com/owner/repo"},
		{"https prefix", "https://github.com/owner/repo", "github.com/owner/repo"},
		{"http prefix", "http://github.com/owner/repo", "github.com/owner/repo"},
		{"trailing slash", "github.com/owner/repo/", "github.com/owner/repo"},
		{"trailing .git", "github.com/owner/repo.git", "github.com/owner/repo"},
		{"https + .git", "https://github.com/owner/repo.git", "github.com/owner/repo"},
		{"whitespace", "  github.com/owner/repo  ", "github.com/owner/repo"},
		{"https + trailing slash + .git", "https://github.com/owner/repo.git/", "github.com/owner/repo"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeRepoURL(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeRepoURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- HandlePage ---

func TestHandlePage_EmptyDB(t *testing.T) {
	p := testPage(t)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	p.HandlePage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "No repositories yet") {
		t.Error("expected empty state message in body")
	}
}

func TestHandlePage_WithRepositories(t *testing.T) {
	p := testPage(t)

	// Seed a repository and prompt request so ListRepositorySummaries returns data
	repo, err := p.Queries.UpsertRepository("github.com/test/repo", "/tmp/test/repo")
	if err != nil {
		t.Fatalf("creating repo: %v", err)
	}
	if _, err := p.Queries.CreatePromptRequest(repo.ID, "sess-1"); err != nil {
		t.Fatalf("creating PR: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	p.HandlePage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "github.com/test/repo") {
		t.Error("expected repository URL in body")
	}
	if strings.Contains(body, "No repositories yet") {
		t.Error("should not show empty state when repositories exist")
	}
}

func TestHandlePage_RepoWithNoPromptRequests(t *testing.T) {
	p := testPage(t)

	// Seed a repository with no prompt requests
	if _, err := p.Queries.UpsertRepository("github.com/test/empty", "/tmp/test/empty"); err != nil {
		t.Fatalf("creating repo: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	p.HandlePage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "github.com/test/empty") {
		t.Error("expected repo with 0 prompt requests to be visible on dashboard")
	}
	if strings.Contains(body, "No repositories yet") {
		t.Error("should not show empty state when a repository exists")
	}
}

// --- HandleCreateRepository ---

func TestHandleCreateRepository_Success(t *testing.T) {
	p := testPage(t)

	form := url.Values{"repo_url": {"github.com/owner/repo"}}
	req := httptest.NewRequest("POST", "/hx/dashboard/create-repository",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	p.HandleCreateRepository(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	loc := rec.Header().Get("HX-Location")
	if loc != "/github.com/owner/repo/prompt-requests" {
		t.Errorf("HX-Location = %q, want %q", loc, "/github.com/owner/repo/prompt-requests")
	}
}

func TestHandleCreateRepository_SanitizesInput(t *testing.T) {
	p := testPage(t)

	form := url.Values{"repo_url": {"https://github.com/owner/repo.git"}}
	req := httptest.NewRequest("POST", "/hx/dashboard/create-repository",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	p.HandleCreateRepository(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	loc := rec.Header().Get("HX-Location")
	if loc != "/github.com/owner/repo/prompt-requests" {
		t.Errorf("HX-Location = %q, want %q", loc, "/github.com/owner/repo/prompt-requests")
	}
}

func TestHandleCreateRepository_InvalidURL(t *testing.T) {
	p := testPage(t)

	form := url.Values{"repo_url": {"not-a-valid-url"}}
	req := httptest.NewRequest("POST", "/hx/dashboard/create-repository",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	p.HandleCreateRepository(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (form re-rendered)", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "form-error") {
		t.Error("expected form-error in body for invalid URL")
	}
	if !strings.Contains(body, "Invalid repository URL") {
		t.Error("expected validation error message")
	}
	if rec.Header().Get("HX-Location") != "" {
		t.Error("should not redirect on validation error")
	}
}

func TestHandleCreateRepository_GitHubVerificationFails(t *testing.T) {
	p := testPage(t)
	p.VerifyRepo = func(ctx context.Context, org, repoName string) error {
		return context.DeadlineExceeded // simulate failure
	}

	form := url.Values{"repo_url": {"github.com/owner/nonexistent"}}
	req := httptest.NewRequest("POST", "/hx/dashboard/create-repository",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	p.HandleCreateRepository(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (form re-rendered)", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "form-error") {
		t.Error("expected form-error in body for GitHub verification failure")
	}
	if !strings.Contains(body, "exist on GitHub") {
		t.Errorf("expected GitHub verification error message, got: %s", body)
	}
}

func TestHandleCreateRepository_GitHubVerificationPreservesOriginalInput(t *testing.T) {
	p := testPage(t)
	p.VerifyRepo = func(ctx context.Context, org, repoName string) error {
		return context.DeadlineExceeded
	}

	rawInput := "https://github.com/owner/private.git"
	form := url.Values{"repo_url": {rawInput}}
	req := httptest.NewRequest("POST", "/hx/dashboard/create-repository",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	p.HandleCreateRepository(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, rawInput) {
		t.Errorf("expected original input %q preserved in form, got: %s", rawInput, body)
	}
}

func TestHandleCreateRepository_EmptyInput(t *testing.T) {
	p := testPage(t)

	form := url.Values{"repo_url": {""}}
	req := httptest.NewRequest("POST", "/hx/dashboard/create-repository",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	p.HandleCreateRepository(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (form re-rendered)", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "form-error") {
		t.Error("expected form-error for empty input")
	}
}

func TestHandleCreateRepository_IdempotentUpsert(t *testing.T) {
	p := testPage(t)

	form := url.Values{"repo_url": {"github.com/owner/repo"}}
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/hx/dashboard/create-repository",
			strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		p.HandleCreateRepository(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, want %d", i+1, rec.Code, http.StatusOK)
		}
		if loc := rec.Header().Get("HX-Location"); loc == "" {
			t.Fatalf("attempt %d: expected HX-Location header", i+1)
		}
	}
}
