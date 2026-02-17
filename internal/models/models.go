package models

import "time"

type Repository struct {
	ID        int64
	URL       string
	LocalPath string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type PromptRequest struct {
	ID           int64
	RepositoryID int64
	Title        string
	Status       string // "draft", "published", "deleted"
	SessionID    string
	IssueNumber  *int
	IssueURL     *string
	CreatedAt    time.Time
	UpdatedAt    time.Time

	// Joined fields (not stored directly)
	RepoURL        string
	RepoLocalPath  string
	MessageCount   int
	RevisionCount  int
	LatestRevision *time.Time
}

type Message struct {
	ID              int64
	PromptRequestID int64
	Role            string // "user", "assistant"
	Content         string
	RawResponse     *string
	CreatedAt       time.Time
}

type Revision struct {
	ID              int64
	PromptRequestID int64
	Content         string
	PublishedAt     time.Time
}
