package gotk

import "html/template"

// TestContext is used in tests. Provides inspection methods
// not available on the production Context.
type TestContext struct {
	*Context
}

// NewTestContext creates a new TestContext for use in tests.
func NewTestContext() *TestContext {
	return &TestContext{
		Context: &Context{
			Payload: NewPayload(nil),
		},
	}
}

// SetPayload sets the context payload from a map.
func (tc *TestContext) SetPayload(data map[string]any) {
	tc.Context.Payload = NewPayload(data)
}

// SetTemplates configures templates for ctx.Render in tests.
func (tc *TestContext) SetTemplates(t *template.Template) {
	tc.Context.setTemplates(t)
}

// Instructions returns all instructions produced by handlers.
func (tc *TestContext) Instructions() []Instruction {
	return tc.Context.instructions
}
