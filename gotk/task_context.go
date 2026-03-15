package gotk

import (
	"html/template"
	"log"
	"log/slog"
	"reflect"
	"sync"
)

// noopDispatcher is a singleton empty dispatcher returned for stale connections.
// Avoids allocating a new Dispatcher on every Dispatcher() call.
var noopDispatcher = NewDispatcher()

// TaskContext is available to background goroutines. It provides access
// to the originating connection's dispatcher (targeted push) and to all
// connections' dispatchers (broadcast).
type TaskContext struct {
	conn         *Conn
	connRegistry *sync.Map          // all connections: conn ID (int64) → *ConnEntry
	templates    *template.Template // for creating ViewContext during push
	viewFactory  ViewFactory
}

// DispatchTo dispatches an event to the originating connection's active view,
// flushing any resulting instructions to the client.
func (t *TaskContext) DispatchTo(event any) {
	eventName := reflect.TypeOf(event).Name()
	v, ok := t.connRegistry.Load(t.conn.ID())
	if !ok {
		slog.Debug("gotk: task dispatch-to skipped (conn gone)", "event", eventName, "conn", t.conn.ID())
		return // connection gone
	}
	slog.Debug("gotk: task dispatch-to", "event", eventName, "conn", t.conn.ID(), "payload", event)
	entry := v.(*ConnEntry)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	view := entry.activeView()
	if view == nil {
		slog.Debug("gotk: task dispatch-to skipped (no active view)", "event", eventName, "conn", t.conn.ID())
		return
	}

	view.Dispatcher.dispatchAny(event)
	t.flushView(entry.Conn, view)
}

// Broadcast dispatches an event to all connected clients' active views,
// flushing resulting instructions to each client.
func (t *TaskContext) Broadcast(event any) {
	eventName := reflect.TypeOf(event).Name()
	slog.Debug("gotk: task broadcast", "event", eventName, "payload", event)
	t.connRegistry.Range(func(_, v any) bool {
		entry := v.(*ConnEntry)
		entry.mu.Lock()

		view := entry.activeView()
		if view != nil {
			view.Dispatcher.dispatchAny(event)
			t.flushView(entry.Conn, view)
		}

		entry.mu.Unlock()
		return true
	})
}

// flushView sends any accumulated instructions from the view's ViewContext
// to the client, then resets the ViewContext.
func (t *TaskContext) flushView(conn *Conn, view *viewEntry) {
	ins := view.ViewCtx.Instructions()
	if len(ins) == 0 {
		return
	}
	// Copy instructions before reset (Reset reuses the slice)
	copied := make([]Instruction, len(ins))
	copy(copied, ins)
	view.ViewCtx.Reset()

	if err := conn.Push(copied); err != nil {
		log.Printf("gotk: task push error (conn %d): %v", conn.ID(), err)
	}
}
