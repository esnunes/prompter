package conversation

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/esnunes/prompter/internal/models"
)

type archiveBannerData struct {
	PromptRequest *models.PromptRequest
}

func (p *Page) HandleArchive(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	pr, err := p.Queries.GetPromptRequest(id)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	if err := p.Queries.ArchivePromptRequest(id); err != nil {
		log.Printf("archiving prompt request: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// If HTMX request (from conversation page), return the archived banner fragment
	if r.Header.Get("HX-Request") == "true" {
		pr, _ = p.Queries.GetPromptRequest(id)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := p.Tmpl.ExecuteTemplate(w, "pages/conversation/archive_banner.html", archiveBannerData{
			PromptRequest: pr,
		}); err != nil {
			log.Printf("render error (archive_banner): %v", err)
		}
		return
	}

	// Otherwise (from list page), redirect back
	referer := r.Header.Get("Referer")
	if referer == "" {
		parts := strings.SplitN(pr.RepoURL, "/", 3)
		referer = fmt.Sprintf("/github.com/%s/%s/prompt-requests", parts[1], parts[2])
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}

func (p *Page) HandleUnarchive(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	pr, err := p.Queries.GetPromptRequest(id)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	if err := p.Queries.UnarchivePromptRequest(id); err != nil {
		log.Printf("unarchiving prompt request: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// If HTMX request (from conversation page), return empty banner (removes it)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<div id="archive-banner"></div>`)
		return
	}

	// Otherwise (from list page), redirect back
	referer := r.Header.Get("Referer")
	if referer == "" {
		parts := strings.SplitN(pr.RepoURL, "/", 3)
		referer = fmt.Sprintf("/github.com/%s/%s/prompt-requests", parts[1], parts[2])
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}
