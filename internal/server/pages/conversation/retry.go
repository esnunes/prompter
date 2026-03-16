package conversation

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/esnunes/prompter/internal/repo"
)

func (p *Page) HandleRetry(w http.ResponseWriter, r *http.Request) {
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

	// Derive repoURL from pr
	parts := strings.SplitN(pr.RepoURL, "/", 3)
	repoURL := "github.com/" + parts[1] + "/" + parts[2]

	cloned, _ := repo.IsCloned(repoURL)
	if cloned {
		p.SetRepoStatus(id, "pulling", "")
	} else {
		p.SetRepoStatus(id, "cloning", "")
	}

	go p.AsyncEnsureCloned(id, repoURL)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.Tmpl.ExecuteTemplate(w, "pages/conversation/status.html", statusFragmentData{
		PromptRequestID: id,
		Status:          p.GetRepoStatus(id).Status,
	}); err != nil {
		log.Printf("render error (status): %v", err)
	}
}
