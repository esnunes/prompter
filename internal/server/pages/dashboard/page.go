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
