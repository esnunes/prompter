package gotk

import (
	"html/template"
	"sync"
)

// CommandHandlerFunc is the signature for command handlers using the new architecture.
type CommandHandlerFunc func(ctx *CommandContext) error

// viewEntry holds the dispatcher and view context for a single URL within a connection.
type viewEntry struct {
	Dispatcher *Dispatcher
	ViewCtx    *ViewContext
}

// ConnEntry tracks the state of a single WebSocket connection.
// It holds a map of views keyed by URL, lazily created via the ViewFactory.
type ConnEntry struct {
	Conn    *Conn
	views   map[string]*viewEntry // URL → view
	lastURL string                // last URL seen from client
	mu      sync.Mutex            // protects views, lastURL, and ViewCtx instruction writes during push
}

// getOrCreateView returns the viewEntry for the given URL, creating it lazily
// if it doesn't exist yet. Must be called with entry.mu held.
func (e *ConnEntry) getOrCreateView(url string, factory ViewFactory, tmpl *template.Template) *viewEntry {
	if e.views == nil {
		e.views = make(map[string]*viewEntry)
	}
	if v, ok := e.views[url]; ok {
		return v
	}
	d := NewDispatcher()
	vctx := NewViewContext(tmpl)
	if factory != nil {
		factory(url, d, vctx)
	}
	v := &viewEntry{Dispatcher: d, ViewCtx: vctx}
	e.views[url] = v
	return v
}

// activeView returns the viewEntry for the last-seen URL, or nil if none.
// Must be called with entry.mu held.
func (e *ConnEntry) activeView() *viewEntry {
	if e.lastURL == "" || e.views == nil {
		return nil
	}
	return e.views[e.lastURL]
}

// CommandContext is passed to command handlers registered via HandleCommand.
// It provides access to the command payload, the connection's dispatcher for
// emitting events, and the ability to spawn background tasks.
type CommandContext struct {
	Payload Payload

	conn         *Conn
	view         *viewEntry
	connRegistry *sync.Map // all connections: conn ID (int64) → *ConnEntry
	templates    *template.Template
	viewFactory  ViewFactory
	navigateURL  string // set by Navigate(), consumed by dispatchCommand
}

// Dispatcher returns the current view's dispatcher for emitting events.
func (c *CommandContext) Dispatcher() *Dispatcher { return c.view.Dispatcher }

// Conn returns the originating WebSocket connection.
func (c *CommandContext) Conn() *Conn { return c.conn }

// Navigate sets a pending navigation URL. The navigate instruction will be
// included in the response after all view instructions.
func (c *CommandContext) Navigate(url string) {
	c.navigateURL = url
}

// NavigateURL returns the pending navigation URL, if any.
// Used by tests to verify navigation was requested.
func (c *CommandContext) NavigateURL() string {
	return c.navigateURL
}

// NewTask creates a TaskContext for use in background goroutines.
// The TaskContext provides access to the originating connection's dispatcher
// and to all connections for broadcast.
func (c *CommandContext) NewTask() *TaskContext {
	return &TaskContext{
		conn:         c.conn,
		connRegistry: c.connRegistry,
		templates:    c.templates,
		viewFactory:  c.viewFactory,
	}
}
