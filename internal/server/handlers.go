package server

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/esnunes/prompter/internal/models"

	"github.com/google/uuid"
)

type dashboardData struct {
	PromptRequests []models.PromptRequest
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	prs, err := s.queries.ListPromptRequests()
	if err != nil {
		log.Printf("listing prompt requests: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, "dashboard.html", dashboardData{PromptRequests: prs})
}

type newData struct {
	Repositories []models.Repository
}

func (s *Server) handleNew(w http.ResponseWriter, r *http.Request) {
	repos, err := s.queries.ListRepositories()
	if err != nil {
		log.Printf("listing repositories: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, "new.html", newData{Repositories: repos})
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	repoID, err := strconv.ParseInt(r.FormValue("repo_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid repository", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	sessionID := uuid.New().String()

	pr, err := s.queries.CreatePromptRequest(repoID, sessionID)
	if err != nil {
		log.Printf("creating prompt request: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if title != "" {
		if err := s.queries.UpdatePromptRequestTitle(pr.ID, title); err != nil {
			log.Printf("updating title: %v", err)
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/prompt-requests/%d", pr.ID), http.StatusSeeOther)
}

type conversationData struct {
	PromptRequest *models.PromptRequest
	Messages      []models.Message
	LastQuestion  *questionData
	PromptReady   bool
}

type questionData struct {
	Text    string
	Options []optionData
}

type optionData struct {
	Label       string
	Description string
}

func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	pr, err := s.queries.GetPromptRequest(id)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	messages, err := s.queries.ListMessages(id)
	if err != nil {
		log.Printf("listing messages: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if pr.Status == "published" {
		revisions, err := s.queries.ListRevisions(id)
		if err != nil {
			log.Printf("listing revisions: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		s.renderPage(w, "published.html", publishedData{
			PromptRequest: pr,
			Messages:      messages,
			Revisions:     revisions,
		})
		return
	}

	s.renderPage(w, "conversation.html", conversationData{
		PromptRequest: pr,
		Messages:      messages,
	})
}

type publishedData struct {
	PromptRequest *models.PromptRequest
	Messages      []models.Message
	Revisions     []models.Revision
}

// handleSendMessage is a stub — will be implemented in Phase 3 (Claude CLI integration)
func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented yet", http.StatusNotImplemented)
}

// handlePublish is a stub — will be implemented in Phase 4 (GitHub integration)
func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented yet", http.StatusNotImplemented)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	if err := s.queries.DeletePromptRequest(id); err != nil {
		log.Printf("deleting prompt request: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
