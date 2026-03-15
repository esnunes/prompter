package gotk

import "testing"

type testEventA struct {
	Value string
}

type testEventB struct {
	Count int
}

type testEventUnregistered struct{}

func TestDispatcher_RegisterAndDispatch(t *testing.T) {
	d := NewDispatcher()
	var received string

	Register(d, func(e testEventA) {
		received = e.Value
	})

	Dispatch(d, testEventA{Value: "hello"})

	if received != "hello" {
		t.Errorf("received = %q, want %q", received, "hello")
	}
}

func TestDispatcher_MultipleEventTypes(t *testing.T) {
	d := NewDispatcher()
	var aValue string
	var bCount int

	Register(d, func(e testEventA) {
		aValue = e.Value
	})
	Register(d, func(e testEventB) {
		bCount = e.Count
	})

	Dispatch(d, testEventA{Value: "world"})
	Dispatch(d, testEventB{Count: 42})

	if aValue != "world" {
		t.Errorf("aValue = %q, want %q", aValue, "world")
	}
	if bCount != 42 {
		t.Errorf("bCount = %d, want %d", bCount, 42)
	}
}

func TestDispatcher_UnregisteredEventIsNoop(t *testing.T) {
	d := NewDispatcher()

	// Should not panic
	Dispatch(d, testEventUnregistered{})
}

func TestDispatcher_LastRegistrationWins(t *testing.T) {
	d := NewDispatcher()
	var received string

	Register(d, func(e testEventA) {
		received = "first"
	})
	Register(d, func(e testEventA) {
		received = "second"
	})

	Dispatch(d, testEventA{Value: "test"})

	if received != "second" {
		t.Errorf("received = %q, want %q", received, "second")
	}
}

func TestDispatcher_DispatchAny(t *testing.T) {
	d := NewDispatcher()
	var received string

	Register(d, func(e testEventA) {
		received = e.Value
	})

	// dispatchAny uses reflection — same result as Dispatch
	d.dispatchAny(testEventA{Value: "reflected"})

	if received != "reflected" {
		t.Errorf("received = %q, want %q", received, "reflected")
	}
}

func TestDispatcher_DispatchAny_UnregisteredIsNoop(t *testing.T) {
	d := NewDispatcher()

	// Should not panic
	d.dispatchAny(testEventUnregistered{})
}

func TestDispatcher_EmptyDispatcher(t *testing.T) {
	d := NewDispatcher()

	// Dispatching on empty dispatcher should not panic
	Dispatch(d, testEventA{Value: "test"})
	d.dispatchAny(testEventB{Count: 1})
}
