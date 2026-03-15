package gotk

import (
	"errors"
	"testing"
)

func TestMux_Handle_Dispatch(t *testing.T) {
	m := NewMux()
	m.Handle("greet", func(ctx *Context) error {
		name := ctx.Payload.String("name")
		ctx.HTML("#greeting", "Hello, "+name)
		return nil
	})

	ins, errMsg := m.dispatch(nil, "greet", "/", map[string]any{"name": "World"})
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if len(ins) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(ins))
	}
	if ins[0].HTML != "Hello, World" {
		t.Errorf("HTML = %q", ins[0].HTML)
	}
}

func TestMux_UnknownCommand(t *testing.T) {
	m := NewMux()
	ins, errMsg := m.dispatch(nil, "nope", "/", nil)
	if errMsg == "" {
		t.Fatal("expected error for unknown command")
	}
	if len(ins) != 1 || ins[0].Op != "exec" {
		t.Errorf("expected exec instruction, got %+v", ins)
	}
}

func TestMux_HandlerError(t *testing.T) {
	m := NewMux()
	m.Handle("fail", func(ctx *Context) error {
		return errors.New("oops")
	})

	ins, errMsg := m.dispatch(nil, "fail", "/", nil)
	if errMsg == "" {
		t.Fatal("expected error")
	}
	if len(ins) != 1 || ins[0].Op != "exec" {
		t.Errorf("expected exec instruction, got %+v", ins)
	}
}
