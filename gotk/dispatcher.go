package gotk

import (
	"log/slog"
	"reflect"
)

// Handler is a typed event handler function.
type Handler[T any] func(T)

// Dispatcher routes typed events to registered handlers.
// Each connection has its own Dispatcher instance.
type Dispatcher struct {
	handlers map[any]any
}

// NewDispatcher creates a new Dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: make(map[any]any)}
}

// Register registers a typed event handler. Only one handler per event type
// is allowed — registering again overwrites the previous handler.
func Register[T any](d *Dispatcher, h Handler[T]) {
	key := reflect.TypeOf((*T)(nil)).Elem()
	d.handlers[key] = h
}

// Dispatch dispatches a typed event to the registered handler, if any.
// If no handler is registered for the event type, the call is a no-op.
func Dispatch[T any](d *Dispatcher, event T) {
	key := reflect.TypeOf((*T)(nil)).Elem()
	if h, ok := d.handlers[key]; ok {
		slog.Debug("gotk: event dispatched", "event", key.Name(), "payload", event)
		h.(Handler[T])(event)
	}
}

// dispatchAny dispatches an event of unknown type using reflection.
// Used by TaskContext.Broadcast where the generic type is erased.
func (d *Dispatcher) dispatchAny(event any) {
	key := reflect.TypeOf(event)
	if h, ok := d.handlers[key]; ok {
		slog.Debug("gotk: event dispatched (async)", "event", key.Name(), "payload", event)
		reflect.ValueOf(h).Call([]reflect.Value{reflect.ValueOf(event)})
	}
}
