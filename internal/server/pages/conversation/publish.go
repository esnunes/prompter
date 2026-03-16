package conversation

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/esnunes/prompter/internal/github"
)

func (p *Page) HandlePublish(w http.ResponseWriter, r *http.Request) {
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

	// Get the generated content (motivation + prompt)
	gc, err := p.Queries.GetLatestGeneratedContent(id)
	if err != nil {
		log.Printf("getting generated content: %v", err)
		http.Error(w, "No generated prompt found. Continue the conversation until the AI generates a prompt.", http.StatusBadRequest)
		return
	}

	// Compose issue body: motivation, prompt, and copyable raw prompt
	copyBlock := "\n\n<details>\n<summary>Copy prompt</summary>\n\n```\n" + gc.Prompt + "\n```\n\n</details>"
	var body string
	if gc.Motivation != "" {
		body = "## Why\n\n" + gc.Motivation + "\n\n## Prompt\n\n" + gc.Prompt + copyBlock
	} else {
		body = gc.Prompt + copyBlock
	}

	title := pr.Title
	if gc.Title != "" {
		title = gc.Title
		p.Queries.UpdatePromptRequestTitle(id, title)
	} else if title == "" {
		title = "Prompt Request"
	}

	issueTitle := "Prompt Request: " + title

	if pr.IssueNumber != nil {
		// Update existing issue
		if err := github.EditIssue(r.Context(), pr.RepoURL, *pr.IssueNumber, body); err != nil {
			log.Printf("editing issue: %v", err)
			http.Error(w, fmt.Sprintf("Failed to update GitHub issue: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		// Ensure "prompter" label exists (best-effort, don't block publish)
		var labels []string
		if err := github.EnsureLabel(r.Context(), pr.RepoURL, github.LabelName); err != nil {
			log.Printf("warning: ensuring label %q: %v", github.LabelName, err)
		} else {
			labels = []string{github.LabelName}
		}

		// Create new issue
		issue, err := github.CreateIssue(r.Context(), pr.RepoURL, issueTitle, body, labels)
		if err != nil {
			log.Printf("creating issue: %v", err)
			http.Error(w, fmt.Sprintf("Failed to create GitHub issue: %v", err), http.StatusInternalServerError)
			return
		}
		if err := p.Queries.UpdatePromptRequestIssue(id, issue.Number, issue.URL); err != nil {
			log.Printf("updating issue info: %v", err)
		}
	}

	// Create revision, linking it to the last message for inline marker placement
	var afterMsgID *int64
	if lastMsg, err := p.Queries.GetLastMessage(id); err == nil {
		afterMsgID = &lastMsg.ID
	}
	if _, err := p.Queries.CreateRevision(id, body, afterMsgID); err != nil {
		log.Printf("creating revision: %v", err)
	}

	// Update status to published
	if err := p.Queries.UpdatePromptRequestStatus(id, "published"); err != nil {
		log.Printf("updating status: %v", err)
	}

	// Build redirect URL from pr.RepoURL
	parts := strings.SplitN(pr.RepoURL, "/", 3)
	redirectURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d", parts[1], parts[2], id)

	// Use HX-Redirect for HTMX requests to trigger a full page navigation
	// (regular http.Redirect would be followed inline, producing malformed DOM)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", redirectURL)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}
