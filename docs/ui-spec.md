# gotk UI Spec

## Overview

**Server renders. Views update. Commands are pure business logic.**

gotk is a command-based web application toolkit where every behavior is a Go function. Commands execute business logic and emit typed events. Views receive events and produce DOM instructions. A thin JS client (~200 LOC) applies those instructions to the DOM. That is the entire frontend runtime.

The architecture targets **web apps**, not websites. It prioritizes testability, locality of behavior, minimal dependencies, zero build steps for the frontend, and a single language (Go) across the full stack.

### How gotk Compares

| Framework | Who renders | Who holds state | Frontend complexity | Testability |
| --- | --- | --- | --- | --- |
| React/Vue | Client | Client | High (state mgmt, build tools, JSX) | Requires JSDOM/browser |
| LiveView | Server | Server | None (magic diffing) | Tight coupling to framework |
| htmx | Server | Server | Minimal (HTTP attributes) | Test HTTP endpoints |
| **gotk** | **Server** | **Server (data) + DOM (UI)** | **Minimal (~200 LOC JS client)** | **`go test` for everything** |

**Not React.** The frontend does not render. It orchestrates — show this, hide that, send this command. No virtual DOM, no state management library, no build step.

**Not LiveView.** The server does not track the DOM or diff state. It receives a command, runs a function, and emits events. Views translate events into explicit DOM instructions. No magic, no session-bound DOM tree, no framework coupling.

**Not htmx.** gotk has a richer command protocol — commands emit typed events, views produce structured instructions, background tasks can broadcast to all clients. htmx stops at declarative HTTP attributes. gotk's server-side views provide the programmable layer for UI decisions.

### Why This Architecture Is AI-Friendly

gotk is designed so that both AI code generators (writing gotk apps) and AI agents (using gotk apps) work well with the same architecture.

#### For AI Generating Code

The biggest problem with AI-generated frontend code is not writing it — it is testing it. React, Vue, Angular, and similar frameworks require browser-based or JSDOM-based test environments, complex mocking of DOM APIs, and framework-specific patterns (`act()` wrappers, async rendering, effect cleanup). AI frequently gets these wrong, producing code that looks correct but fails in subtle ways — stale closures, missing dependency arrays, race conditions between renders.

gotk eliminates this class of problems entirely. Every command and view handler is a pure Go function that produces a data structure. Testing is asserting on that data structure. No browser, no DOM, no async timing.

**Every command has the same shape.** AI sees three examples and can generate the hundredth correctly. The pattern never varies:

```go
func (s *Server) CreateTodo(ctx *gotk.CommandContext) error {
    title := ctx.Payload.String("title")
    todo := s.db.Create(title)
    gotk.Dispatch(ctx.Dispatcher(), TodoCreatedEvent{ID: todo.ID, Title: todo.Title})
    return nil
}
```

There are no hooks, no lifecycle methods, no component trees, no state management patterns to choose between. Every command is a standalone function with no ambient context.

**Views are equally predictable.** A view registers handlers for events and produces instructions:

```go
func (v *TodoView) OnTodoCreated(e TodoCreatedEvent) {
    v.ctx.HTML("#todo-list", v.ctx.Render("todo-item", e), gotk.Append)
    v.ctx.SetValue("#title-input", "")
    v.ctx.Focus("#title-input")
}
```

**Tests are mechanically derivable.** Commands test event emission. Views test instruction output. Both are simple data assertions:

```go
func TestCreateTodo(t *testing.T) {
    app := newTestApp()
    ctx := gotk.NewTestCommandContext()
    ctx.SetPayload(map[string]any{"title": "Buy milk"})

    app.CreateTodo(ctx)

    events := ctx.Events()
    assert.Equal(t, "Buy milk", events[0].(TodoCreatedEvent).Title)
}

func TestOnTodoCreated(t *testing.T) {
    view := newTestTodoView()

    view.OnTodoCreated(TodoCreatedEvent{ID: 1, Title: "Buy milk"})

    ins := view.ctx.Instructions()
    assert.Equal(t, "html", ins[0].Op)
    assert.Contains(t, ins[0].HTML, "Buy milk")
}
```

No DOM mocking. No async/await. No `act()` wrappers. No timing issues.

**The error surface is narrow.** Commands can emit the wrong event or wrong data — caught by event assertions. Views can produce wrong instructions — caught by instruction assertions. Both are concrete data structures, not rendered DOM state.

**No build configuration to break.** gotk needs a `.go` file and an `.html` template. `go build`. Done.

**Templates are standard Go `html/template`.** Well-known to every AI model, no custom DSL to learn.

#### For AI Agents Consuming the App

- **The command protocol is structured JSON.** An agent connects via WebSocket and sends `{"cmd": "create-todo", "payload": {"title": "Buy milk"}}`. No browser, no screenshots, no simulated clicks.
- **The HTML is self-describing.** `gotk-click="save-user"`, `gotk-collect="#form"`, `<input name="email">` — an agent reading the page knows what commands exist, what inputs they need, and what the current state is.
- **No separate API to build.** The command layer that serves humans serves agents. You ship one interface, not two.

See the [AI Agent Interface](#ai-agent-interface) section for details.

## Mental Model

Think of the browser as a WebView in a desktop app:

- **Commands** are messages sent from the UI to a handler (like IPC in Electron). Commands run business logic and emit events.
- **Views** are per-connection objects that receive events and decide what UI updates to make. They own all DOM instruction logic for a page.
- **Instructions** are responses that tell the UI what to change (like render updates). Only views produce instructions.
- **The thin client** is the event loop — it binds events, sends commands, and applies instructions.
- **State** lives server-side (business data) or in the DOM (UI state). There is no client-side state store.

### Architecture

```
Browser (thin client ~200 LOC JS)
    ↕ WebSocket
Server
    ├── Mux ─── routes command ─── CommandHandler
    │                                   │
    │                              emits Event
    │                                   │
    │                              Dispatcher
    │                                   │
    │                          View (per connection)
    │                                   │
    │                          produces Instructions
    │                                   │
    └──────────────────────── sends to client ◄──────┘
```

1. User clicks a `gotk-click` button.
2. Thin client collects payload from DOM, sends `{cmd, payload, ref, url}` over WebSocket.
3. Mux routes to the command handler.
4. Command handler executes business logic and emits typed events via the connection's Dispatcher.
5. Dispatcher calls the current view's registered handler for that event type.
6. View handler produces DOM instructions.
7. Thin client applies instructions sequentially to the DOM.
8. Thin client re-scans affected subtree for new `gotk-*` attributes.

## Core Concepts

### Commands

Commands are Go functions that execute business logic. They read input from the payload, interact with databases and services, and emit typed events. **Commands never produce DOM instructions directly** — they don't know what page the user is on or how the UI should update.

```go
func (s *Server) CreateTodo(ctx *gotk.CommandContext) error {
    title := ctx.Payload.String("title")
    if title == "" {
        gotk.Dispatch(ctx.Dispatcher(), ValidationErrorEvent{Field: "title", Message: "Title is required"})
        return nil
    }

    todo, err := s.db.CreateTodo(title)
    if err != nil {
        gotk.Dispatch(ctx.Dispatcher(), ErrorEvent{Message: "Failed to create todo"})
        return nil
    }

    gotk.Dispatch(ctx.Dispatcher(), TodoCreatedEvent{ID: todo.ID, Title: todo.Title})
    return nil
}
```

Registration:

```go
mux := gotk.NewMux()
mux.Handle("create-todo", s.CreateTodo)
```

### Events

Events are plain Go structs that carry data from commands to views. They represent what happened — not what to do about it.

```go
type TodoCreatedEvent struct {
    ID    int64
    Title string
}

type ValidationErrorEvent struct {
    Field   string
    Message string
}

type ErrorEvent struct {
    Message string
}
```

Events are the contract between commands and views. A command emits events without knowing which views (if any) will handle them.

### Views

Views are per-connection objects that own all UI update logic for a page. A view registers handlers for the events it cares about and produces DOM instructions in response.

```go
type TodoView struct {
    ctx *gotk.ViewContext
}

func NewTodoView(d *gotk.Dispatcher, ctx *gotk.ViewContext) *TodoView {
    v := &TodoView{ctx: ctx}
    gotk.Register(d, v.OnTodoCreated)
    gotk.Register(d, v.OnValidationError)
    gotk.Register(d, v.OnError)
    return v
}

func (v *TodoView) OnTodoCreated(e TodoCreatedEvent) {
    v.ctx.HTML("#todo-list", v.ctx.Render("todo-item", e), gotk.Append)
    v.ctx.SetValue("#title-input", "")
    v.ctx.Focus("#title-input")
}

func (v *TodoView) OnValidationError(e ValidationErrorEvent) {
    v.ctx.Error("#"+e.Field+"-error", e.Message)
}

func (v *TodoView) OnError(e ErrorEvent) {
    v.ctx.Error("#page-error", e.Message)
}
```

Key properties:

- **One active view per connection.** gotk tracks the active view for each WebSocket connection based on the URL sent with every message. Views are cached per URL — navigating back to a previously visited page reuses the existing view.
- **Views own locality.** All DOM selectors, template rendering, and instruction logic for a page live in the view. The template, CSS, and view Go code can be co-located in the same directory.
- **Views are opt-in subscribers.** A view only receives events it registered for. Unregistered events are silently ignored.
- **Navigation switches the active view.** When a user navigates to a different page, the next command from the client carries the new URL. The server looks up or lazily creates the view for that URL. Previous views remain cached on the connection.

### Dispatcher

The Dispatcher routes typed events to view handlers. Each connection has its own Dispatcher instance.

```go
type Handler[T any] func(T)

type Dispatcher struct {
    handlers map[any]any
}

func Register[T any](d *Dispatcher, h Handler[T]) {
    key := reflect.TypeOf((*T)(nil)).Elem()
    d.handlers[key] = h
}

func Dispatch[T any](d *Dispatcher, event T) {
    key := reflect.TypeOf((*T)(nil)).Elem()
    if h, ok := d.handlers[key]; ok {
        h.(Handler[T])(event)
    }
}
```

The entire dispatch machinery is ~10 lines. Type safety at the edges (`Register` and `Dispatch` are generic), type-erased in the middle (`map[any]any`). The `reflect.TypeOf((*T)(nil)).Elem()` pattern avoids heap allocation — it creates a nil pointer and extracts the type descriptor.

### Three Contexts

Each context type has a distinct role and a distinct set of capabilities:

| Context | Who uses it | What it holds |
| --- | --- | --- |
| `CommandContext` | Command handlers | Payload, the connection's Dispatcher, ability to spawn tasks |
| `ViewContext` | View event handlers | Instruction builder (HTML, Remove, Exec, etc.), template renderer |
| `TaskContext` | Background goroutines | Access to all connections' dispatchers (broadcast) or a specific connection's dispatcher (targeted push) |

#### CommandContext

Available to command handlers. Provides access to the command payload, the originating connection's dispatcher (for emitting events), and the ability to spawn background tasks.

```go
func (s *Server) SendMessage(ctx *gotk.CommandContext) error {
    message := ctx.Payload.String("message")

    msg, err := s.db.CreateMessage(message)
    if err != nil {
        gotk.Dispatch(ctx.Dispatcher(), ErrorEvent{Message: "Failed to save message"})
        return nil
    }

    gotk.Dispatch(ctx.Dispatcher(), MessageSentEvent{ID: msg.ID, Content: msg.Content})

    // Spawn background task for long-running work
    go s.processMessage(ctx.NewTask(), msg.ID)

    return nil
}
```

**CommandContext cannot produce DOM instructions.** It has no `HTML()`, `Remove()`, or other instruction methods. The separation is enforced by the type system.

#### ViewContext

Available to view event handlers. Provides the full instruction builder (HTML, AttrSet, Remove, Focus, Exec, etc.) and template rendering. This is the only context that can produce DOM instructions.

```go
func (v *ConversationView) OnMessageSent(e MessageSentEvent) {
    v.ctx.HTML("#conversation", v.ctx.Render("message", struct{ Role, Content string }{"user", e.Content}), gotk.Append)
    v.ctx.SetValue("#message-input", "")
    v.ctx.AttrSet("#message-input", "disabled", "true")
    v.ctx.AttrSet("#send-btn", "disabled", "true")
    v.ctx.Exec("scrollConversation")
}
```

#### TaskContext

Available to background goroutines. Provides access to the originating connection's dispatcher (for targeted event dispatch) and to all connections' dispatchers (for broadcast).

```go
func (s *Server) processMessage(tctx *gotk.TaskContext, msgID int64) {
    result, err := s.claude.Run(msgID)
    if err != nil {
        // Push error to originating connection
        gotk.Dispatch(tctx.Dispatcher(), ProcessingErrorEvent{Message: err.Error()})
        return
    }

    // Push result to originating connection
    gotk.Dispatch(tctx.Dispatcher(), ResponseReceivedEvent{ID: result.ID, Content: result.Content})

    // Broadcast sidebar update to all connections
    tctx.Broadcast(SidebarUpdatedEvent{})
}
```

**Stale connections.** If the originating connection disconnects before the task finishes, `tctx.Dispatcher()` returns nil (or a no-op dispatcher). The task can check or let dispatch silently no-op.

**Broadcast.** `tctx.Broadcast(event)` iterates all connections' dispatchers and calls `Dispatch` on each. Views that don't handle the event type silently ignore it.

## Protocol

All communication between the thin client and the server happens over WebSocket, using JSON messages. The initial page load is a standard HTTP request that returns a full server-side rendered page.

### Client to Server (Command)

```json
{
  "cmd": "create-todo",
  "payload": { "title": "Buy milk" },
  "ref": "a1b2",
  "url": "/todos"
}
```

| Field | Type | Description |
| --- | --- | --- |
| `cmd` | string | Command name. |
| `payload` | object | Arbitrary data collected from the DOM or hardcoded. |
| `ref` | string | Client-generated ID to correlate the response. |
| `url` | string | The client's current page URL (`location.pathname + location.search`). Used by the server to look up or lazily create the view for the page. |

### Server to Client (Instructions)

```json
{
  "ref": "a1b2",
  "ins": [
    { "op": "html", "target": "#todo-list", "html": "<li>...</li>" },
    { "op": "navigate", "url": "/todos" }
  ]
}
```

| Field | Type | Description |
| --- | --- | --- |
| `ref` | string | Correlates to the command `ref`. Empty for server-push. |
| `ins` | array | Ordered list of instructions to apply sequentially. |

Server-initiated pushes (no corresponding command) omit `ref`.

## Instructions

Instructions are the atomic units of UI change. The thin client applies them sequentially. **Only views produce instructions.**

### `html`

Replace, append, or prepend HTML content in a target element.

```json
{
  "op": "html",
  "target": "#todo-list",
  "html": "<li>New item</li>",
  "mode": "append"
}
```

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `target` | string | required | CSS selector for the target element. |
| `html` | string | required | HTML content. |
| `mode` | string | `"replace"` | One of `replace`, `append`, `prepend`, `remove`. `remove` ignores `html` and removes the target element. |

### `template`

Clone a `<template>` element's content into a target.

```json
{
  "op": "template",
  "source": "#tpl-user-form",
  "target": "#modal"
}
```

| Field | Type | Description |
| --- | --- | --- |
| `source` | string | CSS selector for the `<template>` element. |
| `target` | string | CSS selector for the target element. Content is replaced. |

### `populate`

Fill named form elements within a container using a key-value map. For each entry, finds `[name="<key>"]` within the target and sets its value.

```json
{
  "op": "populate",
  "target": "#modal-body",
  "data": { "name": "Alice", "email": "alice@test.com" }
}
```

| Field | Type | Description |
| --- | --- | --- |
| `target` | string | CSS selector for the container. |
| `data` | object | Map of `name` attribute to value. |

### `navigate`

Trigger SPA navigation. The thin client fetches the page via HTTP, swaps the DOM content, and pushes the URL to browser history. The server does not need a separate navigate command — the next command from the client will carry the new URL, and the server will lazily create the view for it. If `target` and `html` are provided, the content is swapped inline without an HTTP fetch.

```json
{
  "op": "navigate",
  "url": "/settings",
  "target": "#content",
  "html": "<div>...</div>"
}
```

| Field | Type | Description |
| --- | --- | --- |
| `url` | string | URL to navigate to. |
| `target` | string | Optional. CSS selector for inline content swap (skips HTTP fetch). |
| `html` | string | Optional. HTML to set on the target for inline swap. |

### `attr-set`

Set an attribute on an element.

```json
{ "op": "attr-set", "target": "#sidebar", "attr": "hidden", "value": "" }
```

### `attr-remove`

Remove an attribute from an element.

```json
{ "op": "attr-remove", "target": "#modal", "attr": "hidden" }
```

### `set-value`

Set the value of a form element.

```json
{ "op": "set-value", "target": "#search-input", "value": "query" }
```

### `dispatch`

Dispatch a CustomEvent on an element.

```json
{
  "op": "dispatch",
  "target": "#form",
  "event": "reset",
  "detail": {}
}
```

| Field | Type | Description |
| --- | --- | --- |
| `target` | string | CSS selector. |
| `event` | string | Event name. |
| `detail` | object | Optional. Passed as `CustomEvent.detail`. |

### `focus`

Focus an element.

```json
{ "op": "focus", "target": "#title-input" }
```

### `exec`

Execute a registered client-side JS function. Escape hatch for behaviors that require direct DOM access (animations, scroll position, focus traps).

```json
{ "op": "exec", "name": "lockScroll", "args": {} }
```

### `cmd`

Trigger a server command from within an instruction sequence. The thin client sends this as a regular WebSocket command.

```json
{ "op": "cmd", "cmd": "edit-user", "payload": { "id": 42 } }
```

## HTML Attributes (Thin Client API)

The thin client scans the DOM on page load and after every `html` or `template` instruction. It binds behavior based on `gotk-*` attributes.

### `gotk-click`

Send a command when the element is clicked. The thin client collects the payload and sends the command over WebSocket.

```html
<button gotk-click="toggle-sidebar">☰</button>
<button gotk-click="delete-todo" gotk-collect="#todo-42">Delete</button>
```

### `gotk-input`

Send a command on the `input` event.

```html
<input gotk-input="search" gotk-debounce="300" name="q">
```

### `gotk-on`

Send a command on an arbitrary DOM event. Format: `event:command`.

```html
<div gotk-on="dragend:reorder-items" gotk-collect=".sortable">...</div>
```

### `gotk-navigate`

Intercept a link click and perform SPA navigation — fetches the page via HTTP, swaps the DOM, and pushes the URL to history. The server discovers the new page on the next command, which carries the updated URL. The `href` is preserved for right-click, new-tab, and bookmarking.

```html
<a href="/settings" gotk-navigate>Settings</a>
```

### `gotk-poll`

Send a command at a fixed interval.

```html
<div gotk-poll="refresh-notifications" gotk-every="30s" id="notifs">...</div>
```

### `gotk-collect`

Specifies a CSS selector for a container. When a command fires, the thin client gathers all elements with a `name` attribute within the container and sends their values as `payload`. Without `gotk-collect`, the payload is empty (unless `gotk-payload` is used).

```html
<button gotk-click="create-user" gotk-collect="#user-form">Save</button>
```

### `gotk-payload`

Hardcoded JSON payload.

```html
<button gotk-click="delete-todo" gotk-payload='{"id": 42}'>Delete</button>
```

### `gotk-val-*`

Attach payload values directly on elements. Cleaner than `gotk-payload` for simple key-value pairs. All `gotk-val-*` attributes are collected into the payload. When used alongside `gotk-collect`, values are merged with `gotk-val-*` taking precedence.

```html
<button gotk-click="delete" gotk-val-id="42" gotk-val-type="user">Delete</button>
```

Produces payload: `{"id": "42", "type": "user"}`.

### `gotk-debounce`

Debounce interval in milliseconds. Applies to `gotk-input` and `gotk-on`.

```html
<input gotk-input="validate-email" gotk-debounce="300" name="email">
```

### `gotk-throttle`

Throttle interval in milliseconds. Unlike debounce, throttle fires at most once per interval and guarantees the last event is delivered. Applies to `gotk-input` and `gotk-on`.

```html
<div gotk-on="scroll:update-position" gotk-throttle="100">...</div>
```

### `gotk-loading`

Text to display while a server command is in flight. The thin client disables the element and swaps its text content when the command fires, then restores the original text and re-enables the element when the response arrives. Prevents double submissions and provides immediate feedback.

```html
<button gotk-click="save-user" gotk-collect="#form" gotk-loading="Saving...">Save</button>
```

## Payload Collection

When a command fires, the thin client assembles the payload from multiple sources. Sources are merged in the following priority order (highest wins):

1. **`gotk-val-*` attributes** — Explicit key-value pairs on the element.
2. **`gotk-collect` container** — Named elements within the referenced container.
3. **`gotk-payload` JSON** — Hardcoded JSON object.

When sources overlap on a key, `gotk-val-*` wins over `gotk-collect`, which wins over `gotk-payload`. In practice, most commands use only one source.

## Command Types

### Server Commands

Handled by Go functions on the server. Transported via WebSocket. Have access to databases, sessions, and server-side state. Emit events via the connection's Dispatcher.

```go
func (app *App) CreateTodo(ctx *gotk.CommandContext) error {
    title := ctx.Payload.String("title")
    todo := app.db.Create(title)
    gotk.Dispatch(ctx.Dispatcher(), TodoCreatedEvent{ID: todo.ID, Title: todo.Title})
    return nil
}
```

Registration:

```go
mux := gotk.NewMux()
mux.Handle("create-todo", app.CreateTodo)
```

## Error Handling

### Command Errors

When a command encounters an error, it emits an error event. The view decides how to present it:

```go
// Command
func (app *App) EditUser(ctx *gotk.CommandContext) error {
    user, err := app.db.GetUser(ctx.Payload.Int("id"))
    if err != nil {
        gotk.Dispatch(ctx.Dispatcher(), UserLoadErrorEvent{Message: "Failed to load user"})
        return nil
    }
    gotk.Dispatch(ctx.Dispatcher(), UserLoadedEvent{Name: user.Name, Email: user.Email})
    return nil
}

// View
func (v *UserModalView) OnUserLoadError(e UserLoadErrorEvent) {
    v.ctx.Error("#modal-body", e.Message)
    v.ctx.AttrRemove("#modal-body", "aria-busy")
}

func (v *UserModalView) OnUserLoaded(e UserLoadedEvent) {
    v.ctx.Populate("#modal-body", map[string]any{
        "name":  e.Name,
        "email": e.Email,
    })
    v.ctx.AttrRemove("#modal-body", "aria-busy")
}
```

`ctx.Error(target, message)` is a convenience method on ViewContext that produces:

```json
{
  "op": "html",
  "target": "#modal-body",
  "html": "<div class=\"gotk-error\">Failed to load user</div>"
}
```

This is not a new instruction type — it is a convenience method that produces a standard `html` instruction. Applications style `.gotk-error` as needed.

## ViewContext Convenience Methods

The `gotk.ViewContext` provides convenience methods that produce standard instructions. These methods exist to reduce boilerplate and enforce consistent patterns. None of them introduce new instruction types.

| Method | Produces | Description |
| --- | --- | --- |
| `ctx.HTML(target, html, mode?)` | `html` | Set/append/prepend HTML content. |
| `ctx.Remove(target)` | `html` (mode: remove) | Remove an element from the DOM. |
| `ctx.Template(source, target)` | `template` | Clone a `<template>` into a target. |
| `ctx.Populate(target, data)` | `populate` | Fill named form elements from a map. |
| `ctx.Navigate(url)` | `navigate` | Push a URL to browser history. |
| `ctx.AttrSet(target, attr, value)` | `attr-set` | Set an attribute. |
| `ctx.AttrRemove(target, attr)` | `attr-remove` | Remove an attribute. |
| `ctx.SetValue(target, value)` | `set-value` | Set a form element value. |
| `ctx.Dispatch(target, event, detail?)` | `dispatch` | Fire a CustomEvent. |
| `ctx.Focus(target)` | `focus` | Focus an element. |
| `ctx.Exec(name, args?)` | `exec` | Call a registered JS function. |
| `ctx.Error(target, message)` | `html` | Insert error markup with `gotk-error` class. |
| `ctx.Render(template, data)` | (returns string) | Render a Go template to HTML string. |

## Server-Side Rendering and Routing

### Initial Page Load

Every URL has a standard HTTP handler that returns a full server-rendered page. This ensures pages are bookmarkable and work without JavaScript on first load.

```go
router.GET("/todos", func(w http.ResponseWriter, r *http.Request) {
    todos := app.db.ListTodos()
    gotk.RenderPage(w, "layouts/app", "pages/todos", todos)
})
```

The rendered page includes the thin client JS. On load, it establishes the WebSocket connection and scans the DOM for `gotk-*` attributes.

### Lazy View Creation

Views are created lazily on the server. Every WebSocket message from the client includes a `url` field with `location.pathname + location.search`. When the server receives a command, it looks up the view for that URL in the connection's view cache. If the view doesn't exist yet, the server creates it by calling the registered view factory:

```go
mux.HandleView(func(url string, d *gotk.Dispatcher, vctx *gotk.ViewContext) {
    switch {
    case strings.HasPrefix(url, "/todos"):
        NewTodoView(d, vctx)
    case strings.HasPrefix(url, "/settings"):
        NewSettingsView(d, vctx)
    }
})
```

This means no special initialization protocol is needed. The first command on any page (whether from a click, input, or any other interaction) triggers view creation. On reconnect, the first command also carries the current URL, so the view is recreated transparently.

Views are cached per URL on the connection. Navigating back to a previously visited page reuses the existing view — no recreation needed. This is important for back/forward navigation where the user may return to a page they've already interacted with.

### Client-Side Navigation

There are two paths that trigger SPA navigation:

**From a `gotk-navigate` link:** The thin client intercepts the click, fetches the page via HTTP, swaps the DOM content, and pushes the URL to browser history. The server discovers the new page when the next command arrives with the updated URL.

**From a command's `ctx.Navigate(url)`:** The server includes a `navigate` instruction in the response. The thin client processes it the same way — fetches the page, swaps DOM, pushes URL.

In both cases, the page is fully rendered by the HTTP response (SSR). The server lazily creates the view when the next command arrives from the new page.

### Back/Forward

The thin client listens for `popstate` events and performs SPA navigation (fetch page, swap DOM). No `pushState` is called since the browser already updated the URL. The server discovers the page change when the next command arrives with the updated URL, and reuses the cached view if one exists for that URL.

### Template Reuse

The same Go templates are used for both SSR full-page renders and view event handlers. No duplication.

## Connection Management

### WebSocket Lifecycle

The thin client manages the WebSocket connection automatically, including reconnection after network interruptions, server restarts, or laptop sleep/wake.

### Per-Connection State (ConnEntry)

Each WebSocket connection has a `ConnEntry` stored in the Mux's connection registry:

- **Conn** — the WebSocket connection handle.
- **views** — a map of URL → `viewEntry` (Dispatcher + ViewContext). Views are created lazily on first command for each URL and cached for the lifetime of the connection.
- **lastURL** — the URL from the most recent command. Used by background tasks to find the active view for broadcasts.
- **mu** — a mutex protecting views and lastURL during concurrent access (command handling vs. broadcast).

When the connection is established, the framework creates a ConnEntry with an empty views map. Views are created lazily as commands arrive with different URLs. Navigating back to a previously visited URL reuses the cached view.

### Reconnection Strategy

On disconnect, the thin client:

1. Applies the `gotk-disconnected` state (see below).
2. Restores any buttons stuck in `gotk-loading` state.
3. Attempts to reconnect with exponential backoff: immediate, 2s, 5s, 10s, capped at 30s.
4. On successful reconnect, opens a new WebSocket. The first command sent after reconnect carries the current URL (`location.pathname`), which triggers lazy view creation for the correct page.
5. Applies the `gotk-connected` state.

### Connection State CSS

The thin client toggles CSS classes on `<body>` to reflect connection state:

- `gotk-connected` — WebSocket is open.
- `gotk-disconnected` — WebSocket is closed, reconnecting.

Applications can use these to provide visual feedback:

```css
body.gotk-disconnected #main { opacity: 0.5; pointer-events: none; }
body.gotk-disconnected .gotk-offline-banner { display: flex; }
```

## Background Tasks

Background tasks (goroutines spawned by commands) run outside the request/response cycle. They need to push events to connections when they complete.

### Spawning a Task

Commands create a TaskContext via `ctx.NewTask()`:

```go
func (s *Server) SendMessage(ctx *gotk.CommandContext) error {
    msg, _ := s.db.CreateMessage(ctx.Payload.String("message"))
    gotk.Dispatch(ctx.Dispatcher(), MessageSentEvent{ID: msg.ID, Content: msg.Content})

    go s.processWithClaude(ctx.NewTask(), msg.ID)
    return nil
}
```

### Targeted Push

Push an event to the originating connection's active view:

```go
func (s *Server) processWithClaude(tctx *gotk.TaskContext, msgID int64) {
    result := s.claude.Run(msgID)

    // This dispatches to the originating connection's active view (lastURL)
    tctx.DispatchTo(ResponseReceivedEvent{Content: result.Content})
}
```

If the connection has disconnected or has no active view (no commands received yet), the event is silently dropped.

### Broadcast

Push an event to all connections' active views:

```go
func (s *Server) processWithClaude(tctx *gotk.TaskContext, msgID int64) {
    result := s.claude.Run(msgID)

    // Targeted: originating connection's active view
    tctx.DispatchTo(ResponseReceivedEvent{Content: result.Content})

    // Broadcast: all connections' active views (each view decides whether to handle it)
    tctx.Broadcast(SidebarUpdatedEvent{})
}
```

`Broadcast` iterates all connections and dispatches the event to each connection's active view (the view for the `lastURL`). Views that haven't registered for `SidebarUpdatedEvent` silently ignore it. Connections with no active view are skipped.

## Thin Client Behavior

Pseudocode for the thin client:

```
on page load:
  scan DOM for gotk-* attributes, bind handlers
  connect WebSocket

on WS open:
  set body.gotk-connected, remove body.gotk-disconnected

on WS close:
  set body.gotk-disconnected, remove body.gotk-connected
  restore any buttons stuck in gotk-loading state
  schedule reconnect with exponential backoff

on WS reconnect:
  connect WebSocket
  (first command carries current URL, server lazily creates view)

on gotk-click / gotk-input / gotk-on:
  collect payload from gotk-collect / gotk-val-* / gotk-payload
  if gotk-loading:
    disable element, swap text to gotk-loading value
    store ref for restoration on response
  send {cmd, payload, ref, url: location.pathname + location.search} over WS

on WS message:
  if ref matches a gotk-loading element:
    restore original text, re-enable element
  for each instruction in ins:
    html        → set innerHTML / append / prepend / remove
    template    → clone <template> content into target
    populate    → set values on named elements in container
    navigate    → fetch page, swap DOM, pushState
    attr-set    → element.setAttribute
    attr-remove → element.removeAttribute
    set-value   → element.value = value
    dispatch    → element.dispatchEvent(new CustomEvent(...))
    focus       → element.focus()
    exec        → call registered JS function
  re-scan new/changed DOM nodes for gotk-* attributes

on gotk-navigate click:
  fetch page, swap DOM, pushState

on popstate (back/forward):
  fetch page, swap DOM (no pushState)
```

After every `html` or `template` instruction, the thin client re-scans the affected subtree for new `gotk-*` attributes. This ensures dynamically inserted content is interactive.

## Project Structure

```
gotk/
  command_context.go   # CommandContext — payload, dispatcher, task spawning
  view_context.go      # ViewContext — instruction builder, template rendering
  task_context.go      # TaskContext — background task broadcast/dispatch
  dispatcher.go        # Dispatcher, Register[T], Dispatch[T] — generic event routing
  instructions.go      # Instruction types
  mux.go               # Command routing, view tracking, connection registry
  websocket.go         # WebSocket handling, connection lifecycle
  payload.go           # Typed payload accessors
  client.js            # Thin client (~200 LOC) — embedded via go:embed
  test_helpers.go      # TestCommandContext with event capture
```

## Testability

### Commands

Unit tested with `go test`. Call the handler, inspect emitted events.

```go
func TestCreateTodo(t *testing.T) {
    app := newTestApp()
    ctx := gotk.NewTestCommandContext()
    ctx.SetPayload(map[string]any{"title": "Buy milk"})

    err := app.CreateTodo(ctx)
    require.NoError(t, err)

    events := ctx.Events()
    require.Len(t, events, 1)
    e := events[0].(TodoCreatedEvent)
    assert.Equal(t, "Buy milk", e.Title)
}
```

### Views

Unit tested with `go test`. Call the event handler, inspect instructions.

```go
func TestOnTodoCreated(t *testing.T) {
    vctx := gotk.NewTestViewContext()
    d := gotk.NewDispatcher()
    view := NewTodoView(d, vctx)

    view.OnTodoCreated(TodoCreatedEvent{ID: 1, Title: "Buy milk"})

    ins := vctx.Instructions()
    assert.Equal(t, "html", ins[0].Op)
    assert.Equal(t, "#todo-list", ins[0].Target)
    assert.Contains(t, ins[0].HTML, "Buy milk")
}
```

### Integration

Test the full flow: command → event → view → instructions.

```go
func TestCreateTodoIntegration(t *testing.T) {
    app := newTestApp()
    vctx := gotk.NewTestViewContext()
    d := gotk.NewDispatcher()
    NewTodoView(d, vctx)

    ctx := gotk.NewTestCommandContext()
    ctx.SetDispatcher(d)
    ctx.SetPayload(map[string]any{"title": "Buy milk"})

    app.CreateTodo(ctx)

    ins := vctx.Instructions()
    assert.Equal(t, "html", ins[0].Op)
    assert.Contains(t, ins[0].HTML, "Buy milk")
}
```

### What Doesn't Need Testing

The thin client is ~200 lines of stable code. It maps instructions to DOM operations. It is tested once and rarely changes. No Playwright or Selenium required for the vast majority of application testing.

The Dispatcher is ~10 lines of framework code — tested once in the gotk package, not per-application.

## Design Principles

1. **Locality of behavior.** All UI update logic for a page lives in the view. The view, its template, and its CSS can be co-located in the same directory. Looking at a view tells you everything about how a page responds to events.

2. **Commands are pure business logic.** Commands don't know about the DOM, CSS selectors, or page structure. They read input, do work, and emit events. This makes commands reusable — the same command can serve different views, or be called from different pages.

3. **Views are opt-in subscribers.** A view registers only for events it cares about. New events can be added without touching existing views. Events that nobody handles are silently dropped.

4. **Explicit over implicit.** Views explicitly declare what to change in the DOM via instructions. There is no automatic re-rendering, no diffing, no virtual DOM. The developer controls what updates and when.

5. **Server is the source of truth for data.** Business state lives in databases, accessed by commands. The DOM holds only UI state (what's visible, what's focused, what's loading).

6. **Single language.** Go for commands, Go for views, Go templates for HTML rendering. `go test` covers all layers except the thin client.

7. **The thin client is stable infrastructure.** It should rarely change. All application behavior is expressed through commands, events, and views — not client-side code.

8. **Progressive enhancement of interactivity.** Initial page load is full SSR over HTTP. WebSocket adds real-time interactivity (commands, events, server push). Each layer is optional — an app works with SSR alone, WebSocket adds live updates.

## Summary Table

| Concern | Where it runs | How it's tested |
| --- | --- | --- |
| Business logic | Server (commands) | `go test`, assert on emitted events |
| Data access | Server (commands) | `go test`, unit |
| UI updates | Server (views) | `go test`, assert on instructions |
| HTML rendering | Server (Go templates) | `go test`, render + assert |
| UI state (modals, toggles) | Server (commands + views) | `go test`, unit |
| Event binding | Browser (thin client) | Manual / stable |
| Instruction application | Browser (thin client) | Manual / stable |
| Event dispatch | Server (Dispatcher) | `go test` in gotk package |

## AI Agent Interface

### Why the Command Layer Is Not Just Another REST API

A common question: if commands are structured JSON in/out over WebSocket, how is this different from building a REST API?

For a pure backend-to-backend integration, it is not meaningfully different. A REST API would serve just as well. The difference is: **you don't build one.**

With a typical web app, you build the UI layer AND a separate REST API, then maintain both, keep them in sync, and test both. With gotk, the command layer that serves the UI *is* the programmatic interface. There is nothing extra to build, document, or maintain.

The second difference is **contextual discovery**. A REST API gives you a flat list of endpoints (via OpenAPI, Swagger, etc.). The gotk HTML gives you state-aware actions in context:

```html
<!-- This only appears when the user has items -->
<button gotk-click="delete-todo" gotk-val-id="42">Delete</button>

<!-- This shows what fields are needed, with current values -->
<input name="name" value="Alice">
<input name="email" value="alice@test.com">
<button gotk-click="save-user" gotk-collect="#user-form">Save</button>
```

An agent reading this page knows: "right now, I can delete todo 42, and I can save a user whose current name is Alice." A REST API endpoint like `DELETE /todos/:id` tells you the shape but not the current state.

### How an AI Agent Uses gotk

An agent does not need a browser. It connects via WebSocket and sends commands directly:

```json
{"cmd": "create-todo", "payload": {"title": "Buy milk"}, "ref": "a1"}
```

It receives structured instructions in response:

```json
{"ref": "a1", "ins": [{"op": "html", "target": "#todo-list", "html": "<li>Buy milk</li>", "mode": "append"}]}
```

No screenshots, no DOM parsing, no simulated clicks. The agent can also read the HTML fragments in `html` instructions to understand what changed in human-readable terms.

### Agent Interaction Levels

An agent can choose its level of interaction depending on capability:

| Level | What the agent does | Requires |
| --- | --- | --- |
| Command-only | Sends commands, reads instruction JSON | WebSocket connection |
| HTML-aware | Reads SSR pages to discover available commands and current state | HTTP GET + WebSocket |
| Full context | Parses `gotk-*` attributes, `name` fields, and page structure | HTML parsing |

All three levels use the same command protocol. No separate API surface needed.

### The Self-Describing UI

The `gotk-*` attributes on HTML elements serve as a machine-readable description of the UI's capabilities. An agent that can read HTML understands:

- `gotk-click="create-user"` — "I can create a user by triggering this command."
- `gotk-collect="#user-form"` — "I need to provide data from these fields."
- `<input name="email">` — "email is a required/available input."

The HTML IS the API documentation. The rendered page shows what actions are available, what inputs they expect, and what the current state is.

## Implementation Details

This section defines the exact Go types, conventions, and behaviors needed to implement the framework without ambiguity.

### Go Types

#### Handler and Dispatcher

```go
// Handler is a typed event handler function.
type Handler[T any] func(T)

// Dispatcher routes typed events to view handlers.
// Each WebSocket connection has its own Dispatcher.
type Dispatcher struct {
    handlers map[any]any  // reflect.Type → Handler[T] (type-erased)
}

func NewDispatcher() *Dispatcher

// Register adds a typed handler to the dispatcher.
// Only one handler per event type — last registration wins.
func Register[T any](d *Dispatcher, h Handler[T]) {
    key := reflect.TypeOf((*T)(nil)).Elem()
    d.handlers[key] = h
}

// Dispatch routes an event to its registered handler.
// If no handler is registered for the event type, the event is silently dropped.
func Dispatch[T any](d *Dispatcher, event T) {
    key := reflect.TypeOf((*T)(nil)).Elem()
    if h, ok := d.handlers[key]; ok {
        h.(Handler[T])(event)
    }
}
```

#### CommandHandler

```go
// CommandHandlerFunc is the signature for server command handlers.
type CommandHandlerFunc func(ctx *CommandContext) error
```

#### Payload

```go
// Payload wraps the command payload with typed accessors.
// All accessors parse from the underlying map[string]any.
// String values from the DOM (e.g., gotk-val-id="42") are coerced:
// Int/Float parse strings via strconv. Bool treats "true"/"1" as true.
// Missing keys return zero values (no error).
type Payload struct {
    data map[string]any
}

func (p Payload) String(key string) string
func (p Payload) Int(key string) int
func (p Payload) Int64(key string) int64
func (p Payload) Float(key string) float64
func (p Payload) Bool(key string) bool
func (p Payload) Map() map[string]any  // returns the raw map
```

#### Instruction

```go
// Instruction is a single DOM operation. All fields are optional except Op.
// Only the fields relevant to each Op are serialized to JSON.
type Instruction struct {
    Op      string         `json:"op"`
    Target  string         `json:"target,omitempty"`
    HTML    string         `json:"html,omitempty"`
    Mode    string         `json:"mode,omitempty"`     // replace (default), append, prepend, remove
    Source  string         `json:"source,omitempty"`   // template source selector
    Attr    string         `json:"attr,omitempty"`
    Value   string         `json:"value,omitempty"`
    Event   string         `json:"event,omitempty"`
    Detail  map[string]any `json:"detail,omitempty"`
    URL     string         `json:"url,omitempty"`
    Name    string         `json:"name,omitempty"`     // exec function name
    Args    map[string]any `json:"args,omitempty"`
    Cmd     string         `json:"cmd,omitempty"`      // async command name
    Payload map[string]any `json:"payload,omitempty"`
    Data    map[string]any `json:"data,omitempty"`     // populate data
}
```

A single struct with optional fields, not separate types per op. This keeps JSON serialization simple and allows the thin client to use a single switch on `op`.

#### CommandContext

```go
// CommandContext is passed to server command handlers. It provides access to
// the command payload, the connection's Dispatcher, and task spawning.
// It does NOT have instruction-producing methods — commands emit events,
// views produce instructions.
type CommandContext struct {
    Payload Payload
    // internal: dispatcher *Dispatcher, conn *Conn
}

// Dispatcher returns the connection's Dispatcher for emitting events.
func (c *CommandContext) Dispatcher() *Dispatcher

// NewTask creates a TaskContext for background work.
// The TaskContext holds a reference to the originating connection's
// Dispatcher and to all connections for broadcast.
func (c *CommandContext) NewTask() *TaskContext
```

#### ViewContext

```go
// ViewContext is available to view event handlers. It provides the full
// instruction builder and template rendering. This is the only context
// that can produce DOM instructions.
type ViewContext struct {
    // internal: instructions []Instruction, templates any
}

// Instruction producers — each appends to the internal instructions slice.
func (c *ViewContext) HTML(target, html string, mode ...string)
func (c *ViewContext) Remove(target string)
func (c *ViewContext) Template(source, target string)
func (c *ViewContext) Populate(target string, data map[string]any)
func (c *ViewContext) Navigate(url string, targetAndHTML ...string)
func (c *ViewContext) AttrSet(target, attr string, value ...string)
func (c *ViewContext) AttrRemove(target, attr string)
func (c *ViewContext) SetValue(target, value string)
func (c *ViewContext) Dispatch(target, event string, detail ...map[string]any)
func (c *ViewContext) Focus(target string)
func (c *ViewContext) Exec(name string, args ...map[string]any)
func (c *ViewContext) Error(target, message string)

// Template rendering — delegates to the registered template engine.
func (c *ViewContext) Render(name string, data any) string
```

#### TaskContext

```go
// TaskContext is available to background goroutines. It provides access
// to the originating connection's Dispatcher and broadcast capability.
type TaskContext struct {
    // internal: dispatcher *Dispatcher (originating conn), connRegistry
}

// Dispatcher returns the originating connection's Dispatcher.
// Returns a no-op Dispatcher if the connection has disconnected.
func (t *TaskContext) Dispatcher() *Dispatcher

// Broadcast dispatches an event to all connections' current views.
// Views that haven't registered for the event type silently ignore it.
func (t *TaskContext) Broadcast(event any)
```

#### TestCommandContext

```go
// TestCommandContext is used to test command handlers.
type TestCommandContext struct {
    CommandContext
}

func NewTestCommandContext() *TestCommandContext
func (tc *TestCommandContext) SetPayload(data map[string]any)
func (tc *TestCommandContext) SetDispatcher(d *Dispatcher)
func (tc *TestCommandContext) Events() []any  // returns emitted events (when using a test dispatcher)
```

#### TestViewContext

```go
// TestViewContext is used to test view event handlers.
type TestViewContext struct {
    ViewContext
}

func NewTestViewContext() *TestViewContext
func (tc *TestViewContext) Instructions() []Instruction
```

#### Mux

```go
// Mux routes commands to handlers and manages per-connection views.
type Mux struct { /* internal */ }

func NewMux() *Mux
func (m *Mux) Handle(name string, handler CommandHandlerFunc)

// HandleView registers a factory that creates views based on URL.
// Called lazily when a command arrives for a URL not yet seen on this connection.
func (m *Mux) HandleView(factory func(url string, d *Dispatcher, vctx *ViewContext))

// HandleConnect / HandleDisconnect — connection lifecycle hooks.
func (m *Mux) HandleConnect(fn func(conn *Conn))
func (m *Mux) HandleDisconnect(fn func(conn *Conn))

// ServeWebSocket upgrades an HTTP request to a WebSocket connection
// and starts reading commands / writing instruction responses.
func (m *Mux) ServeWebSocket(w http.ResponseWriter, r *http.Request)
```

### HTML Mode `replace` — innerHTML

The `html` instruction with `mode: "replace"` (default) sets the **innerHTML** of the target element, not outerHTML. The target element itself is preserved. This means the element's attributes, event bindings, and identity remain stable. To remove the element entirely, use `mode: "remove"`.

### Template Loading and Rendering

Templates are loaded using Go's standard `template.ParseGlob` or `template.ParseFS` at application startup. The developer registers a `*template.Template` with the framework:

```go
tmpl := template.Must(template.ParseGlob("templates/**/*.html"))
mux := gotk.NewMux()
mux.SetTemplates(tmpl)
```

`ctx.Render("partials/todo-item", data)` calls `tmpl.ExecuteTemplate` with the given name and data, returning the rendered HTML string.

`gotk.RenderPage(w, "layouts/app", "pages/todos", data)` renders a full page by executing the layout template. The layout uses Go's `{{template "content" .}}` or `{{block "content" .}}` to include the page template. The thin client `<script>` tag is injected automatically at the end of `<body>` by the framework.

### Thin Client Serving

The thin client JS is embedded in the Go binary via `//go:embed`. The framework registers an HTTP handler at `/gotk/client.js` that serves it. `gotk.RenderPage` automatically includes `<script src="/gotk/client.js"></script>` at the end of `<body>`.

### `ref` Generation

The thin client generates `ref` values using an incrementing integer counter starting at 1. Simple, unique within a connection lifetime, and small on the wire. Format: `"1"`, `"2"`, `"3"`, etc.

### View Routing via URL Field

There is no built-in `navigate` command. Instead, every WebSocket message includes a `url` field with the client's current page URL. The server uses this to look up or lazily create the appropriate view:

1. On each command, the server reads the `url` field from the message.
2. It looks up the view in the connection's `views` map (keyed by URL).
3. If no view exists for that URL, it creates one by calling the view factory.
4. It updates `lastURL` on the ConnEntry (used by background tasks for broadcasts).

This eliminates the need for a separate navigate command. When `CommandContext.Navigate(url)` is used, the server includes a `navigate` instruction in the response. The client fetches and renders the page via HTTP, then future commands carry the new URL — the view is created lazily on the next interaction.

### Element Not Found

When an instruction's `target` (or `source`) selector matches no element, the thin client logs a warning to the console and skips the instruction. It does not throw, halt instruction processing, or send an error back to the server. Remaining instructions in the batch continue to execute.

### `gotk-collect` — Collection Rules

The thin client collects values from all elements with a `name` attribute within the `gotk-collect` container:

| Element type | How value is read |
| --- | --- |
| `<input type="text">`, `<textarea>` | `.value` |
| `<input type="number">` | `.value` (string — Payload accessors coerce) |
| `<input type="checkbox">` | `.checked` (boolean) |
| `<input type="radio">` | `.value` of the checked radio in the group |
| `<select>` | `.value` (selected option's value) |
| `<select multiple>` | Array of `.value` for all selected options |
| `[contenteditable]` | `.innerText` |
| Elements with `gotk-val-*` | Collected via dataset, not `.value` |

**Duplicate names:** If multiple elements share the same `name` (e.g., checkboxes), values are collected as an array.

### `gotk-every` Format

Accepts a number followed by a unit suffix:

- `ms` — milliseconds: `500ms`
- `s` — seconds: `30s`
- `m` — minutes: `5m`

Plain numbers without suffix are treated as milliseconds: `5000` = 5 seconds. Minimum interval: `1000ms` (1 second). The thin client clamps lower values.

### `gotk-poll` Cleanup

When an `html` or `template` instruction replaces a subtree that contains `gotk-poll` elements, the thin client clears the associated intervals before replacing the content. The re-scan logic works in two phases:

1. **Teardown:** Before applying `html` (replace mode) or `template`, collect all `gotk-poll` elements within the target and clear their intervals.
2. **Setup:** After applying the instruction, scan the new subtree for `gotk-*` attributes and bind handlers (including new `gotk-poll` intervals).

This also applies to `gotk-on` and `gotk-input` event listeners.

### `exec` Function Registration

Client-side JS functions for the `exec` instruction are registered via a global registry on the thin client:

```html
<script>
  gotk.register("lockScroll", function(args) {
    document.body.style.overflow = "hidden";
  });

  gotk.register("unlockScroll", function(args) {
    document.body.style.overflow = "";
  });
</script>
```

`gotk.register(name, fn)` adds the function to an internal map. The `exec` instruction calls `registeredFns[name](args)`. Unknown function names log a console warning and are skipped.

### Session and Authentication

WebSocket connections are authenticated via the HTTP upgrade request. The server's WebSocket handler has access to the standard `http.Request`, including cookies and headers. The framework does not impose an authentication mechanism — the developer uses middleware or checks credentials in the upgrade handler:

```go
router.GET("/ws", authMiddleware(mux.ServeWebSocket))
```

Each WebSocket connection is independent. The framework does not provide cross-connection state or session storage — the developer uses their existing session mechanism (cookies, JWTs, database sessions).

### Server Push

The server can push instructions to connections through the event system. Commands and tasks emit events; views produce instructions; the framework sends them to the client.

For cases that don't fit the view model (e.g., system-wide notifications), the connection reference is still available:

```go
mux.HandleConnect(func(conn *gotk.Conn) {
    app.connections[conn.ID()] = conn
})

mux.HandleDisconnect(func(conn *gotk.Conn) {
    delete(app.connections, conn.ID())
})
```

### Error Response for Unknown Commands

When the mux receives a command name that has no registered handler, it responds with an instruction sequence containing an error:

```json
{
  "ref": "a1b2",
  "ins": [{"op": "exec", "name": "console.warn", "args": {"message": "unknown command: foo"}}],
  "error": "unknown command: foo"
}
```

The `error` field is a top-level string on the response. The thin client logs it to the console. No DOM changes are made unless the developer registers an error handler.

### Concurrent Commands

Multiple commands can be in flight simultaneously. Each has a unique `ref`. Responses may arrive in any order — the thin client uses `ref` to correlate responses to their originating elements (for `gotk-loading` restoration). The server mux handles commands sequentially per connection (one goroutine per connection reads messages, dispatches handlers). If a handler is slow, it blocks subsequent commands on that connection. Long-running handlers should spawn background tasks via `ctx.NewTask()`.

### Re-scan Scope

After an `html` (any mode) or `template` instruction, the thin client re-scans **only the target element and its subtree** for new `gotk-*` attributes. After `populate` or `set-value`, no re-scan occurs (values changed, not structure).

## Worked Example: Conversation Page

This example shows how the architecture works end-to-end for a conversation feature with message sending, background AI processing, and real-time updates.

### Events

```go
type MessageSentEvent struct {
    ID      int64
    Content string
}

type ProcessingStartedEvent struct {
    PromptRequestID int64
    StartedAt       int64
}

type ResponseReceivedEvent struct {
    ID      int64
    Content string
}

type ProcessingErrorEvent struct {
    Message string
}

type SidebarUpdatedEvent struct{}
```

### Command

```go
func (s *Server) SendMessage(ctx *gotk.CommandContext) error {
    prID := ctx.Payload.Int64("prompt_request_id")
    message := strings.TrimSpace(ctx.Payload.String("message"))
    if message == "" {
        return nil
    }

    msg, err := s.db.CreateMessage(prID, "user", message)
    if err != nil {
        gotk.Dispatch(ctx.Dispatcher(), ProcessingErrorEvent{Message: "Failed to save message"})
        return nil
    }

    gotk.Dispatch(ctx.Dispatcher(), MessageSentEvent{ID: msg.ID, Content: msg.Content})

    // Launch background AI processing
    tctx := ctx.NewTask()
    gotk.Dispatch(ctx.Dispatcher(), ProcessingStartedEvent{
        PromptRequestID: prID,
        StartedAt:       time.Now().Unix(),
    })
    go s.backgroundProcess(tctx, prID)

    return nil
}
```

### View

```go
type ConversationView struct {
    ctx *gotk.ViewContext
}

func NewConversationView(d *gotk.Dispatcher, ctx *gotk.ViewContext) *ConversationView {
    v := &ConversationView{ctx: ctx}
    gotk.Register(d, v.OnMessageSent)
    gotk.Register(d, v.OnProcessingStarted)
    gotk.Register(d, v.OnResponseReceived)
    gotk.Register(d, v.OnProcessingError)
    gotk.Register(d, v.OnSidebarUpdated)
    return v
}

func (v *ConversationView) OnMessageSent(e MessageSentEvent) {
    v.ctx.HTML("#conversation", v.ctx.Render("message", struct{ Role, Content string }{"user", e.Content}), gotk.Append)
    v.ctx.SetValue("#message-input", "")
    v.ctx.AttrSet("#message-input", "disabled", "true")
    v.ctx.AttrSet("#send-btn", "disabled", "true")
    v.ctx.Exec("scrollConversation")
}

func (v *ConversationView) OnProcessingStarted(e ProcessingStartedEvent) {
    v.ctx.Remove("#repo-status")
    v.ctx.HTML("#conversation", v.ctx.Render("processing-indicator", e), gotk.Append)
    v.ctx.Exec("updateElapsedTimers")
}

func (v *ConversationView) OnResponseReceived(e ResponseReceivedEvent) {
    v.ctx.Remove("#repo-status")
    v.ctx.HTML("#conversation", v.ctx.Render("message", struct{ Role, Content string }{"assistant", e.Content}), gotk.Append)
    v.ctx.AttrRemove("#message-input", "disabled")
    v.ctx.AttrRemove("#send-btn", "disabled")
    v.ctx.Focus("#message-input")
    v.ctx.Exec("scrollConversation")
}

func (v *ConversationView) OnProcessingError(e ProcessingErrorEvent) {
    v.ctx.Error("#conversation", e.Message)
    v.ctx.AttrRemove("#message-input", "disabled")
    v.ctx.AttrRemove("#send-btn", "disabled")
}

func (v *ConversationView) OnSidebarUpdated(e SidebarUpdatedEvent) {
    // Re-render sidebar from current state
    v.ctx.HTML("#sidebar", v.ctx.Render("sidebar", nil))
}
```

### Background Task

```go
func (s *Server) backgroundProcess(tctx *gotk.TaskContext, prID int64) {
    result, err := s.claude.Run(prID)
    if err != nil {
        gotk.Dispatch(tctx.Dispatcher(), ProcessingErrorEvent{Message: err.Error()})
        return
    }

    gotk.Dispatch(tctx.Dispatcher(), ResponseReceivedEvent{ID: result.ID, Content: result.Content})
    tctx.Broadcast(SidebarUpdatedEvent{})
}
```

### Flow

```
User clicks Send
  └─ thin client sends {cmd: "send-message", payload: {message: "hello"}}
       └─ Server: SendMessage(ctx)
            ├─ saves message to DB
            ├─ Dispatch → MessageSentEvent
            │     └─ ConversationView.OnMessageSent
            │          ├─ html: append user bubble
            │          ├─ set-value: clear input
            │          ├─ attr-set: disable input
            │          └─ exec: scroll
            ├─ Dispatch → ProcessingStartedEvent
            │     └─ ConversationView.OnProcessingStarted
            │          ├─ remove: old status
            │          ├─ html: append spinner
            │          └─ exec: start timer
            └─ go backgroundProcess(tctx, prID)
                  │
                  ... Claude API call ...
                  │
                  ├─ Dispatch(tctx.Dispatcher()) → ResponseReceivedEvent
                  │     └─ ConversationView.OnResponseReceived
                  │          ├─ remove: spinner
                  │          ├─ html: append assistant bubble
                  │          ├─ attr-remove: enable input
                  │          └─ exec: scroll
                  └─ tctx.Broadcast → SidebarUpdatedEvent
                        └─ all connections' views that handle it
```

## Future Considerations

### `gotk-do` — Inline Actions (Locality of Behavior)

**Status:** Needs further design work.

**Problem:** Simple UI operations like toggling a sidebar or closing a modal currently require a server command — a Go function in a separate file. Looking at the HTML, you see `gotk-click="toggle-sidebar"` but have no idea what it does without finding the Go function. This violates the Locality of Behavior principle: the behavior of a unit of code should be as obvious as possible by looking only at that unit of code.

**Idea:** A `gotk-do` attribute for simple DOM manipulations executed directly by the thin client, no server round-trip:

```html
<!-- Toggle an attribute -->
<button gotk-do="attr-toggle #sidebar hidden">☰</button>

<!-- Set an attribute (close modal) -->
<button gotk-do="attr-set #modal hidden">Close</button>

<!-- Remove an attribute (open modal) -->
<button gotk-do="attr-remove #modal hidden">Open</button>

<!-- Remove an element (dismiss notification) -->
<button gotk-do="remove #notification-5">Dismiss</button>

<!-- Toggle a CSS class -->
<button gotk-do="class-toggle #menu active">Toggle Menu</button>

<!-- Focus an element -->
<button gotk-do="focus #search-input">Search</button>
```

**Chaining** with `;`:

```html
<button gotk-do="attr-remove #modal hidden; focus #modal-title">Open</button>
```

**Combining** with server commands — `gotk-do` runs first (instant), then the command fires:

```html
<button gotk-do="attr-set #save-btn disabled" gotk-click="save-user" gotk-collect="#form">Save</button>
```

**Supported actions** (intentionally small and fixed — not a scripting language):

| Action | Syntax | Effect |
| --- | --- | --- |
| `attr-set` | `attr-set <target> <attr> [value]` | Set an attribute. |
| `attr-remove` | `attr-remove <target> <attr>` | Remove an attribute. |
| `attr-toggle` | `attr-toggle <target> <attr>` | Toggle an attribute. |
| `class-add` | `class-add <target> <class>` | Add a CSS class. |
| `class-remove` | `class-remove <target> <class>` | Remove a CSS class. |
| `class-toggle` | `class-toggle <target> <class>` | Toggle a CSS class. |
| `remove` | `remove <target>` | Remove element from DOM. |
| `focus` | `focus <target>` | Focus an element. |

No conditionals, no data access, no loops. If you need logic, use a server command (via `gotk-click`).

### Component Pattern — Co-located Templates and Commands

**Status:** Usage pattern, not a framework feature.

**Problem:** A command like `gotk-click="open-user-modal"` tells you a command name, but not where to find the code. The template lives in one directory, the commands in another, the views in a third. Understanding the full behavior of a UI element requires jumping between files.

**Idea:** Co-locate a component's template, view, commands, and tests in a single Go package:

```
app/
  conversation/
    conversation.html       # the HTML template
    conversation.go         # ConversationView + event handlers
    commands.go             # command handlers (SendMessage, etc.)
    events.go               # event types
    conversation_test.go    # tests for commands and view handlers
```

The view, its template, and the commands that serve it live together. Looking at the directory tells you everything about how the conversation page works.

### JSON-First Server Commands with Frontend Rendering

**Status:** Needs further design work.

**Problem:** Server commands currently return pre-rendered HTML via view instructions. This means AI agents receive opaque HTML strings rather than structured data.

**Current recommendation:** Keep server-rendered HTML via views as the default for now. The `populate` instruction handles the common case of filling form fields with data.

### Linking HTTP Page Requests to WebSocket Connections

**Status:** Aspirational. Needs design work.

**Problem:** When a browser makes an HTTP page request (e.g., `GET /todos/42`) and then opens a WebSocket, the server has no way to link the two. The HTTP request and the WebSocket connect are independent. The view is created lazily on the first command, not during the HTTP request. This means the server can't pre-build the view during the HTTP request or pass page-specific context from the HTTP handler to the view factory.

**Idea:** Generate a unique page token during the HTTP request, embed it in the rendered page (e.g., in a `<meta>` tag or a JS variable), and pass it as a query parameter when opening the WebSocket (`/ws?token=abc123`). The server maps the token to the HTTP request context (URL, session, pre-computed data) and uses it to initialize the view.

**Constraint:** Cookies cannot be used because they are shared across tabs. A per-tab token is needed so each tab gets its own view. The token must be short-lived and single-use to avoid stale state.
