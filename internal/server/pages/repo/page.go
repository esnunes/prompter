package repo

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/esnunes/prompter/internal/db"
	"github.com/esnunes/prompter/internal/models"
)

type Page struct {
	Tmpl    *template.Template
	Queries *db.Queries
}

type pageData struct {
	RepoURL        string
	Org            string
	Repo           string
	Error          string
	PromptRequests []models.PromptRequest
	ShowArchived   bool
}

func (p *Page) HandlePage(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")
	repoURL := fmt.Sprintf("github.com/%s/%s", org, repoName)

	if _, err := p.Queries.GetRepositoryByURL(repoURL); err != nil {
		p.renderPage(w, pageData{
			RepoURL: repoURL,
			Org:     org,
			Repo:    repoName,
			Error:   "Repository not found. Add it from the dashboard first.",
		})
		return
	}

	showArchived := r.URL.Query().Get("archived") == "1"
	prs, err := p.Queries.ListPromptRequestsByRepoURL(repoURL, showArchived)
	if err != nil {
		log.Printf("listing prompt requests for repo: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	p.renderPage(w, pageData{
		RepoURL:        repoURL,
		Org:            org,
		Repo:           repoName,
		PromptRequests: prs,
		ShowArchived:   showArchived,
	})
}

func (p *Page) renderPage(w http.ResponseWriter, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.Tmpl.ExecuteTemplate(w, "pages/base.html", data); err != nil {
		log.Printf("render error (repo): %v", err)
	}
}
