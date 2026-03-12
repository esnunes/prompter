# Prompter

Prompter is a web app that helps generate and publish GitHub issue prompts through
conversational AI (Claude CLI). Built with Go, SQLite, gotk (WebSocket command framework),
and server-rendered HTML templates.

## Build & Test

```bash
go generate ./cmd/wasm/  # compile WASM frontend commands (requires TinyGo)
go build ./...            # build all packages
go test ./...             # run all tests
go vet ./...              # static analysis
```

CSS/JS are served via `go:embed` from `internal/server/static/`. WASM binary is compiled
with TinyGo and embedded from `gotk/app.wasm`. A placeholder exists so `go build` works
without TinyGo installed (frontend commands fall back to server-side via WebSocket).

## Project Structure

- `cmd/prompter/main.go` — CLI entry point
- `cmd/wasm/` — TinyGo WASM entry point (frontend commands compiled to WebAssembly)
- `internal/server/server.go` — HTTP server, routes, gotk mux setup
- `internal/server/handlers.go` — HTTP handlers + gotk command handlers
- `internal/server/templates/` — Go HTML templates (for `go:embed`)
- `internal/server/static/` — CSS, JS assets (for `go:embed`)
- `internal/db/db.go` — SQLite schema + migrations
- `internal/db/queries.go` — Database queries
- `internal/claude/claude.go` — Claude CLI wrapper
- `internal/models/models.go` — Data models
- `internal/paths/paths.go` — Cache directory helper (`$XDG_CACHE_HOME/prompter/`)
- `gotk/` — gotk framework (WebSocket command framework, embedded in this repo)

## Conventions

- **Go 1.25.5** with `modernc.org/sqlite` (pure Go, no CGO)
- **Storage:** `$XDG_CACHE_HOME/prompter/` (or `~/.cache/prompter/`)
- **CSS:** Theme UI spec with `tokens.css` + `style.css`
- **gotk for all interactivity** — server commands via WebSocket, client commands via TinyGo/WASM

## gotk Framework

gotk is a command-based web framework where every UI interaction is a Go function
that receives input and returns DOM instructions. The browser is treated as a thin
WebView: commands go from UI to server, instructions come back to update the DOM.

Full spec: https://github.com/esnunes/gotk/blob/main/docs/ui-spec.md

### Architecture

```
Browser (thin client ~200 LOC JS)
    ↕ WebSocket
Server (Go command handlers)
    → returns []gotk.Instruction
```

1. User clicks a `gotk-click` button
2. Thin client collects payload from DOM, sends `{cmd, payload, ref}` over WebSocket
3. Server handler runs, produces instructions
4. Client applies instructions sequentially to the DOM
5. Client re-scans affected subtree for new `gotk-*` attributes

### HTML Attributes

| Attribute | Purpose | Example |
|-----------|---------|---------|
| `gotk-click="cmd"` | Send command on click | `<button gotk-click="save">Save</button>` |
| `gotk-collect="#sel"` | Collect named form fields from container | `gotk-collect="#my-form"` |
| `gotk-val-key="val"` | Attach key-value to payload | `gotk-val-id="42"` |
| `gotk-payload='{"k":"v"}'` | Hardcoded JSON payload | `gotk-payload='{"id": 42}'` |
| `gotk-loading="text"` | Disable + swap text while in flight | `gotk-loading="Saving..."` |
| `gotk-navigate` | SPA navigation via WebSocket | `<a href="/page" gotk-navigate>` |
| `gotk-input="cmd"` | Send command on input event | `<input gotk-input="search">` |
| `gotk-debounce="ms"` | Debounce for gotk-input | `gotk-debounce="300"` |
| `gotk-on="event:cmd"` | Send command on arbitrary DOM event | `gotk-on="dragend:reorder"` |

**Payload priority** (highest wins): `gotk-val-*` > `gotk-collect` > `gotk-payload`

**Collection behavior for `gotk-collect`:**
- Text inputs, textareas: string value
- Checkboxes: collected as arrays (multiple checked values)
- Radio buttons: string value of checked radio
- Select: value of selected option
- Hidden inputs: string value

### Server-Side: Registering Commands

```go
// In registerGotkCommands():
s.gotkMux.Handle("my-command", func(ctx *gotk.Context) error {
    // Read payload
    id := ctx.Payload.String("id")        // string accessor
    count := ctx.Payload.Int("count")      // int accessor (coerces strings)
    data := ctx.Payload.Map()              // raw map[string]any

    // Produce DOM instructions
    ctx.HTML("#target", "<p>Hello</p>")                  // replace innerHTML
    ctx.HTML("#list", "<li>New</li>", gotk.Append)       // append to element
    ctx.HTML("#list", "<li>First</li>", gotk.Prepend)    // prepend to element
    ctx.Remove("#old-element")                            // remove element from DOM
    ctx.AttrSet("#el", "disabled", "true")                // set attribute
    ctx.AttrRemove("#el", "disabled")                     // remove attribute
    ctx.SetValue("#input", "new value")                   // set form value
    ctx.Focus("#input")                                   // focus element
    ctx.Navigate("/new-url")                              // push URL to history
    ctx.Exec("jsFunction")                                // call registered JS function
    ctx.Exec("jsFunction", map[string]any{"key": "val"})  // with args
    ctx.Error("#container", "Something went wrong")       // show error div
    ctx.Dispatch("#el", "custom-event")                   // dispatch CustomEvent

    return nil
})
```

### Server Push (Broadcasting)

To push updates to all connected clients (e.g., after background processing):

```go
// Track connections
s.gotkMux.HandleConnect(func(conn *gotk.Conn) {
    s.gotkConns.Store(conn.ID(), conn)
})
s.gotkMux.HandleDisconnect(func(conn *gotk.Conn) {
    s.gotkConns.Delete(conn.ID())
})

// Push instructions to all clients
func (s *Server) pushAll(ins []gotk.Instruction) {
    s.gotkConns.Range(func(_, v any) bool {
        conn := v.(*gotk.Conn)
        conn.Push(ins)
        return true
    })
}

// Usage: push from background goroutine
s.pushAll([]gotk.Instruction{
    {Op: "html", Target: "#status", HTML: "Done!", Mode: gotk.Replace},
})
```

### Building Instruction Sets

For complex pushes, build `[]gotk.Instruction` slices:

```go
func (s *Server) buildMyPush() []gotk.Instruction {
    var ins []gotk.Instruction
    ins = append(ins, gotk.Instruction{Op: "html", Target: "#el", HTML: "...", Mode: gotk.Append})
    ins = append(ins, gotk.Instruction{Op: "attr-remove", Target: "#btn", Attr: "disabled"})
    ins = append(ins, gotk.Instruction{Op: "exec", Name: "scrollToBottom"})
    return ins
}
```

### HTML in Templates (gotk attributes)

```html
<!-- Simple command button -->
<button gotk-click="delete-item" gotk-val-id="42" class="btn">Delete</button>

<!-- Collect form fields -->
<div id="my-form-fields">
  <input type="hidden" name="item_id" value="42">
  <input type="text" name="title" placeholder="Title">
  <textarea name="body"></textarea>
</div>
<button gotk-click="save-item"
        gotk-collect="#my-form-fields"
        gotk-loading="Saving..."
        class="btn btn-primary">Save</button>
```

### Navigate Instruction

`ctx.Navigate(url)` only pushes to browser history — it does NOT fetch new HTML.
To update page content after an action, push DOM instructions directly instead of
navigating. Only use Navigate when you also provide target + HTML, or when you want
to update the URL bar alongside DOM instruction updates.

### Registering Client-Side JS Functions

For the `exec` instruction, register functions in your app JS:

```js
gotk.register("scrollConversation", function() {
    var el = document.getElementById("conversation");
    if (el) el.scrollTop = el.scrollHeight;
});
```

### Testing Commands

```go
func TestMyCommand(t *testing.T) {
    ctx := gotk.NewTestContext()
    ctx.SetPayload(map[string]any{"id": "42"})

    err := handler(ctx)
    require.NoError(t, err)

    ins := ctx.Instructions()
    assert.Equal(t, "html", ins[0].Op)
    assert.Equal(t, "#target", ins[0].Target)
    assert.Contains(t, ins[0].HTML, "expected content")
}
```

### DOMPurify

gotk sanitizes all HTML through DOMPurify before inserting into the DOM.
`gotk-*` attributes are allowlisted. If you add custom attributes that need to
survive sanitization, add them to the DOMPurify hook in `gotk/client.js`.

### Connection State CSS

```css
body.gotk-disconnected #main { opacity: 0.5; pointer-events: none; }
```

The thin client sets `gotk-connected` / `gotk-disconnected` classes on `<body>`.
On reconnect, it sends a `navigate` command for the current URL to re-sync state.

## Claude CLI Integration

- `claude -p --output-format json --json-schema <schema>` returns `structured_output` object (NOT `result` string)
- Always parse `structured_output` first, fall back to `result` string, then direct parse
- See `docs/solutions/integration-issues/claude-cli-structured-output-parsing.md`
