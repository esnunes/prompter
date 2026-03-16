package conversation

import (
	"context"
	"log"
	"net/http"
	"strconv"
)

func (p *Page) HandleCancel(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Call the cancel function if processing
	entry := p.GetRepoStatus(id)
	if entry.Status == "processing" {
		if v, ok := p.CancelFuncs.Load(id); ok {
			if cancel, ok := v.(context.CancelFunc); ok {
				cancel()
			}
		}
	}

	// Return current status — the background goroutine will transition to "cancelled"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.Tmpl.ExecuteTemplate(w, "pages/conversation/status.html", statusFragmentData{
		PromptRequestID: id,
		Status:          "processing",
		StartedAt:       entry.StartedAt.Unix(),
	}); err != nil {
		log.Printf("render error (status): %v", err)
	}
}
