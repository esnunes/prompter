package server

import "time"

// Events emitted by command handlers and background tasks.
// Views register handlers for these events and produce DOM instructions.

// MessageSentEvent is emitted when a user sends a message.
type MessageSentEvent struct {
	PromptRequestID int64
	MessageContent  string
}

// ProcessingStartedEvent is emitted when Claude processing begins.
type ProcessingStartedEvent struct {
	PromptRequestID int64
	StartedAt       time.Time
}

// ResponseReceivedEvent is emitted when Claude returns a response.
type ResponseReceivedEvent struct {
	PromptRequestID int64
	Message         string
	RawJSON         *string
}

// ProcessingErrorEvent is emitted when Claude processing fails.
type ProcessingErrorEvent struct {
	PromptRequestID int64
	Message         string
}

// ProcessingCancelledEvent is emitted when Claude processing is cancelled.
type ProcessingCancelledEvent struct {
	PromptRequestID int64
}

// StatusChangedEvent is emitted when the repo/processing status changes.
type StatusChangedEvent struct {
	PromptRequestID int64
	Status          string
	Entry           repoStatusEntry
}

// SidebarUpdatedEvent is emitted when the sidebar needs to be refreshed.
type SidebarUpdatedEvent struct{}

// PublishedEvent is emitted after publishing to GitHub.
type PublishedEvent struct {
	PromptRequestID int64
}

// ArchivedEvent is emitted when a prompt request is archived.
type ArchivedEvent struct {
	PromptRequestID int64
}

// UnarchivedEvent is emitted when a prompt request is unarchived.
type UnarchivedEvent struct {
	PromptRequestID int64
}

// QuestionAnsweredEvent is emitted when a user answers Claude's questions.
type QuestionAnsweredEvent struct {
	PromptRequestID int64
	MessageContent  string
}

// ErrorEvent is emitted to display an error message in the UI.
type ErrorEvent struct {
	Target  string
	Message string
}
