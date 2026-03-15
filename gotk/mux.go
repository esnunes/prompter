package gotk

import (
	"html/template"
	"log/slog"
	"sync"
)

// ViewFactory creates a view for a given URL. Called lazily when a command
// arrives for a URL that hasn't been seen on this connection yet.
type ViewFactory func(url string, d *Dispatcher, vctx *ViewContext)

// NavigateFunc is the callback for handling navigate commands (legacy).
type NavigateFunc func(ctx *Context, url string) error

// ConnectFunc is called when a new WebSocket connection is established.
type ConnectFunc func(conn *Conn)

// DisconnectFunc is called when a WebSocket connection is closed.
type DisconnectFunc func(conn *Conn)

// Mux routes commands to handlers.
type Mux struct {
	mu           sync.RWMutex
	handlers     map[string]HandlerFunc        // legacy handlers
	cmdHandlers  map[string]CommandHandlerFunc  // new view-aware handlers
	navigateFn   NavigateFunc
	connectFn    ConnectFunc
	disconnFn    DisconnectFunc
	templates    *template.Template
	viewFactory  ViewFactory
	connRegistry sync.Map // conn ID (int64) → *ConnEntry
}

// NewMux creates a new command router.
func NewMux() *Mux {
	return &Mux{
		handlers:    make(map[string]HandlerFunc),
		cmdHandlers: make(map[string]CommandHandlerFunc),
	}
}

// Handle registers a legacy command handler.
func (m *Mux) Handle(name string, handler HandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[name] = handler
}

// HandleCommand registers a command handler using the new architecture.
// Commands registered here receive a CommandContext and emit events via
// the connection's Dispatcher.
func (m *Mux) HandleCommand(name string, handler CommandHandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cmdHandlers[name] = handler
}

// HandleView registers a view factory. Called lazily when a command arrives
// for a URL that hasn't been seen on this connection yet.
func (m *Mux) HandleView(factory ViewFactory) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.viewFactory = factory
}

// HandleNavigate registers the callback for navigate commands (legacy).
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

// ConnRegistry returns the connection registry for external iteration.
// Values are *ConnEntry keyed by conn ID (int64).
func (m *Mux) ConnRegistry() *sync.Map {
	return &m.connRegistry
}

// connIDFor returns the connection ID, or -1 if conn is nil (test contexts).
func connIDFor(conn *Conn) int64 {
	if conn == nil {
		return -1
	}
	return conn.ID()
}

// dispatch routes a command to the appropriate handler.
// The url parameter is the client's current page URL, sent with every message.
// Returns instructions and an optional error string.
func (m *Mux) dispatch(conn *Conn, cmd string, url string, payload map[string]any) ([]Instruction, string) {
	m.mu.RLock()
	legacyHandler, legacyOK := m.handlers[cmd]
	cmdHandler, cmdOK := m.cmdHandlers[cmd]
	tmpl := m.templates
	viewFactory := m.viewFactory
	m.mu.RUnlock()

	connID := connIDFor(conn)
	slog.Debug("gotk: command received", "cmd", cmd, "conn", connID, "url", url)

	// New architecture: CommandHandlerFunc
	if cmdOK {
		ins, errStr := m.dispatchCommand(conn, cmd, url, cmdHandler, payload, tmpl, viewFactory)
		if errStr != "" {
			slog.Error("gotk: command failed", "cmd", cmd, "conn", connID, "error", errStr)
		} else {
			slog.Debug("gotk: command completed", "cmd", cmd, "conn", connID, "instructions", len(ins))
		}
		return ins, errStr
	}

	// Legacy: HandlerFunc
	if legacyOK {
		ins, errStr := m.dispatchLegacy(conn, legacyHandler, payload, tmpl)
		if errStr != "" {
			slog.Error("gotk: command failed", "cmd", cmd, "conn", connID, "error", errStr)
		} else {
			slog.Debug("gotk: command completed", "cmd", cmd, "conn", connID, "instructions", len(ins))
		}
		return ins, errStr
	}

	errMsg := "unknown command: " + cmd
	slog.Warn("gotk: unknown command", "cmd", cmd, "conn", connID)
	return []Instruction{{
		Op:   "exec",
		Name: "console.warn",
		Args: map[string]any{"message": errMsg},
	}}, errMsg
}

// dispatchCommand handles a command registered via HandleCommand.
// It lazily creates or reuses the view for the given URL.
func (m *Mux) dispatchCommand(conn *Conn, cmd string, url string, handler CommandHandlerFunc, payload map[string]any, tmpl *template.Template, viewFactory ViewFactory) ([]Instruction, string) {
	v, ok := m.connRegistry.Load(conn.ID())
	if !ok {
		return nil, "connection not found"
	}
	entry := v.(*ConnEntry)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	// Update last URL and get or create the view for this URL
	entry.lastURL = url
	view := entry.getOrCreateView(url, viewFactory, tmpl)

	ctx := &CommandContext{
		Payload:      NewPayload(payload),
		conn:         conn,
		view:         view,
		connRegistry: &m.connRegistry,
		templates:    tmpl,
		viewFactory:  viewFactory,
	}

	if err := handler(ctx); err != nil {
		errMsg := "command error: " + err.Error()
		return []Instruction{{
			Op:   "exec",
			Name: "console.warn",
			Args: map[string]any{"message": errMsg},
		}}, errMsg
	}

	// Collect instructions produced by view handlers (triggered by events)
	ins := view.ViewCtx.Instructions()
	var copied []Instruction
	if len(ins) > 0 {
		copied = make([]Instruction, len(ins))
		copy(copied, ins)
		view.ViewCtx.Reset()
	}

	// Append navigate instruction if requested
	if ctx.navigateURL != "" {
		copied = append(copied, Instruction{Op: "navigate", URL: ctx.navigateURL})
	}

	return copied, ""
}

// dispatchLegacy handles a command registered via Handle (backward compat).
func (m *Mux) dispatchLegacy(conn *Conn, handler HandlerFunc, payload map[string]any, tmpl *template.Template) ([]Instruction, string) {
	ctx := &Context{
		Payload: NewPayload(payload),
	}
	ctx.setTemplates(tmpl)
	ctx.setConn(conn)

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
