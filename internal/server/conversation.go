package server

import (
	"context"
	"strings"

	"github.com/esnunes/prompter/gotk"
)

func (s *Server) SendMessage(ctx *gotk.CommandContext) error {
	id := ctx.Payload.Int64("prompt_request_id")
	if id == 0 {
		gotk.Dispatch(ctx.Dispatcher(), ErrorEvent{Target: "#conversation", Message: "Invalid prompt request ID"})
		return nil
	}

	message := strings.TrimSpace(ctx.Payload.String("message"))
	if message == "" {
		return nil
	}

	// Save user message
	userMsg, err := s.queries.CreateMessage(id, "user", message, nil)
	if err != nil {
		gotk.Dispatch(ctx.Dispatcher(), ErrorEvent{Target: "#conversation", Message: "Failed to save message"})
		return nil
	}

	// Dispatch message sent event (view clears input, disables form, shows bubble)
	gotk.Dispatch(ctx.Dispatcher(), MessageSentEvent{
		PromptRequestID: id,
		MessageContent:  userMsg.Content,
	})

	// Check repo status — if not ready, just save and disable form
	statusEntry := s.getRepoStatus(id)
	if statusEntry.Status != "" && statusEntry.Status != "ready" {
		return nil
	}

	// Repo is ready — launch async Claude call
	bgCtx, cancel := context.WithCancel(context.Background())
	s.setRepoStatusProcessing(id, cancel)
	tctx := ctx.NewTask()
	go s.backgroundSendMessage(bgCtx, id, tctx)

	// Show processing indicator
	gotk.Dispatch(ctx.Dispatcher(), ProcessingStartedEvent{
		PromptRequestID: id,
		StartedAt:       s.getRepoStatus(id).StartedAt,
	})

	return nil
}
