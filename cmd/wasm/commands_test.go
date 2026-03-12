package main

import (
	"testing"

	"github.com/esnunes/prompter/gotk"
)

func TestScrollConversation(t *testing.T) {
	ctx := gotk.NewTestContext()
	if err := ScrollConversation(ctx.Context); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ins := ctx.Instructions()
	if len(ins) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(ins))
	}
	if ins[0].Op != "exec" || ins[0].Name != "scrollConversation" {
		t.Errorf("ins[0] = %+v, want exec:scrollConversation", ins[0])
	}
}

func TestUpdateFormVisibility_WithQuestions(t *testing.T) {
	ctx := gotk.NewTestContext()
	ctx.SetPayload(map[string]any{"has_questions": true})
	if err := UpdateFormVisibility(ctx.Context); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ins := ctx.Instructions()
	if len(ins) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(ins))
	}
	if ins[0].Op != "attr-set" || ins[0].Target != "#message-form" || ins[0].Value != "display:none" {
		t.Errorf("ins[0] = %+v, want attr-set style=display:none", ins[0])
	}
}

func TestUpdateFormVisibility_WithoutQuestions(t *testing.T) {
	ctx := gotk.NewTestContext()
	ctx.SetPayload(map[string]any{"has_questions": false})
	if err := UpdateFormVisibility(ctx.Context); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ins := ctx.Instructions()
	if len(ins) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(ins))
	}
	if ins[0].Op != "attr-remove" || ins[0].Target != "#message-form" || ins[0].Attr != "style" {
		t.Errorf("ins[0] = %+v, want attr-remove style on #message-form", ins[0])
	}
}

func TestCheckEnter_Enter(t *testing.T) {
	ctx := gotk.NewTestContext()
	ctx.SetPayload(map[string]any{
		"_event": map[string]any{
			"key":         "Enter",
			"shiftKey":    false,
			"isComposing": false,
		},
	})
	if err := CheckEnter(ctx.Context); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ins := ctx.Instructions()
	if len(ins) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(ins))
	}
	if ins[0].Op != "exec" || ins[0].Name != "clickSendButton" {
		t.Errorf("ins[0] = %+v, want exec:clickSendButton", ins[0])
	}
}

func TestCheckEnter_ShiftEnter(t *testing.T) {
	ctx := gotk.NewTestContext()
	ctx.SetPayload(map[string]any{
		"_event": map[string]any{
			"key":         "Enter",
			"shiftKey":    true,
			"isComposing": false,
		},
	})
	if err := CheckEnter(ctx.Context); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ins := ctx.Instructions()
	if len(ins) != 0 {
		t.Errorf("expected 0 instructions for Shift+Enter, got %d", len(ins))
	}
}

func TestCheckEnter_IMEComposing(t *testing.T) {
	ctx := gotk.NewTestContext()
	ctx.SetPayload(map[string]any{
		"_event": map[string]any{
			"key":         "Enter",
			"shiftKey":    false,
			"isComposing": true,
		},
	})
	if err := CheckEnter(ctx.Context); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ins := ctx.Instructions()
	if len(ins) != 0 {
		t.Errorf("expected 0 instructions for IME composing, got %d", len(ins))
	}
}

func TestCheckEnter_OtherKey(t *testing.T) {
	ctx := gotk.NewTestContext()
	ctx.SetPayload(map[string]any{
		"_event": map[string]any{
			"key":      "a",
			"shiftKey": false,
		},
	})
	if err := CheckEnter(ctx.Context); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ins := ctx.Instructions()
	if len(ins) != 0 {
		t.Errorf("expected 0 instructions for non-Enter key, got %d", len(ins))
	}
}

func TestCheckEnter_NoEvent(t *testing.T) {
	ctx := gotk.NewTestContext()
	ctx.SetPayload(map[string]any{})
	if err := CheckEnter(ctx.Context); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ins := ctx.Instructions()
	if len(ins) != 0 {
		t.Errorf("expected 0 instructions when no event, got %d", len(ins))
	}
}
