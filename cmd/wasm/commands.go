package main

import "github.com/esnunes/prompter/gotk"

// ScrollConversation scrolls the conversation to the latest message
// or to the question form if one is present.
// Delegates the actual DOM scroll to a registered JS function.
func ScrollConversation(ctx *gotk.Context) error {
	ctx.Exec("scrollConversation")
	return nil
}

// UpdateFormVisibility hides the message form when a question form
// is present and shows it otherwise.
func UpdateFormVisibility(ctx *gotk.Context) error {
	hasQuestions := ctx.Payload.Bool("has_questions")
	if hasQuestions {
		ctx.AttrSet("#message-form", "style", "display:none")
	} else {
		ctx.AttrRemove("#message-form", "style")
	}
	return nil
}

// CheckEnter handles the Enter-to-send keyboard interaction.
// Reads keyboard event metadata from payload._event.
// Enter without Shift dispatches send-message to the server.
// Shift+Enter and IME composition are ignored (browser handles newline).
func CheckEnter(ctx *gotk.Context) error {
	event := ctx.Payload.Map()["_event"]
	if event == nil {
		return nil
	}
	eventMap, ok := event.(map[string]any)
	if !ok {
		return nil
	}

	key, _ := eventMap["key"].(string)
	if key != "Enter" {
		return nil
	}

	// Allow Shift+Enter for newlines
	shiftKey, _ := eventMap["shiftKey"].(bool)
	if shiftKey {
		return nil
	}

	// Don't submit during IME composition
	isComposing, _ := eventMap["isComposing"].(bool)
	if isComposing {
		return nil
	}

	// Dispatch send-message to server via async
	// The send button's gotk-click handler collects the form payload
	// so we trigger a click on it via exec
	ctx.Exec("clickSendButton")
	return nil
}
