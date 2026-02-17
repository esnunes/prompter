package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/esnunes/prompter/internal/claude"
	"github.com/esnunes/prompter/internal/github"
	"github.com/esnunes/prompter/internal/models"

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

type newData struct {
	Repositories []models.Repository
}

func (s *Server) handleNew(w http.ResponseWriter, r *http.Request) {
	repos, err := s.queries.ListRepositories()
	if err != nil {
		log.Printf("listing repositories: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, "new.html", newData{Repositories: repos})
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	repoID, err := strconv.ParseInt(r.FormValue("repo_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid repository", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	sessionID := uuid.New().String()

	pr, err := s.queries.CreatePromptRequest(repoID, sessionID)
	if err != nil {
		log.Printf("creating prompt request: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if title != "" {
		if err := s.queries.UpdatePromptRequestTitle(pr.ID, title); err != nil {
			log.Printf("updating title: %v", err)
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/prompt-requests/%d", pr.ID), http.StatusSeeOther)
}

type conversationData struct {
	PromptRequest *models.PromptRequest
	Messages      []models.Message
	LastQuestion  *questionData
	PromptReady   bool
}

type questionData struct {
	Text    string
	Options []optionData
}

type optionData struct {
	Label       string
	Description string
}

func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
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

	if pr.Status == "published" {
		revisions, err := s.queries.ListRevisions(id)
		if err != nil {
			log.Printf("listing revisions: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		s.renderPage(w, "published.html", publishedData{
			PromptRequest: pr,
			Messages:      messages,
			Revisions:     revisions,
		})
		return
	}

	// Build conversation data with last question if present
	data := conversationData{
		PromptRequest: pr,
		Messages:      messages,
	}

	// Check the last assistant message for a pending question
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		if last.Role == "assistant" && last.RawResponse != nil {
			if q, promptReady := extractQuestionFromRaw(*last.RawResponse); q != nil {
				data.LastQuestion = q
				data.PromptReady = promptReady
			} else {
				data.PromptReady = promptReady
			}
		}
	}

	s.renderPage(w, "conversation.html", data)
}

type publishedData struct {
	PromptRequest *models.PromptRequest
	Messages      []models.Message
	Revisions     []models.Revision
}

type messageFragmentData struct {
	PromptRequestID int64
	Messages        []models.Message
	Question        *questionData
	PromptReady     bool
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
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

	// Save user message
	userMsg, err := s.queries.CreateMessage(id, "user", userMessage, nil)
	if err != nil {
		log.Printf("saving user message: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Call Claude CLI
	resp, rawJSON, err := claude.SendMessage(r.Context(), pr.SessionID, pr.RepoLocalPath, userMessage)
	if err != nil {
		log.Printf("claude error: %v", err)
		// Save error as assistant message so user sees it
		errMsg := fmt.Sprintf("Sorry, I encountered an error: %v", err)
		s.queries.CreateMessage(id, "assistant", errMsg, nil)

		fragment := messageFragmentData{
			PromptRequestID: id,
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

	// If AI generated a title and PR has none, auto-set it
	if pr.Title == "" && resp.Message != "" {
		// Use first 60 chars of the message as a rough title
		title := resp.Message
		if len(title) > 60 {
			title = title[:60] + "..."
		}
		s.queries.UpdatePromptRequestTitle(id, title)
	}

	// Build fragment response
	fragment := messageFragmentData{
		PromptRequestID: id,
		Messages:        []models.Message{*userMsg, *assistantMsg},
	}

	if resp.Question != nil {
		fragment.Question = &questionData{
			Text: resp.Question.Text,
		}
		for _, opt := range resp.Question.Options {
			fragment.Question.Options = append(fragment.Question.Options, optionData{
				Label:       opt.Label,
				Description: opt.Description,
			})
		}
	}

	fragment.PromptReady = resp.PromptReady

	s.renderFragment(w, "message_fragment.html", fragment)
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
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

	// Get the generated prompt
	prompt, err := s.queries.GetLatestGeneratedPrompt(id)
	if err != nil {
		log.Printf("getting generated prompt: %v", err)
		http.Error(w, "No generated prompt found. Continue the conversation until the AI generates a prompt.", http.StatusBadRequest)
		return
	}

	title := pr.Title
	if title == "" {
		title = "Prompt Request"
	}

	if pr.IssueNumber != nil {
		// Update existing issue
		if err := github.EditIssue(r.Context(), pr.RepoURL, *pr.IssueNumber, prompt); err != nil {
			log.Printf("editing issue: %v", err)
			http.Error(w, fmt.Sprintf("Failed to update GitHub issue: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		// Create new issue
		issue, err := github.CreateIssue(r.Context(), pr.RepoURL, title, prompt)
		if err != nil {
			log.Printf("creating issue: %v", err)
			http.Error(w, fmt.Sprintf("Failed to create GitHub issue: %v", err), http.StatusInternalServerError)
			return
		}
		if err := s.queries.UpdatePromptRequestIssue(id, issue.Number, issue.URL); err != nil {
			log.Printf("updating issue info: %v", err)
		}
	}

	// Create revision
	if _, err := s.queries.CreateRevision(id, prompt); err != nil {
		log.Printf("creating revision: %v", err)
	}

	// Update status to published
	if err := s.queries.UpdatePromptRequestStatus(id, "published"); err != nil {
		log.Printf("updating status: %v", err)
	}

	// Redirect to the published view
	http.Redirect(w, r, fmt.Sprintf("/prompt-requests/%d", id), http.StatusSeeOther)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
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

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// extractQuestionFromRaw parses the raw Claude response to find a pending question
func extractQuestionFromRaw(rawJSON string) (*questionData, bool) {
	resp := parseRawResponse(rawJSON)
	if resp == nil {
		return nil, false
	}

	if resp.Question == nil {
		return nil, resp.PromptReady
	}

	q := &questionData{Text: resp.Question.Text}
	for _, opt := range resp.Question.Options {
		q.Options = append(q.Options, optionData{Label: opt.Label, Description: opt.Description})
	}
	return q, resp.PromptReady
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
