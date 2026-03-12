---
date: 2026-03-12
topic: gotk-wasm-frontend-commands
issue: https://github.com/esnunes/prompter/issues/50
---

# gotk WASM Frontend Commands

## What We're Building

Extend the gotk framework to support client-side commands compiled as Go/WASM (via TinyGo). Frontend commands follow the exact same pattern as server commands: a Go function receives a payload and returns a list of DOM instructions. The gotk thin client automatically routes commands to WASM or WebSocket based on a cached command registry.

This completes the gotk migration (issue #50) by replacing the remaining ~150 LOC of plain JS in `app.js` with testable Go functions, while also removing all HTMX remnants (htmx.min.js, idiomorph-ext.min.js, polling, HX-Request detection).

## Why This Approach

**Primary motivation: testability.** Server-side gotk commands are already testable via `go test` using `gotk.NewTestContext()`. Frontend commands should be equally testable — call the function, assert on the returned instructions. No browser needed.

**The gotk spec already defines the WASM boundary:**

1. `wasm.listCommands()` — called once at init, returns a JSON array of command names, cached as a JS `Set`
2. `wasm.execCommand(cmd, payloadJSON)` — dispatches a command, returns `{ "ins": [...], "async": [...] }`
3. Routing: `localCmds.has(cmd)` -> WASM; else -> WebSocket

The HTML needs no changes for routing — `gotk-click="validate-form"` works identically whether the handler is WASM or server-side.

## Key Decisions

- **Instruction-based, not DOM-direct:** WASM commands never touch the DOM via syscall/js. They return instruction descriptors (same types as server commands), and the existing JS executor applies them. This keeps command logic as pure Go, testable without a browser.

- **Extend gotk's instruction set:** Start with the existing instruction types (html, attr-set, remove, focus, exec, etc.) and add new types as needed for client-only operations (scroll-to, class-toggle, alert/validate, etc.).

- **TinyGo for compilation:** Use TinyGo instead of standard Go WASM to keep binary size small (~50-200KB vs 2-5MB).

- **`async` field for deferred server calls:** The `execCommand` return value has two fields:
  - `ins`: Immediate DOM instructions applied synchronously
  - `async`: Server command names to dispatch over WebSocket as follow-ups
  This enables patterns like: validate locally -> if invalid, return error instructions -> if valid, return `async: ["send-message"]` to forward to the server.

- **Unified command registry:** No separate attributes or prefixes for client vs server commands. The thin client's cached `Set` from `listCommands()` handles routing transparently.

- **Part of the gotk framework:** Client-side command support will be added to the gotk package itself, not as a separate package. Other gotk users benefit from this capability.

- **Markdown rendering stays in JS:** `marked.js` + `DOMPurify` remain as the one JS exception, invoked via gotk's `exec` instruction.

## Scope

This brainstorm covers the architectural decision for WASM frontend commands. The specific list of which behaviors become WASM commands (validation, scrolling, timers, visibility toggles, etc.) is deferred to the planning phase.

## Open Questions

None — all key architectural decisions are resolved.

## Next Steps

-> `/workflows:plan` to define implementation phases:
1. Extend gotk with client-side WASM command support
2. Implement specific frontend commands for prompter
3. Remove HTMX and remaining plain JS
4. Add TinyGo build step
