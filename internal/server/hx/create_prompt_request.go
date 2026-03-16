package hx

import (
	"fmt"
	"log"
	"net/http"

	"github.com/esnunes/prompter/internal/db"
	"github.com/esnunes/prompter/internal/repo"
	"github.com/google/uuid"
)

type CreatePromptRequestHandler struct {
	Queries           *db.Queries
	SetRepoStatus     func(int64, string, string)
	AsyncEnsureCloned func(int64, string)
}

func (h *CreatePromptRequestHandler) Handle(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoName := r.PathValue("repo")
	repoURL := fmt.Sprintf("github.com/%s/%s", org, repoName)

	localPath, err := repo.LocalPath(repoURL)
	if err != nil {
		log.Printf("computing local path: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	repoRecord, err := h.Queries.UpsertRepository(repoURL, localPath)
	if err != nil {
		log.Printf("upserting repository: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	sessionID := uuid.New().String()
	pr, err := h.Queries.CreatePromptRequest(repoRecord.ID, sessionID)
	if err != nil {
		log.Printf("creating prompt request: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	cloned, _ := repo.IsCloned(repoURL)
	if cloned {
		h.SetRepoStatus(pr.ID, "pulling", "")
	} else {
		h.SetRepoStatus(pr.ID, "cloning", "")
	}

	go h.AsyncEnsureCloned(pr.ID, repoURL)

	http.Redirect(w, r, fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d", org, repoName, pr.ID), http.StatusSeeOther)
}
