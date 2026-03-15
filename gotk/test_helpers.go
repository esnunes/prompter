package gotk

import (
	"html/template"
	"reflect"
	"sync"
)

// TestCommandContext wraps CommandContext for use in tests.
// It captures all events dispatched via the connection's dispatcher.
type TestCommandContext struct {
	*CommandContext
	events []any
}

// NewTestCommandContext creates a TestCommandContext with a capturing dispatcher.
// All events dispatched via Dispatch(ctx.Dispatcher(), event) are recorded
// in Events(). This works by using a Dispatcher with a custom handlers map
// that captures any event type on first dispatch.
func NewTestCommandContext() *TestCommandContext {
	d := NewDispatcher()
	vctx := NewViewContext(nil)
	tc := &TestCommandContext{
		CommandContext: &CommandContext{
			Payload:      NewPayload(nil),
			view:         &viewEntry{Dispatcher: d, ViewCtx: vctx},
			connRegistry: &sync.Map{},
		},
	}
	return tc
}

// SetPayload sets the command payload from a map.
func (tc *TestCommandContext) SetPayload(data map[string]any) {
	tc.CommandContext.Payload = NewPayload(data)
}

// SetTemplates configures templates for rendering in tests.
func (tc *TestCommandContext) SetTemplates(t *template.Template) {
	tc.CommandContext.templates = t
}

// Events returns all events captured by the dispatcher.
func (tc *TestCommandContext) Events() []any {
	return tc.events
}

// RegisterCapture registers a capturing handler for event type T on the test
// context's dispatcher. All dispatched events of type T are appended to Events().
//
// Usage:
//
//	tc := gotk.NewTestCommandContext()
//	gotk.RegisterCapture[MyEvent](tc)
//	handler(tc.CommandContext)
//	events := tc.Events()
func RegisterCapture[T any](tc *TestCommandContext) {
	Register(tc.CommandContext.view.Dispatcher, Handler[T](func(event T) {
		tc.events = append(tc.events, event)
	}))
}

// TestViewContext wraps ViewContext for use in tests.
type TestViewContext struct {
	*ViewContext
}

// NewTestViewContext creates a TestViewContext for testing view handlers.
func NewTestViewContext() *TestViewContext {
	return &TestViewContext{
		ViewContext: NewViewContext(nil),
	}
}

// NewTestViewContextWithTemplates creates a TestViewContext with templates configured.
func NewTestViewContextWithTemplates(t *template.Template) *TestViewContext {
	return &TestViewContext{
		ViewContext: NewViewContext(t),
	}
}

// EventsOfType returns all captured events that match type T.
// Useful when multiple event types are captured and you want to filter.
func EventsOfType[T any](tc *TestCommandContext) []T {
	var result []T
	targetType := reflect.TypeOf((*T)(nil)).Elem()
	for _, e := range tc.events {
		if reflect.TypeOf(e) == targetType {
			result = append(result, e.(T))
		}
	}
	return result
}
