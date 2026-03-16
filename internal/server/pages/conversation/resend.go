package conversation

import (
	"context"
	"log"
	"net/http"
	"strconv"
)

func (p *Page) HandleResend(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Delete the synthetic cancelled assistant message
	lastMsg, err := p.Queries.GetLastMessage(id)
	if err == nil && lastMsg.Role == "assistant" && lastMsg.Content == "Request cancelled by user." {
		p.Queries.DeleteMessage(lastMsg.ID)
	}

	// Launch async Claude call
	ctx, cancel := context.WithCancel(context.Background())
	p.SetRepoStatusProcessing(id, cancel)
	go p.backgroundSendMessage(ctx, id)

	// Return processing status fragment
	entry := p.GetRepoStatus(id)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.Tmpl.ExecuteTemplate(w, "pages/conversation/status.html", statusFragmentData{
		PromptRequestID: id,
		Status:          "processing",
		StartedAt:       entry.StartedAt.Unix(),
	}); err != nil {
		log.Printf("render error (status): %v", err)
	}
}
