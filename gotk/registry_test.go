package gotk

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestCommandRegistry_ListCommandsJSON_Empty(t *testing.T) {
	r := NewCommandRegistry()
	got := r.ListCommandsJSON()
	if got != "[]" {
		t.Errorf("ListCommandsJSON() = %q, want %q", got, "[]")
	}
}

func TestCommandRegistry_ListCommandsJSON(t *testing.T) {
	r := NewCommandRegistry()
	r.Register("cmd-a", func(ctx *Context) error { return nil })
	r.Register("cmd-b", func(ctx *Context) error { return nil })

	got := r.ListCommandsJSON()
	var names []string
	if err := json.Unmarshal([]byte(got), &names); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(names) != 2 || names[0] != "cmd-a" || names[1] != "cmd-b" {
		t.Errorf("ListCommandsJSON() = %q, want [cmd-a, cmd-b]", got)
	}
}

func TestCommandRegistry_Register_Overwrite(t *testing.T) {
	r := NewCommandRegistry()
	r.Register("cmd-a", func(ctx *Context) error { return nil })
	r.Register("cmd-a", func(ctx *Context) error {
		ctx.HTML("#target", "replaced")
		return nil
	})

	// Should not duplicate the name
	var names []string
	_ = json.Unmarshal([]byte(r.ListCommandsJSON()), &names)
	if len(names) != 1 {
		t.Errorf("expected 1 name after overwrite, got %d", len(names))
	}

	// Should use the new handler
	result := r.ExecCommandJSON("cmd-a", "{}")
	var res execResult
	_ = json.Unmarshal([]byte(result), &res)
	if len(res.Ins) != 1 || res.Ins[0].HTML != "replaced" {
		t.Errorf("expected replaced handler, got %+v", res.Ins)
	}
}

func TestCommandRegistry_ExecCommandJSON_Success(t *testing.T) {
	r := NewCommandRegistry()
	r.Register("greet", func(ctx *Context) error {
		name := ctx.Payload.String("name")
		ctx.HTML("#output", "Hello, "+name)
		return nil
	})

	result := r.ExecCommandJSON("greet", `{"name":"World"}`)
	var res execResult
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(res.Ins) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(res.Ins))
	}
	if res.Ins[0].Op != "html" || res.Ins[0].HTML != "Hello, World" {
		t.Errorf("ins[0] = %+v", res.Ins[0])
	}
	if len(res.Async) != 0 {
		t.Errorf("expected no async calls, got %d", len(res.Async))
	}
}

func TestCommandRegistry_ExecCommandJSON_WithAsync(t *testing.T) {
	r := NewCommandRegistry()
	r.Register("validate", func(ctx *Context) error {
		ctx.HTML("#status", "Valid")
		ctx.Async("save", map[string]any{"id": 42})
		return nil
	})

	result := r.ExecCommandJSON("validate", `{}`)
	var res execResult
	_ = json.Unmarshal([]byte(result), &res)

	// Instructions: html + cmd (from Async)
	if len(res.Ins) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(res.Ins))
	}
	if res.Ins[1].Op != "cmd" || res.Ins[1].Cmd != "save" {
		t.Errorf("ins[1] = %+v, want cmd:save", res.Ins[1])
	}

	// Async calls
	if len(res.Async) != 1 || res.Async[0].Cmd != "save" {
		t.Errorf("async = %+v, want [save]", res.Async)
	}
}

func TestCommandRegistry_ExecCommandJSON_UnknownCommand(t *testing.T) {
	r := NewCommandRegistry()
	result := r.ExecCommandJSON("nonexistent", `{}`)
	var res execResult
	_ = json.Unmarshal([]byte(result), &res)

	if len(res.Ins) != 1 || res.Ins[0].Op != "exec" {
		t.Errorf("expected console.warn instruction for unknown command, got %+v", res.Ins)
	}
}

func TestCommandRegistry_ExecCommandJSON_HandlerError(t *testing.T) {
	r := NewCommandRegistry()
	r.Register("fail", func(ctx *Context) error {
		return errors.New("something broke")
	})

	result := r.ExecCommandJSON("fail", `{}`)
	var res execResult
	_ = json.Unmarshal([]byte(result), &res)

	if len(res.Ins) != 1 || res.Ins[0].Op != "exec" {
		t.Errorf("expected console.warn instruction for error, got %+v", res.Ins)
	}
}

func TestCommandRegistry_ExecCommandJSON_EmptyPayload(t *testing.T) {
	r := NewCommandRegistry()
	r.Register("noop", func(ctx *Context) error { return nil })

	result := r.ExecCommandJSON("noop", "")
	var res execResult
	_ = json.Unmarshal([]byte(result), &res)

	// Should return empty ins array, not null
	if res.Ins == nil {
		t.Error("expected non-nil ins array")
	}
	if len(res.Ins) != 0 {
		t.Errorf("expected 0 instructions, got %d", len(res.Ins))
	}
}
