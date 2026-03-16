package server

import (
	"io/fs"
	"net/http"

	"github.com/esnunes/prompter/internal/server/hx"
	"github.com/esnunes/prompter/internal/server/pages/conversation"
	"github.com/esnunes/prompter/internal/server/pages/dashboard"
	"github.com/esnunes/prompter/internal/server/pages/repo"
)

func (s *Server) registerRoutes(mux *http.ServeMux) error {
	// Static files
	staticSub, err := fs.Sub(contentFS, "static")
	if err != nil {
		return err
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// --- Pages ---

	dashboardPage := &dashboard.Page{
		Tmpl:         s.tmpls["dashboard"],
		Queries:      s.queries,
		BuildSidebar: s.buildSidebarAny,
	}
	mux.HandleFunc("GET /{$}", dashboardPage.HandlePage)
	mux.HandleFunc("POST /hx/dashboard/create-repository", dashboardPage.HandleCreateRepository)

	repoPage := &repo.Page{
		Tmpl:         s.tmpls["repo"],
		Queries:      s.queries,
		BuildSidebar: s.buildSidebarAny,
	}
	mux.HandleFunc("GET /github.com/{org}/{repo}/prompt-requests", repoPage.HandlePage)

	convPage := &conversation.Page{
		Tmpl:                    s.tmpls["conversation"],
		Queries:                 s.queries,
		BuildSidebar:            s.buildSidebarAny,
		GetRepoStatus:           s.getRepoStatus,
		SetRepoStatus:           s.setRepoStatus,
		SetRepoStatusProcessing: s.setRepoStatusProcessing,
		ClearCancelFunc:         s.clearCancelFunc,
		RepoStatus:              &s.repoStatus,
		CancelFuncs:             &s.cancelFuncs,
		LockSession:             s.lockSession,
		LockRepo:                s.lockRepo,
	}
	mux.HandleFunc("GET /github.com/{org}/{repo}/prompt-requests/{id}", convPage.HandlePage)
	mux.HandleFunc("POST /hx/conversation/send-message", convPage.HandleSendMessage)
	mux.HandleFunc("GET /hx/conversation/status", convPage.HandleStatus)
	mux.HandleFunc("POST /hx/conversation/publish", convPage.HandlePublish)
	mux.HandleFunc("POST /hx/conversation/delete", convPage.HandleDelete)
	mux.HandleFunc("POST /hx/conversation/archive", convPage.HandleArchive)
	mux.HandleFunc("POST /hx/conversation/unarchive", convPage.HandleUnarchive)
	mux.HandleFunc("POST /hx/conversation/cancel", convPage.HandleCancel)
	mux.HandleFunc("POST /hx/conversation/retry", convPage.HandleRetry)
	mux.HandleFunc("POST /hx/conversation/resend", convPage.HandleResend)

	// Create prompt request (shared — used from repo + conversation pages)
	createPR := &hx.CreatePromptRequestHandler{
		Queries:           s.queries,
		SetRepoStatus:     s.setRepoStatus,
		AsyncEnsureCloned: convPage.AsyncEnsureCloned,
	}
	mux.HandleFunc("POST /hx/create-prompt-request", createPR.Handle)

	// Shared HX fragments
	hxHandler := hx.New(s.tmpls["dashboard"], s.queries, s.getRepoStatusString)
	mux.HandleFunc("GET /hx/sidebar", hxHandler.HandleSidebar)

	return nil
}
