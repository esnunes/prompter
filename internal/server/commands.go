package server

import (
	"fmt"
	"log"

	"github.com/esnunes/prompter/gotk"
	"github.com/esnunes/prompter/internal/repo"
	"github.com/google/uuid"
)

func (s *Server) CreatePromptRequest(ctx *gotk.CommandContext) error {
	org := ctx.Payload.String("org")
	repoName := ctx.Payload.String("repo")
	repoURL := fmt.Sprintf("github.com/%s/%s", org, repoName)

	localPath, err := repo.LocalPath(repoURL)
	if err != nil {
		log.Printf("computing local path: %v", err)
		return nil
	}

	repoRecord, err := s.queries.UpsertRepository(repoURL, localPath)
	if err != nil {
		log.Printf("upserting repository: %v", err)
		return nil
	}

	sessionID := uuid.New().String()
	pr, err := s.queries.CreatePromptRequest(repoRecord.ID, sessionID)
	if err != nil {
		log.Printf("creating prompt request: %v", err)
		return nil
	}

	cloned, _ := repo.IsCloned(repoURL)
	if cloned {
		s.setRepoStatus(pr.ID, "pulling", "")
	} else {
		s.setRepoStatus(pr.ID, "cloning", "")
	}

	tctx := ctx.NewTask()
	go s.asyncEnsureCloned(pr.ID, repoURL, tctx)

	url := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d", org, repoName, pr.ID)
	ctx.Navigate(url)

	return nil
}
