package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/esnunes/prompter/internal/claude"
	"github.com/esnunes/prompter/internal/github"
	"github.com/esnunes/prompter/internal/models"
	"github.com/esnunes/prompter/internal/repo"

	"github.com/google/uuid"
)

type dashboardData struct {
	PromptRequests []models.PromptRequest
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	prs, err := s.queries.ListPromptRequests()
	if err != nil {
		log.Printf("listing prompt requests: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, "dashboard.html", dashboardData{PromptRequests: prs})
}

type repoData struct {
	RepoURL        string
	Org            string
	Repo           string
	Error          string
	PromptRequests []models.PromptRequest
}

func (s *Server) handleRepoPage(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")
	repoURL := fmt.Sprintf("github.com/%s/%s", org, repoName)

	if err := repo.ValidateURL(repoURL); err != nil {
		s.renderPage(w, "repo.html", repoData{
			RepoURL: repoURL,
			Org:     org,
			Repo:    repoName,
			Error:   "Invalid repository URL format.",
		})
		return
	}

	// Verify repo exists on GitHub
	if err := github.VerifyRepo(r.Context(), org, repoName); err != nil {
		s.renderPage(w, "repo.html", repoData{
			RepoURL: repoURL,
			Org:     org,
			Repo:    repoName,
			Error:   fmt.Sprintf("This repository doesn't exist on GitHub or is not accessible."),
		})
		return
	}

	prs, err := s.queries.ListPromptRequestsByRepoURL(repoURL)
	if err != nil {
		log.Printf("listing prompt requests for repo: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	s.renderPage(w, "repo.html", repoData{
		RepoURL:        repoURL,
		Org:            org,
		Repo:           repoName,
		PromptRequests: prs,
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

	http.Redirect(w, r, fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d", org, repoName, pr.ID), http.StatusSeeOther)
}

type conversationData struct {
	PromptRequest *models.PromptRequest
	Org           string
	Repo          string
	Timeline      []timelineItem
	LastQuestions  []questionData
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

	data := conversationData{
		PromptRequest: pr,
		Org:           org,
		Repo:          repoName,
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

	userMessage := r.FormValue("message")
	// If no direct message, try assembling from multi-question form fields
	if userMessage == "" {
		userMessage = assembleQuestionAnswers(r)
	}
	if userMessage == "" {
		http.Error(w, "Message is required", http.StatusBadRequest)
		return
	}

	pr, err := s.queries.GetPromptRequest(id)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Serialize Claude CLI calls per session to avoid "session already in use"
	mu := s.lockSession(pr.SessionID)
	defer mu.Unlock()

	// Check if this session already has messages (resume vs. new)
	existingMsgs, err := s.queries.ListMessages(id)
	if err != nil {
		log.Printf("checking existing messages: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	resume := len(existingMsgs) > 0

	// Save user message
	userMsg, err := s.queries.CreateMessage(id, "user", userMessage, nil)
	if err != nil {
		log.Printf("saving user message: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Call Claude CLI
	resp, rawJSON, err := claude.SendMessage(r.Context(), pr.SessionID, pr.RepoLocalPath, userMessage, resume)
	if err != nil {
		log.Printf("claude error: %v", err)
		// Save error as assistant message so user sees it
		errMsg := fmt.Sprintf("Sorry, I encountered an error: %v", err)
		s.queries.CreateMessage(id, "assistant", errMsg, nil)

		fragment := messageFragmentData{
			PromptRequestID: id,
			Org:             org,
			Repo:            repoName,
			Messages: []models.Message{
				*userMsg,
				{Role: "assistant", Content: errMsg},
			},
		}
		s.renderFragment(w, "message_fragment.html", fragment)
		return
	}

	// Save assistant message
	assistantMsg, err := s.queries.CreateMessage(id, "assistant", resp.Message, &rawJSON)
	if err != nil {
		log.Printf("saving assistant message: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Set title from generated_title when prompt is ready, or fall back to message truncation
	if pr.Title == "" {
		if resp.GeneratedTitle != "" {
			s.queries.UpdatePromptRequestTitle(id, resp.GeneratedTitle)
		} else if resp.Message != "" {
			title := resp.Message
			if len(title) > 60 {
				title = title[:60] + "..."
			}
			s.queries.UpdatePromptRequestTitle(id, title)
		}
	} else if resp.GeneratedTitle != "" {
		// Update title with the generated one even if a rough one was set earlier
		s.queries.UpdatePromptRequestTitle(id, resp.GeneratedTitle)
	}

	// Build fragment response
	fragment := messageFragmentData{
		PromptRequestID: id,
		Org:             org,
		Repo:            repoName,
		Messages:        []models.Message{*userMsg, *assistantMsg},
	}

	for i, q := range resp.Questions {
		qd := questionData{
			Header:      q.Header,
			Text:        q.Text,
			MultiSelect: q.MultiSelect,
			Index:       i,
		}
		for _, opt := range q.Options {
			qd.Options = append(qd.Options, optionData{
				Label:       opt.Label,
				Description: opt.Description,
			})
		}
		fragment.Questions = append(fragment.Questions, qd)
	}

	fragment.PromptReady = resp.PromptReady

	s.renderFragment(w, "message_fragment.html", fragment)
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
		// Create new issue
		issue, err := github.CreateIssue(r.Context(), pr.RepoURL, issueTitle, body)
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

	// Redirect to the published view
	http.Redirect(w, r, fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d", org, repoName, id), http.StatusSeeOther)
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
