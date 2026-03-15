package server

import (
	"context"
	"encoding/json"
	"fmt"
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

type conversationData struct {
	basePageData
	PromptRequest *models.PromptRequest
	Org           string
	Repo          string
	RepoStatus    string // "cloning", "pulling", "ready", "processing", "cancelled", "error", or "" (no active operation)
	RepoStartedAt int64  // Unix timestamp for processing timer
	Timeline      []timelineItem
	LastQuestions []questionData
	PromptReady   bool
	Revisions     []models.Revision
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
		basePageData:  basePageData{Sidebar: sidebar},
		PromptRequest: pr,
		Org:           org,
		Repo:          repoName,
		RepoStatus:    repoStatus,
		RepoStartedAt: repoStartedAt,
		Timeline:      buildTimeline(messages, revisions),
		Revisions:     revisions,
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

// asyncEnsureCloned runs clone/pull in the background, updating status in sync.Map.
// On completion, pushes status to connected clients and auto-sends pending user messages.
func (s *Server) asyncEnsureCloned(prID int64, repoURL string, tctx *gotk.TaskContext) {
	// Serialize clone/pull operations per repo to prevent concurrent git corruption
	mu := s.lockRepo(repoURL)
	defer mu.Unlock()

	_, err := repo.EnsureCloned(context.Background(), repoURL)
	if err != nil {
		log.Printf("async clone/pull failed for %s: %v", repoURL, err)
		s.setRepoStatus(prID, "error", err.Error())
		tctx.Broadcast(StatusChangedEvent{
			PromptRequestID: prID,
			Status:          "error",
			Entry:           s.getRepoStatus(prID),
		})
		tctx.Broadcast(SidebarUpdatedEvent{})
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
			tctx.Broadcast(StatusChangedEvent{
				PromptRequestID: prID,
				Status:          "processing",
				Entry:           s.getRepoStatus(prID),
			})
			go s.backgroundSendMessage(ctx, prID, tctx)
			return
		}
	}

	// No auto-send needed — push "ready" status and update sidebar
	tctx.Broadcast(StatusChangedEvent{
		PromptRequestID: prID,
		Status:          "ready",
		Entry:           s.getRepoStatus(prID),
	})
	tctx.Broadcast(SidebarUpdatedEvent{})
}

// backgroundSendMessage processes a pending user message with Claude in a background goroutine.
// It saves the response to DB and updates the repo status to "responded" or "cancelled".
func (s *Server) backgroundSendMessage(ctx context.Context, prID int64, tctx *gotk.TaskContext) {
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
			tctx.Broadcast(ResponseReceivedEvent{
				PromptRequestID: prID,
				Message:         "Request cancelled by user.",
			})
			return
		}
		log.Printf("auto-send: claude error: %v", err)
		errMsg := fmt.Sprintf("Sorry, I encountered an error: %v", err)
		s.queries.CreateMessage(prID, "assistant", errMsg, nil)
		s.setRepoStatus(prID, "responded", "")
		tctx.Broadcast(ResponseReceivedEvent{
			PromptRequestID: prID,
			Message:         errMsg,
		})
		return
	}

	if _, err := s.queries.CreateMessage(prID, "assistant", resp.Message, &rawJSON); err != nil {
		log.Printf("auto-send: saving assistant message: %v", err)
		s.setRepoStatus(prID, "error", "Failed to save response")
		tctx.Broadcast(ResponseReceivedEvent{
			PromptRequestID: prID,
			Message:         "Failed to save response",
		})
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
	tctx.Broadcast(ResponseReceivedEvent{
		PromptRequestID: prID,
		Message:         resp.Message,
		RawJSON:         &rawJSON,
	})
	tctx.Broadcast(SidebarUpdatedEvent{})
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

// assembleQuestionAnswersFromPayload assembles question answers from a gotk payload.
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

	return sidebarData{
		Items:     items,
		Scope:     scope,
		CurrentID: currentID,
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


// registerGotkCommands registers gotk command handlers on the mux.
func (s *Server) registerGotkCommands() {
	s.gotkMux.HandleCommand("create-prompt-request", s.CreatePromptRequest)
	s.gotkMux.HandleCommand("send-message", s.SendMessage)

	s.gotkMux.HandleCommand("cancel-message", func(ctx *gotk.CommandContext) error {
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

	s.gotkMux.HandleCommand("answer-question", func(ctx *gotk.CommandContext) error {
		idStr := ctx.Payload.String("prompt_request_id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			gotk.Dispatch(ctx.Dispatcher(), ErrorEvent{Target: "#conversation", Message: "Invalid prompt request ID"})
			return nil
		}

		message := assembleQuestionAnswersFromPayload(ctx.Payload)
		if message == "" {
			return nil
		}

		// Save user message
		_, err = s.queries.CreateMessage(id, "user", message, nil)
		if err != nil {
			gotk.Dispatch(ctx.Dispatcher(), ErrorEvent{Target: "#conversation", Message: "Failed to save message"})
			return nil
		}

		// Dispatch question answered event (view removes form, shows message)
		gotk.Dispatch(ctx.Dispatcher(), QuestionAnsweredEvent{
			PromptRequestID: id,
			MessageContent:  message,
		})

		// Launch async Claude call
		bgCtx, cancel := context.WithCancel(context.Background())
		s.setRepoStatusProcessing(id, cancel)
		tctx := ctx.NewTask()
		go s.backgroundSendMessage(bgCtx, id, tctx)

		// Show processing indicator
		gotk.Dispatch(ctx.Dispatcher(), ProcessingStartedEvent{
			PromptRequestID: id,
			StartedAt:       s.getRepoStatus(id).StartedAt,
		})

		return nil
	})

	s.gotkMux.HandleCommand("publish", func(ctx *gotk.CommandContext) error {
		idStr := ctx.Payload.String("prompt_request_id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			gotk.Dispatch(ctx.Dispatcher(), ErrorEvent{Target: "#conversation", Message: "Invalid prompt request ID"})
			return nil
		}

		pr, err := s.queries.GetPromptRequest(id)
		if err != nil {
			gotk.Dispatch(ctx.Dispatcher(), ErrorEvent{Target: "#conversation", Message: "Prompt request not found"})
			return nil
		}

		gc, err := s.queries.GetLatestGeneratedContent(id)
		if err != nil {
			gotk.Dispatch(ctx.Dispatcher(), ErrorEvent{Target: "#conversation", Message: "No generated prompt found. Continue the conversation until the AI generates a prompt."})
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
				gotk.Dispatch(ctx.Dispatcher(), ErrorEvent{Target: "#conversation", Message: fmt.Sprintf("Failed to update GitHub issue: %v", err)})
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
				gotk.Dispatch(ctx.Dispatcher(), ErrorEvent{Target: "#conversation", Message: fmt.Sprintf("Failed to create GitHub issue: %v", err)})
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
		if _, err := s.queries.CreateRevision(id, body, afterMsgID); err != nil {
			log.Printf("creating revision: %v", err)
		}

		if err := s.queries.UpdatePromptRequestStatus(id, "published"); err != nil {
			log.Printf("updating status: %v", err)
		}

		// Dispatch published event (view handles all UI updates)
		gotk.Dispatch(ctx.Dispatcher(), PublishedEvent{PromptRequestID: id})
		gotk.Dispatch(ctx.Dispatcher(), SidebarUpdatedEvent{})

		return nil
	})

	s.gotkMux.HandleCommand("retry", func(ctx *gotk.CommandContext) error {
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

		// Show status immediately via event
		entry := s.getRepoStatus(id)
		gotk.Dispatch(ctx.Dispatcher(), StatusChangedEvent{
			PromptRequestID: id,
			Status:          entry.Status,
			Entry:           entry,
		})

		tctx := ctx.NewTask()
		go s.asyncEnsureCloned(id, pr.RepoURL, tctx)

		return nil
	})

	s.gotkMux.HandleCommand("resend", func(ctx *gotk.CommandContext) error {
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
		tctx := ctx.NewTask()
		go s.backgroundSendMessage(bgCtx, id, tctx)

		// Show processing indicator
		gotk.Dispatch(ctx.Dispatcher(), ProcessingStartedEvent{
			PromptRequestID: id,
			StartedAt:       s.getRepoStatus(id).StartedAt,
		})

		return nil
	})

	s.gotkMux.HandleCommand("archive", func(ctx *gotk.CommandContext) error {
		idStr := ctx.Payload.String("prompt_request_id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return nil
		}

		if err := s.queries.ArchivePromptRequest(id); err != nil {
			log.Printf("archiving prompt request: %v", err)
			gotk.Dispatch(ctx.Dispatcher(), ErrorEvent{Target: "#conversation", Message: "Failed to archive"})
			return nil
		}

		gotk.Dispatch(ctx.Dispatcher(), ArchivedEvent{PromptRequestID: id})
		gotk.Dispatch(ctx.Dispatcher(), SidebarUpdatedEvent{})

		return nil
	})

	s.gotkMux.HandleCommand("unarchive", func(ctx *gotk.CommandContext) error {
		idStr := ctx.Payload.String("prompt_request_id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return nil
		}

		if err := s.queries.UnarchivePromptRequest(id); err != nil {
			log.Printf("unarchiving prompt request: %v", err)
			gotk.Dispatch(ctx.Dispatcher(), ErrorEvent{Target: "#conversation", Message: "Failed to unarchive"})
			return nil
		}

		gotk.Dispatch(ctx.Dispatcher(), UnarchivedEvent{PromptRequestID: id})
		gotk.Dispatch(ctx.Dispatcher(), SidebarUpdatedEvent{})

		return nil
	})
}
