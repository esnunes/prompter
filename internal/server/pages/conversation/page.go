package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/esnunes/prompter/internal/claude"
	"github.com/esnunes/prompter/internal/db"
	"github.com/esnunes/prompter/internal/models"
	"github.com/esnunes/prompter/internal/repo"
)

// RepoStatusEntry tracks async operation state. Matches server.repoStatusEntry.
type RepoStatusEntry struct {
	Status    string
	Error     string
	StartedAt time.Time
}

type Page struct {
	Tmpl                    *template.Template
	Queries                 *db.Queries
	BuildSidebar            func(prs []models.PromptRequest, scope string, currentID int64) any
	GetRepoStatus           func(int64) RepoStatusEntry
	SetRepoStatus           func(int64, string, string)
	SetRepoStatusProcessing func(int64, context.CancelFunc)
	ClearCancelFunc         func(int64)
	RepoStatus              *sync.Map
	CancelFuncs             *sync.Map
	LockSession             func(string) *sync.Mutex
	LockRepo                func(string) *sync.Mutex
}

type conversationData struct {
	Sidebar        any
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

func (p *Page) HandlePage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
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

	// Update last_viewed_at for unread tracking
	p.Queries.UpdateLastViewedAt(id)

	messages, err := p.Queries.ListMessages(id)
	if err != nil {
		log.Printf("listing messages: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	revisions, err := p.Queries.ListRevisions(id)
	if err != nil {
		log.Printf("listing revisions: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Check repo status for polling div
	statusEntry := p.GetRepoStatus(id)
	repoStatus := statusEntry.Status
	if repoStatus == "" {
		// Server restart recovery: check filesystem
		cloned, _ := repo.IsCloned(pr.RepoURL)
		if cloned {
			repoStatus = "ready"
		}
	}
	// When status is "responded", the assistant message is already in the DB
	// and will be rendered by the template. Clear the map entry so that
	// subsequent actions (e.g., sending a new message) see "ready" state
	// and can trigger a new Claude call.
	if repoStatus == "responded" {
		p.RepoStatus.Delete(id)
		repoStatus = "ready"
	}

	var repoStartedAt int64
	if !statusEntry.StartedAt.IsZero() {
		repoStartedAt = statusEntry.StartedAt.Unix()
	}

	// Build sidebar with repo-scoped active prompt requests (never archived)
	sidebarPRs, _ := p.Queries.ListPromptRequestsByRepoURL(pr.RepoURL, false)
	sidebar := p.BuildSidebar(sidebarPRs, "repo", id)

	data := conversationData{
		Sidebar:        sidebar,
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.Tmpl.ExecuteTemplate(w, "pages/base.html", data); err != nil {
		log.Printf("render error (conversation): %v", err)
	}
}

// buildTimeline interleaves messages and revision markers into a single chronological timeline.
func buildTimeline(messages []models.Message, revisions []models.Revision) []timelineItem {
	// Map afterMessageID -> revisions for O(1) lookup
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
