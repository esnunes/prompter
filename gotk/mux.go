package gotk

import (
	"html/template"
	"sync"
)

// NavigateFunc is the callback for handling navigate commands.
type NavigateFunc func(ctx *Context, url string) error

// ConnectFunc is called when a new WebSocket connection is established.
type ConnectFunc func(conn *Conn)

// DisconnectFunc is called when a WebSocket connection is closed.
type DisconnectFunc func(conn *Conn)

// Mux routes commands to handlers.
type Mux struct {
	mu         sync.RWMutex
	handlers   map[string]HandlerFunc
	navigateFn NavigateFunc
	connectFn  ConnectFunc
	disconnFn  DisconnectFunc
	templates  *template.Template
}

// NewMux creates a new command router.
func NewMux() *Mux {
	return &Mux{
		handlers: make(map[string]HandlerFunc),
	}
}

// Handle registers a command handler.
func (m *Mux) Handle(name string, handler HandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[name] = handler
}

// HandleNavigate registers the callback for navigate commands.
func (m *Mux) HandleNavigate(fn NavigateFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.navigateFn = fn
}

// HandleConnect registers a callback for new connections.
func (m *Mux) HandleConnect(fn ConnectFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectFn = fn
}

// HandleDisconnect registers a callback for closed connections.
func (m *Mux) HandleDisconnect(fn DisconnectFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disconnFn = fn
}

// SetTemplates configures the template engine used by ctx.Render.
func (m *Mux) SetTemplates(t *template.Template) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.templates = t
}

// dispatch routes a command to the appropriate handler.
// Returns instructions and an optional error string.
func (m *Mux) dispatch(cmd string, payload map[string]any) ([]Instruction, string) {
	m.mu.RLock()
	handler, ok := m.handlers[cmd]
	navigateFn := m.navigateFn
	tmpl := m.templates
	m.mu.RUnlock()

	ctx := &Context{
		Payload: NewPayload(payload),
	}
	ctx.setTemplates(tmpl)

	// Built-in navigate command
	if cmd == "navigate" {
		if navigateFn == nil {
			return []Instruction{{
				Op:   "exec",
				Name: "console.warn",
				Args: map[string]any{"message": "no navigate handler registered"},
			}}, "no navigate handler registered"
		}
		url := ctx.Payload.String("url")
		if err := navigateFn(ctx, url); err != nil {
			return []Instruction{{
				Op:   "exec",
				Name: "console.warn",
				Args: map[string]any{"message": "navigate error: " + err.Error()},
			}}, "navigate error: " + err.Error()
		}
		return ctx.instructions, ""
	}

	if !ok {
		errMsg := "unknown command: " + cmd
		return []Instruction{{
			Op:   "exec",
			Name: "console.warn",
			Args: map[string]any{"message": errMsg},
		}}, errMsg
	}

	if err := handler(ctx); err != nil {
		errMsg := "command error: " + err.Error()
		return []Instruction{{
			Op:   "exec",
			Name: "console.warn",
			Args: map[string]any{"message": errMsg},
		}}, errMsg
	}

	return ctx.instructions, ""
}
