package gotk

import (
	"html/template"
	"testing"
)

func TestViewContext_HTML(t *testing.T) {
	vc := NewTestViewContext()
	vc.HTML("#target", "<p>Hello</p>")
	vc.HTML("#list", "<li>Item</li>", Append)

	ins := vc.Instructions()
	if len(ins) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(ins))
	}
	if ins[0].Op != "html" || ins[0].Target != "#target" || ins[0].Mode != Replace {
		t.Errorf("ins[0] = %+v", ins[0])
	}
	if ins[1].Mode != Append {
		t.Errorf("ins[1].Mode = %q, want append", ins[1].Mode)
	}
}

func TestViewContext_Reset(t *testing.T) {
	vc := NewTestViewContext()
	vc.HTML("#target", "<p>Hello</p>")

	if len(vc.Instructions()) != 1 {
		t.Fatal("expected 1 instruction before reset")
	}

	vc.Reset()

	if len(vc.Instructions()) != 0 {
		t.Errorf("expected 0 instructions after reset, got %d", len(vc.Instructions()))
	}
}

func TestViewContext_Render(t *testing.T) {
	tmpl := template.Must(template.New("item").Parse(`<li>{{.Name}}</li>`))
	vc := NewTestViewContextWithTemplates(tmpl)

	html := vc.Render("item", struct{ Name string }{"Alice"})
	if html != "<li>Alice</li>" {
		t.Errorf("Render = %q", html)
	}
}

func TestViewContext_Render_NoTemplates(t *testing.T) {
	vc := NewTestViewContext()
	html := vc.Render("anything", nil)
	if html != "" {
		t.Errorf("expected empty, got %q", html)
	}
}

func TestViewContext_AllInstructions(t *testing.T) {
	vc := NewTestViewContext()

	vc.HTML("#a", "<div>A</div>")
	vc.Remove("#b")
	vc.Template("#tpl", "#target")
	vc.Populate("#form", map[string]any{"name": "Bob"})
	vc.Navigate("/page")
	vc.AttrSet("#el", "hidden")
	vc.AttrRemove("#el", "hidden")
	vc.SetValue("#input", "value")
	vc.Dispatch("#form", "submit")
	vc.Focus("#input")
	vc.Exec("doThing")
	vc.Error("#err", "bad")

	ins := vc.Instructions()
	if len(ins) != 12 {
		t.Fatalf("expected 12 instructions, got %d", len(ins))
	}

	ops := make([]string, len(ins))
	for i, in := range ins {
		ops[i] = in.Op
	}

	expected := []string{
		"html", "html", "template", "populate", "navigate",
		"attr-set", "attr-remove", "set-value", "dispatch", "focus",
		"exec", "html",
	}
	for i, op := range expected {
		if ops[i] != op {
			t.Errorf("ins[%d].Op = %q, want %q", i, ops[i], op)
		}
	}
}

func TestViewContext_EventDriven(t *testing.T) {
	// Simulates the full View pattern: dispatcher → view handler → instructions
	type TodoCreated struct {
		Title string
	}

	tmpl := template.Must(template.New("todo-item").Parse(`<li>{{.Title}}</li>`))
	vc := NewTestViewContextWithTemplates(tmpl)
	d := NewDispatcher()

	// Register view handler
	Register(d, func(e TodoCreated) {
		vc.HTML("#list", vc.Render("todo-item", e), Append)
		vc.SetValue("#title", "")
		vc.Focus("#title")
	})

	// Dispatch event (simulating what a command would do)
	Dispatch(d, TodoCreated{Title: "Buy milk"})

	ins := vc.Instructions()
	if len(ins) != 3 {
		t.Fatalf("expected 3 instructions, got %d", len(ins))
	}
	if ins[0].HTML != "<li>Buy milk</li>" {
		t.Errorf("ins[0].HTML = %q", ins[0].HTML)
	}
	if ins[1].Op != "set-value" {
		t.Errorf("ins[1].Op = %q", ins[1].Op)
	}
	if ins[2].Op != "focus" {
		t.Errorf("ins[2].Op = %q", ins[2].Op)
	}
}
