# HTMX WebSocket Extension Support

**Date:** 2026-03-15
**Status:** Brainstorm

## What We're Building

A bidirectional WebSocket communication layer using the HTMX ws extension, extended
with a thin JS interceptor to support HX-* response headers over WebSocket. This
enables:

1. **Server push broadcasting** — backend can push HTML fragment updates to all
   connected clients without polling
2. **Client-to-server messaging** — forms and user actions sent over WebSocket via
   standard `hx-ws` send behavior, replacing some HTTP POST endpoints
3. **Full HX-* header support** — server responses over WebSocket can include
   HX-Redirect, HX-Refresh, HX-Retarget, HX-Reswap, HX-Trigger, HX-Push-Url,
   HX-Replace-Url, processed via the htmx JS API

## Why This Approach

**Extended HTMX ws extension** was chosen over alternatives because:

- Stays close to HTMX idioms and reuses existing swap/morph machinery
- Minimal custom JS — just an interceptor layer on top of the standard extension
- Gets reconnection, form serialization, and DOM scanning for free from the extension
- Avoids fork maintenance burden

Rejected alternatives:
- **Custom WebSocket layer** — more JS to maintain, reimplements what the extension provides
- **Forked ws extension** — maintenance burden, harder to upgrade with HTMX releases

## Key Decisions

1. **Message format:** Each WebSocket message is either plain HTML or JSON headers,
   never both in the same message. When headers and HTML are both needed, they are
   sent as two separate messages.
   - **Plain HTML message:** `<div id="status">Done</div>` — passed directly to
     the HTMX ws extension's standard swap flow
   - **JSON headers message:** `{"HX-Redirect": "/new-page"}` — intercepted and
     processed via the htmx JS API

2. **Interceptor pattern:** A thin JS wrapper intercepts incoming WebSocket messages
   before they reach the HTMX ws extension's default handler. If the message starts
   with `{` and parses as JSON, the wrapper processes the HX-* headers via the htmx
   JS API. Otherwise, the message is plain HTML and passes through to the standard
   swap flow.

3. **Broadcasting:** Simple broadcast to all connected clients. No page-aware or
   topic-based filtering initially. Server maintains a connection registry (`sync.Map`)
   and iterates all connections to push messages.

4. **Server-side API:** Go handlers send plain HTML messages for DOM updates and
   separate JSON messages for HX-* header actions. Helper functions handle each type.

5. **Supported HX-* headers over WebSocket:**
   - `HX-Redirect` — navigate to URL
   - `HX-Refresh` — refresh current page
   - `HX-Retarget` — change swap target
   - `HX-Reswap` — change swap strategy
   - `HX-Trigger` — trigger client-side events
   - `HX-Push-Url` — push URL to history
   - `HX-Replace-Url` — replace URL in history

## Resolved Questions

1. **Reconnection behavior:** Rely on the HTMX ws extension's built-in reconnection.
   No message queuing — missed messages are acceptable.

2. **Client-to-server message format:** Use standard HTMX ws form serialization
   (built-in `hx-ws` send behavior). No custom JSON command routing needed.

3. **Authentication/authorization:** Deferred — not needed for current local-only
   use case. Revisit if the app becomes multi-user.
