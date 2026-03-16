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
	locationURL := "/" + parts[0] + "/" + parts[1] + "/" + parts[2] + "/prompt-requests"
	w.Header().Set("HX-Location", locationURL)
	w.WriteHeader(http.StatusOK)
}
