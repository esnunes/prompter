package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/esnunes/prompter/internal/models"
)

type Queries struct {
	db *sql.DB
}

func NewQueries(db *sql.DB) *Queries {
	return &Queries{db: db}
}

// Repositories

func (q *Queries) ListRepositories() ([]models.Repository, error) {
	rows, err := q.db.Query(`SELECT id, url, local_path, created_at, updated_at FROM repositories ORDER BY url ASC`)
	if err != nil {
		return nil, fmt.Errorf("listing repositories: %w", err)
	}
	defer rows.Close()

	var results []models.Repository
	for rows.Next() {
		var r models.Repository
		var createdAt, updatedAt string
		if err := rows.Scan(&r.ID, &r.URL, &r.LocalPath, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scanning repository: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		r.UpdatedAt, _ = time.Parse(time.DateTime, updatedAt)
		results = append(results, r)
	}
	return results, rows.Err()
}

func (q *Queries) UpsertRepository(url, localPath string) (*models.Repository, error) {
	_, err := q.db.Exec(
		`INSERT INTO repositories (url, local_path) VALUES (?, ?)
		 ON CONFLICT(url) DO UPDATE SET local_path = excluded.local_path, updated_at = datetime('now')`,
		url, localPath,
	)
	if err != nil {
		return nil, fmt.Errorf("upserting repository: %w", err)
	}
	return q.GetRepositoryByURL(url)
}

func (q *Queries) GetRepositoryByURL(url string) (*models.Repository, error) {
	r := &models.Repository{}
	var createdAt, updatedAt string
	err := q.db.QueryRow(
		`SELECT id, url, local_path, created_at, updated_at FROM repositories WHERE url = ?`, url,
	).Scan(&r.ID, &r.URL, &r.LocalPath, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting repository: %w", err)
	}
	r.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	r.UpdatedAt, _ = time.Parse(time.DateTime, updatedAt)
	return r, nil
}

// Prompt Requests

func (q *Queries) CreatePromptRequest(repoID int64, sessionID string) (*models.PromptRequest, error) {
	res, err := q.db.Exec(
		`INSERT INTO prompt_requests (repository_id, session_id) VALUES (?, ?)`,
		repoID, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("creating prompt request: %w", err)
	}
	id, _ := res.LastInsertId()
	return q.GetPromptRequest(id)
}

func (q *Queries) GetPromptRequest(id int64) (*models.PromptRequest, error) {
	pr := &models.PromptRequest{}
	var createdAt, updatedAt string
	err := q.db.QueryRow(
		`SELECT pr.id, pr.repository_id, pr.title, pr.status, pr.session_id,
		        pr.issue_number, pr.issue_url, pr.created_at, pr.updated_at,
		        r.url
		 FROM prompt_requests pr
		 JOIN repositories r ON r.id = pr.repository_id
		 WHERE pr.id = ?`, id,
	).Scan(&pr.ID, &pr.RepositoryID, &pr.Title, &pr.Status, &pr.SessionID,
		&pr.IssueNumber, &pr.IssueURL, &createdAt, &updatedAt, &pr.RepoURL)
	if err != nil {
		return nil, fmt.Errorf("getting prompt request: %w", err)
	}
	pr.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	pr.UpdatedAt, _ = time.Parse(time.DateTime, updatedAt)
	return pr, nil
}

func (q *Queries) ListPromptRequests() ([]models.PromptRequest, error) {
	rows, err := q.db.Query(
		`SELECT pr.id, pr.repository_id, pr.title, pr.status, pr.session_id,
		        pr.issue_number, pr.issue_url, pr.created_at, pr.updated_at,
		        r.url,
		        (SELECT COUNT(*) FROM messages WHERE prompt_request_id = pr.id) as message_count,
		        (SELECT COUNT(*) FROM revisions WHERE prompt_request_id = pr.id) as revision_count
		 FROM prompt_requests pr
		 JOIN repositories r ON r.id = pr.repository_id
		 WHERE pr.status != 'deleted'
		 ORDER BY pr.updated_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing prompt requests: %w", err)
	}
	defer rows.Close()

	var results []models.PromptRequest
	for rows.Next() {
		var pr models.PromptRequest
		var createdAt, updatedAt string
		if err := rows.Scan(&pr.ID, &pr.RepositoryID, &pr.Title, &pr.Status, &pr.SessionID,
			&pr.IssueNumber, &pr.IssueURL, &createdAt, &updatedAt, &pr.RepoURL,
			&pr.MessageCount, &pr.RevisionCount); err != nil {
			return nil, fmt.Errorf("scanning prompt request: %w", err)
		}
		pr.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		pr.UpdatedAt, _ = time.Parse(time.DateTime, updatedAt)
		results = append(results, pr)
	}
	return results, rows.Err()
}

func (q *Queries) UpdatePromptRequestTitle(id int64, title string) error {
	_, err := q.db.Exec(
		`UPDATE prompt_requests SET title = ?, updated_at = datetime('now') WHERE id = ?`,
		title, id,
	)
	return err
}

func (q *Queries) UpdatePromptRequestStatus(id int64, status string) error {
	_, err := q.db.Exec(
		`UPDATE prompt_requests SET status = ?, updated_at = datetime('now') WHERE id = ?`,
		status, id,
	)
	return err
}

func (q *Queries) UpdatePromptRequestIssue(id int64, issueNumber int, issueURL string) error {
	_, err := q.db.Exec(
		`UPDATE prompt_requests SET issue_number = ?, issue_url = ?, status = 'published', updated_at = datetime('now') WHERE id = ?`,
		issueNumber, issueURL, id,
	)
	return err
}

func (q *Queries) DeletePromptRequest(id int64) error {
	return q.UpdatePromptRequestStatus(id, "deleted")
}

// Messages

func (q *Queries) CreateMessage(promptRequestID int64, role, content string, rawResponse *string) (*models.Message, error) {
	res, err := q.db.Exec(
		`INSERT INTO messages (prompt_request_id, role, content, raw_response) VALUES (?, ?, ?, ?)`,
		promptRequestID, role, content, rawResponse,
	)
	if err != nil {
		return nil, fmt.Errorf("creating message: %w", err)
	}
	id, _ := res.LastInsertId()
	return q.GetMessage(id)
}

func (q *Queries) GetMessage(id int64) (*models.Message, error) {
	m := &models.Message{}
	var createdAt string
	err := q.db.QueryRow(
		`SELECT id, prompt_request_id, role, content, raw_response, created_at FROM messages WHERE id = ?`, id,
	).Scan(&m.ID, &m.PromptRequestID, &m.Role, &m.Content, &m.RawResponse, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("getting message: %w", err)
	}
	m.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return m, nil
}

func (q *Queries) ListMessages(promptRequestID int64) ([]models.Message, error) {
	rows, err := q.db.Query(
		`SELECT id, prompt_request_id, role, content, raw_response, created_at
		 FROM messages WHERE prompt_request_id = ? ORDER BY created_at ASC`, promptRequestID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing messages: %w", err)
	}
	defer rows.Close()

	var results []models.Message
	for rows.Next() {
		var m models.Message
		var createdAt string
		if err := rows.Scan(&m.ID, &m.PromptRequestID, &m.Role, &m.Content, &m.RawResponse, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning message: %w", err)
		}
		m.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		results = append(results, m)
	}
	return results, rows.Err()
}

// Revisions

func (q *Queries) CreateRevision(promptRequestID int64, content string) (*models.Revision, error) {
	res, err := q.db.Exec(
		`INSERT INTO revisions (prompt_request_id, content) VALUES (?, ?)`,
		promptRequestID, content,
	)
	if err != nil {
		return nil, fmt.Errorf("creating revision: %w", err)
	}
	id, _ := res.LastInsertId()
	r := &models.Revision{}
	var publishedAt string
	err = q.db.QueryRow(
		`SELECT id, prompt_request_id, content, published_at FROM revisions WHERE id = ?`, id,
	).Scan(&r.ID, &r.PromptRequestID, &r.Content, &publishedAt)
	if err != nil {
		return nil, fmt.Errorf("getting revision: %w", err)
	}
	r.PublishedAt, _ = time.Parse(time.DateTime, publishedAt)
	return r, nil
}

func (q *Queries) ListRevisions(promptRequestID int64) ([]models.Revision, error) {
	rows, err := q.db.Query(
		`SELECT id, prompt_request_id, content, published_at
		 FROM revisions WHERE prompt_request_id = ? ORDER BY published_at DESC`, promptRequestID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing revisions: %w", err)
	}
	defer rows.Close()

	var results []models.Revision
	for rows.Next() {
		var r models.Revision
		var publishedAt string
		if err := rows.Scan(&r.ID, &r.PromptRequestID, &r.Content, &publishedAt); err != nil {
			return nil, fmt.Errorf("scanning revision: %w", err)
		}
		r.PublishedAt, _ = time.Parse(time.DateTime, publishedAt)
		results = append(results, r)
	}
	return results, rows.Err()
}
