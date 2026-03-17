package dashboard

import (
	"log"
	"net/http"
	"strings"

	"github.com/esnunes/prompter/internal/github"
	"github.com/esnunes/prompter/internal/repo"
)

type CreateRepositoryData struct {
	RepoURL string
	Error   string
}

func (p *Page) renderCreateRepository(w http.ResponseWriter, data CreateRepositoryData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.Tmpl.ExecuteTemplate(w, "pages/dashboard/create_repository.html", data); err != nil {
		log.Printf("render error (create_repository): %v", err)
	}
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

func (p *Page) HandleCreateRepository(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		p.renderCreateRepository(w, CreateRepositoryData{Error: "Invalid form submission."})
		return
	}

	repoURL := sanitizeRepoURL(r.FormValue("repo_url"))

	if err := repo.ValidateURL(repoURL); err != nil {
		p.renderCreateRepository(w, CreateRepositoryData{
			RepoURL: r.FormValue("repo_url"),
			Error:   "Invalid repository URL. Expected format: github.com/owner/repo",
		})
		return
	}

	// Extract org/repo for GitHub verification
	parts := strings.SplitN(repoURL, "/", 3)
	if err := github.VerifyRepo(r.Context(), parts[1], parts[2]); err != nil {
		p.renderCreateRepository(w, CreateRepositoryData{
			RepoURL: repoURL,
			Error:   "This repository doesn't exist on GitHub or is not accessible.",
		})
		return
	}

	localPath, err := repo.LocalPath(repoURL)
	if err != nil {
		p.renderCreateRepository(w, CreateRepositoryData{
			RepoURL: repoURL,
			Error:   "Failed to compute local path.",
		})
		return
	}

	if _, err := p.Queries.UpsertRepository(repoURL, localPath); err != nil {
		p.renderCreateRepository(w, CreateRepositoryData{
			RepoURL: repoURL,
			Error:   "Failed to create repository.",
		})
		return
	}

	locationURL := "/" + repoURL + "/prompt-requests"
	w.Header().Set("HX-Location", locationURL)
	w.WriteHeader(http.StatusOK)
}
