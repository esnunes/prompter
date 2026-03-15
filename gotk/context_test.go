package gotk

import (
	"html/template"
	"testing"
)

func TestContext_HTML(t *testing.T) {
	tc := NewTestContext()
	tc.HTML("#target", "<p>Hello</p>")
	tc.HTML("#list", "<li>Item</li>", Append)

	ins := tc.Instructions()
	if len(ins) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(ins))
	}

	if ins[0].Op != "html" || ins[0].Target != "#target" || ins[0].Mode != Replace {
		t.Errorf("ins[0] = %+v, want html replace #target", ins[0])
	}
	if ins[1].Mode != Append {
		t.Errorf("ins[1].Mode = %q, want append", ins[1].Mode)
	}
}

func TestContext_Remove(t *testing.T) {
	tc := NewTestContext()
	tc.Remove("#item-1")

	ins := tc.Instructions()
	if len(ins) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(ins))
	}
	if ins[0].Op != "html" || ins[0].Mode != Remove || ins[0].Target != "#item-1" {
		t.Errorf("ins[0] = %+v", ins[0])
	}
}

func TestContext_AttrSetRemove(t *testing.T) {
	tc := NewTestContext()
	tc.AttrSet("#el", "hidden")
	tc.AttrSet("#el", "data-id", "42")
	tc.AttrRemove("#el", "hidden")

	ins := tc.Instructions()
	if len(ins) != 3 {
		t.Fatalf("expected 3, got %d", len(ins))
	}
	if ins[0].Op != "attr-set" || ins[0].Attr != "hidden" || ins[0].Value != "" {
		t.Errorf("ins[0] = %+v", ins[0])
	}
	if ins[1].Value != "42" {
		t.Errorf("ins[1].Value = %q, want 42", ins[1].Value)
	}
	if ins[2].Op != "attr-remove" {
		t.Errorf("ins[2].Op = %q, want attr-remove", ins[2].Op)
	}
}

func TestContext_Navigate(t *testing.T) {
	tc := NewTestContext()
	tc.Navigate("/settings")
	tc.Navigate("/todos", "#content", "<div>Todos</div>")

	ins := tc.Instructions()
	if ins[0].URL != "/settings" {
		t.Errorf("ins[0].URL = %q", ins[0].URL)
	}
	if ins[1].Target != "#content" || ins[1].HTML != "<div>Todos</div>" {
		t.Errorf("ins[1] = %+v", ins[1])
	}
}

func TestContext_Dispatch(t *testing.T) {
	tc := NewTestContext()
	tc.Dispatch("#form", "reset")
	tc.Dispatch("#el", "custom", map[string]any{"key": "val"})

	ins := tc.Instructions()
	if ins[0].Event != "reset" {
		t.Errorf("ins[0].Event = %q", ins[0].Event)
	}
	if ins[1].Detail["key"] != "val" {
		t.Errorf("ins[1].Detail = %v", ins[1].Detail)
	}
}

func TestContext_Focus(t *testing.T) {
	tc := NewTestContext()
	tc.Focus("#input")
	ins := tc.Instructions()
	if ins[0].Op != "focus" || ins[0].Target != "#input" {
		t.Errorf("ins[0] = %+v", ins[0])
	}
}

func TestContext_Exec(t *testing.T) {
	tc := NewTestContext()
	tc.Exec("lockScroll")
	tc.Exec("doThing", map[string]any{"x": 1})

	ins := tc.Instructions()
	if ins[0].Name != "lockScroll" {
		t.Errorf("ins[0].Name = %q", ins[0].Name)
	}
	if ins[1].Args["x"] != 1 {
		t.Errorf("ins[1].Args = %v", ins[1].Args)
	}
}

func TestContext_Error(t *testing.T) {
	tc := NewTestContext()
	tc.Error("#body", "Something failed")

	ins := tc.Instructions()
	if ins[0].Op != "html" || ins[0].Target != "#body" {
		t.Errorf("ins[0] = %+v", ins[0])
	}
	expected := `<div class="gotk-error">Something failed</div>`
	if ins[0].HTML != expected {
		t.Errorf("HTML = %q, want %q", ins[0].HTML, expected)
	}
}

func TestContext_Error_EscapesHTML(t *testing.T) {
	tc := NewTestContext()
	tc.Error("#x", "<script>alert('xss')</script>")
	ins := tc.Instructions()
	if ins[0].HTML == `<div class="gotk-error"><script>alert('xss')</script></div>` {
		t.Error("Error should escape HTML in message")
	}
}

func TestContext_Render(t *testing.T) {
	tmpl := template.Must(template.New("item").Parse(`<li>{{.Name}}</li>`))
	tc := NewTestContext()
	tc.SetTemplates(tmpl)

	html := tc.Render("item", struct{ Name string }{"Alice"})
	if html != "<li>Alice</li>" {
		t.Errorf("Render = %q", html)
	}
}

func TestContext_Render_NoTemplates(t *testing.T) {
	tc := NewTestContext()
	html := tc.Render("anything", nil)
	if html != "" {
		t.Errorf("expected empty, got %q", html)
	}
}

func TestContext_Template(t *testing.T) {
	tc := NewTestContext()
	tc.Template("#tpl-form", "#modal")
	ins := tc.Instructions()
	if ins[0].Op != "template" || ins[0].Source != "#tpl-form" || ins[0].Target != "#modal" {
		t.Errorf("ins[0] = %+v", ins[0])
	}
}

func TestContext_Populate(t *testing.T) {
	tc := NewTestContext()
	tc.Populate("#form", map[string]any{"name": "Bob", "email": "bob@test.com"})
	ins := tc.Instructions()
	if ins[0].Op != "populate" || ins[0].Data["name"] != "Bob" {
		t.Errorf("ins[0] = %+v", ins[0])
	}
}

func TestContext_SetValue(t *testing.T) {
	tc := NewTestContext()
	tc.SetValue("#search", "hello")
	ins := tc.Instructions()
	if ins[0].Op != "set-value" || ins[0].Value != "hello" {
		t.Errorf("ins[0] = %+v", ins[0])
	}
}

// Integration-style test: realistic handler usage
func TestHandler_CreateTodo(t *testing.T) {
	tmpl := template.Must(template.New("todo-item").Parse(`<li>{{.Title}}</li>`))

	createTodo := func(ctx *Context) error {
		title := ctx.Payload.String("title")
		html := ctx.Render("todo-item", struct{ Title string }{title})
		ctx.HTML("#todo-list", html, Append)
		ctx.Dispatch("#new-todo", "reset")
		ctx.Focus("#title-input")
		return nil
	}

	tc := NewTestContext()
	tc.SetTemplates(tmpl)
	tc.SetPayload(map[string]any{"title": "Buy milk"})

	if err := createTodo(tc.Context); err != nil {
		t.Fatal(err)
	}

	ins := tc.Instructions()
	if len(ins) != 3 {
		t.Fatalf("expected 3 instructions, got %d", len(ins))
	}
	if ins[0].Op != "html" || ins[0].Mode != Append {
		t.Errorf("ins[0] = %+v", ins[0])
	}
	if ins[0].HTML != "<li>Buy milk</li>" {
		t.Errorf("HTML = %q", ins[0].HTML)
	}
	if ins[1].Op != "dispatch" || ins[1].Event != "reset" {
		t.Errorf("ins[1] = %+v", ins[1])
	}
	if ins[2].Op != "focus" {
		t.Errorf("ins[2] = %+v", ins[2])
	}
}
