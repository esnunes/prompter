package server

import (
	"context"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/esnunes/prompter/internal/db"
	"github.com/esnunes/prompter/internal/models"
	"github.com/esnunes/prompter/internal/server/hx"
	"github.com/esnunes/prompter/internal/server/pages/conversation"
)

type Server struct {
	queries     *db.Queries
	tmpls       map[string]*template.Template
	httpSrv     *http.Server
	ln          net.Listener
	addr        string
	sessionMu   sync.Map // per-session mutex: session ID → *sync.Mutex
	repoStatus  sync.Map // per-prompt-request status: prompt request ID (int64) → conversation.RepoStatusEntry
	cancelFuncs sync.Map // per-prompt-request cancel: prompt request ID (int64) → context.CancelFunc
	repoMu      sync.Map // per-repo mutex: repo URL (string) → *sync.Mutex
}

func New(queries *db.Queries) (*Server, error) {
	tmpls, err := loadTemplates()
	if err != nil {
		return nil, fmt.Errorf("loading templates: %w", err)
	}

	s := &Server{
		queries: queries,
		tmpls:   tmpls,
	}

	mux := http.NewServeMux()
	if err := s.registerRoutes(mux); err != nil {
		return nil, fmt.Errorf("registering routes: %w", err)
	}
	s.httpSrv = &http.Server{Handler: mux}
	return s, nil
}

// Listen binds the server to the given address. Call Serve to start handling requests.
func (s *Server) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("binding port: %w", err)
	}
	s.ln = ln
	s.addr = ln.Addr().String()
	return nil
}

// Serve starts handling HTTP requests. Blocks until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		s.httpSrv.Shutdown(context.Background())
	}()

	fmt.Printf("Listening on http://%s\n", s.addr)
	fmt.Println("Press Ctrl+C to stop.")

	if err := s.httpSrv.Serve(s.ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serving: %w", err)
	}
	fmt.Println("\nShutting down...")
	return nil
}

func (s *Server) Addr() string {
	return s.addr
}

// State management

// lockSession returns the mutex for a given session ID. Callers must
// call Unlock when done to allow subsequent requests for the same session.
func (s *Server) lockSession(sessionID string) *sync.Mutex {
	v, _ := s.sessionMu.LoadOrStore(sessionID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu
}

func (s *Server) setRepoStatus(prID int64, status, errMsg string) {
	s.repoStatus.Store(prID, conversation.RepoStatusEntry{Status: status, Error: errMsg})
}

func (s *Server) setRepoStatusProcessing(prID int64, cancelFunc context.CancelFunc) {
	s.repoStatus.Store(prID, conversation.RepoStatusEntry{Status: "processing", StartedAt: time.Now()})
	s.cancelFuncs.Store(prID, cancelFunc)
}

func (s *Server) clearCancelFunc(prID int64) {
	s.cancelFuncs.Delete(prID)
}

func (s *Server) getRepoStatus(prID int64) conversation.RepoStatusEntry {
	v, ok := s.repoStatus.Load(prID)
	if !ok {
		return conversation.RepoStatusEntry{}
	}
	return v.(conversation.RepoStatusEntry)
}

// lockRepo returns the mutex for a given repo URL. Callers must call Unlock when done.
func (s *Server) lockRepo(repoURL string) *sync.Mutex {
	v, _ := s.repoMu.LoadOrStore(repoURL, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu
}

func (s *Server) getRepoStatusString(prID int64) string {
	return s.getRepoStatus(prID).Status
}

func (s *Server) buildSidebarAny(prs []models.PromptRequest, scope string, currentID int64) any {
	return hx.BuildSidebar(prs, scope, currentID, s.getRepoStatusString)
}
