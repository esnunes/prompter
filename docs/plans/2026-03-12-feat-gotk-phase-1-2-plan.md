# gotk Phase 1 & 2 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a `gotk` package implementing the command/instruction model (Phase 1) and the server Mux + WebSocket + thin client (Phase 2), then migrate the prompter conversation page from HTMX to gotk.

**Architecture:** gotk is a command-based web toolkit where Go handler functions receive a Context, read a Payload, and produce Instructions (DOM operations). The server routes commands via a Mux over WebSocket. A thin JS client (~200 LOC) scans `gotk-*` HTML attributes, collects payloads, sends commands, and applies returned instructions to the DOM. The existing prompter conversation page will be migrated from HTMX polling to gotk WebSocket commands.

**Tech Stack:** Go 1.25.5, `github.com/coder/websocket` (pure Go WebSocket), `go:embed` for thin client JS, standard `html/template` for rendering.

---

### Task 1: Scaffold gotk Package — Instruction and Payload Types

**Files:**
- Create: `gotk/instruction.go`
- Create: `gotk/payload.go`
- Create: `gotk/payload_test.go`

**Step 1: Create `gotk/instruction.go`**

```go
package gotk

// Mode constants for the html instruction.
const (
	Replace = "replace"
	Append  = "append"
	Prepend = "prepend"
	Remove  = "remove"
)

// Instruction is a single DOM operation. All fields are optional except Op.
// Only the fields relevant to each Op are serialized to JSON.
type Instruction struct {
	Op      string         `json:"op"`
	Target  string         `json:"target,omitempty"`
	HTML    string         `json:"html,omitempty"`
	Mode    string         `json:"mode,omitempty"`
	Source  string         `json:"source,omitempty"`
	Attr    string         `json:"attr,omitempty"`
	Value   string         `json:"value,omitempty"`
	Event   string         `json:"event,omitempty"`
	Detail  map[string]any `json:"detail,omitempty"`
	URL     string         `json:"url,omitempty"`
	Name    string         `json:"name,omitempty"`
	Args    map[string]any `json:"args,omitempty"`
	Cmd     string         `json:"cmd,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

// HandlerFunc is the signature for all command handlers.
type HandlerFunc func(ctx *Context) error
```

**Step 2: Create `gotk/payload.go`**

```go
package gotk

import (
	"fmt"
	"strconv"
)

// Payload wraps the command payload with typed accessors.
// Missing keys return zero values. String values from the DOM are coerced.
type Payload struct {
	data map[string]any
}

// NewPayload creates a Payload from a map.
func NewPayload(data map[string]any) Payload {
	if data == nil {
		data = map[string]any{}
	}
	return Payload{data: data}
}

// String returns the string value for key, or "".
func (p Payload) String(key string) string {
	v, ok := p.data[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
}

// Int returns the int value for key, or 0. Coerces strings and floats.
func (p Payload) Int(key string) int {
	v, ok := p.data[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		n, _ := strconv.Atoi(val)
		return n
	default:
		return 0
	}
}

// Float returns the float64 value for key, or 0.
func (p Payload) Float(key string) float64 {
	v, ok := p.data[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

// Bool returns the bool value for key, or false. Treats "true"/"1" as true.
func (p Payload) Bool(key string) bool {
	v, ok := p.data[key]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true" || val == "1"
	default:
		return false
	}
}

// Map returns the raw underlying map.
func (p Payload) Map() map[string]any {
	return p.data
}
```

**Step 3: Write Payload tests in `gotk/payload_test.go`**

```go
package gotk

import "testing"

func TestPayload_String(t *testing.T) {
	p := NewPayload(map[string]any{"name": "Alice", "count": 42})
	if got := p.String("name"); got != "Alice" {
		t.Errorf("String(name) = %q, want Alice", got)
	}
	if got := p.String("count"); got != "42" {
		t.Errorf("String(count) = %q, want 42", got)
	}
	if got := p.String("missing"); got != "" {
		t.Errorf("String(missing) = %q, want empty", got)
	}
}

func TestPayload_Int(t *testing.T) {
	p := NewPayload(map[string]any{"a": 42, "b": "7", "c": 3.9, "d": "bad"})
	if got := p.Int("a"); got != 42 {
		t.Errorf("Int(a) = %d, want 42", got)
	}
	if got := p.Int("b"); got != 7 {
		t.Errorf("Int(b) = %d, want 7", got)
	}
	if got := p.Int("c"); got != 3 {
		t.Errorf("Int(c) = %d, want 3", got)
	}
	if got := p.Int("d"); got != 0 {
		t.Errorf("Int(d) = %d, want 0", got)
	}
	if got := p.Int("missing"); got != 0 {
		t.Errorf("Int(missing) = %d, want 0", got)
	}
}

func TestPayload_Float(t *testing.T) {
	p := NewPayload(map[string]any{"a": 3.14, "b": "2.5", "c": 7})
	if got := p.Float("a"); got != 3.14 {
		t.Errorf("Float(a) = %f, want 3.14", got)
	}
	if got := p.Float("b"); got != 2.5 {
		t.Errorf("Float(b) = %f, want 2.5", got)
	}
	if got := p.Float("c"); got != 7.0 {
		t.Errorf("Float(c) = %f, want 7.0", got)
	}
}

func TestPayload_Bool(t *testing.T) {
	p := NewPayload(map[string]any{"a": true, "b": "true", "c": "1", "d": "false", "e": false})
	if !p.Bool("a") {
		t.Error("Bool(a) should be true")
	}
	if !p.Bool("b") {
		t.Error("Bool(b) should be true")
	}
	if !p.Bool("c") {
		t.Error("Bool(c) should be true")
	}
	if p.Bool("d") {
		t.Error("Bool(d) should be false")
	}
	if p.Bool("e") {
		t.Error("Bool(e) should be false")
	}
	if p.Bool("missing") {
		t.Error("Bool(missing) should be false")
	}
}

func TestPayload_NilMap(t *testing.T) {
	p := NewPayload(nil)
	if got := p.String("x"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if p.Map() == nil {
		t.Error("Map() should not be nil after NewPayload(nil)")
	}
}
```

**Step 4: Run tests**

Run: `cd /Users/nunes/src/github.com/esnunes/prompter && go test ./gotk/ -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add gotk/instruction.go gotk/payload.go gotk/payload_test.go
git commit -m "feat(gotk): add Instruction types and Payload with typed accessors"
```

---

### Task 2: Context and TestContext

**Files:**
- Create: `gotk/context.go`
- Create: `gotk/context_test.go`

**Step 1: Create `gotk/context.go`**

```go
package gotk

import (
	"bytes"
	"html/template"
)

// AsyncCall represents a server command scheduled by ctx.Async.
type AsyncCall struct {
	Cmd     string
	Payload map[string]any
}

// Context is passed to every command handler.
type Context struct {
	Payload Payload

	instructions []Instruction
	asyncCalls   []AsyncCall
	templates    *template.Template
}

// HTML produces an html instruction.
func (c *Context) HTML(target, html string, mode ...string) {
	m := Replace
	if len(mode) > 0 && mode[0] != "" {
		m = mode[0]
	}
	c.instructions = append(c.instructions, Instruction{
		Op:     "html",
		Target: target,
		HTML:   html,
		Mode:   m,
	})
}

// Remove produces an html instruction with mode "remove".
func (c *Context) Remove(target string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "html",
		Target: target,
		Mode:   Remove,
	})
}

// Template produces a template instruction.
func (c *Context) Template(source, target string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "template",
		Source:  source,
		Target: target,
	})
}

// Populate produces a populate instruction.
func (c *Context) Populate(target string, data map[string]any) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "populate",
		Target: target,
		Data:   data,
	})
}

// Navigate produces a navigate instruction.
func (c *Context) Navigate(url string, targetAndHTML ...string) {
	ins := Instruction{Op: "navigate", URL: url}
	if len(targetAndHTML) >= 1 {
		ins.Target = targetAndHTML[0]
	}
	if len(targetAndHTML) >= 2 {
		ins.HTML = targetAndHTML[1]
	}
	c.instructions = append(c.instructions, ins)
}

// AttrSet produces an attr-set instruction.
func (c *Context) AttrSet(target, attr string, value ...string) {
	v := ""
	if len(value) > 0 {
		v = value[0]
	}
	c.instructions = append(c.instructions, Instruction{
		Op:     "attr-set",
		Target: target,
		Attr:   attr,
		Value:  v,
	})
}

// AttrRemove produces an attr-remove instruction.
func (c *Context) AttrRemove(target, attr string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "attr-remove",
		Target: target,
		Attr:   attr,
	})
}

// SetValue produces a set-value instruction.
func (c *Context) SetValue(target, value string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "set-value",
		Target: target,
		Value:  value,
	})
}

// Dispatch produces a dispatch instruction.
func (c *Context) Dispatch(target, event string, detail ...map[string]any) {
	ins := Instruction{Op: "dispatch", Target: target, Event: event}
	if len(detail) > 0 {
		ins.Detail = detail[0]
	}
	c.instructions = append(c.instructions, ins)
}

// Focus produces a focus instruction.
func (c *Context) Focus(target string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "focus",
		Target: target,
	})
}

// Exec produces an exec instruction.
func (c *Context) Exec(name string, args ...map[string]any) {
	ins := Instruction{Op: "exec", Name: name}
	if len(args) > 0 {
		ins.Args = args[0]
	}
	c.instructions = append(c.instructions, ins)
}

// Async schedules a server command from a frontend command.
func (c *Context) Async(cmd string, payload map[string]any) {
	c.asyncCalls = append(c.asyncCalls, AsyncCall{Cmd: cmd, Payload: payload})
	c.instructions = append(c.instructions, Instruction{
		Op:      "cmd",
		Cmd:     cmd,
		Payload: payload,
	})
}

// Error produces an html instruction with a gotk-error div.
func (c *Context) Error(target, message string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "html",
		Target: target,
		HTML:   `<div class="gotk-error">` + template.HTMLEscapeString(message) + `</div>`,
	})
}

// Render renders a Go template by name and returns the HTML string.
// Returns empty string if no templates are set or template not found.
func (c *Context) Render(name string, data any) string {
	if c.templates == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := c.templates.ExecuteTemplate(&buf, name, data); err != nil {
		return ""
	}
	return buf.String()
}

// setTemplates configures the template engine for ctx.Render.
func (c *Context) setTemplates(t *template.Template) {
	c.templates = t
}
```

**Step 2: Add TestContext below in same file or separate — create `gotk/testing.go`**

Create `gotk/testing.go`:

```go
package gotk

// TestContext is used in tests. Provides inspection methods.
type TestContext struct {
	*Context
}

// NewTestContext creates a new TestContext for use in tests.
func NewTestContext() *TestContext {
	return &TestContext{
		Context: &Context{
			Payload: NewPayload(nil),
		},
	}
}

// SetPayload sets the context payload from a map.
func (tc *TestContext) SetPayload(data map[string]any) {
	tc.Context.Payload = NewPayload(data)
}

// Instructions returns all instructions produced by handlers.
func (tc *TestContext) Instructions() []Instruction {
	return tc.Context.instructions
}

// AsyncCalls returns all async calls scheduled by handlers.
func (tc *TestContext) AsyncCalls() []AsyncCall {
	return tc.Context.asyncCalls
}

// SetTemplates configures templates for ctx.Render in tests.
func (tc *TestContext) SetTemplates(t interface{ ExecuteTemplate(w interface{ Write([]byte) (int, error) }, name string, data any) error }) {
	// Accept *template.Template without importing html/template in test code.
	// We use the concrete type on the Context.
	// TestContext users should pass *template.Template directly.
}
```

Actually, simpler — TestContext just exposes the unexported field and uses setTemplates:

Replace `gotk/testing.go` with:

```go
package gotk

import "html/template"

// TestContext is used in tests. Provides inspection methods
// not available on the production Context.
type TestContext struct {
	*Context
}

// NewTestContext creates a new TestContext for use in tests.
func NewTestContext() *TestContext {
	return &TestContext{
		Context: &Context{
			Payload: NewPayload(nil),
		},
	}
}

// SetPayload sets the context payload from a map.
func (tc *TestContext) SetPayload(data map[string]any) {
	tc.Context.Payload = NewPayload(data)
}

// SetTemplates configures templates for ctx.Render in tests.
func (tc *TestContext) SetTemplates(t *template.Template) {
	tc.Context.setTemplates(t)
}

// Instructions returns all instructions produced by handlers.
func (tc *TestContext) Instructions() []Instruction {
	return tc.Context.instructions
}

// AsyncCalls returns all async calls scheduled by handlers.
func (tc *TestContext) AsyncCalls() []AsyncCall {
	return tc.Context.asyncCalls
}
```

**Step 3: Create `gotk/context_test.go`**

```go
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

func TestContext_Async(t *testing.T) {
	tc := NewTestContext()
	tc.Async("load-user", map[string]any{"id": 42})

	ins := tc.Instructions()
	if ins[0].Op != "cmd" || ins[0].Cmd != "load-user" {
		t.Errorf("ins[0] = %+v", ins[0])
	}

	calls := tc.AsyncCalls()
	if len(calls) != 1 || calls[0].Cmd != "load-user" {
		t.Errorf("async = %+v", calls)
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
```

**Step 4: Run tests**

Run: `cd /Users/nunes/src/github.com/esnunes/prompter && go test ./gotk/ -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add gotk/context.go gotk/testing.go gotk/context_test.go
git commit -m "feat(gotk): add Context with instruction methods and TestContext"
```

---

### Task 3: Mux — Command Router

**Files:**
- Create: `gotk/mux.go`
- Create: `gotk/mux_test.go`

**Step 1: Create `gotk/mux.go`**

```go
package gotk

import (
	"html/template"
	"sync"
)

// NavigateFunc is the callback for handling navigate commands.
type NavigateFunc func(ctx *Context, url string) error

// ConnectFunc is called when a new WebSocket connection is established.
type ConnectFunc func(conn *Conn)

// DisconnectFunc is called when a WebSocket connection is closed.
type DisconnectFunc func(conn *Conn)

// Mux routes commands to handlers.
type Mux struct {
	mu         sync.RWMutex
	handlers   map[string]HandlerFunc
	navigateFn NavigateFunc
	connectFn  ConnectFunc
	disconnFn  DisconnectFunc
	templates  *template.Template
}

// NewMux creates a new command router.
func NewMux() *Mux {
	return &Mux{
		handlers: make(map[string]HandlerFunc),
	}
}

// Handle registers a command handler.
func (m *Mux) Handle(name string, handler HandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[name] = handler
}

// HandleNavigate registers the callback for navigate commands.
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

// dispatch routes a command to the appropriate handler.
// Returns instructions and an optional error string.
func (m *Mux) dispatch(cmd string, payload map[string]any) ([]Instruction, string) {
	m.mu.RLock()
	handler, ok := m.handlers[cmd]
	navigateFn := m.navigateFn
	tmpl := m.templates
	m.mu.RUnlock()

	ctx := &Context{
		Payload: NewPayload(payload),
	}
	ctx.setTemplates(tmpl)

	// Built-in navigate command
	if cmd == "navigate" {
		if navigateFn == nil {
			return []Instruction{{
				Op:   "exec",
				Name: "console.warn",
				Args: map[string]any{"message": "no navigate handler registered"},
			}}, "no navigate handler registered"
		}
		url := ctx.Payload.String("url")
		if err := navigateFn(ctx, url); err != nil {
			return []Instruction{{
				Op:   "exec",
				Name: "console.warn",
				Args: map[string]any{"message": "navigate error: " + err.Error()},
			}}, "navigate error: " + err.Error()
		}
		return ctx.instructions, ""
	}

	if !ok {
		errMsg := "unknown command: " + cmd
		return []Instruction{{
			Op:   "exec",
			Name: "console.warn",
			Args: map[string]any{"message": errMsg},
		}}, errMsg
	}

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
```

**Step 2: Write tests in `gotk/mux_test.go`**

```go
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
```

**Step 3: Run tests**

Run: `cd /Users/nunes/src/github.com/esnunes/prompter && go test ./gotk/ -v`
Expected: All PASS

**Step 4: Commit**

```bash
git add gotk/mux.go gotk/mux_test.go
git commit -m "feat(gotk): add Mux command router with navigate support"
```

---

### Task 4: Conn and WebSocket Handler

**Files:**
- Modify: `go.mod` (add `github.com/coder/websocket`)
- Create: `gotk/conn.go`
- Create: `gotk/websocket.go`
- Create: `gotk/websocket_test.go`

**Step 1: Add websocket dependency**

Run: `cd /Users/nunes/src/github.com/esnunes/prompter && go get github.com/coder/websocket@latest`

**Step 2: Create `gotk/conn.go`**

```go
package gotk

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"
)

var connIDCounter atomic.Int64

// Conn wraps a WebSocket connection with thread-safe writes.
type Conn struct {
	id   int64
	ws   *websocket.Conn
	mu   sync.Mutex
}

func newConn(ws *websocket.Conn) *Conn {
	return &Conn{
		id: connIDCounter.Add(1),
		ws: ws,
	}
}

// ID returns the unique connection identifier.
func (c *Conn) ID() int64 {
	return c.id
}

// Push sends server-initiated instructions (no ref).
func (c *Conn) Push(ins []Instruction) error {
	msg := wsResponse{Instructions: ins}
	return c.writeJSON(msg)
}

func (c *Conn) writeJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.ws.Write(context.Background(), websocket.MessageText, data)
}
```

**Step 3: Create `gotk/websocket.go`**

```go
package gotk

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/coder/websocket"
)

// wsCommand is the JSON shape sent from client to server.
type wsCommand struct {
	Cmd     string         `json:"cmd"`
	Payload map[string]any `json:"payload"`
	Ref     string         `json:"ref"`
}

// wsResponse is the JSON shape sent from server to client.
type wsResponse struct {
	Ref          string        `json:"ref,omitempty"`
	Instructions []Instruction `json:"ins"`
	Error        string        `json:"error,omitempty"`
}

// ServeWebSocket upgrades an HTTP request to a WebSocket connection
// and starts the command read/dispatch loop.
func (m *Mux) ServeWebSocket(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Allow all origins for development. In production, configure this.
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("gotk: websocket accept: %v", err)
		return
	}
	defer ws.CloseNow()

	conn := newConn(ws)

	// Notify connect handler
	m.mu.RLock()
	connectFn := m.connectFn
	disconnFn := m.disconnFn
	m.mu.RUnlock()

	if connectFn != nil {
		connectFn(conn)
	}

	defer func() {
		if disconnFn != nil {
			disconnFn(conn)
		}
	}()

	// Read loop: one goroutine per connection, sequential command processing
	for {
		_, data, err := ws.Read(r.Context())
		if err != nil {
			// Connection closed (normal or abnormal)
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
				websocket.CloseStatus(err) == websocket.StatusGoingAway ||
				r.Context().Err() != nil {
				return
			}
			log.Printf("gotk: ws read: %v", err)
			return
		}

		var cmd wsCommand
		if err := json.Unmarshal(data, &cmd); err != nil {
			log.Printf("gotk: ws unmarshal: %v", err)
			continue
		}

		ins, errMsg := m.dispatch(cmd.Cmd, cmd.Payload)

		resp := wsResponse{
			Ref:          cmd.Ref,
			Instructions: ins,
			Error:        errMsg,
		}

		if err := conn.writeJSON(resp); err != nil {
			log.Printf("gotk: ws write: %v", err)
			return
		}
	}
}

// ServeHTTP implements http.Handler so Mux can be used as a handler.
// It serves the WebSocket endpoint.
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.ServeWebSocket(w, r)
}

// Close gracefully shuts down all resources. Currently a no-op placeholder.
func (m *Mux) Close() error {
	return nil
}

// ignore is a helper to suppress unused context warning.
func init() {
	_ = context.Background
}
```

**Step 4: Write integration test in `gotk/websocket_test.go`**

```go
package gotk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestServeWebSocket_RoundTrip(t *testing.T) {
	m := NewMux()
	m.Handle("echo", func(ctx *Context) error {
		msg := ctx.Payload.String("msg")
		ctx.HTML("#out", msg)
		return nil
	})

	srv := httptest.NewServer(http.HandlerFunc(m.ServeWebSocket))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:] // http -> ws

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.CloseNow()

	// Send command
	cmd := wsCommand{Cmd: "echo", Payload: map[string]any{"msg": "hello"}, Ref: "1"}
	data, _ := json.Marshal(cmd)
	if err := ws.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read response
	_, respData, err := ws.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var resp wsResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Ref != "1" {
		t.Errorf("ref = %q, want 1", resp.Ref)
	}
	if len(resp.Instructions) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(resp.Instructions))
	}
	if resp.Instructions[0].HTML != "hello" {
		t.Errorf("HTML = %q, want hello", resp.Instructions[0].HTML)
	}
}

func TestServeWebSocket_UnknownCommand(t *testing.T) {
	m := NewMux()

	srv := httptest.NewServer(http.HandlerFunc(m.ServeWebSocket))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ws, _, err := websocket.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.CloseNow()

	cmd := wsCommand{Cmd: "nope", Ref: "2"}
	data, _ := json.Marshal(cmd)
	ws.Write(ctx, websocket.MessageText, data)

	_, respData, err := ws.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var resp wsResponse
	json.Unmarshal(respData, &resp)

	if resp.Error == "" {
		t.Error("expected error for unknown command")
	}
	if resp.Ref != "2" {
		t.Errorf("ref = %q, want 2", resp.Ref)
	}
}

func TestServeWebSocket_ConnectDisconnect(t *testing.T) {
	m := NewMux()

	connected := make(chan int64, 1)
	disconnected := make(chan int64, 1)

	m.HandleConnect(func(conn *Conn) {
		connected <- conn.ID()
	})
	m.HandleDisconnect(func(conn *Conn) {
		disconnected <- conn.ID()
	})

	srv := httptest.NewServer(http.HandlerFunc(m.ServeWebSocket))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ws, _, err := websocket.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	select {
	case id := <-connected:
		if id <= 0 {
			t.Errorf("expected positive conn ID, got %d", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for connect")
	}

	ws.Close(websocket.StatusNormalClosure, "bye")

	select {
	case <-disconnected:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for disconnect")
	}
}
```

**Step 5: Run tests**

Run: `cd /Users/nunes/src/github.com/esnunes/prompter && go test ./gotk/ -v -count=1`
Expected: All PASS

**Step 6: Commit**

```bash
git add gotk/conn.go gotk/websocket.go gotk/websocket_test.go go.mod go.sum
git commit -m "feat(gotk): add WebSocket handler with Conn and command dispatch"
```

---

### Task 5: Thin Client JS and Embed

**Files:**
- Create: `gotk/client.js`
- Create: `gotk/embed.go`

**Step 1: Create `gotk/client.js`**

```javascript
// gotk thin client — connects WebSocket, scans gotk-* attributes,
// sends commands, applies instructions.
(function() {
  "use strict";

  var ws = null;
  var refCounter = 0;
  var pendingLoading = {};  // ref -> {el, originalText}
  var registeredFns = {};
  var reconnectDelay = 0;
  var reconnectDelays = [0, 2000, 5000, 10000, 30000];
  var reconnectAttempt = 0;
  var boundElements = new WeakSet();

  // Public API
  window.gotk = {
    register: function(name, fn) {
      registeredFns[name] = fn;
    }
  };

  function nextRef() {
    refCounter++;
    return String(refCounter);
  }

  // --- WebSocket ---

  function connect() {
    var proto = location.protocol === "https:" ? "wss:" : "ws:";
    var url = proto + "//" + location.host + "/ws";
    ws = new WebSocket(url);

    ws.onopen = function() {
      reconnectAttempt = 0;
      document.body.classList.add("gotk-connected");
      document.body.classList.remove("gotk-disconnected");
    };

    ws.onclose = function() {
      document.body.classList.remove("gotk-connected");
      document.body.classList.add("gotk-disconnected");
      scheduleReconnect();
    };

    ws.onerror = function() {};

    ws.onmessage = function(e) {
      var msg;
      try { msg = JSON.parse(e.data); } catch(_) { return; }
      handleResponse(msg);
    };
  }

  function scheduleReconnect() {
    var delay = reconnectDelays[Math.min(reconnectAttempt, reconnectDelays.length - 1)];
    reconnectAttempt++;
    setTimeout(function() {
      connect();
    }, delay);
  }

  function send(cmd, payload, ref) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ cmd: cmd, payload: payload || {}, ref: ref || "" }));
  }

  // --- Response handling ---

  function handleResponse(msg) {
    // Restore gotk-loading element
    if (msg.ref && pendingLoading[msg.ref]) {
      var info = pendingLoading[msg.ref];
      info.el.textContent = info.originalText;
      info.el.disabled = false;
      delete pendingLoading[msg.ref];
    }

    if (msg.error) {
      console.warn("gotk:", msg.error);
    }

    if (msg.ins) {
      for (var i = 0; i < msg.ins.length; i++) {
        applyInstruction(msg.ins[i]);
      }
    }
  }

  // --- Instruction application ---

  function applyInstruction(ins) {
    switch (ins.op) {
      case "html":
        applyHTML(ins);
        break;
      case "template":
        applyTemplate(ins);
        break;
      case "populate":
        applyPopulate(ins);
        break;
      case "navigate":
        applyNavigate(ins);
        break;
      case "attr-set":
        var el = document.querySelector(ins.target);
        if (el) el.setAttribute(ins.attr, ins.value || "");
        else console.warn("gotk: target not found:", ins.target);
        break;
      case "attr-remove":
        var el2 = document.querySelector(ins.target);
        if (el2) el2.removeAttribute(ins.attr);
        else console.warn("gotk: target not found:", ins.target);
        break;
      case "set-value":
        var el3 = document.querySelector(ins.target);
        if (el3) el3.value = ins.value || "";
        else console.warn("gotk: target not found:", ins.target);
        break;
      case "dispatch":
        var el4 = document.querySelector(ins.target);
        if (el4) el4.dispatchEvent(new CustomEvent(ins.event, { detail: ins.detail || {}, bubbles: true }));
        else console.warn("gotk: target not found:", ins.target);
        break;
      case "focus":
        var el5 = document.querySelector(ins.target);
        if (el5) el5.focus();
        else console.warn("gotk: target not found:", ins.target);
        break;
      case "exec":
        var fn = registeredFns[ins.name];
        if (fn) fn(ins.args || {});
        else console.warn("gotk: unknown function:", ins.name);
        break;
      case "cmd":
        send(ins.cmd, ins.payload);
        break;
      default:
        console.warn("gotk: unknown instruction op:", ins.op);
    }
  }

  function applyHTML(ins) {
    var target = document.querySelector(ins.target);
    if (!target) { console.warn("gotk: target not found:", ins.target); return; }

    var mode = ins.mode || "replace";
    if (mode === "remove") {
      target.remove();
      return;
    }
    if (mode === "replace") {
      target.innerHTML = ins.html;
      scanElement(target);
    } else if (mode === "append") {
      var tmp = document.createElement("div");
      tmp.innerHTML = ins.html;
      while (tmp.firstChild) {
        var child = tmp.firstChild;
        target.appendChild(child);
        if (child.nodeType === 1) scanElement(child);
      }
    } else if (mode === "prepend") {
      var tmp2 = document.createElement("div");
      tmp2.innerHTML = ins.html;
      var first = target.firstChild;
      while (tmp2.firstChild) {
        var child2 = tmp2.firstChild;
        target.insertBefore(child2, first);
        if (child2.nodeType === 1) scanElement(child2);
      }
    }
  }

  function applyTemplate(ins) {
    var source = document.querySelector(ins.source);
    var target = document.querySelector(ins.target);
    if (!source || !target) {
      console.warn("gotk: template source/target not found:", ins.source, ins.target);
      return;
    }
    var clone = source.content.cloneNode(true);
    target.innerHTML = "";
    target.appendChild(clone);
    scanElement(target);
  }

  function applyPopulate(ins) {
    var target = document.querySelector(ins.target);
    if (!target) { console.warn("gotk: target not found:", ins.target); return; }
    var data = ins.data || {};
    for (var key in data) {
      var el = target.querySelector('[name="' + key + '"]');
      if (el) el.value = data[key];
    }
  }

  function applyNavigate(ins) {
    if (ins.url) {
      history.pushState(null, "", ins.url);
    }
    if (ins.target && ins.html) {
      var target = document.querySelector(ins.target);
      if (target) {
        target.innerHTML = ins.html;
        scanElement(target);
      }
    }
  }

  // --- Payload collection ---

  function collectPayload(el) {
    var payload = {};

    // gotk-payload (lowest priority)
    var payloadJSON = el.getAttribute("gotk-payload");
    if (payloadJSON) {
      try { payload = JSON.parse(payloadJSON); } catch(_) {}
    }

    // gotk-collect (middle priority)
    var collectSel = el.getAttribute("gotk-collect");
    if (collectSel) {
      var container = document.querySelector(collectSel);
      if (container) {
        var named = container.querySelectorAll("[name]");
        for (var i = 0; i < named.length; i++) {
          var input = named[i];
          var name = input.getAttribute("name");
          if (input.type === "checkbox") {
            if (payload[name] === undefined) payload[name] = [];
            if (input.checked) {
              if (Array.isArray(payload[name])) payload[name].push(input.value);
              else payload[name] = input.checked;
            }
          } else if (input.type === "radio") {
            if (input.checked) payload[name] = input.value;
          } else if (input.tagName === "SELECT" && input.multiple) {
            payload[name] = Array.from(input.selectedOptions).map(function(o) { return o.value; });
          } else {
            payload[name] = input.value;
          }
        }
      }
    }

    // gotk-val-* (highest priority)
    var attrs = el.attributes;
    for (var j = 0; j < attrs.length; j++) {
      if (attrs[j].name.indexOf("gotk-val-") === 0) {
        var key = attrs[j].name.substring(9); // strip "gotk-val-"
        payload[key] = attrs[j].value;
      }
    }

    return payload;
  }

  // --- DOM scanning ---

  function scanElement(root) {
    if (!root || !root.querySelectorAll) return;

    // Scan root itself
    bindElement(root);

    // Scan descendants
    var els = root.querySelectorAll("[gotk-click],[gotk-navigate]");
    for (var i = 0; i < els.length; i++) {
      bindElement(els[i]);
    }
  }

  function bindElement(el) {
    if (boundElements.has(el)) return;

    // gotk-click
    var clickCmd = el.getAttribute("gotk-click");
    if (clickCmd) {
      boundElements.add(el);
      el.addEventListener("click", function(e) {
        e.preventDefault();
        var cmd = el.getAttribute("gotk-click");
        var payload = collectPayload(el);
        var ref = nextRef();

        // gotk-loading
        var loadingText = el.getAttribute("gotk-loading");
        if (loadingText) {
          pendingLoading[ref] = { el: el, originalText: el.textContent };
          el.textContent = loadingText;
          el.disabled = true;
        }

        send(cmd, payload, ref);
      });
    }

    // gotk-navigate
    if (el.hasAttribute("gotk-navigate")) {
      boundElements.add(el);
      el.addEventListener("click", function(e) {
        e.preventDefault();
        var url = el.getAttribute("href") || el.getAttribute("gotk-navigate");
        if (url) {
          send("navigate", { url: url }, nextRef());
        }
      });
    }
  }

  // --- Popstate ---

  window.addEventListener("popstate", function() {
    send("navigate", { url: location.pathname + location.search }, nextRef());
  });

  // --- Init ---

  document.addEventListener("DOMContentLoaded", function() {
    scanElement(document.body);
    connect();
  });
})();
```

**Step 2: Create `gotk/embed.go`**

```go
package gotk

import (
	_ "embed"
	"net/http"
)

//go:embed client.js
var clientJS []byte

// ClientJSHandler returns an http.HandlerFunc that serves the thin client JS.
// Mount at "/gotk/client.js".
func ClientJSHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(clientJS)
	}
}
```

**Step 3: Run tests (existing tests should still pass)**

Run: `cd /Users/nunes/src/github.com/esnunes/prompter && go test ./gotk/ -v -count=1`
Expected: All PASS

**Step 4: Commit**

```bash
git add gotk/client.js gotk/embed.go
git commit -m "feat(gotk): add thin client JS and embed handler"
```

---

### Task 6: RenderPage SSR Helper

**Files:**
- Create: `gotk/render.go`
- Create: `gotk/render_test.go`

**Step 1: Create `gotk/render.go`**

```go
package gotk

import (
	"html/template"
	"io"
	"log"
	"net/http"
)

// RenderPage renders a full server-side page using the layout and page templates.
// It executes the layout template, which should use {{block "content" .}} to
// include page content. The thin client <script> tag is NOT auto-injected —
// include it in your layout template.
//
// Usage:
//
//	gotk.RenderPage(w, templates, "layout.html", data)
func RenderPage(w http.ResponseWriter, tmpl *template.Template, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("gotk: render error (%s): %v", name, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// RenderFragment renders a named template fragment to the response.
func RenderFragment(w http.ResponseWriter, tmpl *template.Template, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("gotk: render error (%s): %v", name, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// RenderString renders a named template to a string.
func RenderString(tmpl *template.Template, name string, data any) string {
	if tmpl == nil {
		return ""
	}
	var w stringWriter
	if err := tmpl.ExecuteTemplate(&w, name, data); err != nil {
		return ""
	}
	return w.String()
}

type stringWriter struct {
	buf []byte
}

func (w *stringWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *stringWriter) String() string {
	return string(w.buf)
}

// Discard is an io.Writer that discards all writes. Useful for testing.
var _ io.Writer = (*stringWriter)(nil)
```

**Step 2: Create `gotk/render_test.go`**

```go
package gotk

import (
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderPage(t *testing.T) {
	tmpl := template.Must(template.New("layout").Parse(`<html><body>{{block "content" .}}{{end}}</body></html>`))
	template.Must(tmpl.New("content").Parse(`<h1>{{.Title}}</h1>`))

	w := httptest.NewRecorder()
	RenderPage(w, tmpl, "layout", struct{ Title string }{"Hello"})

	body := w.Body.String()
	if !strings.Contains(body, "<h1>Hello</h1>") {
		t.Errorf("body = %q", body)
	}
	if w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("content-type = %q", w.Header().Get("Content-Type"))
	}
}

func TestRenderString(t *testing.T) {
	tmpl := template.Must(template.New("item").Parse(`<li>{{.}}</li>`))
	got := RenderString(tmpl, "item", "test")
	if got != "<li>test</li>" {
		t.Errorf("got %q", got)
	}
}

func TestRenderString_Nil(t *testing.T) {
	got := RenderString(nil, "x", nil)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
```

**Step 3: Run tests**

Run: `cd /Users/nunes/src/github.com/esnunes/prompter && go test ./gotk/ -v -count=1`
Expected: All PASS

**Step 4: Commit**

```bash
git add gotk/render.go gotk/render_test.go
git commit -m "feat(gotk): add RenderPage, RenderFragment, and RenderString helpers"
```

---

### Task 7: Wire gotk into Prompter Server

This task integrates gotk into the existing prompter server: mount the WebSocket endpoint, serve the thin client JS, and add the script tag to the layout.

**Files:**
- Modify: `internal/server/server.go` (add routes for `/ws` and `/gotk/client.js`)
- Modify: `internal/server/templates/layout.html` (add `<script src="/gotk/client.js"></script>`)

**Step 1: Modify `internal/server/server.go`**

Add the gotk Mux as a field on Server and mount routes. In `New()`:

After the import block, add `"github.com/esnunes/prompter/gotk"` to imports.

Add `gotkMux *gotk.Mux` field to the `Server` struct.

In `New()`, after creating the server, create the gotk mux and mount it:

```go
// In New(), after s := &Server{...}:
s.gotkMux = gotk.NewMux()

// Mount gotk routes
mux.HandleFunc("GET /ws", s.gotkMux.ServeWebSocket)
mux.HandleFunc("GET /gotk/client.js", gotk.ClientJSHandler())
```

**Step 2: Modify `internal/server/templates/layout.html`**

Add `<script src="/gotk/client.js" defer></script>` after the existing scripts in `<head>`:

```html
  <script src="/static/app.js"></script>
  <script src="/gotk/client.js" defer></script>
```

**Step 3: Verify build compiles**

Run: `cd /Users/nunes/src/github.com/esnunes/prompter && go build ./...`
Expected: No errors

**Step 4: Run all tests**

Run: `cd /Users/nunes/src/github.com/esnunes/prompter && go test ./... -count=1`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/server/server.go internal/server/templates/layout.html
git commit -m "feat: wire gotk WebSocket and thin client into prompter server"
```

---

### Task 8: Migrate Send Message to gotk Command

Convert the "send message" interaction on the conversation page from HTMX `hx-post` to a gotk `gotk-click` command. This is the core interaction — user types a message, clicks Send, the message appears in the conversation.

**Files:**
- Modify: `internal/server/server.go` (register "send-message" command on gotkMux)
- Modify: `internal/server/handlers.go` (add gotk command handler)
- Modify: `internal/server/templates/conversation.html` (change form to gotk attributes)

**Step 1: Add the send-message command handler in `internal/server/handlers.go`**

Add a new method on Server that acts as a gotk HandlerFunc. Since gotk handlers receive `*gotk.Context` but need server state (queries, repoStatus), we use a closure pattern:

```go
// In handlers.go, add this method:

func (s *Server) registerGotkCommands() {
	s.gotkMux.Handle("send-message", func(ctx *gotk.Context) error {
		idStr := ctx.Payload.String("prompt_request_id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			ctx.Error("#conversation", "Invalid prompt request ID")
			return nil
		}

		org := ctx.Payload.String("org")
		repoName := ctx.Payload.String("repo")
		message := ctx.Payload.String("message")
		if message == "" {
			return nil
		}

		// Save user message
		userMsg, err := s.queries.CreateMessage(id, "user", message, nil)
		if err != nil {
			ctx.Error("#conversation", "Failed to save message")
			return nil
		}

		// Render user message bubble
		userHTML := `<div class="message message-user"><div class="message-bubble">` +
			template.HTMLEscapeString(userMsg.Content) + `</div></div>`
		ctx.HTML("#conversation", userHTML, gotk.Append)

		// Clear the textarea
		ctx.SetValue("#message-input", "")

		// Check repo status — if not ready, just save
		statusEntry := s.getRepoStatus(id)
		if statusEntry.Status != "" && statusEntry.Status != "ready" {
			// Disable input while waiting
			ctx.AttrSet("#message-input", "disabled", "true")
			ctx.AttrSet("#send-btn", "disabled", "true")
			return nil
		}

		// Repo is ready — launch async Claude call
		bgCtx, cancel := context.WithCancel(context.Background())
		s.setRepoStatusProcessing(id, cancel)
		go s.backgroundSendMessage(bgCtx, id)

		// Show processing indicator
		pollURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d/status", org, repoName, id)
		cancelURL := fmt.Sprintf("/github.com/%s/%s/prompt-requests/%d/cancel", org, repoName, id)
		processingHTML := fmt.Sprintf(
			`<div id="repo-status" class="repo-status" hx-get="%s" hx-trigger="every 2s" hx-swap="morph:outerHTML" data-started-at="%d">`+
				`<div class="processing-indicator"><div class="spinner"></div>`+
				`<span class="processing-text">Thinking...</span>`+
				`<span class="elapsed-timer"></span></div>`+
				`<form hx-post="%s" hx-target="#repo-status" hx-swap="outerHTML" hx-disabled-elt="find button" style="display:inline;">`+
				`<button type="submit" class="btn btn-sm btn-secondary">Cancel</button></form></div>`,
			pollURL, s.getRepoStatus(id).StartedAt.Unix(), cancelURL)
		ctx.HTML("#conversation", processingHTML, gotk.Append)

		// Re-process HTMX on new content
		ctx.Exec("htmxProcess", map[string]any{"selector": "#repo-status"})

		// Disable input while processing
		ctx.AttrSet("#message-input", "disabled", "true")
		ctx.AttrSet("#send-btn", "disabled", "true")

		// Scroll to bottom
		ctx.Exec("scrollConversation")

		return nil
	})
}
```

**Step 2: Register commands in `New()` in `internal/server/server.go`**

After `s.gotkMux = gotk.NewMux()`, call:

```go
s.registerGotkCommands()
```

**Step 3: Register JS helper functions in the client**

Add to `internal/server/static/app.js` at the top level:

```javascript
// Register gotk exec functions
if (window.gotk) {
  gotk.register("scrollConversation", function() {
    scrollConversation();
  });

  gotk.register("htmxProcess", function(args) {
    if (typeof htmx !== "undefined" && args.selector) {
      var el = document.querySelector(args.selector);
      if (el) htmx.process(el);
    }
  });

  gotk.register("renderMarkdown", function() {
    if (typeof renderMarkdown === "function") renderMarkdown();
  });
}
```

**Step 4: Update the chat form in `conversation.html`**

Change the chat input form from HTMX to gotk. Replace the `<div class="chat-input"...>` block:

```html
      <div class="chat-input" id="message-form"{{if .LastQuestions}} style="display:none"{{end}}>
        <div class="chat-form">
          <input type="hidden" id="send-pr-id" value="{{.PromptRequest.ID}}">
          <input type="hidden" id="send-org" value="{{.Org}}">
          <input type="hidden" id="send-repo" value="{{.Repo}}">
          <textarea id="message-input" name="message" placeholder="Describe the feature you'd like... (Enter to send, Shift+Enter for new line)" rows="2"></textarea>
          <button id="send-btn"
                  gotk-click="send-message"
                  gotk-payload='{"prompt_request_id": "{{.PromptRequest.ID}}", "org": "{{.Org}}", "repo": "{{.Repo}}"}'
                  gotk-loading="Sending..."
                  class="btn btn-primary">Send</button>
        </div>
      </div>
```

Note: We need the button's gotk-click to also collect the textarea value. We'll use `gotk-collect` on a wrapping container, or add a custom approach. Actually, the simpler approach: we collect the message from the textarea using `gotk-collect`.

Revised approach — use `gotk-collect`:

```html
      <div class="chat-input" id="message-form"{{if .LastQuestions}} style="display:none"{{end}}>
        <div class="chat-form" id="message-form-fields">
          <input type="hidden" name="prompt_request_id" value="{{.PromptRequest.ID}}">
          <input type="hidden" name="org" value="{{.Org}}">
          <input type="hidden" name="repo" value="{{.Repo}}">
          <textarea id="message-input" name="message" placeholder="Describe the feature you'd like... (Enter to send, Shift+Enter for new line)" rows="2"></textarea>
          <button id="send-btn"
                  gotk-click="send-message"
                  gotk-collect="#message-form-fields"
                  gotk-loading="Sending..."
                  class="btn btn-primary">Send</button>
        </div>
      </div>
```

**Step 5: Update Enter-to-send in `app.js`**

The existing Enter-to-send handler clicks the submit button. With gotk, the button has `gotk-click` not `type="submit"`, so we need to adjust. Change the keydown handler to click `#send-btn`:

```javascript
  // Enter-to-send: submit chat form on Enter, newline on Shift+Enter
  document.addEventListener("keydown", function (e) {
    if (e.key !== "Enter") return;
    var textarea = e.target;
    if (textarea.id !== "message-input") return;

    if (e.shiftKey) return;
    if (e.isComposing || e.keyCode === 229) return;
    e.preventDefault();
    if (textarea.value.trim() === "") return;
    if (textarea.disabled) return;

    var btn = document.getElementById("send-btn");
    if (btn && !btn.disabled) btn.click();
  });
```

**Step 6: Verify build compiles**

Run: `cd /Users/nunes/src/github.com/esnunes/prompter && go build ./...`
Expected: No errors

**Step 7: Commit**

```bash
git add internal/server/server.go internal/server/handlers.go internal/server/templates/conversation.html internal/server/static/app.js
git commit -m "feat: migrate send-message to gotk command over WebSocket"
```

---

### Task 9: Manual Integration Test

**Step 1: Start the server**

Run: `cd /Users/nunes/src/github.com/esnunes/prompter && go run ./cmd/prompter/ serve`

**Step 2: Open a conversation page in the browser**

Navigate to an existing prompt request. Verify:
1. The browser console shows no JS errors
2. The `<body>` element gets `gotk-connected` class (check devtools)
3. Typing a message and pressing Enter/clicking Send sends the message via WebSocket
4. The user message appears in the conversation
5. The processing indicator appears
6. When Claude responds, the response appears (via existing HTMX polling)

**Step 3: Verify backward compatibility**

- Existing HTMX interactions (sidebar polling, question forms, publish, archive) still work
- Status polling still works via HTMX
- Page navigation works normally

**Step 4: Fix any issues found and commit**

```bash
git add -A
git commit -m "fix: integration fixes for gotk send-message"
```

---

### Task 10: Run Full Test Suite

**Step 1: Run all Go tests**

Run: `cd /Users/nunes/src/github.com/esnunes/prompter && go test ./... -v -count=1`
Expected: All PASS

**Step 2: Run vet and build**

Run: `cd /Users/nunes/src/github.com/esnunes/prompter && go vet ./... && go build ./...`
Expected: Clean

**Step 3: Final commit if any cleanup needed**

```bash
git add -A
git commit -m "chore: cleanup after gotk phase 1+2 integration"
```
