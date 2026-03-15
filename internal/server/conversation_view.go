package server

import (
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"strings"

	"github.com/esnunes/prompter/gotk"
)

// ConversationView handles all DOM updates for the conversation page.
// It registers event handlers on the connection's dispatcher and produces
// instructions via the ViewContext.
type ConversationView struct {
	ctx    *gotk.ViewContext
	server *Server
}

// NewConversationView creates a ConversationView and registers event handlers.
func NewConversationView(d *gotk.Dispatcher, vctx *gotk.ViewContext, s *Server) *ConversationView {
	v := &ConversationView{ctx: vctx, server: s}
	gotk.Register(d, v.OnMessageSent)
	gotk.Register(d, v.OnProcessingStarted)
	gotk.Register(d, v.OnResponseReceived)
	gotk.Register(d, v.OnProcessingError)
	gotk.Register(d, v.OnProcessingCancelled)
	gotk.Register(d, v.OnStatusChanged)
	gotk.Register(d, v.OnSidebarUpdated)
	gotk.Register(d, v.OnPublished)
	gotk.Register(d, v.OnArchived)
	gotk.Register(d, v.OnUnarchived)
	gotk.Register(d, v.OnQuestionAnswered)
	gotk.Register(d, v.OnError)
	return v
}

// OnMessageSent appends the user message bubble, clears the textarea, and disables input.
func (v *ConversationView) OnMessageSent(e MessageSentEvent) {
	v.ctx.SetValue("#message-input", "")
	v.disableForm()
	v.ctx.HTML("#conversation", v.server.render("message", struct{ Role, Content string }{"user", e.MessageContent}), gotk.Append)
}

// OnProcessingStarted shows the processing indicator.
func (v *ConversationView) OnProcessingStarted(e ProcessingStartedEvent) {
	v.ctx.Remove("#repo-status")
	v.ctx.HTML("#conversation", v.server.render("processing-indicator", struct {
		StartedAt       int64
		PromptRequestID int64
	}{e.StartedAt.Unix(), e.PromptRequestID}), gotk.Append)
	v.ctx.Exec("scrollConversation")
	v.ctx.Exec("updateElapsedTimers")
	v.disableForm()
}

// OnResponseReceived removes the spinner, appends the assistant message,
// shows questions/prompt-ready if present, and re-enables input.
func (v *ConversationView) OnResponseReceived(e ResponseReceivedEvent) {
	// Remove spinner
	v.ctx.Remove("#repo-status")

	// Append assistant message
	v.ctx.HTML("#conversation", v.server.render("message", struct{ Role, Content string }{"assistant", e.Message}), gotk.Append)

	// Handle questions / prompt-ready from raw response
	hasQuestions := false
	if e.RawJSON != nil {
		questions, promptReady := extractQuestionsFromRaw(*e.RawJSON)
		org, repoName := v.server.orgRepoForPR(e.PromptRequestID)
		if len(questions) > 0 && org != "" {
			v.appendQuestionForm(e.PromptRequestID, org, repoName, questions)
			hasQuestions = true
		}
		if promptReady && org != "" {
			v.ctx.HTML("#conversation", v.server.render("prompt-ready", struct{ PromptRequestID int64 }{e.PromptRequestID}), gotk.Append)
		}
	}

	// Re-enable input (but hide message form if questions are shown)
	v.enableForm()
	if hasQuestions {
		v.ctx.AttrSet("#message-form", "style", "display:none")
	}

	v.ctx.Exec("renderMarkdown")
	v.ctx.Exec("scrollConversation")
}

// OnProcessingError shows the error in the conversation.
func (v *ConversationView) OnProcessingError(e ProcessingErrorEvent) {
	v.ctx.Remove("#repo-status")
	v.ctx.HTML("#conversation", v.server.render("status-error", struct {
		PromptRequestID int64
		Error           string
	}{e.PromptRequestID, e.Message}), gotk.Append)
	v.enableForm()
	v.ctx.Exec("scrollConversation")
}

// OnProcessingCancelled shows the cancellation message.
func (v *ConversationView) OnProcessingCancelled(e ProcessingCancelledEvent) {
	v.ctx.Remove("#repo-status")
	v.ctx.HTML("#conversation", v.server.render("status-cancelled", struct{ PromptRequestID int64 }{e.PromptRequestID}), gotk.Append)
	v.enableForm()
	v.ctx.Exec("scrollConversation")
}

// OnStatusChanged updates the repo status indicator.
func (v *ConversationView) OnStatusChanged(e StatusChangedEvent) {
	slog.Debug("OnStatusChanged", "event", e)
	var statusHTML string

	switch e.Status {
	case "cloning":
		statusHTML = v.server.render("status-cloning", nil)
	case "pulling":
		statusHTML = v.server.render("status-pulling", nil)
	case "processing":
		statusHTML = v.server.render("processing-indicator", struct {
			StartedAt       int64
			PromptRequestID int64
		}{e.Entry.StartedAt.Unix(), e.PromptRequestID})
	case "cancelled":
		statusHTML = v.server.render("status-cancelled", struct{ PromptRequestID int64 }{e.PromptRequestID})
		v.enableForm()
	case "error":
		statusHTML = v.server.render("status-error", struct {
			PromptRequestID int64
			Error           string
		}{e.PromptRequestID, e.Entry.Error})
	case "ready":
		statusHTML = v.server.render("status-ready", nil)
		v.enableForm()
		v.ctx.Focus("#message-input")
	default:
		return
	}

	v.ctx.Remove("#repo-status")
	v.ctx.HTML("#conversation", statusHTML, gotk.Append)
	v.ctx.Exec("scrollConversation")
	v.ctx.Exec("updateElapsedTimers")
}

// OnSidebarUpdated refreshes the sidebar content.
func (v *ConversationView) OnSidebarUpdated(e SidebarUpdatedEvent) {
	prs, err := v.server.queries.ListPromptRequests(false)
	if err != nil {
		log.Printf("sidebar push: query error: %v", err)
		return
	}

	sidebar := v.server.buildSidebar(prs, "repo", 0)

	tmpl, ok := v.server.pages["sidebar.html"]
	if !ok {
		log.Printf("sidebar push: template not found")
		return
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "sidebar.html", sidebar); err != nil {
		log.Printf("sidebar push: render error: %v", err)
		return
	}

	v.ctx.HTML("#prompt-sidebar", buf.String())
	v.ctx.Exec("highlightCurrentSidebar")
}

// OnPublished updates the UI after publishing to GitHub.
func (v *ConversationView) OnPublished(e PublishedEvent) {
	pr, _ := v.server.queries.GetPromptRequest(e.PromptRequestID)
	revisions, _ := v.server.queries.ListRevisions(e.PromptRequestID)

	// Remove publish form
	v.ctx.Remove("#publish-form")

	// Update badge to "published"
	v.ctx.HTML("#status-badge", "published")
	v.ctx.AttrSet("#status-badge", "class", "badge badge-published")

	// Add "View Issue" link in header
	if pr != nil && pr.IssueURL != nil {
		issueLink := fmt.Sprintf(`<a href="%s" target="_blank" class="btn btn-sm btn-secondary">View Issue</a>`,
			template.HTMLEscapeString(*pr.IssueURL))
		v.ctx.HTML("#header-actions-extra", issueLink)
	}

	// Update revision sidebar content
	var sidebarHTML strings.Builder
	sidebarHTML.WriteString(`<h3 class="sidebar-heading">Revisions</h3>`)
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
		if pr != nil && pr.IssueURL != nil {
			sidebarHTML.WriteString(fmt.Sprintf(
				`<a href="%s" target="_blank" class="sidebar-issue-link">View GitHub Issue</a>`,
				template.HTMLEscapeString(*pr.IssueURL)))
		}
	}

	// Include archive button
	sidebarHTML.WriteString(`<div class="sidebar-archive-action">`)
	if pr != nil && pr.Archived {
		sidebarHTML.WriteString(fmt.Sprintf(
			`<button gotk-click="unarchive" gotk-val-prompt_request_id="%d" `+
				`gotk-loading="Unarchiving..." class="btn btn-sm btn-secondary btn-block">Unarchive</button>`,
			e.PromptRequestID))
	} else {
		sidebarHTML.WriteString(fmt.Sprintf(
			`<button gotk-click="archive" gotk-val-prompt_request_id="%d" `+
				`gotk-loading="Archiving..." class="btn btn-sm btn-secondary btn-block">Archive</button>`,
			e.PromptRequestID))
	}
	sidebarHTML.WriteString(`</div>`)

	v.ctx.HTML(".revision-sidebar", sidebarHTML.String())

	// Append revision marker to conversation
	if len(revisions) > 0 {
		rev := revisions[len(revisions)-1]
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
		v.ctx.HTML("#conversation", markerHTML, gotk.Append)
	}

	v.ctx.Exec("renderMarkdown")
	v.ctx.Exec("scrollConversation")
}

// OnArchived shows the archive banner and updates sidebar.
func (v *ConversationView) OnArchived(e ArchivedEvent) {
	v.ctx.HTML("#archive-banner",
		fmt.Sprintf(`<div class="archive-banner" id="archive-banner">`+
			`<span>This prompt request is archived.</span>`+
			`<button gotk-click="unarchive" gotk-val-prompt_request_id="%d" `+
			`gotk-loading="Unarchiving..." class="btn btn-sm btn-secondary">Unarchive</button></div>`, e.PromptRequestID))
	v.ctx.HTML(".sidebar-archive-action",
		fmt.Sprintf(`<button gotk-click="unarchive" gotk-val-prompt_request_id="%d" `+
			`gotk-loading="Unarchiving..." class="btn btn-sm btn-secondary btn-block">Unarchive</button>`, e.PromptRequestID))
	v.ctx.Remove(fmt.Sprintf("#pr-card-%d", e.PromptRequestID))
}

// OnUnarchived removes the archive banner and updates sidebar.
func (v *ConversationView) OnUnarchived(e UnarchivedEvent) {
	v.ctx.HTML("#archive-banner", `<div id="archive-banner"></div>`)
	v.ctx.HTML(".sidebar-archive-action",
		fmt.Sprintf(`<button gotk-click="archive" gotk-val-prompt_request_id="%d" `+
			`gotk-loading="Archiving..." class="btn btn-sm btn-secondary btn-block">Archive</button>`, e.PromptRequestID))
	v.ctx.Remove(fmt.Sprintf("#pr-card-%d", e.PromptRequestID))
}

// OnQuestionAnswered removes the question form and shows the message form.
func (v *ConversationView) OnQuestionAnswered(e QuestionAnsweredEvent) {
	v.ctx.Remove("#question-form")
	v.ctx.AttrRemove("#message-form", "style")
	v.ctx.HTML("#conversation", v.server.render("message", struct{ Role, Content string }{"user", e.MessageContent}), gotk.Append)
}

// disableForm disables the message input and send button.
func (v *ConversationView) disableForm() {
	v.ctx.AttrSet("#message-input", "disabled", "true")
	v.ctx.AttrSet("#send-btn", "disabled", "true")
}

// enableForm re-enables the message input and send button.
func (v *ConversationView) enableForm() {
	v.ctx.AttrRemove("#message-input", "disabled")
	v.ctx.AttrRemove("#send-btn", "disabled")
}

// OnError displays an error message in the UI.
func (v *ConversationView) OnError(e ErrorEvent) {
	v.ctx.Error(e.Target, e.Message)
}

// appendQuestionForm builds and appends the question form HTML.
func (v *ConversationView) appendQuestionForm(prID int64, org, repoName string, questions []questionData) {
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

	v.ctx.HTML("#conversation", html.String(), gotk.Append)
}
