package server

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/esnunes/prompter/gotk"
	"github.com/esnunes/prompter/internal/db"

	_ "modernc.org/sqlite"
)

// newTestServer creates a Server with an in-memory SQLite database for testing.
// It sets XDG_CACHE_HOME to a temporary directory so repo.LocalPath works.
func newTestServer(t *testing.T) *Server {
	t.Helper()

	// Use os.MkdirTemp instead of t.TempDir to avoid cleanup races with
	// background goroutines (asyncEnsureCloned writes to this directory).
	tmpDir, err := os.MkdirTemp("", "prompter-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	t.Setenv("XDG_CACHE_HOME", tmpDir)
	t.Cleanup(func() {
		// Best-effort cleanup; background goroutines may still hold files.
		time.Sleep(50 * time.Millisecond)
		os.RemoveAll(tmpDir)
	})

	sqlDB, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	s := &Server{
		queries: db.NewQueries(sqlDB),
		gotkMux: gotk.NewMux(),
	}

	return s
}

// newTestCommandContext creates a gotk.TestCommandContext for testing command handlers.
func newTestCommandContext() *gotk.TestCommandContext {
	return gotk.NewTestCommandContext()
}

// createTestPromptRequest sets up a repo + prompt request in the database and
// returns the prompt request ID. Useful for tests that need a valid PR to operate on.
func createTestPromptRequest(t *testing.T, s *Server) int64 {
	t.Helper()

	tc := newTestCommandContext()
	tc.SetPayload(map[string]any{
		"org":  "octocat",
		"repo": "hello-world",
	})
	if err := s.CreatePromptRequest(tc.CommandContext); err != nil {
		t.Fatalf("creating prompt request: %v", err)
	}

	prs, err := s.queries.ListPromptRequestsByRepoURL("github.com/octocat/hello-world", false)
	if err != nil {
		t.Fatalf("listing prompt requests: %v", err)
	}
	if len(prs) == 0 {
		t.Fatal("no prompt requests found")
	}
	return prs[len(prs)-1].ID
}

func TestCreatePromptRequest(t *testing.T) {
	s := newTestServer(t)

	tc := newTestCommandContext()
	tc.SetPayload(map[string]any{
		"org":  "octocat",
		"repo": "hello-world",
	})

	err := s.CreatePromptRequest(tc.CommandContext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify navigate URL was set
	navURL := tc.CommandContext.NavigateURL()
	expectedPrefix := "/github.com/octocat/hello-world/prompt-requests/"
	if !strings.HasPrefix(navURL, expectedPrefix) {
		t.Errorf("NavigateURL() = %q, want prefix %q", navURL, expectedPrefix)
	}

	// Verify the prompt request was persisted in the database
	prs, err := s.queries.ListPromptRequestsByRepoURL("github.com/octocat/hello-world", false)
	if err != nil {
		t.Fatalf("listing prompt requests: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("expected 1 prompt request, got %d", len(prs))
	}
	if prs[0].Status != "draft" {
		t.Errorf("status = %q, want %q", prs[0].Status, "draft")
	}

	// Verify repo status was set (cloning for a new repo)
	entry := s.getRepoStatus(prs[0].ID)
	if entry.Status != "cloning" {
		t.Errorf("repo status = %q, want %q", entry.Status, "cloning")
	}
}

func TestCreatePromptRequest_CreatesRepository(t *testing.T) {
	s := newTestServer(t)

	tc := newTestCommandContext()
	tc.SetPayload(map[string]any{
		"org":  "acme",
		"repo": "widgets",
	})

	if err := s.CreatePromptRequest(tc.CommandContext); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	repoRecord, err := s.queries.GetRepositoryByURL("github.com/acme/widgets")
	if err != nil {
		t.Fatalf("getting repository: %v", err)
	}
	if repoRecord.URL != "github.com/acme/widgets" {
		t.Errorf("repo URL = %q, want %q", repoRecord.URL, "github.com/acme/widgets")
	}
	if repoRecord.LocalPath == "" {
		t.Error("repo local path should not be empty")
	}
}

func TestCreatePromptRequest_NavigateURLContainsID(t *testing.T) {
	s := newTestServer(t)

	// Create two prompt requests to verify IDs are distinct
	for i := range 2 {
		tc := newTestCommandContext()
		tc.SetPayload(map[string]any{
			"org":  "octocat",
			"repo": "hello-world",
		})
		if err := s.CreatePromptRequest(tc.CommandContext); err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
	}

	prs, err := s.queries.ListPromptRequestsByRepoURL("github.com/octocat/hello-world", false)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("expected 2 prompt requests, got %d", len(prs))
	}
	if prs[0].ID == prs[1].ID {
		t.Error("prompt request IDs should be distinct")
	}
}

func TestSendMessage(t *testing.T) {
	s := newTestServer(t)
	prID := createTestPromptRequest(t, s)

	// Mark repo as ready so SendMessage proceeds to launch processing
	s.setRepoStatus(prID, "ready", "")

	tc := newTestCommandContext()
	gotk.RegisterCapture[MessageSentEvent](tc)
	gotk.RegisterCapture[ProcessingStartedEvent](tc)
	tc.SetPayload(map[string]any{
		"prompt_request_id": prID,
		"message":           "Hello, world!",
	})

	if err := s.SendMessage(tc.CommandContext); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify MessageSentEvent was dispatched
	msgEvents := gotk.EventsOfType[MessageSentEvent](tc)
	if len(msgEvents) != 1 {
		t.Fatalf("expected 1 MessageSentEvent, got %d", len(msgEvents))
	}
	if msgEvents[0].MessageContent != "Hello, world!" {
		t.Errorf("MessageContent = %q, want %q", msgEvents[0].MessageContent, "Hello, world!")
	}

	// Verify ProcessingStartedEvent was dispatched (repo was ready)
	procEvents := gotk.EventsOfType[ProcessingStartedEvent](tc)
	if len(procEvents) != 1 {
		t.Fatalf("expected 1 ProcessingStartedEvent, got %d", len(procEvents))
	}
	if procEvents[0].PromptRequestID != prID {
		t.Errorf("PromptRequestID = %d, want %d", procEvents[0].PromptRequestID, prID)
	}

	// Verify message was persisted
	msgs, err := s.queries.ListMessages(prID)
	if err != nil {
		t.Fatalf("listing messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "Hello, world!" {
		t.Errorf("message content = %q, want %q", msgs[0].Content, "Hello, world!")
	}
}

func TestSendMessage_EmptyMessage(t *testing.T) {
	s := newTestServer(t)
	prID := createTestPromptRequest(t, s)

	tc := newTestCommandContext()
	gotk.RegisterCapture[MessageSentEvent](tc)
	tc.SetPayload(map[string]any{
		"prompt_request_id": prID,
		"message":           "   ",
	})

	if err := s.SendMessage(tc.CommandContext); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := tc.Events()
	if len(events) != 0 {
		t.Errorf("expected 0 events for empty message, got %d", len(events))
	}
}

func TestSendMessage_RepoNotReady(t *testing.T) {
	s := newTestServer(t)
	prID := createTestPromptRequest(t, s)

	// Repo is still cloning — message should save but not launch processing
	s.setRepoStatus(prID, "cloning", "")

	tc := newTestCommandContext()
	gotk.RegisterCapture[MessageSentEvent](tc)
	gotk.RegisterCapture[ProcessingStartedEvent](tc)
	tc.SetPayload(map[string]any{
		"prompt_request_id": prID,
		"message":           "Hello",
	})

	if err := s.SendMessage(tc.CommandContext); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have MessageSentEvent but no ProcessingStartedEvent
	msgEvents := gotk.EventsOfType[MessageSentEvent](tc)
	if len(msgEvents) != 1 {
		t.Fatalf("expected 1 MessageSentEvent, got %d", len(msgEvents))
	}

	procEvents := gotk.EventsOfType[ProcessingStartedEvent](tc)
	if len(procEvents) != 0 {
		t.Error("should not dispatch ProcessingStartedEvent when repo not ready")
	}

	// Verify message was persisted
	msgs, err := s.queries.ListMessages(prID)
	if err != nil {
		t.Fatalf("listing messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "Hello" {
		t.Errorf("message content = %q, want %q", msgs[0].Content, "Hello")
	}
}
