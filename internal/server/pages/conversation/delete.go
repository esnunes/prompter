package conversation

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

func (p *Page) HandleDelete(w http.ResponseWriter, r *http.Request) {
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

	if err := p.Queries.DeletePromptRequest(id); err != nil {
		log.Printf("deleting prompt request: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Redirect to repo's prompt requests list
	parts := strings.SplitN(pr.RepoURL, "/", 3)
	http.Redirect(w, r, fmt.Sprintf("/github.com/%s/%s/prompt-requests", parts[1], parts[2]), http.StatusSeeOther)
}
