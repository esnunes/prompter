package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/esnunes/prompter/gotk"
	"github.com/esnunes/prompter/internal/claude"
	"github.com/esnunes/prompter/internal/github"
	"github.com/esnunes/prompter/internal/models"
	"github.com/esnunes/prompter/internal/repo"

	"github.com/google/uuid"
)

// Sidebar types

type sidebarItem struct {
	ID         int64
	Title      string
	Status     string // "draft", "published"
	Processing bool   // true if repoStatus shows cloning/pulling/processing
	Unread     bool   // true if new assistant response since last_viewed_at
	RepoURL    string // shown only on dashboard
	UpdatedAt  time.Time
	Org        string // for URL construction
	Repo       string // for URL construction
}

type sidebarData struct {
	Items     []sidebarItem
	Scope     string // "all" (dashboard) or "repo"
	CurrentID int64  // highlighted item (0 if not on conversation page)
	PollURL   string // URL for HTMX polling
}

// Base page data embedded in all page data structs
type basePageData struct {
	Sidebar sidebarData
}

type dashboardData struct {
	basePageData
	Repositories []models.RepositorySummary
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	repos, err := s.queries.ListRepositorySummaries()
	if err != nil {
		log.Printf("listing repository summaries: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	sidebarPRs, _ := s.queries.ListPromptRequests(false)
	sidebar := s.buildSidebar(sidebarPRs, "all", 0)
	s.renderPage(w, "dashboard.html", dashboardData{
		basePageData: basePageData{Sidebar: sidebar},
		Repositories: repos,
	})
}

type repoData struct {
	basePageData
	RepoURL        string
	Org            string
	Repo           string
	Error          string
	PromptRequests []models.PromptRequest
	ShowArchived   bool
}

func (s *Server) handleRepoPage(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")
	repoURL := fmt.Sprintf("github.com/%s/%s", org, repoName)

	if err := repo.ValidateURL(repoURL); err != nil {
		s.renderPage(w, "repo.html", repoData{
			basePageData: basePageData{Sidebar: s.buildSidebar(nil, "repo", 0)},
			RepoURL:      repoURL,
			Org:          org,
			Repo:         repoName,
			Error:        "Invalid repository URL format.",
		})
		return
	}

	// Verify repo exists on GitHub
	if err := github.VerifyRepo(r.Context(), org, repoName); err != nil {
		s.renderPage(w, "repo.html", repoData{
			basePageData: basePageData{Sidebar: s.buildSidebar(nil, "repo", 0)},
			RepoURL:      repoURL,
			Org:          org,
			Repo:         repoName,
			Error:        "This repository doesn't exist on GitHub or is not accessible.",
		})
		return
	}

	showArchived := r.URL.Query().Get("archived") == "1"
	prs, err := s.queries.ListPromptRequestsByRepoURL(repoURL, showArchived)
	if err != nil {
		log.Printf("listing prompt requests for repo: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Sidebar always gets active prompts
	sidebarPRs := prs
	if showArchived {
		sidebarPRs, _ = s.queries.ListPromptRequestsByRepoURL(repoURL, false)
	}
	sidebar := s.buildSidebar(sidebarPRs, "repo", 0)
	s.renderPage(w, "repo.html", repoData{
		basePageData:   basePageData{Sidebar: sidebar},
		RepoURL:        repoURL,
		Org:            org,
		Repo:           repoName,
		PromptRequests: prs,
		ShowArchived:   showArchived,
	})
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")
	repoURL := fmt.Sprintf("github.com/%s/%s", org, repoName)

	// Compute local path and upsert repo
	localPath, err := repo.LocalPath(repoURL)
	if err != nil {
		log.Printf("computing local path: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	repoRecord, err := s.queries.UpsertRepository(repoURL, localPath)
	if err != nil {
		log.Printf("upserting repository: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	sessionID := uuid.New().String()
	pr, err := s.queries.CreatePromptRequest(repoRecord.ID, sessionID)
	if err != nil {
		log.Printf("creating prompt request: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Determine initial status based on whether the repo is already cloned
	cloned, _ := repo.IsCloned(repoURL)
	if cloned {
		s.setRepoStatus(pr.ID, "pulling", "")
	} else {
		s.setRepoStatus(pr.ID, "cloning", "")
	}

	// Launch async clone/pull
	go s.asyncEnsureCloned(pr.ID, repoURL)

	http.Redirect(w, r, fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d", org, repoName, pr.ID), http.StatusSeeOther)
}

type conversationData struct {
	basePageData
	PromptRequest  *models.PromptRequest
	Org            string
	Repo           string
	RepoStatus     string // "cloning", "pulling", "ready", "processing", "cancelled", "error", or "" (no active operation)
	RepoStartedAt int64  // Unix timestamp for processing timer
	Timeline       []timelineItem
	LastQuestions   []questionData
	PromptReady    bool
	Revisions      []models.Revision
}

type timelineItem struct {
	Type     string // "message" or "revision-marker"
	Message  *models.Message
	Revision *models.Revision
}

type questionData struct {
	Header      string
	Text        string
	MultiSelect bool
	Options     []optionData
	Index       int
}

type optionData struct {
	Label       string
	Description string
}

func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")
	repoURL := fmt.Sprintf("github.com/%s/%s", org, repoName)

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	pr, err := s.queries.GetPromptRequest(id)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Update last_viewed_at for unread tracking
	s.queries.UpdateLastViewedAt(id)

	messages, err := s.queries.ListMessages(id)
	if err != nil {
		log.Printf("listing messages: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	revisions, err := s.queries.ListRevisions(id)
	if err != nil {
		log.Printf("listing revisions: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Check repo status for polling div
	statusEntry := s.getRepoStatus(id)
	repoStatus := statusEntry.Status
	if repoStatus == "" {
		// Server restart recovery: check filesystem
		cloned, _ := repo.IsCloned(repoURL)
		if cloned {
			repoStatus = "ready"
		}
	}
	// When status is "responded", the assistant message is already in the DB
	// and will be rendered by the template. Clear the map entry so that
	// subsequent actions (e.g., sending a new message) see "ready" state
	// and can trigger a new Claude call.
	if repoStatus == "responded" {
		s.repoStatus.Delete(id)
		repoStatus = "ready"
	}

	var repoStartedAt int64
	if !statusEntry.StartedAt.IsZero() {
		repoStartedAt = statusEntry.StartedAt.Unix()
	}

	// Build sidebar with repo-scoped active prompt requests (never archived)
	sidebarPRs, _ := s.queries.ListPromptRequestsByRepoURL(repoURL, false)
	sidebar := s.buildSidebar(sidebarPRs, "repo", id)

	data := conversationData{
		basePageData:   basePageData{Sidebar: sidebar},
		PromptRequest:  pr,
		Org:            org,
		Repo:           repoName,
		RepoStatus:     repoStatus,
		RepoStartedAt: repoStartedAt,
		Timeline:       buildTimeline(messages, revisions),
		Revisions:      revisions,
	}

	// Check the last assistant message for pending questions / prompt ready
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		if last.Role == "assistant" && last.RawResponse != nil {
			questions, promptReady := extractQuestionsFromRaw(*last.RawResponse)
			data.LastQuestions = questions
			data.PromptReady = promptReady
		}

		// Suppress prompt_ready if the last message was already published
		if data.PromptReady && len(revisions) > 0 {
			latestRev := revisions[len(revisions)-1] // ordered by published_at ASC
			if latestRev.AfterMessageID != nil && last.ID <= *latestRev.AfterMessageID {
				data.PromptReady = false
			}
		}
	}

	s.renderPage(w, "conversation.html", data)
}

type messageFragmentData struct {
	PromptRequestID int64
	Org             string
	Repo            string
	Messages        []models.Message
	Questions       []questionData
	PromptReady     bool
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

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
	userMsg, err := s.queries.CreateMessage(id, "user", userMessage, nil)
	if err != nil {
		log.Printf("saving user message: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// If repo is not ready, just save and disable form — auto-send kicks in when ready
	statusEntry := s.getRepoStatus(id)
	if statusEntry.Status != "" && statusEntry.Status != "ready" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fragment := messageFragmentData{
			PromptRequestID: id,
			Org:             org,
			Repo:            repoName,
			Messages:        []models.Message{*userMsg},
		}
		s.pages["message_fragment.html"].ExecuteTemplate(w, "message_fragment.html", fragment)
		fmt.Fprint(w, `<script>(function(){var f=document.getElementById('message-form');if(f){f.querySelector('textarea').disabled=true;f.querySelector('button').disabled=true;}})();</script>`)
		return
	}

	// Repo is ready — launch async Claude call
	ctx, cancel := context.WithCancel(context.Background())
	s.setRepoStatusProcessing(id, cancel)
	go s.backgroundSendMessage(ctx, id)

	// Return user message bubble + processing status div for polling
	pollURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d/status", org, repoName, id)
	cancelURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d/cancel", org, repoName, id)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fragment := messageFragmentData{
		PromptRequestID: id,
		Org:             org,
		Repo:            repoName,
		Messages:        []models.Message{*userMsg},
	}
	s.pages["message_fragment.html"].ExecuteTemplate(w, "message_fragment.html", fragment)

	// Remove any stale #repo-status element (e.g. leftover "Repository ready!" div)
	// before appending the new processing div to avoid duplicate IDs.
	fmt.Fprint(w, `<script>(function(){var old=document.getElementById('repo-status');if(old)old.remove();})();</script>`)

	// Append processing status div that starts polling
	entry := s.getRepoStatus(id)
	fmt.Fprintf(w, `<div id="repo-status" class="repo-status" hx-get="%s" hx-trigger="every 2s" hx-swap="morph:outerHTML" data-started-at="%d">`, pollURL, entry.StartedAt.Unix())
	fmt.Fprint(w, `<div class="processing-indicator"><div class="spinner"></div><span class="processing-text">Thinking...</span><span class="elapsed-timer"></span></div>`)
	fmt.Fprintf(w, `<form hx-post="%s" hx-target="#repo-status" hx-swap="outerHTML" hx-disabled-elt="find button" style="display:inline;"><button type="submit" class="btn btn-sm btn-secondary">Cancel</button></form>`, cancelURL)
	fmt.Fprint(w, `</div>`)

	// Disable the message form while processing (setTimeout to run after HTMX re-enables hx-disabled-elt)
	fmt.Fprint(w, `<script>setTimeout(function(){var f=document.getElementById('message-form');if(f){f.querySelector('textarea').disabled=true;f.querySelector('button').disabled=true;}if(typeof updateElapsedTimers==='function')updateElapsedTimers();},0);</script>`)
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	pr, err := s.queries.GetPromptRequest(id)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Get the generated content (motivation + prompt)
	gc, err := s.queries.GetLatestGeneratedContent(id)
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
		s.queries.UpdatePromptRequestTitle(id, title)
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
		if err := s.queries.UpdatePromptRequestIssue(id, issue.Number, issue.URL); err != nil {
			log.Printf("updating issue info: %v", err)
		}
	}

	// Create revision, linking it to the last message for inline marker placement
	var afterMsgID *int64
	if lastMsg, err := s.queries.GetLastMessage(id); err == nil {
		afterMsgID = &lastMsg.ID
	}
	if _, err := s.queries.CreateRevision(id, body, afterMsgID); err != nil {
		log.Printf("creating revision: %v", err)
	}

	// Update status to published
	if err := s.queries.UpdatePromptRequestStatus(id, "published"); err != nil {
		log.Printf("updating status: %v", err)
	}

	// Use HX-Redirect for HTMX requests to trigger a full page navigation
	// (regular http.Redirect would be followed inline, producing malformed DOM)
	redirectURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d", org, repoName, id)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", redirectURL)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	if err := s.queries.DeletePromptRequest(id); err != nil {
		log.Printf("deleting prompt request: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/github.com/%s/%s/prompt-requests", org, repoName), http.StatusSeeOther)
}

type archiveBannerData struct {
	Org           string
	Repo          string
	PromptRequest *models.PromptRequest
}

func (s *Server) handleArchive(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	if err := s.queries.ArchivePromptRequest(id); err != nil {
		log.Printf("archiving prompt request: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// If HTMX request (from conversation page), return the archived banner fragment
	if r.Header.Get("HX-Request") == "true" {
		pr, _ := s.queries.GetPromptRequest(id)
		s.renderFragment(w, "archive_banner_fragment.html", archiveBannerData{
			Org:           org,
			Repo:          repoName,
			PromptRequest: pr,
		})
		return
	}

	// Otherwise (from list page), redirect back
	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = fmt.Sprintf("/github.com/%s/%s/prompt-requests", org, repoName)
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}

func (s *Server) handleUnarchive(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	if err := s.queries.UnarchivePromptRequest(id); err != nil {
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
		referer = fmt.Sprintf("/github.com/%s/%s/prompt-requests", org, repoName)
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}

// asyncEnsureCloned runs clone/pull in the background, updating status in sync.Map.
// On completion, pushes status to connected clients and auto-sends pending user messages.
func (s *Server) asyncEnsureCloned(prID int64, repoURL string) {
	// Serialize clone/pull operations per repo to prevent concurrent git corruption
	mu := s.lockRepo(repoURL)
	defer mu.Unlock()

	_, err := repo.EnsureCloned(context.Background(), repoURL)
	if err != nil {
		log.Printf("async clone/pull failed for %s: %v", repoURL, err)
		s.setRepoStatus(prID, "error", err.Error())
		s.pushStatusUpdate(prID, "error", s.getRepoStatus(prID))
		return
	}
	s.setRepoStatus(prID, "ready", "")

	// Check for pending user message to auto-send
	lastMsg, msgErr := s.queries.GetLastMessage(prID)
	if msgErr == nil && lastMsg.Role == "user" {
		// Atomically transition to "processing" to prevent duplicate Claude calls
		old := repoStatusEntry{Status: "ready"}
		if s.repoStatus.CompareAndSwap(prID, old, repoStatusEntry{Status: "processing"}) {
			ctx, cancel := context.WithCancel(context.Background())
			s.setRepoStatusProcessing(prID, cancel)
			s.pushStatusUpdate(prID, "processing", s.getRepoStatus(prID))
			go s.backgroundSendMessage(ctx, prID)
			return
		}
	}

	// No auto-send needed — push "ready" status
	s.pushStatusUpdate(prID, "ready", s.getRepoStatus(prID))
}

type statusFragmentData struct {
	Status    string
	Error     string
	PollURL   string
	RetryURL  string
	CancelURL string
	ResendURL string
	StartedAt int64 // Unix timestamp, 0 if not processing
}

func (s *Server) handleRepoStatus(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")
	repoURL := fmt.Sprintf("github.com/%s/%s", org, repoName)

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	pollURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d/status", org, repoName, id)
	retryURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d/retry", org, repoName, id)

	entry := s.getRepoStatus(id)

	// Server restart recovery: if no status tracked, check filesystem
	if entry.Status == "" {
		cloned, _ := repo.IsCloned(repoURL)
		if cloned {
			s.setRepoStatus(id, "ready", "")
			entry = repoStatusEntry{Status: "ready"}
		} else {
			// Auto-start clone
			s.setRepoStatus(id, "cloning", "")
			go s.asyncEnsureCloned(id, repoURL)
			entry = repoStatusEntry{Status: "cloning"}
		}
	}

	// If ready, check for a pending user message to auto-send
	if entry.Status == "ready" {
		lastMsg, err := s.queries.GetLastMessage(id)
		if err == nil && lastMsg.Role == "user" {
			// Atomically transition to "processing" to prevent duplicate Claude calls
			old := repoStatusEntry{Status: "ready"}
			if s.repoStatus.CompareAndSwap(id, old, repoStatusEntry{Status: "processing"}) {
				ctx, cancel := context.WithCancel(context.Background())
				s.setRepoStatusProcessing(id, cancel)
				go s.backgroundSendMessage(ctx, id)
			}
			entry = s.getRepoStatus(id)
		}
	}

	// If responded, deliver the assistant message and stop polling.
	// We replace #repo-status with the response content plus a script that
	// moves the messages into #conversation at the correct position.
	if entry.Status == "responded" {
		s.repoStatus.Delete(id)
		lastMsg, err := s.queries.GetLastMessage(id)
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
			s.pages["message_fragment.html"].ExecuteTemplate(w, "message_fragment.html", fragment)
			fmt.Fprint(w, `</div><script>`)
			fmt.Fprint(w, `(function(){var s=document.getElementById('repo-status');var c=document.getElementById('conversation');while(s.firstChild){c.appendChild(s.firstChild);}s.remove();htmx.process(c);if(typeof renderMarkdown==='function')renderMarkdown();if(typeof scrollConversation==='function')scrollConversation();else{c.scrollTop=c.scrollHeight;}var f=document.getElementById('message-form');if(f){f.querySelector('textarea').disabled=false;f.querySelector('button').disabled=false;}})();`)
			fmt.Fprint(w, `</script>`)
			return
		}
	}

	cancelURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d/cancel", org, repoName, id)
	resendURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d/resend", org, repoName, id)

	var startedAt int64
	if !entry.StartedAt.IsZero() {
		startedAt = entry.StartedAt.Unix()
	}

	s.renderFragment(w, "status_fragment.html", statusFragmentData{
		Status:    entry.Status,
		Error:     entry.Error,
		PollURL:   pollURL,
		RetryURL:  retryURL,
		CancelURL: cancelURL,
		ResendURL: resendURL,
		StartedAt: startedAt,
	})
}

// backgroundSendMessage processes a pending user message with Claude in a background goroutine.
// It saves the response to DB and updates the repo status to "responded" or "cancelled".
func (s *Server) backgroundSendMessage(ctx context.Context, prID int64) {
	defer s.clearCancelFunc(prID)

	pr, err := s.queries.GetPromptRequest(prID)
	if err != nil {
		log.Printf("auto-send: getting prompt request: %v", err)
		s.setRepoStatus(prID, "error", fmt.Sprintf("Failed to load prompt request: %v", err))
		return
	}

	lastMsg, err := s.queries.GetLastMessage(prID)
	if err != nil || lastMsg.Role != "user" {
		log.Printf("auto-send: no pending user message for PR %d", prID)
		s.setRepoStatus(prID, "ready", "")
		return
	}

	// Acquire session lock to prevent concurrent Claude calls
	mu := s.lockSession(pr.SessionID)
	defer mu.Unlock()

	// Re-check: ensure last message is still from user (not already processed)
	lastMsg, err = s.queries.GetLastMessage(prID)
	if err != nil || lastMsg.Role != "user" {
		s.setRepoStatus(prID, "ready", "")
		return
	}

	// Determine resume vs new
	existingMsgs, err := s.queries.ListMessages(prID)
	if err != nil {
		log.Printf("auto-send: listing messages: %v", err)
		s.setRepoStatus(prID, "error", fmt.Sprintf("Failed to list messages: %v", err))
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
			s.queries.CreateMessage(prID, "assistant", "Request cancelled by user.", nil)
			s.setRepoStatus(prID, "cancelled", "")
			s.pushAll(s.buildResponsePush(prID, "Request cancelled by user.", nil))
			return
		}
		log.Printf("auto-send: claude error: %v", err)
		errMsg := fmt.Sprintf("Sorry, I encountered an error: %v", err)
		s.queries.CreateMessage(prID, "assistant", errMsg, nil)
		s.setRepoStatus(prID, "responded", "")
		s.pushAll(s.buildResponsePush(prID, errMsg, nil))
		return
	}

	if _, err := s.queries.CreateMessage(prID, "assistant", resp.Message, &rawJSON); err != nil {
		log.Printf("auto-send: saving assistant message: %v", err)
		s.setRepoStatus(prID, "error", "Failed to save response")
		s.pushAll(s.buildResponsePush(prID, "Failed to save response", nil))
		return
	}

	// Set title from response
	if pr.Title == "" {
		if resp.GeneratedTitle != "" {
			s.queries.UpdatePromptRequestTitle(prID, resp.GeneratedTitle)
		} else if resp.Message != "" {
			title := resp.Message
			if len(title) > 60 {
				title = title[:60] + "..."
			}
			s.queries.UpdatePromptRequestTitle(prID, title)
		}
	} else if resp.GeneratedTitle != "" {
		s.queries.UpdatePromptRequestTitle(prID, resp.GeneratedTitle)
	}

	s.setRepoStatus(prID, "responded", "")
	s.pushAll(s.buildResponsePush(prID, resp.Message, &rawJSON))
	s.pushSidebarUpdate()
}

func (s *Server) handleRetry(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")
	repoURL := fmt.Sprintf("github.com/%s/%s", org, repoName)

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	cloned, _ := repo.IsCloned(repoURL)
	if cloned {
		s.setRepoStatus(id, "pulling", "")
	} else {
		s.setRepoStatus(id, "cloning", "")
	}

	go s.asyncEnsureCloned(id, repoURL)

	pollURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d/status", org, repoName, id)
	retryURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d/retry", org, repoName, id)

	s.renderFragment(w, "status_fragment.html", statusFragmentData{
		Status:   s.getRepoStatus(id).Status,
		PollURL:  pollURL,
		RetryURL: retryURL,
	})
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Call the cancel function if processing
	entry := s.getRepoStatus(id)
	if entry.Status == "processing" {
		if v, ok := s.cancelFuncs.Load(id); ok {
			if cancel, ok := v.(context.CancelFunc); ok {
				cancel()
			}
		}
	}

	// Return current status — the background goroutine will transition to "cancelled"
	pollURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d/status", org, repoName, id)
	cancelURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d/cancel", org, repoName, id)

	s.renderFragment(w, "status_fragment.html", statusFragmentData{
		Status:    "processing",
		PollURL:   pollURL,
		CancelURL: cancelURL,
		StartedAt: entry.StartedAt.Unix(),
	})
}

func (s *Server) handleResend(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Delete the synthetic cancelled assistant message
	lastMsg, err := s.queries.GetLastMessage(id)
	if err == nil && lastMsg.Role == "assistant" && lastMsg.Content == "Request cancelled by user." {
		s.queries.DeleteMessage(lastMsg.ID)
	}

	// Launch async Claude call
	ctx, cancel := context.WithCancel(context.Background())
	s.setRepoStatusProcessing(id, cancel)
	go s.backgroundSendMessage(ctx, id)

	// Return processing status fragment
	pollURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d/status", org, repoName, id)
	cancelURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d/cancel", org, repoName, id)
	entry := s.getRepoStatus(id)

	s.renderFragment(w, "status_fragment.html", statusFragmentData{
		Status:    "processing",
		PollURL:   pollURL,
		CancelURL: cancelURL,
		StartedAt: entry.StartedAt.Unix(),
	})
}

// buildTimeline interleaves messages and revision markers into a single chronological timeline.
func buildTimeline(messages []models.Message, revisions []models.Revision) []timelineItem {
	// Map afterMessageID → revisions for O(1) lookup
	revByMsg := map[int64][]models.Revision{}
	var orphanRevs []models.Revision
	for _, rev := range revisions {
		if rev.AfterMessageID != nil {
			revByMsg[*rev.AfterMessageID] = append(revByMsg[*rev.AfterMessageID], rev)
		} else {
			orphanRevs = append(orphanRevs, rev)
		}
	}

	var items []timelineItem
	for i := range messages {
		items = append(items, timelineItem{Type: "message", Message: &messages[i]})
		if revs, ok := revByMsg[messages[i].ID]; ok {
			for j := range revs {
				items = append(items, timelineItem{Type: "revision-marker", Revision: &revs[j]})
			}
		}
	}
	// Append orphan revisions (legacy data with NULL after_message_id)
	for i := range orphanRevs {
		items = append(items, timelineItem{Type: "revision-marker", Revision: &orphanRevs[i]})
	}
	return items
}

// extractQuestionsFromRaw parses the raw Claude response to find pending questions.
// It supports both the new "questions" array and the old singular "question" field
// for backward compatibility with existing sessions.
func extractQuestionsFromRaw(rawJSON string) ([]questionData, bool) {
	resp := parseRawResponse(rawJSON)
	if resp == nil {
		return nil, false
	}

	if len(resp.Questions) == 0 {
		// Try the old singular "question" field for backward compat
		questions := extractLegacyQuestion(rawJSON)
		return questions, resp.PromptReady
	}

	var questions []questionData
	for i, q := range resp.Questions {
		qd := questionData{
			Header:      q.Header,
			Text:        q.Text,
			MultiSelect: q.MultiSelect,
			Index:       i,
		}
		for _, opt := range q.Options {
			qd.Options = append(qd.Options, optionData{Label: opt.Label, Description: opt.Description})
		}
		questions = append(questions, qd)
	}
	return questions, resp.PromptReady
}

// extractLegacyQuestion handles old raw_response JSON that used the singular "question" field.
func extractLegacyQuestion(rawJSON string) []questionData {
	// Parse looking for the old schema shape: {"question": {"text": "...", "options": [...]}}
	var legacy struct {
		StructuredOutput *struct {
			Question *struct {
				Text    string `json:"text"`
				Options []struct {
					Label       string `json:"label"`
					Description string `json:"description"`
				} `json:"options"`
			} `json:"question"`
		} `json:"structured_output"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &legacy); err != nil {
		return nil
	}
	if legacy.StructuredOutput == nil || legacy.StructuredOutput.Question == nil {
		return nil
	}

	q := legacy.StructuredOutput.Question
	qd := questionData{Text: q.Text, Index: 0}
	for _, opt := range q.Options {
		qd.Options = append(qd.Options, optionData{Label: opt.Label, Description: opt.Description})
	}
	return []questionData{qd}
}

// assembleQuestionAnswers reads multi-question form fields (q_0, q_0_other, q_1, etc.)
// and assembles them into a single answer string to send to Claude.
func assembleQuestionAnswers(r *http.Request) string {
	var answers []string
	var headers []string

	for i := 0; ; i++ {
		key := fmt.Sprintf("q_%d", i)
		header := r.FormValue(fmt.Sprintf("q_%d_header", i))

		// Check if this question exists in the form
		values, exists := r.Form[key]
		if !exists {
			break
		}

		otherText := strings.TrimSpace(r.FormValue(fmt.Sprintf("q_%d_other", i)))

		// Build the answer for this question
		var parts []string
		for _, v := range values {
			if v == "__other__" {
				if otherText != "" {
					parts = append(parts, "Other: "+otherText)
				}
			} else if v != "" {
				parts = append(parts, v)
			}
		}

		if len(parts) > 0 {
			answers = append(answers, strings.Join(parts, ", "))
			headers = append(headers, header)
		}
	}

	if len(answers) == 0 {
		return ""
	}

	// Single question: just the answer, no prefix
	if len(answers) == 1 {
		return answers[0]
	}

	// Multiple questions: prefix each with header or question index
	var lines []string
	for i, answer := range answers {
		if headers[i] != "" {
			lines = append(lines, headers[i]+": "+answer)
		} else {
			lines = append(lines, fmt.Sprintf("Q%d: %s", i+1, answer))
		}
	}
	return strings.Join(lines, "\n")
}

// assembleQuestionAnswersFromPayload assembles question answers from a gotk payload.
// Mirrors assembleQuestionAnswers but works with gotk.Payload instead of *http.Request.
func assembleQuestionAnswersFromPayload(p gotk.Payload) string {
	data := p.Map()
	var answers []string
	var headers []string

	for i := 0; ; i++ {
		key := fmt.Sprintf("q_%d", i)
		header := p.String(fmt.Sprintf("q_%d_header", i))

		raw, exists := data[key]
		if !exists {
			break
		}

		otherText := strings.TrimSpace(p.String(fmt.Sprintf("q_%d_other", i)))

		// Collect values: may be a string (radio) or []any (checkboxes)
		var values []string
		switch v := raw.(type) {
		case string:
			if v != "" {
				values = append(values, v)
			}
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok && s != "" {
					values = append(values, s)
				}
			}
		}

		var parts []string
		for _, v := range values {
			if v == "__other__" {
				if otherText != "" {
					parts = append(parts, "Other: "+otherText)
				}
			} else {
				parts = append(parts, v)
			}
		}

		if len(parts) > 0 {
			answers = append(answers, strings.Join(parts, ", "))
			headers = append(headers, header)
		}
	}

	if len(answers) == 0 {
		return ""
	}

	if len(answers) == 1 {
		return answers[0]
	}

	var lines []string
	for i, answer := range answers {
		if headers[i] != "" {
			lines = append(lines, headers[i]+": "+answer)
		} else {
			lines = append(lines, fmt.Sprintf("Q%d: %s", i+1, answer))
		}
	}
	return strings.Join(lines, "\n")
}

// buildSidebar creates sidebar data from a list of prompt requests, merging in
// processing state from the in-memory repoStatus map and computing unread flags.
func (s *Server) buildSidebar(prs []models.PromptRequest, scope string, currentID int64) sidebarData {
	var items []sidebarItem
	for _, pr := range prs {
		// Parse org/repo from RepoURL (github.com/org/repo)
		parts := strings.SplitN(pr.RepoURL, "/", 3)
		var org, repoName string
		if len(parts) == 3 {
			org = parts[1]
			repoName = parts[2]
		}

		// Check processing state from in-memory status
		processing := false
		entry := s.getRepoStatus(pr.ID)
		if entry.Status == "cloning" || entry.Status == "pulling" || entry.Status == "processing" {
			processing = true
		}

		// Compute unread: has assistant response newer than last_viewed_at
		unread := false
		if pr.LatestAssistantAt != nil && pr.ID != currentID {
			if pr.LastViewedAt == nil {
				unread = true
			} else if pr.LatestAssistantAt.After(*pr.LastViewedAt) {
				unread = true
			}
		}

		items = append(items, sidebarItem{
			ID:         pr.ID,
			Title:      pr.Title,
			Status:     pr.Status,
			Processing: processing,
			Unread:     unread,
			RepoURL:    pr.RepoURL,
			UpdatedAt:  pr.UpdatedAt,
			Org:        org,
			Repo:       repoName,
		})
	}

	// Sort: processing first, then drafts, then published; within each group by UpdatedAt DESC
	// The DB query already sorts drafts before published by updated_at DESC,
	// so we just need to float processing items to the top.
	sortSidebarItems(items)

	// Build poll URL
	pollURL := "/api/sidebar?scope=" + scope
	if scope == "repo" && len(prs) > 0 {
		pollURL += "&repo_url=" + prs[0].RepoURL
	}
	if currentID != 0 {
		pollURL += "&current_id=" + strconv.FormatInt(currentID, 10)
	}

	return sidebarData{
		Items:     items,
		Scope:     scope,
		CurrentID: currentID,
		PollURL:   pollURL,
	}
}

// sortSidebarItems sorts items: processing first, then drafts, then published.
func sortSidebarItems(items []sidebarItem) {
	// Stable sort to preserve the DB's updated_at DESC ordering within groups.
	// We do a simple insertion-sort-style partition since the list is small.
	n := len(items)
	if n <= 1 {
		return
	}

	// Assign sort keys: processing=0, draft=1, published=2
	key := func(item sidebarItem) int {
		if item.Processing {
			return 0
		}
		if item.Status == "draft" {
			return 1
		}
		return 2
	}

	// Simple stable sort (bubble sort is fine for small N)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-1-i; j++ {
			if key(items[j]) > key(items[j+1]) {
				items[j], items[j+1] = items[j+1], items[j]
			}
		}
	}
}

// handleSidebarFragment returns the sidebar HTML fragment for HTMX polling.
func (s *Server) handleSidebarFragment(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope")
	repoURL := r.URL.Query().Get("repo_url")
	currentID, _ := strconv.ParseInt(r.URL.Query().Get("current_id"), 10, 64)

	var prs []models.PromptRequest
	var err error
	if scope == "repo" && repoURL != "" {
		prs, err = s.queries.ListPromptRequestsByRepoURL(repoURL, false)
	} else {
		prs, err = s.queries.ListPromptRequests(false)
	}
	if err != nil {
		log.Printf("sidebar query error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	sidebar := s.buildSidebar(prs, scope, currentID)
	s.renderFragment(w, "sidebar.html", sidebar)
}

// pushSidebarUpdate renders the sidebar and pushes it to all connected clients.
func (s *Server) pushSidebarUpdate() {
	// Fetch all non-archived prompt requests for the sidebar
	prs, err := s.queries.ListPromptRequests(false)
	if err != nil {
		log.Printf("sidebar push: query error: %v", err)
		return
	}

	sidebar := s.buildSidebar(prs, "all", 0)

	tmpl, ok := s.pages["sidebar.html"]
	if !ok {
		log.Printf("sidebar push: template not found")
		return
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "sidebar.html", sidebar); err != nil {
		log.Printf("sidebar push: render error: %v", err)
		return
	}

	s.pushAll([]gotk.Instruction{
		{Op: "html", Target: "#prompt-sidebar", HTML: buf.String()},
	})
}

// parseRawResponse extracts a claude.Response from the raw JSON stored in the DB.
func parseRawResponse(rawJSON string) *claude.Response {
	// The raw JSON is the full claude CLI output: {"type":"result","structured_output":{...},...}
	var wrapper struct {
		StructuredOutput *claude.Response `json:"structured_output"`
		Result           string           `json:"result"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &wrapper); err == nil {
		if wrapper.StructuredOutput != nil {
			return wrapper.StructuredOutput
		}
		if wrapper.Result != "" {
			var resp claude.Response
			if json.Unmarshal([]byte(wrapper.Result), &resp) == nil {
				return &resp
			}
		}
	}

	// Try direct parse
	var resp claude.Response
	if json.Unmarshal([]byte(rawJSON), &resp) == nil && resp.Message != "" {
		return &resp
	}
	return nil
}

// buildStatusPush builds gotk instructions to push a status update to all clients.
func (s *Server) buildStatusPush(prID int64, status string, entry repoStatusEntry) []gotk.Instruction {
	var ins []gotk.Instruction
	var statusHTML string

	switch status {
	case "cloning":
		statusHTML = `<div id="repo-status" class="repo-status">` +
			`<div class="spinner"></div> Cloning repository...</div>`
	case "pulling":
		statusHTML = `<div id="repo-status" class="repo-status">` +
			`<div class="spinner"></div> Pulling latest changes...</div>`
	case "processing":
		statusHTML = fmt.Sprintf(
			`<div id="repo-status" class="repo-status" data-started-at="%d">`+
				`<div class="processing-indicator"><div class="spinner"></div>`+
				`<span class="processing-text">Thinking...</span>`+
				`<span class="elapsed-timer"></span></div>`+
				`<button gotk-click="cancel-message" gotk-val-prompt_request_id="%d" `+
				`class="btn btn-sm btn-secondary">Cancel</button></div>`,
			entry.StartedAt.Unix(), prID)
	case "cancelled":
		statusHTML = fmt.Sprintf(
			`<div id="repo-status" class="repo-status repo-status-cancelled">`+
				`<span>Request cancelled.</span>`+
				`<button gotk-click="resend" gotk-val-prompt_request_id="%d" `+
				`gotk-loading="Resending..." class="btn btn-sm btn-primary">Retry</button></div>`,
			prID)
		// Re-enable form on cancel
		ins = append(ins, gotk.Instruction{Op: "attr-remove", Target: "#message-input", Attr: "disabled"})
		ins = append(ins, gotk.Instruction{Op: "attr-remove", Target: "#send-btn", Attr: "disabled"})
	case "error":
		statusHTML = fmt.Sprintf(
			`<div id="repo-status" class="repo-status repo-status-error">`+
				`<span>Error: %s</span>`+
				`<button gotk-click="retry" gotk-val-prompt_request_id="%d" `+
				`gotk-loading="Retrying..." class="btn btn-sm btn-secondary">Retry</button></div>`,
			template.HTMLEscapeString(entry.Error), prID)
	case "ready":
		statusHTML = `<div id="repo-status" class="repo-status repo-status-ready">Repository ready!</div>`
		// Re-enable form
		ins = append(ins, gotk.Instruction{Op: "attr-remove", Target: "#message-input", Attr: "disabled"})
		ins = append(ins, gotk.Instruction{Op: "attr-remove", Target: "#send-btn", Attr: "disabled"})
		ins = append(ins, gotk.Instruction{Op: "focus", Target: "#message-input"})
	default:
		return nil
	}

	// Replace existing #repo-status or append to #conversation
	ins = append([]gotk.Instruction{
		{Op: "remove", Target: "#repo-status"},
		{Op: "html", Target: "#conversation", HTML: statusHTML, Mode: gotk.Append},
	}, ins...)

	ins = append(ins, gotk.Instruction{Op: "exec", Name: "scrollConversation"})
	ins = append(ins, gotk.Instruction{Op: "exec", Name: "updateElapsedTimers"})

	return ins
}

// pushStatusUpdate pushes a status change to all connected clients.
func (s *Server) pushStatusUpdate(prID int64, status string, entry repoStatusEntry) {
	ins := s.buildStatusPush(prID, status, entry)
	if ins != nil {
		s.pushAll(ins)
	}
}

// orgRepoForPR returns the org and repo name for a prompt request.
func (s *Server) orgRepoForPR(prID int64) (string, string) {
	pr, err := s.queries.GetPromptRequest(prID)
	if err != nil {
		return "", ""
	}
	// RepoURL format: "github.com/org/repo"
	parts := strings.SplitN(pr.RepoURL, "/", 3)
	if len(parts) < 3 {
		return "", ""
	}
	return parts[1], parts[2]
}

// buildResponsePush builds gotk instructions to push a Claude response to the client.
// It removes the spinner, appends the assistant message, re-enables the form, and triggers
// markdown rendering and scroll.
func (s *Server) buildResponsePush(prID int64, message string, rawJSON *string) []gotk.Instruction {
	var ins []gotk.Instruction

	// Remove spinner
	ins = append(ins, gotk.Instruction{Op: "html", Target: "#repo-status", Mode: gotk.Remove})

	// Append assistant message
	msgHTML := `<div class="message message-assistant"><div class="message-bubble">` +
		template.HTMLEscapeString(message) + `</div></div>`
	ins = append(ins, gotk.Instruction{Op: "html", Target: "#conversation", HTML: msgHTML, Mode: gotk.Append})

	// Handle questions / prompt-ready from raw response
	hasQuestions := false
	if rawJSON != nil {
		questions, promptReady := extractQuestionsFromRaw(*rawJSON)
		// Get org/repo for form URLs
		org, repoName := s.orgRepoForPR(prID)
		if len(questions) > 0 && org != "" {
			ins = append(ins, s.buildQuestionPush(prID, org, repoName, questions)...)
			hasQuestions = true
		}
		if promptReady && org != "" {
			ins = append(ins, s.buildPromptReadyPush(prID, org, repoName)...)
		}
	}

	// Re-enable input (but hide message form if questions are shown)
	ins = append(ins, gotk.Instruction{Op: "attr-remove", Target: "#message-input", Attr: "disabled"})
	ins = append(ins, gotk.Instruction{Op: "attr-remove", Target: "#send-btn", Attr: "disabled"})
	if hasQuestions {
		ins = append(ins, gotk.Instruction{Op: "attr-set", Target: "#message-form", Attr: "style", Value: "display:none"})
	}

	// Render markdown and scroll
	ins = append(ins, gotk.Instruction{Op: "exec", Name: "renderMarkdown"})
	ins = append(ins, gotk.Instruction{Op: "exec", Name: "scrollConversation"})

	return ins
}

// buildQuestionPush builds gotk instructions to display Claude's questions.
func (s *Server) buildQuestionPush(prID int64, org, repoName string, questions []questionData) []gotk.Instruction {
	var html strings.Builder
	html.WriteString(`<div class="question-block" id="question-form">`)
	html.WriteString(`<div id="question-form-fields">`)
	html.WriteString(fmt.Sprintf(`<input type="hidden" name="prompt_request_id" value="%d">`, prID))

	for _, q := range questions {
		html.WriteString(`<div class="question-group">`)
		if q.Header != "" {
			html.WriteString(fmt.Sprintf(`<span class="question-header">%s</span>`, template.HTMLEscapeString(q.Header)))
		}
		html.WriteString(fmt.Sprintf(`<h4>%s</h4>`, template.HTMLEscapeString(q.Text)))
		html.WriteString(fmt.Sprintf(`<input type="hidden" name="q_%d_header" value="%s">`, q.Index, template.HTMLEscapeString(q.Header)))
		html.WriteString(`<div class="options-list">`)
		for _, opt := range q.Options {
			inputType := "radio"
			if q.MultiSelect {
				inputType = "checkbox"
			}
			html.WriteString(fmt.Sprintf(`<label class="option-item"><input type="%s" name="q_%d" value="%s"><div><div class="option-label">%s</div><div class="option-description">%s</div></div></label>`,
				inputType, q.Index, template.HTMLEscapeString(opt.Label), template.HTMLEscapeString(opt.Label), template.HTMLEscapeString(opt.Description)))
		}
		inputType := "radio"
		if q.MultiSelect {
			inputType = "checkbox"
		}
		html.WriteString(fmt.Sprintf(`<label class="option-item other-option"><input type="%s" name="q_%d" value="__other__"><div><div class="option-label">Other</div></div></label>`, inputType, q.Index))
		html.WriteString(`</div>`)
		html.WriteString(fmt.Sprintf(`<input type="text" name="q_%d_other" class="other-input" placeholder="Type your answer..." maxlength="500">`, q.Index))
		html.WriteString(`</div>`)
	}
	html.WriteString(`</div>`) // close #question-form-fields
	html.WriteString(`<div class="mt-4">`)
	html.WriteString(`<button gotk-click="answer-question" gotk-collect="#question-form-fields" gotk-loading="Sending..." class="btn btn-primary">Answer</button>`)
	html.WriteString(`</div>`)
	html.WriteString(`</div>`)

	return []gotk.Instruction{
		{Op: "html", Target: "#conversation", HTML: html.String(), Mode: gotk.Append},
	}
}

// buildPromptReadyPush builds gotk instructions to display the publish form.
func (s *Server) buildPromptReadyPush(prID int64, org, repoName string) []gotk.Instruction {
	publishHTML := fmt.Sprintf(`<div class="prompt-ready" id="publish-form">`+
		`<p>Prompt is ready to publish!</p>`+
		`<button gotk-click="publish" gotk-val-prompt_request_id="%d" `+
		`gotk-loading="Publishing..." class="btn btn-primary">Publish to GitHub</button>`+
		`</div>`, prID)

	return []gotk.Instruction{
		{Op: "html", Target: "#conversation", HTML: publishHTML, Mode: gotk.Append},
	}
}

// registerGotkCommands registers gotk command handlers on the mux.
func (s *Server) registerGotkCommands() {
	s.gotkMux.Handle("send-message", func(ctx *gotk.Context) error {
		idStr := ctx.Payload.String("prompt_request_id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			ctx.Error("#conversation", "Invalid prompt request ID")
			return nil
		}

		message := strings.TrimSpace(ctx.Payload.String("message"))
		if message == "" {
			return nil
		}

		// Save user message
		userMsg, err := s.queries.CreateMessage(id, "user", message, nil)
		if err != nil {
			ctx.Error("#conversation", "Failed to save message")
			return nil
		}

		// Render user message bubble and append to conversation
		userHTML := `<div class="message message-user"><div class="message-bubble">` +
			template.HTMLEscapeString(userMsg.Content) + `</div></div>`
		ctx.HTML("#conversation", userHTML, gotk.Append)

		// Clear the textarea
		ctx.SetValue("#message-input", "")

		// Check repo status — if not ready, just save and disable form
		statusEntry := s.getRepoStatus(id)
		if statusEntry.Status != "" && statusEntry.Status != "ready" {
			ctx.AttrSet("#message-input", "disabled", "true")
			ctx.AttrSet("#send-btn", "disabled", "true")
			return nil
		}

		// Repo is ready — launch async Claude call
		bgCtx, cancel := context.WithCancel(context.Background())
		s.setRepoStatusProcessing(id, cancel)
		go s.backgroundSendMessage(bgCtx, id)

		// Show processing indicator with gotk-based cancel
		entry := s.getRepoStatus(id)
		processingHTML := fmt.Sprintf(
			`<div id="repo-status" class="repo-status" data-started-at="%d">`+
				`<div class="processing-indicator"><div class="spinner"></div>`+
				`<span class="processing-text">Thinking...</span>`+
				`<span class="elapsed-timer"></span></div>`+
				`<button gotk-click="cancel-message" gotk-val-prompt_request_id="%d" `+
				`class="btn btn-sm btn-secondary">Cancel</button></div>`,
			entry.StartedAt.Unix(), id)

		// Remove any stale #repo-status, then append new one
		ctx.Remove("#repo-status")
		ctx.HTML("#conversation", processingHTML, gotk.Append)

		ctx.Exec("scrollConversation")
		ctx.Exec("updateElapsedTimers")

		// Disable input while processing
		ctx.AttrSet("#message-input", "disabled", "true")
		ctx.AttrSet("#send-btn", "disabled", "true")

		return nil
	})

	s.gotkMux.Handle("cancel-message", func(ctx *gotk.Context) error {
		idStr := ctx.Payload.String("prompt_request_id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return nil
		}

		entry := s.getRepoStatus(id)
		if entry.Status == "processing" {
			if v, ok := s.cancelFuncs.Load(id); ok {
				if cancel, ok := v.(context.CancelFunc); ok {
					cancel()
				}
			}
		}

		// The background goroutine will detect cancellation and push UI updates.
		return nil
	})

	s.gotkMux.Handle("answer-question", func(ctx *gotk.Context) error {
		idStr := ctx.Payload.String("prompt_request_id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			ctx.Error("#conversation", "Invalid prompt request ID")
			return nil
		}

		message := assembleQuestionAnswersFromPayload(ctx.Payload)
		if message == "" {
			return nil
		}

		// Save user message
		userMsg, err := s.queries.CreateMessage(id, "user", message, nil)
		if err != nil {
			ctx.Error("#conversation", "Failed to save message")
			return nil
		}

		// Remove question form, show message form again
		ctx.Remove("#question-form")
		ctx.AttrRemove("#message-form", "style")

		// Append user message bubble
		userHTML := `<div class="message message-user"><div class="message-bubble">` +
			template.HTMLEscapeString(userMsg.Content) + `</div></div>`
		ctx.HTML("#conversation", userHTML, gotk.Append)

		// Launch async Claude call
		bgCtx, cancel := context.WithCancel(context.Background())
		s.setRepoStatusProcessing(id, cancel)
		go s.backgroundSendMessage(bgCtx, id)

		// Show processing indicator
		entry := s.getRepoStatus(id)
		processingHTML := fmt.Sprintf(
			`<div id="repo-status" class="repo-status" data-started-at="%d">`+
				`<div class="processing-indicator"><div class="spinner"></div>`+
				`<span class="processing-text">Thinking...</span>`+
				`<span class="elapsed-timer"></span></div>`+
				`<button gotk-click="cancel-message" gotk-val-prompt_request_id="%d" `+
				`class="btn btn-sm btn-secondary">Cancel</button></div>`,
			entry.StartedAt.Unix(), id)

		ctx.Remove("#repo-status")
		ctx.HTML("#conversation", processingHTML, gotk.Append)

		ctx.Exec("scrollConversation")
		ctx.Exec("updateElapsedTimers")

		// Disable input while processing
		ctx.AttrSet("#message-input", "disabled", "true")
		ctx.AttrSet("#send-btn", "disabled", "true")

		return nil
	})

	s.gotkMux.Handle("publish", func(ctx *gotk.Context) error {
		idStr := ctx.Payload.String("prompt_request_id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			ctx.Error("#conversation", "Invalid prompt request ID")
			return nil
		}

		pr, err := s.queries.GetPromptRequest(id)
		if err != nil {
			ctx.Error("#conversation", "Prompt request not found")
			return nil
		}

		gc, err := s.queries.GetLatestGeneratedContent(id)
		if err != nil {
			ctx.Error("#conversation", "No generated prompt found. Continue the conversation until the AI generates a prompt.")
			return nil
		}

		// Compose issue body
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
			s.queries.UpdatePromptRequestTitle(id, title)
		} else if title == "" {
			title = "Prompt Request"
		}

		issueTitle := "Prompt Request: " + title

		bgCtx := context.Background()
		if pr.IssueNumber != nil {
			if err := github.EditIssue(bgCtx, pr.RepoURL, *pr.IssueNumber, body); err != nil {
				log.Printf("editing issue: %v", err)
				ctx.Error("#conversation", fmt.Sprintf("Failed to update GitHub issue: %v", err))
				return nil
			}
		} else {
			var labels []string
			if err := github.EnsureLabel(bgCtx, pr.RepoURL, github.LabelName); err != nil {
				log.Printf("warning: ensuring label %q: %v", github.LabelName, err)
			} else {
				labels = []string{github.LabelName}
			}

			issue, err := github.CreateIssue(bgCtx, pr.RepoURL, issueTitle, body, labels)
			if err != nil {
				log.Printf("creating issue: %v", err)
				ctx.Error("#conversation", fmt.Sprintf("Failed to create GitHub issue: %v", err))
				return nil
			}
			if err := s.queries.UpdatePromptRequestIssue(id, issue.Number, issue.URL); err != nil {
				log.Printf("updating issue info: %v", err)
			}
		}

		// Create revision
		var afterMsgID *int64
		if lastMsg, err := s.queries.GetLastMessage(id); err == nil {
			afterMsgID = &lastMsg.ID
		}
		rev, err := s.queries.CreateRevision(id, body, afterMsgID)
		if err != nil {
			log.Printf("creating revision: %v", err)
		}

		if err := s.queries.UpdatePromptRequestStatus(id, "published"); err != nil {
			log.Printf("updating status: %v", err)
		}

		// Re-fetch PR to get updated issue URL
		pr, _ = s.queries.GetPromptRequest(id)

		// --- Push UI updates ---

		// Remove publish form
		ctx.Remove("#publish-form")

		// Update badge to "published"
		ctx.HTML("#status-badge", "published")
		ctx.AttrSet("#status-badge", "class", "badge badge-published")

		// Add "View Issue" link in header
		if pr.IssueURL != nil {
			issueLink := fmt.Sprintf(`<a href="%s" target="_blank" class="btn btn-sm btn-secondary">View Issue</a>`,
				template.HTMLEscapeString(*pr.IssueURL))
			ctx.HTML("#header-actions-extra", issueLink)
		}

		// Update revision sidebar content
		var sidebarHTML strings.Builder
		sidebarHTML.WriteString(`<h3 class="sidebar-heading">Revisions</h3>`)
		revisions, _ := s.queries.ListRevisions(id)
		if len(revisions) > 0 {
			sidebarHTML.WriteString(`<ul class="revision-list">`)
			for _, r := range revisions {
				sidebarHTML.WriteString(fmt.Sprintf(
					`<li class="revision-list-item"><a href="#revision-%d" class="revision-link">`+
						`<span class="revision-number">Revision %d</span>`+
						`<time class="revision-time text-sm text-secondary">%s</time>`+
						`</a></li>`,
					r.ID, r.ID, r.PublishedAt.Format("Jan 2, 2006 3:04 PM")))
			}
			sidebarHTML.WriteString(`</ul>`)
			if pr.IssueURL != nil {
				sidebarHTML.WriteString(fmt.Sprintf(
					`<a href="%s" target="_blank" class="sidebar-issue-link">View GitHub Issue</a>`,
					template.HTMLEscapeString(*pr.IssueURL)))
			}
		}
		// Include archive button
		sidebarHTML.WriteString(`<div class="sidebar-archive-action">`)
		if pr.Archived {
			sidebarHTML.WriteString(fmt.Sprintf(
				`<button gotk-click="unarchive" gotk-val-prompt_request_id="%d" `+
					`gotk-loading="Unarchiving..." class="btn btn-sm btn-secondary btn-block">Unarchive</button>`,
				id))
		} else {
			sidebarHTML.WriteString(fmt.Sprintf(
				`<button gotk-click="archive" gotk-val-prompt_request_id="%d" `+
					`gotk-loading="Archiving..." class="btn btn-sm btn-secondary btn-block">Archive</button>`,
				id))
		}
		sidebarHTML.WriteString(`</div>`)

		ctx.HTML(".revision-sidebar", sidebarHTML.String())

		// Append revision marker to conversation
		if rev != nil {
			markerHTML := fmt.Sprintf(
				`<div class="submission-marker" id="revision-%d">`+
					`<details class="submission-marker-details">`+
					`<summary class="submission-marker-text">`+
					`Published to GitHub — Revision %d `+
					`<time>%s</time>`+
					`</summary>`+
					`<div class="revision-content">%s</div>`+
					`</details></div>`,
				rev.ID, rev.ID,
				rev.PublishedAt.Format("Jan 2, 2006 3:04 PM"),
				template.HTMLEscapeString(rev.Content))
			ctx.HTML("#conversation", markerHTML, gotk.Append)
		}

		ctx.Exec("renderMarkdown")
		ctx.Exec("scrollConversation")

		// Update prompt sidebar (status changed to published)
		go s.pushSidebarUpdate()

		return nil
	})

	s.gotkMux.Handle("retry", func(ctx *gotk.Context) error {
		idStr := ctx.Payload.String("prompt_request_id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return nil
		}

		pr, err := s.queries.GetPromptRequest(id)
		if err != nil {
			return nil
		}

		// Re-launch clone/pull
		cloned, _ := repo.IsCloned(pr.RepoURL)
		if cloned {
			s.setRepoStatus(id, "pulling", "")
		} else {
			s.setRepoStatus(id, "cloning", "")
		}

		entry := s.getRepoStatus(id)
		status := entry.Status

		// Show status immediately
		ctx.Remove("#repo-status")
		if status == "cloning" {
			ctx.HTML("#conversation", `<div id="repo-status" class="repo-status"><div class="spinner"></div> Cloning repository...</div>`, gotk.Append)
		} else {
			ctx.HTML("#conversation", `<div id="repo-status" class="repo-status"><div class="spinner"></div> Pulling latest changes...</div>`, gotk.Append)
		}
		ctx.Exec("scrollConversation")

		go s.asyncEnsureCloned(id, pr.RepoURL)

		return nil
	})

	s.gotkMux.Handle("resend", func(ctx *gotk.Context) error {
		idStr := ctx.Payload.String("prompt_request_id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return nil
		}

		// Delete the synthetic cancelled assistant message
		lastMsg, err := s.queries.GetLastMessage(id)
		if err == nil && lastMsg.Role == "assistant" && lastMsg.Content == "Request cancelled by user." {
			s.queries.DeleteMessage(lastMsg.ID)
		}

		// Launch async Claude call
		bgCtx, cancel := context.WithCancel(context.Background())
		s.setRepoStatusProcessing(id, cancel)
		go s.backgroundSendMessage(bgCtx, id)

		// Show processing indicator
		entry := s.getRepoStatus(id)
		processingHTML := fmt.Sprintf(
			`<div id="repo-status" class="repo-status" data-started-at="%d">`+
				`<div class="processing-indicator"><div class="spinner"></div>`+
				`<span class="processing-text">Thinking...</span>`+
				`<span class="elapsed-timer"></span></div>`+
				`<button gotk-click="cancel-message" gotk-val-prompt_request_id="%d" `+
				`class="btn btn-sm btn-secondary">Cancel</button></div>`,
			entry.StartedAt.Unix(), id)

		ctx.Remove("#repo-status")
		ctx.HTML("#conversation", processingHTML, gotk.Append)
		ctx.Exec("scrollConversation")
		ctx.Exec("updateElapsedTimers")

		// Disable input while processing
		ctx.AttrSet("#message-input", "disabled", "true")
		ctx.AttrSet("#send-btn", "disabled", "true")

		return nil
	})

	s.gotkMux.Handle("archive", func(ctx *gotk.Context) error {
		idStr := ctx.Payload.String("prompt_request_id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return nil
		}

		if err := s.queries.ArchivePromptRequest(id); err != nil {
			log.Printf("archiving prompt request: %v", err)
			ctx.Error("#conversation", "Failed to archive")
			return nil
		}

		// Conversation page: show archive banner and update sidebar button
		ctx.HTML("#archive-banner",
			fmt.Sprintf(`<div class="archive-banner" id="archive-banner">`+
				`<span>This prompt request is archived.</span>`+
				`<button gotk-click="unarchive" gotk-val-prompt_request_id="%d" `+
				`gotk-loading="Unarchiving..." class="btn btn-sm btn-secondary">Unarchive</button></div>`, id))
		ctx.HTML(".sidebar-archive-action",
			fmt.Sprintf(`<button gotk-click="unarchive" gotk-val-prompt_request_id="%d" `+
				`gotk-loading="Unarchiving..." class="btn btn-sm btn-secondary btn-block">Unarchive</button>`, id))

		// Repo list page: remove the card
		ctx.Remove(fmt.Sprintf("#pr-card-%d", id))

		// Update prompt sidebar
		go s.pushSidebarUpdate()

		return nil
	})

	s.gotkMux.Handle("unarchive", func(ctx *gotk.Context) error {
		idStr := ctx.Payload.String("prompt_request_id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return nil
		}

		if err := s.queries.UnarchivePromptRequest(id); err != nil {
			log.Printf("unarchiving prompt request: %v", err)
			ctx.Error("#conversation", "Failed to unarchive")
			return nil
		}

		// Conversation page: remove archive banner and update sidebar button
		ctx.HTML("#archive-banner", `<div id="archive-banner"></div>`)
		ctx.HTML(".sidebar-archive-action",
			fmt.Sprintf(`<button gotk-click="archive" gotk-val-prompt_request_id="%d" `+
				`gotk-loading="Archiving..." class="btn btn-sm btn-secondary btn-block">Archive</button>`, id))

		// Repo list page: remove the card
		ctx.Remove(fmt.Sprintf("#pr-card-%d", id))

		// Update prompt sidebar
		go s.pushSidebarUpdate()

		return nil
	})
}
