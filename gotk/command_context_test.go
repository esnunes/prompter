package gotk

import (
	"html/template"
	"testing"
)

type cmdTestEvent struct {
	Message string
}

type cmdTestOtherEvent struct {
	Count int
}

func TestCommandContext_DispatchCaptured(t *testing.T) {
	tc := NewTestCommandContext()
	RegisterCapture[cmdTestEvent](tc)

	// Simulate a command handler
	handler := func(ctx *CommandContext) error {
		Dispatch(ctx.Dispatcher(), cmdTestEvent{Message: "hello"})
		return nil
	}

	if err := handler(tc.CommandContext); err != nil {
		t.Fatal(err)
	}

	events := tc.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if e, ok := events[0].(cmdTestEvent); !ok || e.Message != "hello" {
		t.Errorf("event = %+v", events[0])
	}
}

func TestCommandContext_MultipleEventTypes(t *testing.T) {
	tc := NewTestCommandContext()
	RegisterCapture[cmdTestEvent](tc)
	RegisterCapture[cmdTestOtherEvent](tc)

	handler := func(ctx *CommandContext) error {
		Dispatch(ctx.Dispatcher(), cmdTestEvent{Message: "first"})
		Dispatch(ctx.Dispatcher(), cmdTestOtherEvent{Count: 42})
		Dispatch(ctx.Dispatcher(), cmdTestEvent{Message: "second"})
		return nil
	}

	handler(tc.CommandContext)

	events := tc.Events()
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Filter by type
	cmdEvents := EventsOfType[cmdTestEvent](tc)
	if len(cmdEvents) != 2 {
		t.Fatalf("expected 2 cmdTestEvent, got %d", len(cmdEvents))
	}
	if cmdEvents[0].Message != "first" || cmdEvents[1].Message != "second" {
		t.Errorf("cmdEvents = %+v", cmdEvents)
	}

	otherEvents := EventsOfType[cmdTestOtherEvent](tc)
	if len(otherEvents) != 1 || otherEvents[0].Count != 42 {
		t.Errorf("otherEvents = %+v", otherEvents)
	}
}

func TestCommandContext_SetPayload(t *testing.T) {
	tc := NewTestCommandContext()
	tc.SetPayload(map[string]any{"name": "Alice"})

	if tc.CommandContext.Payload.String("name") != "Alice" {
		t.Errorf("payload name = %q", tc.CommandContext.Payload.String("name"))
	}
}

func TestCommandContext_NewTask(t *testing.T) {
	tc := NewTestCommandContext()
	task := tc.CommandContext.NewTask()

	if task == nil {
		t.Fatal("NewTask returned nil")
	}
	if task.connRegistry != tc.CommandContext.connRegistry {
		t.Error("task connRegistry should match command context")
	}
}

func TestCommandContext_SetTemplates(t *testing.T) {
	tmpl := template.Must(template.New("item").Parse(`<li>{{.}}</li>`))
	tc := NewTestCommandContext()
	tc.SetTemplates(tmpl)

	if tc.CommandContext.templates != tmpl {
		t.Error("templates not set")
	}
}
