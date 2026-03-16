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
	org := r.PathValue("org")
	repoName := r.PathValue("repo")
	repoURL := fmt.Sprintf("github.com/%s/%s", org, repoName)

	if err := repoLib.ValidateURL(repoURL); err != nil {
		p.renderPage(w, pageData{
			Sidebar: p.BuildSidebar(nil, "repo", 0),
			RepoURL: repoURL,
			Org:     org,
			Repo:    repoName,
			Error:   "Invalid repository URL format.",
		})
		return
	}

	if err := github.VerifyRepo(r.Context(), org, repoName); err != nil {
		p.renderPage(w, pageData{
			Sidebar: p.BuildSidebar(nil, "repo", 0),
			RepoURL: repoURL,
			Org:     org,
			Repo:    repoName,
			Error:   "This repository doesn't exist on GitHub or is not accessible.",
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

	sidebarPRs := prs
	if showArchived {
		sidebarPRs, _ = p.Queries.ListPromptRequestsByRepoURL(repoURL, false)
	}
	sidebar := p.BuildSidebar(sidebarPRs, "repo", 0)
	p.renderPage(w, pageData{
		Sidebar:        sidebar,
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
