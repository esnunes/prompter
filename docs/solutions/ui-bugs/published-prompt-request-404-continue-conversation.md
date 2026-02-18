---
title: "Unified conversation view replacing dead /continue route"
date: 2026-02-17
category: ui-bugs
component: internal/server (handlers, templates, CSS)
tags:
  - template-consolidation
  - routing
  - revision-history
  - sidebar
  - htmx
severity: medium
symptoms:
  - "404 error on /prompt-requests/{id}/continue after publishing"
  - "No way to resume conversation after publishing to GitHub"
  - "Separate templates created dead-end UX for published requests"
  - "Prompt ready banner shown after content was already published"
---

# Published Prompt Request: 404 on Continue Conversation

## Root Cause

The app maintained separate UI flows for draft and published prompt requests. `handleShow` in `handlers.go` branched on `pr.Status == "published"` and rendered a separate `published.html` template that was a dead-end -- it displayed revisions and messages read-only, with no chat input. A "Continue conversation" link pointed to `/prompt-requests/{id}/continue`, but no route handler was ever registered.

Secondary issue: after publishing, the `prompt_ready` flag from the last assistant message still caused the "Publish to GitHub" button to appear, even though the content had already been published.

## Solution

Unified the two templates into a single conversation view with a revision sidebar.

### 1. Schema change (db.go)

Added nullable `after_message_id` column to revisions table, linking each revision to the last message at publish time for accurate inline marker placement:

```go
db.Exec(`ALTER TABLE revisions ADD COLUMN after_message_id INTEGER REFERENCES messages(id)`)
```

### 2. Unified data model (handlers.go)

Replaced separate `conversationData` and `publishedData` structs with one struct containing a `Timeline []timelineItem` (interleaved messages + revision markers) and `Revisions []models.Revision` for the sidebar:

```go
type timelineItem struct {
    Type     string           // "message" or "revision-marker"
    Message  *models.Message
    Revision *models.Revision
}
```

### 3. Handler simplification (handleShow)

Removed the `if pr.Status == "published"` branch. Now always loads messages AND revisions, builds a timeline via `buildTimeline()`, and extracts questions/promptReady regardless of status.

Suppressed `prompt_ready` when the last message was already published:

```go
if data.PromptReady && len(revisions) > 0 {
    latestRev := revisions[len(revisions)-1]
    if latestRev.AfterMessageID != nil && last.ID <= *latestRev.AfterMessageID {
        data.PromptReady = false
    }
}
```

### 4. Unified template (conversation.html)

Two-column layout: main chat area + sidebar. Timeline renders messages and collapsible `<details>` revision markers (reusing `.revision-content` for markdown rendering via app.js). Sidebar shows revision list or "Not published yet".

### 5. Cleanup

Deleted `published.html`, removed from `pageNames`, removed `publishedData` struct, added `hx-disabled-elt` to publish forms for double-submit protection.

## Prevention Strategies

### Prevent dead links in Go web apps

- Define route paths as constants shared between mux registration and templates to prevent drift
- Use a grep-based check or test that scans templates for links and cross-references against declared routes
- Before adding a link in a template, require the corresponding handler in the same commit

### Prevent stale UI state after actions

- After an action (publish), re-fetch all data from DB before rendering -- don't rely on previous request state
- When UI depends on comparing current data against historical actions, add explicit suppression logic with comments explaining why
- Test the post-action state: verify that publishing causes `prompt_ready` to be false on the next render

### When to unify vs separate templates

- **Unify** when the user can transition between states (draft -> published -> continue chatting) -- separate templates create the illusion that the entity is fundamentally different
- **Separate** only when the layout or available actions are completely different
- Communicate state via visual hierarchy (badges, sidebars, markers) rather than different page structures

## Related Documentation

- `docs/brainstorms/2026-02-17-continue-conversation-brainstorm.md` -- Design decisions for the unified approach
- `docs/plans/2026-02-17-feat-unified-conversation-view-plan.md` -- Implementation plan with acceptance criteria
- `docs/plans/2026-02-16-feat-prompter-mvp-plan.md` -- Original MVP architecture defining the separate template approach (now superseded)
- `docs/solutions/integration-issues/claude-cli-schema-evolution-backward-compatibility.md` -- Related backward-compat parsing patterns used in question extraction
