package server

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/esnunes/prompter/internal/db"
)

//go:embed templates
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

type Server struct {
	queries    *db.Queries
	pages      map[string]*template.Template
	httpSrv    *http.Server
	ln         net.Listener
	addr       string
	sessionMu  sync.Map // per-session mutex: session ID → *sync.Mutex
}

var funcMap = template.FuncMap{
	"deref": func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	},
}

func New(queries *db.Queries) (*Server, error) {
	pages, err := parsePages()
	if err != nil {
		return nil, err
	}

	s := &Server{
		queries: queries,
		pages:   pages,
	}

	mux := http.NewServeMux()

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("getting static subfs: %w", err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	mux.HandleFunc("GET /{$}", s.handleDashboard)
	mux.HandleFunc("GET /github.com/{org}/{repo}/prompt-requests", s.handleRepoPage)
	mux.HandleFunc("POST /github.com/{org}/{repo}/prompt-requests", s.handleCreate)
	mux.HandleFunc("GET /github.com/{org}/{repo}/prompt-requests/{id}", s.handleShow)
	mux.HandleFunc("POST /github.com/{org}/{repo}/prompt-requests/{id}/messages", s.handleSendMessage)
	mux.HandleFunc("POST /github.com/{org}/{repo}/prompt-requests/{id}/publish", s.handlePublish)
	mux.HandleFunc("DELETE /github.com/{org}/{repo}/prompt-requests/{id}", s.handleDelete)

	s.httpSrv = &http.Server{Handler: mux}
	return s, nil
}

// parsePages builds a template for each page by combining layout.html with the page template.
func parsePages() (map[string]*template.Template, error) {
	tmplFS, err := fs.Sub(templatesFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("getting templates subfs: %w", err)
	}

	layoutBytes, err := fs.ReadFile(tmplFS, "layout.html")
	if err != nil {
		return nil, fmt.Errorf("reading layout: %w", err)
	}

	pageNames := []string{
		"dashboard.html",
		"repo.html",
		"conversation.html",
		"message_fragment.html",
	}

	pages := make(map[string]*template.Template, len(pageNames))
	for _, name := range pageNames {
		pageBytes, err := fs.ReadFile(tmplFS, name)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}

		tmpl, err := template.New("layout.html").Funcs(funcMap).Parse(string(layoutBytes))
		if err != nil {
			return nil, fmt.Errorf("parsing layout for %s: %w", name, err)
		}

		if _, err := tmpl.New(name).Parse(string(pageBytes)); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", name, err)
		}

		pages[name] = tmpl
	}
	return pages, nil
}

// Listen binds the server to a random available port. Call Serve to start handling requests.
func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
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

	fmt.Printf("Prompter running at http://%s\n", s.addr)
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

func (s *Server) renderPage(w http.ResponseWriter, name string, data any) {
	tmpl, ok := s.pages[name]
	if !ok {
		log.Printf("template not found: %s", name)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render error (%s): %v", name, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// lockSession returns the mutex for a given session ID. Callers must
// call Unlock when done to allow subsequent requests for the same session.
func (s *Server) lockSession(sessionID string) *sync.Mutex {
	v, _ := s.sessionMu.LoadOrStore(sessionID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu
}

func (s *Server) renderFragment(w http.ResponseWriter, name string, data any) {
	tmpl, ok := s.pages[name]
	if !ok {
		log.Printf("fragment template not found: %s", name)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render error (%s): %v", name, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
