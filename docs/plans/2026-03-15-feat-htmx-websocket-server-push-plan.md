---
title: "feat: Add HTMX WebSocket server push with HX-Trigger events"
type: feat
date: 2026-03-15
---

# feat: Add HTMX WebSocket server push with HX-Trigger events

## Overview

Add WebSocket support using the HTMX ws extension for server-push broadcasting.
The server broadcasts JSON events containing HX-Trigger payloads to all connected
clients. The frontend reacts to these triggers by fetching updated partials via
standard hx-get HTTP requests. Form submissions remain as hx-post (no ws-send
migration in this phase).

## Problem Statement / Motivation

The app currently uses HTMX polling (every 2-3 seconds) for real-time updates:
- `#repo-status` polls `GET /status` every 2s to detect processing state changes
- `#prompt-sidebar` polls `GET /api/sidebar` every 3s for sidebar updates

This creates unnecessary server load, latency (up to 2-3s delay), and a complex
"responded" handling hack where inline `<script>` tags relocate DOM nodes from
`#repo-status` into `#conversation`.

WebSocket server push eliminates polling, reduces latency to near-instant, and
simplifies the status delivery architecture.

## Proposed Solution

### Architecture

```
Browser                          Server
  |                                |
  |-- ws-connect="/ws" ---------->|  (WebSocket upgrade)
  |                                |  (register in connRegistry)
  |                                |
  |                                |  (background: Claude finishes)
  |<-- JSON event ----------------|  {"events":[{"HX-Trigger":...}]}
  |                                |
  |  (interceptor fires trigger)   |
  |  (hx-trigger reacts)          |
  |-- hx-get /status ------------>|  (fetch updated partial)
  |<-- HTML fragment -------------|
  |  (hx-swap into DOM)           |
```

### Message Protocol

Server-to-client messages are **either** plain HTML (OOB swaps) **or** JSON events,
never both in one message. The interceptor distinguishes them by attempting JSON parse.

**JSON event format:**

```json
{"events": [{"HX-Trigger": {"conversation-updated": {"id": "42"}}}]}
```

The `events` array contains objects where each key is an HX-* header name and each
value is the header payload. The wrapper is needed because JSON messages must be
objects (not arrays) at the top level.

**Supported HX-* headers in events:**

| Header | Interceptor behavior |
|--------|---------------------|
| `HX-Trigger` | `htmx.trigger(document.body, name, detail)` for each trigger |
| `HX-Redirect` | `window.location.href = url` |
| `HX-Refresh` | `window.location.reload()` |
| `HX-Push-Url` | `history.pushState({}, "", url)` |
| `HX-Replace-Url` | `history.replaceState({}, "", url)` |
| `HX-Retarget` | Store for next HTML swap (advanced, defer if not needed) |
| `HX-Reswap` | Store for next HTML swap (advanced, defer if not needed) |

**Plain HTML messages** pass through to the HTMX ws extension's standard OOB swap
flow (elements matched by `id`, swapped via `hx-swap-oob`).

### Event-Driven Update Pattern

Instead of pushing rendered HTML directly, the server broadcasts trigger events.
The frontend has elements that listen for these triggers and fetch updated partials:

```html
<!-- Conversation page: listen for status changes -->
<div id="repo-status"
     hx-trigger="conversation-updated from:body"
     hx-get="/github.com/{org}/{repo}/prompt-requests/{id}/status"
     hx-swap="outerHTML">
</div>

<!-- Sidebar: listen for sidebar changes -->
<div id="prompt-sidebar"
     hx-trigger="sidebar-updated from:body"
     hx-get="/api/sidebar"
     hx-swap="morph:outerHTML">
</div>
```

This keeps the existing HTTP handlers and templates unchanged. The only difference
is the trigger source: WebSocket event instead of polling timer.

## Technical Considerations

### Go WebSocket Library

Use `github.com/coder/websocket` (v1.8.x):
- Pure Go, no CGO (matches project philosophy with `modernc.org/sqlite`)
- Concurrent writes are safe (no mutex needed for broadcast)
- Context-based API
- Handles ping/pong automatically

### Connection Registry

Add to `Server` struct:

```go
// internal/server/server.go
type Server struct {
    // ... existing fields ...
    wsConns sync.Map // map[string]*websocket.Conn (conn ID -> conn)
}
```

### HTMX ws Extension

The extension handles:
- Connection via `ws-connect="/ws"` attribute
- Automatic reconnection with exponential backoff + jitter
- OOB swap processing for plain HTML messages
- Message queuing during disconnection

### Interceptor Placement

The JS interceptor listens for `htmx:wsBeforeMessage` on the document body. If the
message parses as JSON with an `events` array, it processes each event, then cancels
the HTMX event (prevents swap). Otherwise, it lets the message through for OOB swap.

### Security

- Validate `Origin` header during WebSocket upgrade (local-only app, but good practice)
- HTML from server uses Go template auto-escaping (existing pattern)
- The HTMX ws extension does NOT sanitize incoming HTML through DOMPurify. If raw
  HTML push is used later, ensure DOMPurify is applied. For now, event-driven
  pattern avoids this since HTML comes via standard hx-get responses.

### Idiomorph Coexistence

Both extensions coexist: `hx-ext="morph,ws"` on `<body>`. The ws extension's OOB
swaps use standard innerHTML by default. Add `hx-swap-oob="morph:outerHTML"` on
elements that need morphing (like the sidebar).

## Acceptance Criteria

### Functional Requirements

- [x] WebSocket connection established on page load via HTMX ws extension
- [x] Server registers/deregisters connections in `sync.Map` registry
- [x] Server can broadcast JSON events to all connected clients
- [x] JS interceptor processes `HX-Trigger` events and fires htmx triggers
- [x] JS interceptor processes `HX-Redirect` events via `window.location.href`
- [x] JS interceptor processes `HX-Refresh` events via `window.location.reload()`
- [x] JS interceptor processes `HX-Push-Url` / `HX-Replace-Url` via History API
- [x] JS interceptor passes plain HTML messages through to ws extension OOB swap
- [x] Unknown/malformed JSON messages logged to console and ignored
- [x] Existing hx-post form submissions continue to work unchanged
- [x] WebSocket reconnection works automatically (ws extension built-in)

### Non-Functional Requirements

- [x] No new CGO dependencies
- [x] No changes to existing HTTP handlers or templates (additive only)
- [x] Builds and passes `go build ./...` and `go vet ./...`

## Implementation Phases

### Phase 1: WebSocket Infrastructure

Server-side WebSocket endpoint and connection management.

**Files to create/modify:**

- `internal/server/ws.go` (new) — WebSocket handler, connection registry, broadcast helper

```go
// ws.go - WebSocket handler and broadcast infrastructure

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
    // Accept WebSocket upgrade via coder/websocket
    // Register connection in s.wsConns
    // Read loop (consume client messages, ignore for now)
    // Deregister on close
}

func (s *Server) broadcast(msg []byte) {
    // Iterate s.wsConns, write msg to each
    // Remove failed connections
}

func (s *Server) broadcastEvents(events []map[string]any) {
    // Marshal {"events": events} to JSON
    // Call s.broadcast(jsonBytes)
}

func (s *Server) broadcastTrigger(name string, detail any) {
    // Convenience: broadcastEvents with single HX-Trigger
}
```

- `internal/server/server.go` — Register `/ws` route, add `wsConns sync.Map`
- `go.mod` / `go.sum` — Add `github.com/coder/websocket`

**Acceptance:**
- [ ] `GET /ws` upgrades to WebSocket
- [ ] Connections tracked in sync.Map
- [ ] `broadcast()` sends text message to all connections
- [ ] Dead connections cleaned up on write failure

### Phase 2: Client-Side Interceptor

HTMX ws extension setup and JS interceptor for JSON events.

**Files to create/modify:**

- `internal/server/static/ws.min.js` (new) — HTMX ws extension (vendor from npm/CDN)
- `internal/server/static/app.js` — Add interceptor logic

```js
// Interceptor in app.js

document.body.addEventListener("htmx:wsBeforeMessage", function(event) {
    var msg = event.detail.message;
    try {
        var data = JSON.parse(msg);
        if (data.events && Array.isArray(data.events)) {
            processEvents(data.events);
            event.preventDefault(); // cancel OOB swap
            return;
        }
    } catch (e) {
        // Not JSON, let it through for OOB swap
    }
});

function processEvents(events) {
    events.forEach(function(evt) {
        if (evt["HX-Trigger"]) { processHXTrigger(evt["HX-Trigger"]); }
        if (evt["HX-Redirect"]) { window.location.href = evt["HX-Redirect"]; }
        if (evt["HX-Refresh"]) { window.location.reload(); }
        if (evt["HX-Push-Url"]) { history.pushState({}, "", evt["HX-Push-Url"]); }
        if (evt["HX-Replace-Url"]) { history.replaceState({}, "", evt["HX-Replace-Url"]); }
    });
}

function processHXTrigger(trigger) {
    // trigger can be: string, {name: detail}, or {name: detail, name2: detail2}
    if (typeof trigger === "string") {
        htmx.trigger(document.body, trigger);
    } else {
        Object.keys(trigger).forEach(function(name) {
            htmx.trigger(document.body, name, trigger[name]);
        });
    }
}
```

- `internal/server/templates/layout.html` — Add ws extension script, `ws-connect="/ws"`,
  update `hx-ext="morph,ws"`

**Acceptance:**
- [ ] HTMX ws extension loaded and connects to `/ws`
- [ ] JSON events with `HX-Trigger` fire htmx triggers on `document.body`
- [ ] JSON events with `HX-Redirect` navigate the page
- [ ] Plain HTML messages are swapped via OOB (passthrough)
- [ ] Malformed JSON logged and ignored
- [ ] Connection indicator CSS classes work (optional)

### Phase 3: Replace Polling with Event-Driven Updates

Migrate existing polling patterns to WebSocket-triggered fetches.

**Files to modify:**

- `internal/server/handlers.go` — Add `s.broadcastTrigger()` calls where status
  changes happen (in `backgroundSendMessage`, after Claude response, etc.)

- `internal/server/templates/conversation.html` — Replace `hx-trigger="every 2s"`
  on `#repo-status` with `hx-trigger="conversation-updated from:body"`. The hx-get
  URL stays the same.

- `internal/server/templates/sidebar.html` — Replace `hx-trigger="every 3s"` with
  `hx-trigger="sidebar-updated from:body"`.

**Broadcast points in handlers.go:**

```go
// After status changes in backgroundSendMessage:
s.broadcastTrigger("conversation-updated", map[string]any{"id": prID})
s.broadcastTrigger("sidebar-updated", nil)

// After create/archive/unarchive/delete:
s.broadcastTrigger("sidebar-updated", nil)
```

**Acceptance:**
- [ ] Status updates delivered via WebSocket trigger instead of 2s polling
- [ ] Sidebar updates delivered via WebSocket trigger instead of 3s polling
- [ ] No `hx-trigger="every Ns"` polling attributes remain
- [ ] Status transitions (cloning -> pulling -> processing -> responded) all broadcast
- [ ] Sidebar reflects changes immediately after create/archive/publish

## Dependencies & Risks

**Dependencies:**
- `github.com/coder/websocket` — pure Go, well-maintained, v1.8.x stable
- HTMX ws extension JS — vendor as static asset

**Risks:**
- **Proxy/firewall blocking WebSocket:** Mitigated by keeping HTTP handlers intact.
  If WebSocket fails, the app still works but without real-time push. Could add
  polling fallback later if needed.
- **Missed events during reconnection:** Mitigated by the event-driven pattern —
  when the ws extension reconnects, the next event triggers a fresh fetch. State
  is always fetched from the server, never cached in the WebSocket layer.
- **Multiple tabs:** All tabs receive all broadcasts. Since events trigger hx-get
  fetches (not direct DOM mutations), each tab fetches its own context-appropriate
  response. No cross-conversation corruption.

## References & Research

### Internal References
- Brainstorm: `docs/brainstorms/2026-03-15-htmx-websocket-brainstorm.md`
- Current polling: `internal/server/templates/conversation.html` (status polling),
  `internal/server/templates/sidebar.html` (sidebar polling)
- Status handler: `internal/server/handlers.go` (`handleRepoStatus`)
- Background processing: `internal/server/handlers.go` (`backgroundSendMessage`)

### External References
- HTMX ws extension: https://htmx.org/extensions/ws/
- HTMX JS API: https://htmx.org/api/
- coder/websocket: https://github.com/coder/websocket
- HTMX OOB swaps: https://htmx.org/docs/#oob_swaps
