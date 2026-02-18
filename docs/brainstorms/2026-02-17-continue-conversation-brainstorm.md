# Continue Conversation After Publishing

**Date:** 2026-02-17
**Status:** Draft

## What We're Building

A unified conversation view that replaces the current separate `conversation.html` and `published.html` templates. The user always sees the full chat history regardless of whether the prompt request has been published or not. A sidebar panel shows the publish/revision state, and inline markers in the chat stream indicate when each submission to GitHub occurred.

### Current Problem

After publishing a prompt request to GitHub, clicking "Continue conversation" redirects to `/prompt-requests/{id}/continue` which returns 404 — no route or handler exists for it.

### Desired Behavior

- The regular `/prompt-requests/{id}` route always renders the conversation view
- A sidebar panel shows revision history (or "Not published yet" for drafts)
- Inline submission markers appear in the chat stream at the point each publish happened
- Users can continue chatting after publishing, and re-publish to update the GitHub issue
- The prompt request status stays `"published"` once published (no revert to draft)

## Why This Approach

**Unified template over separate views** because:
- Eliminates the need for a separate `/continue` route entirely
- Provides consistent UX — the user always sees their full conversation
- The sidebar naturally communicates publish state without needing a different page
- Simpler to maintain one template vs. two divergent views

**Status stays "published"** because:
- Continuing a conversation doesn't undo the fact that an issue was created
- Re-publishing updates the existing issue and creates a new revision
- The sidebar revision history makes the publish state always visible

## Key Decisions

1. **Single unified template** replaces both `conversation.html` and `published.html`
2. **No `/continue` route** — `handleShow` always renders the conversation
3. **Sidebar panel** with revision history (timestamps, links to GitHub issue)
4. **Inline chat markers** — system-message-style banners in the message stream showing "Published to GitHub — Revision N" at the point each submission occurred
5. **Status model** — stays `"published"` after first publish; no status revert
6. **Re-publish flow** — when `prompt_ready: true`, show publish button; re-publishing updates existing GitHub issue and creates a new revision

## Scope

### In Scope
- Unified conversation template with sidebar
- Inline submission markers in chat stream
- Sidebar revision panel ("Not published yet" / revision list)
- Remove `published.html` template
- Update `handleShow` to always render conversation with revision data
- Remove the `/continue` link (no longer needed)

### Out of Scope
- Changing the publish/re-publish backend logic (already works)
- Changing the Claude CLI integration (session resumption already works)
- Database schema changes (revisions table already exists)

## Open Questions

None — all key decisions have been made.
