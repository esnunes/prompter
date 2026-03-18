package db

import (
	"database/sql"
	"encoding/json"
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

func (q *Queries) ListRepositorySummaries() ([]models.RepositorySummary, error) {
	rows, err := q.db.Query(`
		SELECT r.id, r.url,
		       COUNT(CASE WHEN pr.archived = 0 AND pr.status != 'deleted' THEN 1 END) as active_pr_count,
		       COALESCE(MAX(pr.updated_at), r.created_at) as last_activity
		FROM repositories r
		LEFT JOIN prompt_requests pr ON pr.repository_id = r.id
		GROUP BY r.id
		ORDER BY last_activity DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing repository summaries: %w", err)
	}
	defer rows.Close()

	var results []models.RepositorySummary
	for rows.Next() {
		var rs models.RepositorySummary
		var lastActivity string
		if err := rows.Scan(&rs.ID, &rs.URL, &rs.ActivePRCount, &lastActivity); err != nil {
			return nil, fmt.Errorf("scanning repository summary: %w", err)
		}
		rs.LastActivity, _ = time.Parse(time.DateTime, lastActivity)
		results = append(results, rs)
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
	var archived int
	err := q.db.QueryRow(
		`SELECT pr.id, pr.repository_id, pr.title, pr.status, pr.session_id,
		        pr.issue_number, pr.issue_url, pr.created_at, pr.updated_at,
		        r.url, r.local_path, pr.archived
		 FROM prompt_requests pr
		 JOIN repositories r ON r.id = pr.repository_id
		 WHERE pr.id = ?`, id,
	).Scan(&pr.ID, &pr.RepositoryID, &pr.Title, &pr.Status, &pr.SessionID,
		&pr.IssueNumber, &pr.IssueURL, &createdAt, &updatedAt, &pr.RepoURL, &pr.RepoLocalPath,
		&archived)
	if err != nil {
		return nil, fmt.Errorf("getting prompt request: %w", err)
	}
	pr.Archived = archived != 0
	pr.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	pr.UpdatedAt, _ = time.Parse(time.DateTime, updatedAt)
	return pr, nil
}

const listPromptRequestsQuery = `SELECT pr.id, pr.repository_id, pr.title, pr.status, pr.session_id,
		        pr.issue_number, pr.issue_url, pr.created_at, pr.updated_at,
		        r.url,
		        (SELECT COUNT(*) FROM messages WHERE prompt_request_id = pr.id) as message_count,
		        (SELECT COUNT(*) FROM revisions WHERE prompt_request_id = pr.id) as revision_count,
		        pr.last_viewed_at,
		        (SELECT MAX(created_at) FROM messages WHERE prompt_request_id = pr.id AND role = 'assistant') as latest_assistant_at,
		        pr.archived
		 FROM prompt_requests pr
		 JOIN repositories r ON r.id = pr.repository_id
		 WHERE pr.status != 'deleted'`

func scanPromptRequest(rows *sql.Rows) (models.PromptRequest, error) {
	var pr models.PromptRequest
	var createdAt, updatedAt string
	var lastViewedAt, latestAssistantAt *string
	var archived int
	if err := rows.Scan(&pr.ID, &pr.RepositoryID, &pr.Title, &pr.Status, &pr.SessionID,
		&pr.IssueNumber, &pr.IssueURL, &createdAt, &updatedAt, &pr.RepoURL,
		&pr.MessageCount, &pr.RevisionCount, &lastViewedAt, &latestAssistantAt,
		&archived); err != nil {
		return pr, err
	}
	pr.Archived = archived != 0
	pr.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	pr.UpdatedAt, _ = time.Parse(time.DateTime, updatedAt)
	if lastViewedAt != nil {
		t, _ := time.Parse(time.DateTime, *lastViewedAt)
		pr.LastViewedAt = &t
	}
	if latestAssistantAt != nil {
		t, _ := time.Parse(time.DateTime, *latestAssistantAt)
		pr.LatestAssistantAt = &t
	}
	return pr, nil
}

func (q *Queries) ListPromptRequests(archivedOnly bool) ([]models.PromptRequest, error) {
	archivedVal := 0
	if archivedOnly {
		archivedVal = 1
	}
	rows, err := q.db.Query(
		listPromptRequestsQuery+` AND pr.archived = ?
		 ORDER BY
		   CASE WHEN pr.status = 'draft' THEN 0 ELSE 1 END ASC,
		   pr.updated_at DESC`,
		archivedVal,
	)
	if err != nil {
		return nil, fmt.Errorf("listing prompt requests: %w", err)
	}
	defer rows.Close()

	var results []models.PromptRequest
	for rows.Next() {
		pr, err := scanPromptRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning prompt request: %w", err)
		}
		results = append(results, pr)
	}
	return results, rows.Err()
}

func (q *Queries) ListPromptRequestsByRepoURL(repoURL string, archivedOnly bool) ([]models.PromptRequest, error) {
	archivedVal := 0
	if archivedOnly {
		archivedVal = 1
	}
	rows, err := q.db.Query(
		listPromptRequestsQuery+` AND r.url = ? AND pr.archived = ?
		 ORDER BY
		   CASE WHEN pr.status = 'draft' THEN 0 ELSE 1 END ASC,
		   pr.updated_at DESC`, repoURL, archivedVal,
	)
	if err != nil {
		return nil, fmt.Errorf("listing prompt requests by repo: %w", err)
	}
	defer rows.Close()

	var results []models.PromptRequest
	for rows.Next() {
		pr, err := scanPromptRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning prompt request: %w", err)
		}
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

func (q *Queries) ArchivePromptRequest(id int64) error {
	_, err := q.db.Exec(
		`UPDATE prompt_requests SET archived = 1 WHERE id = ?`, id,
	)
	return err
}

func (q *Queries) UnarchivePromptRequest(id int64) error {
	_, err := q.db.Exec(
		`UPDATE prompt_requests SET archived = 0 WHERE id = ?`, id,
	)
	return err
}

func (q *Queries) UpdateLastViewedAt(id int64) error {
	_, err := q.db.Exec(
		`UPDATE prompt_requests SET last_viewed_at = datetime('now') WHERE id = ?`, id,
	)
	return err
}

// GeneratedContent holds the title, motivation, and prompt extracted from a Claude response.
type GeneratedContent struct {
	Title      string
	Motivation string
	Prompt     string
}

// GetLatestGeneratedContent finds the most recent generated_motivation and generated_prompt from assistant messages.
func (q *Queries) GetLatestGeneratedContent(promptRequestID int64) (*GeneratedContent, error) {
	rows, err := q.db.Query(
		`SELECT raw_response FROM messages
		 WHERE prompt_request_id = ? AND role = 'assistant' AND raw_response IS NOT NULL
		 ORDER BY created_at DESC`, promptRequestID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying messages: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		if gc := extractGeneratedContent(raw); gc != nil {
			return gc, nil
		}
	}
	return nil, fmt.Errorf("no generated prompt found")
}

func extractGeneratedContent(rawJSON string) *GeneratedContent {
	type resp struct {
		GeneratedTitle      string `json:"generated_title"`
		GeneratedMotivation string `json:"generated_motivation"`
		GeneratedPrompt     string `json:"generated_prompt"`
	}

	extract := func(r *resp) *GeneratedContent {
		if r != nil && r.GeneratedPrompt != "" {
			return &GeneratedContent{Title: r.GeneratedTitle, Motivation: r.GeneratedMotivation, Prompt: r.GeneratedPrompt}
		}
		return nil
	}

	// The raw JSON is the full claude CLI output: {"type":"result","structured_output":{...},...}
	var wrapper struct {
		StructuredOutput *resp  `json:"structured_output"`
		Result           string `json:"result"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &wrapper); err == nil {
		if gc := extract(wrapper.StructuredOutput); gc != nil {
			return gc
		}
		if wrapper.Result != "" {
			var r resp
			if json.Unmarshal([]byte(wrapper.Result), &r) == nil {
				if gc := extract(&r); gc != nil {
					return gc
				}
			}
		}
	}

	// Try direct parse
	var r resp
	if json.Unmarshal([]byte(rawJSON), &r) == nil {
		return extract(&r)
	}

	return nil
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

func (q *Queries) CreateRevision(promptRequestID int64, content string, afterMessageID *int64) (*models.Revision, error) {
	res, err := q.db.Exec(
		`INSERT INTO revisions (prompt_request_id, content, after_message_id) VALUES (?, ?, ?)`,
		promptRequestID, content, afterMessageID,
	)
	if err != nil {
		return nil, fmt.Errorf("creating revision: %w", err)
	}
	id, _ := res.LastInsertId()
	r := &models.Revision{}
	var publishedAt string
	err = q.db.QueryRow(
		`SELECT id, prompt_request_id, content, after_message_id, published_at FROM revisions WHERE id = ?`, id,
	).Scan(&r.ID, &r.PromptRequestID, &r.Content, &r.AfterMessageID, &publishedAt)
	if err != nil {
		return nil, fmt.Errorf("getting revision: %w", err)
	}
	r.PublishedAt, _ = time.Parse(time.DateTime, publishedAt)
	return r, nil
}

func (q *Queries) ListRevisions(promptRequestID int64) ([]models.Revision, error) {
	rows, err := q.db.Query(
		`SELECT id, prompt_request_id, content, after_message_id, published_at
		 FROM revisions WHERE prompt_request_id = ? ORDER BY published_at ASC`, promptRequestID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing revisions: %w", err)
	}
	defer rows.Close()

	var results []models.Revision
	for rows.Next() {
		var r models.Revision
		var publishedAt string
		if err := rows.Scan(&r.ID, &r.PromptRequestID, &r.Content, &r.AfterMessageID, &publishedAt); err != nil {
			return nil, fmt.Errorf("scanning revision: %w", err)
		}
		r.PublishedAt, _ = time.Parse(time.DateTime, publishedAt)
		results = append(results, r)
	}
	return results, rows.Err()
}

func (q *Queries) DeleteMessage(id int64) error {
	_, err := q.db.Exec(`DELETE FROM messages WHERE id = ?`, id)
	return err
}

func (q *Queries) GetLastMessage(promptRequestID int64) (*models.Message, error) {
	m := &models.Message{}
	var createdAt string
	err := q.db.QueryRow(
		`SELECT id, prompt_request_id, role, content, raw_response, created_at
		 FROM messages WHERE prompt_request_id = ? ORDER BY created_at DESC LIMIT 1`, promptRequestID,
	).Scan(&m.ID, &m.PromptRequestID, &m.Role, &m.Content, &m.RawResponse, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("getting last message: %w", err)
	}
	m.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return m, nil
}
