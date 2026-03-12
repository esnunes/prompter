---
title: "feat: Complete gotk migration with WASM frontend commands"
type: feat
date: 2026-03-12
issue: https://github.com/esnunes/prompter/issues/50
brainstorm: docs/brainstorms/2026-03-12-gotk-wasm-frontend-commands-brainstorm.md
---

# Complete gotk Migration with WASM Frontend Commands

## Overview

Complete the migration from HTMX to gotk by: (1) extending gotk with WASM-based
frontend commands per the gotk spec, (2) migrating remaining HTMX interactions to
gotk server commands with server push, and (3) removing HTMX entirely. After this
work, all UI interactions go through gotk's command system — server commands over
WebSocket, client commands via TinyGo/WASM — with no HTMX, no polling, and all
interactive behavior testable via `go test`.

## Problem Statement

The codebase is mid-migration. Core interactions (send-message, answer-question,
cancel-message, publish) already use gotk. But several interactions still use HTMX:

- **Polling:** Sidebar (every 3s), repo status (every 2s) via `hx-trigger`
- **Forms:** Archive/unarchive via inline `fetch()` + `location.reload()`
- **Status delivery:** `handleRepoStatus` returns HTML fragments with inline `<script>` tags
- **HTTP fallbacks:** `HX-Request` header checks in archive/unarchive/publish handlers
- **JS dependencies:** `htmx.min.js`, `idiomorph-ext.min.js`, `htmx:afterSwap`/`htmx:confirm` listeners

Additionally, ~150 LOC of plain JS in `app.js` handles scrolling, timers, validation,
and visibility — all untestable without a browser.

## Proposed Solution

Five phases, strictly ordered to avoid breaking the hybrid state:

1. **Extend gotk framework** with WASM command infrastructure + `gotk-on` attribute
2. **Implement prompter WASM frontend commands** (validation, scrolling, timers, visibility)
3. **Migrate remaining HTMX to gotk server commands** (archive, status push, sidebar push)
4. **Remove HTMX** (library, attributes, CSS, inline scripts, HTTP fallback handlers)
5. **TinyGo build integration** (`go:generate` + `go:embed`)

## Technical Approach

### Architecture

```
Browser (thin client + WASM binary)
    ↕ WebSocket (server commands)
    ↕ WASM local (client commands)
Server (Go command handlers)
    → returns []gotk.Instruction
    → pushAll() for real-time updates
```

**Command routing (per gotk spec):**
1. Page loads → client fetches WASM binary, calls `wasm.listCommands()`, caches as `Set`
2. User clicks `gotk-click="cmd"` → client checks `localCmds.has(cmd)`
3. If local → `wasm.execCommand(cmd, payload)` → apply `ins`, dispatch `async` over WS
4. If not local → send over WebSocket to server

### Implementation Phases

---

#### Phase 1: Extend gotk Framework

Add WASM client-side command support to the gotk package and implement `gotk-on`.

##### 1.1 CommandRegistry type

**File: `gotk/registry.go`**

```go
// CommandRegistry holds frontend command handlers for WASM compilation.
type CommandRegistry struct {
    handlers map[string]HandlerFunc
}

func NewCommandRegistry() *CommandRegistry { ... }
func (r *CommandRegistry) Register(name string, handler HandlerFunc) { ... }
func (r *CommandRegistry) ListCommandsJSON() string { ... }      // returns JSON array
func (r *CommandRegistry) ExecCommandJSON(cmd, payloadJSON string) string { ... }
```

`ExecCommandJSON` creates a `Context`, dispatches to the handler, and returns
`{"ins": [...], "async": [...]}` as JSON. The `async` array carries both `cmd` and
`payload` (matching the existing `AsyncCall` struct in `context.go`).

**File: `gotk/registry_test.go`**

- Test command registration and listing
- Test `ExecCommandJSON` with various payloads
- Test unknown command returns error
- Test async calls are included in response

##### 1.2 Build-tag Context for TinyGo compatibility

The `Context.Render()` method uses `html/template` which doesn't compile under TinyGo.

**File: `gotk/render.go`** — add `//go:build !tinygo` build tag
**File: `gotk/render_stub.go`** — add `//go:build tinygo` with stub `Render()` that panics

This lets WASM commands use all `Context` methods except `Render()`.

##### 1.3 Update client.js for WASM routing

**File: `gotk/client.js`**

Add WASM initialization to the thin client:

```js
// At init:
//   1. Fetch and instantiate WASM binary from /gotk/app.wasm
//   2. Call wasm.listCommands() → cache as Set
//   3. On command dispatch: check localCmds.has(cmd) → WASM or WebSocket

// After WASM exec:
//   1. Parse result JSON → { ins: [...], async: [...] }
//   2. Apply ins[] via existing applyInstruction()
//   3. Dispatch async[] as WebSocket commands
```

- WASM loaded asynchronously — commands that fire before WASM is ready route to WebSocket
- If WASM fails to load: log console warning, all commands route to WebSocket
- `gotk-loading` attribute only applies to WebSocket commands (WASM is synchronous)

##### 1.4 Implement `gotk-on` attribute

**File: `gotk/client.js`** — extend `scanElement()`:

```js
// gotk-on="event:cmd" — binds event listener, sends command on trigger
// Example: gotk-on="keydown:check-enter"
// Payload includes event metadata: { key, shiftKey, ctrlKey, ... }
```

- Parse `event:cmd` format
- Bind event listener to element
- Collect payload from `gotk-val-*`, `gotk-collect`, `gotk-payload` as usual
- Add event properties to payload under `_event` key
- Route through same WASM/WebSocket dispatch

##### 1.5 WASM binary serving

**File: `gotk/embed.go`** — add:

```go
//go:embed app.wasm
var appWASM []byte

func AppWASMHandler() http.HandlerFunc { ... }  // serves at /gotk/app.wasm
```

**File: `gotk/wasm_exec.js`** — embed TinyGo's `wasm_exec.js` runtime

**Tests:**
- `gotk/registry_test.go` — unit tests for CommandRegistry
- `gotk/client.js` — manual verification of WASM loading and routing

**Acceptance criteria:**
- [ ] `CommandRegistry` registers handlers, lists commands as JSON, executes commands
- [ ] `ExecCommandJSON` returns `{"ins": [...], "async": [...]}` format
- [ ] `Context.Render()` excluded from TinyGo builds via build tags
- [ ] `client.js` loads WASM, caches command list, routes locally or to WebSocket
- [ ] `client.js` handles WASM load failure gracefully (console warning, WebSocket fallback)
- [ ] `gotk-on="event:cmd"` binds event listeners and dispatches commands
- [ ] WASM binary served via `go:embed` at `/gotk/app.wasm`
- [ ] All existing gotk tests pass

---

#### Phase 2: Prompter WASM Frontend Commands

Implement the client-side behaviors from `app.js` as WASM commands.

##### 2.1 WASM entry point

**File: `cmd/prompter/wasm/main.go`** (TinyGo build target)

```go
package main

import "github.com/esnunes/prompter/gotk"

var registry *gotk.CommandRegistry

func init() {
    registry = gotk.NewCommandRegistry()
    registry.Register("scroll-conversation", ScrollConversation)
    registry.Register("update-elapsed-timers", UpdateElapsedTimers)
    registry.Register("validate-question-form", ValidateQuestionForm)
    registry.Register("update-form-visibility", UpdateFormVisibility)
    registry.Register("check-enter", CheckEnter)
    registry.Register("init-page", InitPage)
}

//export listCommands
func listCommands() string { return registry.ListCommandsJSON() }

//export execCommand
func execCommand(cmd, payload string) string { return registry.ExecCommandJSON(cmd, payload) }

func main() {} // required by TinyGo
```

##### 2.2 Frontend command implementations

**File: `cmd/prompter/wasm/commands.go`**

| Command | Replaces | Instructions returned |
|---------|----------|----------------------|
| `scroll-conversation` | `scrollConversation()` | `exec("scrollToElement", {target, behavior})` or new scroll instruction |
| `update-elapsed-timers` | `updateElapsedTimers()` | `exec("updateElapsedTimers")` — timer math stays in JS since it needs `Date.now()` |
| `validate-question-form` | `validateQuestionForm()` | Validates payload fields, returns `exec("alert", {msg})` on failure or `async: ["answer-question"]` on success |
| `update-form-visibility` | `updateMessageFormVisibility()` | `attr-set("#message-form", "style", "display:none")` or `attr-remove` |
| `check-enter` | Enter-to-send keydown handler | Checks `_event.key`, `_event.shiftKey` from payload. If Enter without Shift: `async: ["send-message"]` |
| `init-page` | `DOMContentLoaded` init | `exec("renderMarkdown")`, `exec("scrollConversation")`, `focus("#message-input")` |

**File: `cmd/prompter/wasm/commands_test.go`**

```go
func TestValidateQuestionForm_AllAnswered(t *testing.T) {
    ctx := gotk.NewTestContext()
    ctx.SetPayload(map[string]any{
        "q0": "option-a",
        "q1": "option-b",
    })
    err := ValidateQuestionForm(ctx)
    require.NoError(t, err)
    async := ctx.AsyncCalls()
    assert.Len(t, async, 1)
    assert.Equal(t, "answer-question", async[0].Cmd)
}

func TestValidateQuestionForm_MissingAnswer(t *testing.T) {
    ctx := gotk.NewTestContext()
    ctx.SetPayload(map[string]any{"q0": ""})
    err := ValidateQuestionForm(ctx)
    require.NoError(t, err)
    ins := ctx.Instructions()
    // Should return an error/alert instruction, no async calls
    assert.Empty(t, ctx.AsyncCalls())
}
```

##### 2.3 Update templates for WASM commands

**File: `internal/server/templates/conversation.html`**

- Question form submit button: change from `gotk-click="answer-question"` to
  `gotk-click="validate-question-form"` (WASM validates, then async dispatches
  `answer-question` to server)
- Textarea: add `gotk-on="keydown:check-enter"` for Enter-to-send
- Remove old `gotk-click="send-message"` from send button, replace with
  `gotk-on="click:check-enter"` or keep as `gotk-click="send-message"` (since
  send-message is a server command, it routes there directly)

##### 2.4 Update app.js

Remove functions that are now WASM commands:
- `scrollConversation()` — keep as registered JS function (WASM calls via `exec`)
- `updateElapsedTimers()` — keep as registered JS function
- `validateQuestionForm()` — remove (now WASM command)
- `updateMessageFormVisibility()` — remove (now WASM command)
- Enter-to-send keydown handler — remove (now `gotk-on` + WASM command)
- `DOMContentLoaded` init — simplify (WASM `init-page` handles most logic)
- Keep `renderMarkdown()` as registered JS function (marked.js stays)

**Acceptance criteria:**
- [ ] WASM entry point compiles with TinyGo
- [ ] All 6 frontend commands implemented with tests
- [ ] `validate-question-form` validates and async-dispatches `answer-question`
- [ ] `check-enter` handles Enter/Shift+Enter/IME composition
- [ ] Templates updated with WASM command attributes
- [ ] `app.js` simplified — removed functions replaced by WASM commands
- [ ] All existing gotk and prompter tests pass
- [ ] Manual verification: send message, answer questions, scrolling, timers work

---

#### Phase 3: Migrate Remaining HTMX to gotk

Replace all polling with server push and convert remaining HTMX forms to gotk commands.

##### 3.1 Server push for status updates

Currently: `handleRepoStatus` returns HTML fragments that are polled every 2s.
After: Server pushes status instructions directly when state changes.

**File: `internal/server/handlers.go`**

Add push helpers:

```go
func (s *Server) pushStatusUpdate(prID int64, status string, entry repoStatusEntry) {
    ins := s.buildStatusPush(prID, status, entry)
    s.pushAll(ins)
}

func (s *Server) buildStatusPush(prID int64, status string, entry repoStatusEntry) []gotk.Instruction {
    // Returns instructions to update #repo-status div based on status:
    // - "cloning"/"pulling": show spinner + elapsed timer + cancel button
    // - "processing": show "Thinking..." + elapsed timer + cancel button
    // - "responded": remove spinner, append message, enable form
    // - "cancelled"/"error": show retry button
    // - "ready": remove status div
}
```

**Modify `setRepoStatusCloning/Pulling/Processing/Ready/Error`** methods to call
`pushStatusUpdate()` after updating the status map.

**Modify `backgroundSendMessage()`** to push the response directly instead of
setting status to "responded" and waiting for a poll:

```go
// After Claude responds:
// 1. Save message to DB
// 2. Build response push (already exists: buildResponsePush)
// 3. pushAll(responsePush) — already done
// 4. Remove "processing" status — pushStatusUpdate("ready")
```

The `handleRepoStatus` HTTP handler becomes dead code after this change (removed
in Phase 4).

##### 3.2 Server push for sidebar updates

Currently: Sidebar polls every 3s via `hx-get` + `hx-trigger`.

**File: `internal/server/handlers.go`**

Add sidebar push:

```go
func (s *Server) pushSidebarUpdate() {
    // Render sidebar HTML using the sidebar template
    // Push as gotk.Instruction{Op: "html", Target: "#prompt-sidebar", HTML: sidebarHTML}
}
```

Call `pushSidebarUpdate()` from:
- `backgroundSendMessage()` — after saving assistant message (new message indicator)
- `handleArchive()` / `handleUnarchive()` — after status change
- Publish command — after issue creation
- Any other state change that affects sidebar display

##### 3.3 Archive/unarchive as gotk commands

Currently: Inline `onclick` handlers with `fetch()` + `location.reload()`.

**File: `internal/server/handlers.go`** — register new gotk commands:

```go
s.gotkMux.Handle("archive", func(ctx *gotk.Context) error {
    id := ctx.Payload.Int("prompt_request_id")
    // Archive in DB
    // Push: update conversation banner, update sidebar
    return nil
})

s.gotkMux.Handle("unarchive", func(ctx *gotk.Context) error {
    id := ctx.Payload.Int("prompt_request_id")
    // Unarchive in DB
    // Push: update conversation banner, update sidebar
    return nil
})
```

**File: `internal/server/templates/conversation.html`** — replace `onclick` handlers:

```html
<button gotk-click="unarchive" gotk-val-prompt_request_id="{{.PromptRequest.ID}}"
        gotk-loading="Unarchiving..." class="btn btn-sm btn-secondary">
  Unarchive
</button>
```

Similarly for archive (with confirmation — use `exec("confirm")` or a WASM
validation command before async dispatch).

##### 3.4 Cancel/retry/resend as gotk commands

`cancel-message` already exists as a gotk command. Add:

```go
s.gotkMux.Handle("retry", func(ctx *gotk.Context) error {
    id := ctx.Payload.Int("prompt_request_id")
    // Reset status, re-launch backgroundSendMessage
    // Push: status update with processing indicator
    return nil
})

s.gotkMux.Handle("resend", func(ctx *gotk.Context) error {
    // Same as retry but re-sends last message
    return nil
})
```

**Update templates:** Replace `hx-post` forms with `gotk-click` buttons.

##### 3.5 Status fragment inline scripts → gotk instructions

The `handleRepoStatus` "responded" case (handlers.go:660-672) returns HTML with
inline `<script>` that:
1. Moves children from `#repo-status` to `#conversation`
2. Removes `#repo-status`
3. Calls `htmx.process()`, `renderMarkdown()`, `scrollConversation()`
4. Re-enables message form

With server push (Phase 3.1), this entire flow is replaced by `buildResponsePush()`
which already does all of this via gotk instructions. The `handleRepoStatus` handler
and its inline scripts become dead code.

**Acceptance criteria:**
- [ ] Status updates pushed via WebSocket (no more polling)
- [ ] Sidebar updates pushed on state changes (no more polling)
- [ ] Archive/unarchive work via gotk commands (no more `fetch()` + `location.reload()`)
- [ ] Retry/resend work via gotk commands (no more `hx-post` forms)
- [ ] `handleSendMessage` HTTP handler no longer emits inline `<script>` tags
- [ ] All push helpers have corresponding tests
- [ ] Manual verification: status transitions, sidebar updates, archive/unarchive work

---

#### Phase 4: Remove HTMX

With all interactions on gotk, remove HTMX completely.

##### 4.1 Remove JS libraries

- Delete `internal/server/static/htmx.min.js`
- Delete `internal/server/static/idiomorph-ext.min.js`

##### 4.2 Update layout template

**File: `internal/server/templates/layout.html`**

- Remove `<script src="/static/htmx.min.js"></script>`
- Remove `<script src="/static/idiomorph-ext.min.js"></script>`
- Remove `hx-ext="morph"` from `<body>`

##### 4.3 Remove HTMX attributes from templates

**Files:** `conversation.html`, `sidebar.html`, `message_fragment.html`,
`status_fragment.html`, `archive_banner_fragment.html`, `repo.html`

Remove all `hx-*` attributes: `hx-get`, `hx-post`, `hx-trigger`, `hx-swap`,
`hx-target`, `hx-disabled-elt`, `hx-on::after-request`.

##### 4.4 Remove HTTP handlers that served HTMX

**File: `internal/server/server.go`** — remove routes:

```go
// Remove these routes (replaced by gotk commands or server push):
// POST .../messages      → gotk send-message
// GET  .../status        → server push (no polling)
// POST .../cancel        → gotk cancel-message
// POST .../retry         → gotk retry
// POST .../resend        → gotk resend
// POST .../archive       → gotk archive
// POST .../unarchive     → gotk unarchive
// GET  .../sidebar       → server push (no polling)
```

**File: `internal/server/handlers.go`** — delete:

- `handleSendMessage()` (HTTP version, ~80 lines)
- `handleRepoStatus()` (~90 lines)
- `handleCancel()` (HTTP version)
- `handleResend()` (HTTP version)
- `handleRetry()` (HTTP version)
- `handleArchive()` (HTTP version)
- `handleUnarchive()` (HTTP version)
- `handleSidebarFragment()` (~20 lines)
- `handlePublish()` HTTP version's `HX-Request` branch
- All `PollURL` fields from template data structs
- The `sidebarData.PollURL` field

##### 4.5 Clean up app.js

- Remove `htmx:afterSwap` event listener
- Remove `htmx:confirm` event listener
- Remove `gotk.register("htmxProcess", ...)` helper
- Remove HTMX form-submit fallback in Enter-to-send handler

##### 4.6 Clean up CSS

**File: `internal/server/static/style.css`**

- Remove `.htmx-indicator` styles (lines 933-945)
- Remove `.htmx-request .htmx-indicator` styles
- Remove `.htmx-added` animation styles (lines 1187-1192)
- Keep `@keyframes fadeIn` if used elsewhere; otherwise remove

##### 4.7 Remove fragment templates

- Delete or simplify `status_fragment.html` (status now pushed via gotk instructions)
- Delete or simplify `archive_banner_fragment.html` (now inline in gotk command)
- Delete or simplify `message_fragment.html` (responses now pushed via gotk)

**Acceptance criteria:**
- [ ] `htmx.min.js` and `idiomorph-ext.min.js` deleted
- [ ] No `hx-*` attributes remain in any template
- [ ] No `HX-Request` header checks in Go code
- [ ] No HTMX event listeners in JavaScript
- [ ] No `htmx` references in CSS
- [ ] HTTP routes for HTMX-only endpoints removed
- [ ] Fragment templates deleted or simplified
- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` passes
- [ ] All tests pass
- [ ] Manual verification: full app works without HTMX

---

#### Phase 5: TinyGo Build Integration

##### 5.1 go:generate directive

**File: `cmd/prompter/wasm/generate.go`**

```go
//go:generate tinygo build -target=wasm -o ../../../gotk/app.wasm .
package main
```

Running `go generate ./cmd/prompter/wasm/` compiles the WASM binary and places it
where `gotk/embed.go` expects it for `go:embed`.

##### 5.2 Build documentation

Update `CLAUDE.md` build section:

```bash
go generate ./cmd/prompter/wasm/  # compile WASM (requires TinyGo)
go build ./...                     # build all packages
go test ./...                      # run all tests
```

##### 5.3 CI integration

Ensure CI installs TinyGo and runs `go generate` before `go build`.

**Acceptance criteria:**
- [ ] `go generate ./cmd/prompter/wasm/` produces `gotk/app.wasm`
- [ ] WASM binary embedded and served at `/gotk/app.wasm`
- [ ] `go build ./...` succeeds with embedded WASM
- [ ] CLAUDE.md updated with build instructions

---

## Alternative Approaches Considered

### Keep plain JS instead of WASM

**Rejected.** The primary motivation is testability via `go test`. Plain JS requires
browser-based tests. With WASM commands returning instructions, all UI logic is
testable as pure Go functions.

### Standard Go WASM instead of TinyGo

**Rejected.** Standard Go WASM produces 2-5MB binaries for simple DOM helpers. TinyGo
produces 50-200KB. The functionality is simple enough for TinyGo's subset.

### gotk-poll attribute instead of server push

**Rejected.** Polling contradicts the architectural goal of the migration. Server push
via `pushAll()` provides true real-time updates with no wasted requests. The gotk
framework already has the push infrastructure.

## Dependencies & Prerequisites

- **TinyGo** must be installed for WASM compilation
- **gotk spec** defines the WASM boundary (listCommands, execCommand)
- Phases are strictly ordered — each depends on the previous

## Risk Analysis & Mitigation

| Risk | Impact | Mitigation |
|------|--------|------------|
| TinyGo compilation fails for Context type | Blocks Phase 2 | Build tags (Phase 1.2) isolate incompatible code. Prototype early. |
| WASM binary too large | Slow page load | TinyGo keeps it small. Async loading prevents blocking. |
| Server push misses a client | Stale UI | gotk client already re-syncs on WebSocket reconnect via navigate command |
| Removing polling before push is reliable | Broken status updates | Phase 3 adds push first; Phase 4 removes HTMX after verification |
| `gotk-on` event handling complexity | Keyboard edge cases | Test with IME, Shift+Enter, disabled states |

## References & Research

### Internal References
- Brainstorm: `docs/brainstorms/2026-03-12-gotk-wasm-frontend-commands-brainstorm.md`
- Previous plan: `docs/plans/2026-03-12-feat-gotk-phase-1-2-plan.md`
- gotk framework: `gotk/` (instruction.go, context.go, mux.go, client.js, etc.)
- Server handlers: `internal/server/handlers.go` (inline scripts at lines 352, 376, 382, 386, 670)
- App JS: `internal/server/static/app.js` (~205 LOC, 150 LOC after removing HTMX code)
- Templates with HTMX: `conversation.html`, `sidebar.html`, `status_fragment.html`, `repo.html`

### External References
- gotk spec: https://github.com/esnunes/gotk/blob/main/docs/ui-spec.md
- TinyGo WASM: https://tinygo.org/docs/guides/webassembly/
- Issue: https://github.com/esnunes/prompter/issues/50

### Key Files Changed

| File | Phase | Change |
|------|-------|--------|
| `gotk/registry.go` | 1 | New: CommandRegistry type |
| `gotk/registry_test.go` | 1 | New: Registry tests |
| `gotk/render.go` | 1 | Add `//go:build !tinygo` |
| `gotk/render_stub.go` | 1 | New: TinyGo stub |
| `gotk/client.js` | 1 | WASM loading, routing, gotk-on |
| `gotk/embed.go` | 1 | Add WASM serving |
| `cmd/prompter/wasm/main.go` | 2 | New: WASM entry point |
| `cmd/prompter/wasm/commands.go` | 2 | New: Frontend commands |
| `cmd/prompter/wasm/commands_test.go` | 2 | New: Frontend command tests |
| `internal/server/handlers.go` | 3 | Add push helpers, gotk commands for archive/retry/resend |
| `internal/server/server.go` | 4 | Remove HTTP routes |
| `internal/server/static/app.js` | 2,4 | Simplify, remove HTMX code |
| `internal/server/static/style.css` | 4 | Remove HTMX CSS |
| `internal/server/templates/*.html` | 3,4 | Remove hx-* attributes, add gotk-* |
| `internal/server/static/htmx.min.js` | 4 | Delete |
| `internal/server/static/idiomorph-ext.min.js` | 4 | Delete |
| `CLAUDE.md` | 5 | Update build instructions |
