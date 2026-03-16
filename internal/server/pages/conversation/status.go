package conversation

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/esnunes/prompter/internal/models"
	"github.com/esnunes/prompter/internal/repo"
)

type statusFragmentData struct {
	PromptRequestID int64
	Status          string
	Error           string
	StartedAt       int64 // Unix timestamp, 0 if not processing
}

func (p *Page) HandleStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	pr, err := p.Queries.GetPromptRequest(id)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Derive org/repo from pr.RepoURL
	parts := strings.SplitN(pr.RepoURL, "/", 3)
	org, repoName := parts[1], parts[2]

	entry := p.GetRepoStatus(id)

	// Server restart recovery: if no status tracked, check filesystem
	if entry.Status == "" {
		cloned, _ := repo.IsCloned(pr.RepoURL)
		if cloned {
			p.SetRepoStatus(id, "ready", "")
			entry = RepoStatusEntry{Status: "ready"}
		} else {
			// Auto-start clone
			p.SetRepoStatus(id, "cloning", "")
			go p.AsyncEnsureCloned(id, pr.RepoURL)
			entry = RepoStatusEntry{Status: "cloning"}
		}
	}

	// If ready, check for a pending user message to auto-send
	if entry.Status == "ready" {
		lastMsg, err := p.Queries.GetLastMessage(id)
		if err == nil && lastMsg.Role == "user" {
			// Atomically transition to "processing" to prevent duplicate Claude calls
			old := RepoStatusEntry{Status: "ready"}
			if p.RepoStatus.CompareAndSwap(id, old, RepoStatusEntry{Status: "processing"}) {
				ctx, cancel := context.WithCancel(context.Background())
				p.SetRepoStatusProcessing(id, cancel)
				go p.backgroundSendMessage(ctx, id)
			}
			entry = p.GetRepoStatus(id)
		}
	}

	// If responded, deliver the assistant message and stop polling.
	// We replace #repo-status with the response content plus a script that
	// moves the messages into #conversation at the correct position.
	if entry.Status == "responded" {
		p.RepoStatus.Delete(id)
		lastMsg, err := p.Queries.GetLastMessage(id)
		if err == nil && lastMsg.Role == "assistant" {
			fragment := messageFragmentData{
				PromptRequestID: id,
				Org:             org,
				Repo:            repoName,
				Messages:        []models.Message{*lastMsg},
			}
			if lastMsg.RawResponse != nil {
				questions, promptReady := extractQuestionsFromRaw(*lastMsg.RawResponse)
				fragment.Questions = questions
				fragment.PromptReady = promptReady
			}

			// Render the message fragment into a wrapper div that replaces #repo-status
			// and auto-relocates its children to the end of #conversation via inline script.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<div id="repo-status" style="display:none">`)
			p.Tmpl.ExecuteTemplate(w, "pages/conversation/send_message.html", fragment)
			fmt.Fprint(w, `</div><script>`)
			fmt.Fprint(w, `(function(){var s=document.getElementById('repo-status');var c=document.getElementById('conversation');while(s.firstChild){c.appendChild(s.firstChild);}s.remove();htmx.process(c);if(typeof renderMarkdown==='function')renderMarkdown();if(typeof scrollConversation==='function')scrollConversation();else{c.scrollTop=c.scrollHeight;}var f=document.getElementById('message-form');if(f){f.querySelector('textarea').disabled=false;f.querySelector('button').disabled=false;}})();`)
			fmt.Fprint(w, `</script>`)
			return
		}
	}

	var startedAt int64
	if !entry.StartedAt.IsZero() {
		startedAt = entry.StartedAt.Unix()
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.Tmpl.ExecuteTemplate(w, "pages/conversation/status.html", statusFragmentData{
		PromptRequestID: id,
		Status:          entry.Status,
		Error:           entry.Error,
		StartedAt:       startedAt,
	}); err != nil {
		log.Printf("render error (status): %v", err)
	}
}

// AsyncEnsureCloned runs clone/pull in the background, updating status in sync.Map.
func (p *Page) AsyncEnsureCloned(prID int64, repoURL string) {
	// Serialize clone/pull operations per repo to prevent concurrent git corruption
	mu := p.LockRepo(repoURL)
	defer mu.Unlock()

	_, err := repo.EnsureCloned(context.Background(), repoURL)
	if err != nil {
		log.Printf("async clone/pull failed for %s: %v", repoURL, err)
		p.SetRepoStatus(prID, "error", err.Error())
		return
	}
	p.SetRepoStatus(prID, "ready", "")
}
