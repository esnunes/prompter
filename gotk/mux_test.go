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

	ins, errMsg := m.dispatch("greet", map[string]any{"name": "World"})
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
	ins, errMsg := m.dispatch("nope", nil)
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

	ins, errMsg := m.dispatch("fail", nil)
	if errMsg == "" {
		t.Fatal("expected error")
	}
	if len(ins) != 1 || ins[0].Op != "exec" {
		t.Errorf("expected exec instruction, got %+v", ins)
	}
}

func TestMux_Navigate(t *testing.T) {
	m := NewMux()
	m.HandleNavigate(func(ctx *Context, url string) error {
		ctx.HTML("#content", "<h1>Page: "+url+"</h1>")
		ctx.Navigate(url)
		return nil
	})

	ins, errMsg := m.dispatch("navigate", map[string]any{"url": "/settings"})
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if len(ins) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(ins))
	}
	if ins[0].Op != "html" {
		t.Errorf("ins[0] = %+v", ins[0])
	}
	if ins[1].Op != "navigate" || ins[1].URL != "/settings" {
		t.Errorf("ins[1] = %+v", ins[1])
	}
}

func TestMux_Navigate_NoHandler(t *testing.T) {
	m := NewMux()
	_, errMsg := m.dispatch("navigate", map[string]any{"url": "/x"})
	if errMsg == "" {
		t.Fatal("expected error when no navigate handler")
	}
}
