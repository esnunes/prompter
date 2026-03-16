package conversation

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/esnunes/prompter/internal/claude"
	"github.com/esnunes/prompter/internal/models"
)

type messageFragmentData struct {
	PromptRequestID int64
	Org             string
	Repo            string
	Messages        []models.Message
	Questions       []questionData
	PromptReady     bool
}

func (p *Page) HandleSendMessage(w http.ResponseWriter, r *http.Request) {
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

	// Derive org/repo from pr.RepoURL
	parts := strings.SplitN(pr.RepoURL, "/", 3)
	org, repoName := parts[1], parts[2]

	userMessage := strings.TrimSpace(r.FormValue("message"))
	// If no direct message, try assembling from multi-question form fields
	if userMessage == "" {
		userMessage = assembleQuestionAnswers(r)
	}
	if userMessage == "" {
		http.Error(w, "Message is required", http.StatusBadRequest)
		return
	}

	// Save user message
	userMsg, err := p.Queries.CreateMessage(id, "user", userMessage, nil)
	if err != nil {
		log.Printf("saving user message: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// If repo is not ready, just save and disable form — auto-send kicks in when ready
	statusEntry := p.GetRepoStatus(id)
	if statusEntry.Status != "" && statusEntry.Status != "ready" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fragment := messageFragmentData{
			PromptRequestID: id,
			Org:             org,
			Repo:            repoName,
			Messages:        []models.Message{*userMsg},
		}
		p.Tmpl.ExecuteTemplate(w, "pages/conversation/send_message.html", fragment)
		fmt.Fprint(w, `<script>(function(){var f=document.getElementById('message-form');if(f){f.querySelector('textarea').disabled=true;f.querySelector('button').disabled=true;}})();</script>`)
		return
	}

	// Repo is ready — launch async Claude call
	ctx, cancel := context.WithCancel(context.Background())
	p.SetRepoStatusProcessing(id, cancel)
	go p.backgroundSendMessage(ctx, id)

	// Return user message bubble + processing status div for polling
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fragment := messageFragmentData{
		PromptRequestID: id,
		Org:             org,
		Repo:            repoName,
		Messages:        []models.Message{*userMsg},
	}
	p.Tmpl.ExecuteTemplate(w, "pages/conversation/send_message.html", fragment)

	// Remove any stale #repo-status element (e.g. leftover "Repository ready!" div)
	// before appending the new processing div to avoid duplicate IDs.
	fmt.Fprint(w, `<script>(function(){var old=document.getElementById('repo-status');if(old)old.remove();})();</script>`)

	// Append processing status div that starts polling
	entry := p.GetRepoStatus(id)
	fmt.Fprintf(w, `<div id="repo-status" class="repo-status" hx-get="/hx/conversation/status?id=%d" hx-trigger="every 2s" hx-swap="outerHTML" data-started-at="%d">`, id, entry.StartedAt.Unix())
	fmt.Fprint(w, `<div class="processing-indicator"><div class="spinner"></div><span class="processing-text">Thinking...</span><span class="elapsed-timer"></span></div>`)
	fmt.Fprintf(w, `<form hx-post="/hx/conversation/cancel" hx-target="#repo-status" hx-swap="outerHTML" hx-disabled-elt="find button" style="display:inline;"><input type="hidden" name="id" value="%d"><button type="submit" class="btn btn-sm btn-secondary">Cancel</button></form>`, id)
	fmt.Fprint(w, `</div>`)

	// Disable the message form while processing (setTimeout to run after HTMX re-enables hx-disabled-elt)
	fmt.Fprint(w, `<script>setTimeout(function(){var f=document.getElementById('message-form');if(f){f.querySelector('textarea').disabled=true;f.querySelector('button').disabled=true;}if(typeof updateElapsedTimers==='function')updateElapsedTimers();},0);</script>`)
}

// backgroundSendMessage processes a pending user message with Claude in a background goroutine.
// It saves the response to DB and updates the repo status to "responded" or "cancelled".
func (p *Page) backgroundSendMessage(ctx context.Context, prID int64) {
	defer p.ClearCancelFunc(prID)

	pr, err := p.Queries.GetPromptRequest(prID)
	if err != nil {
		log.Printf("auto-send: getting prompt request: %v", err)
		p.SetRepoStatus(prID, "error", fmt.Sprintf("Failed to load prompt request: %v", err))
		return
	}

	lastMsg, err := p.Queries.GetLastMessage(prID)
	if err != nil || lastMsg.Role != "user" {
		log.Printf("auto-send: no pending user message for PR %d", prID)
		p.SetRepoStatus(prID, "ready", "")
		return
	}

	// Acquire session lock to prevent concurrent Claude calls
	mu := p.LockSession(pr.SessionID)
	defer mu.Unlock()

	// Re-check: ensure last message is still from user (not already processed)
	lastMsg, err = p.Queries.GetLastMessage(prID)
	if err != nil || lastMsg.Role != "user" {
		p.SetRepoStatus(prID, "ready", "")
		return
	}

	// Determine resume vs new
	existingMsgs, err := p.Queries.ListMessages(prID)
	if err != nil {
		log.Printf("auto-send: listing messages: %v", err)
		p.SetRepoStatus(prID, "error", fmt.Sprintf("Failed to list messages: %v", err))
		return
	}
	resume := false
	for _, m := range existingMsgs {
		if m.ID < lastMsg.ID && m.Role == "assistant" {
			resume = true
			break
		}
	}

	resp, rawJSON, err := claude.SendMessage(ctx, pr.SessionID, pr.RepoLocalPath, lastMsg.Content, resume)
	if err != nil {
		if ctx.Err() == context.Canceled {
			log.Printf("auto-send: cancelled for PR %d", prID)
			p.Queries.CreateMessage(prID, "assistant", "Request cancelled by user.", nil)
			p.SetRepoStatus(prID, "cancelled", "")
			return
		}
		log.Printf("auto-send: claude error: %v", err)
		errMsg := fmt.Sprintf("Sorry, I encountered an error: %v", err)
		p.Queries.CreateMessage(prID, "assistant", errMsg, nil)
		p.SetRepoStatus(prID, "responded", "")
		return
	}

	if _, err := p.Queries.CreateMessage(prID, "assistant", resp.Message, &rawJSON); err != nil {
		log.Printf("auto-send: saving assistant message: %v", err)
		p.SetRepoStatus(prID, "error", "Failed to save response")
		return
	}

	// Set title from response
	if pr.Title == "" {
		if resp.GeneratedTitle != "" {
			p.Queries.UpdatePromptRequestTitle(prID, resp.GeneratedTitle)
		} else if resp.Message != "" {
			title := resp.Message
			if len(title) > 60 {
				title = title[:60] + "..."
			}
			p.Queries.UpdatePromptRequestTitle(prID, title)
		}
	} else if resp.GeneratedTitle != "" {
		p.Queries.UpdatePromptRequestTitle(prID, resp.GeneratedTitle)
	}

	p.SetRepoStatus(prID, "responded", "")
}
