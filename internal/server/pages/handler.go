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
