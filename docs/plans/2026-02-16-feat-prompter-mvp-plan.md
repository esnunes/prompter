---
title: "feat: Prompter MVP — AI-guided prompt requests for open source"
type: feat
date: 2026-02-16
---

# feat: Prompter MVP — AI-guided prompt requests for open source

## Overview

Build the MVP of Prompter: a Go CLI tool that starts a local web server where contributors create "prompt requests" for open source repos. Instead of writing code, contributors have an AI-guided conversation (via Claude CLI) that produces a well-crafted prompt, published as a GitHub Issue for maintainers to execute with their AI workflow.

## Problem Statement

Open source maintainers increasingly prefer receiving "prompt requests" over pull requests — a clear description of what to build that they can feed to their own AI agent. Contributors (often non-technical) need a guided way to articulate feature requests with enough context for AI execution. Current GitHub Issues are unstructured and lack the specificity AI agents need.

## Proposed Solution

A single Go binary that:
1. Clones a GitHub repo locally
2. Serves a browser-based dashboard at localhost
3. Guides contributors through AI conversations (Claude CLI in the repo dir)
4. Renders structured questions as UI elements (radio buttons)
5. Publishes the final prompt as a GitHub Issue via `gh` CLI

## Technical Approach

### Architecture

```
┌─────────────────────────────────────────────────┐
│                   Browser (HTMX)                │
│  Dashboard │ Conversation │ Published View      │
└──────────────────┬──────────────────────────────┘
                   │ HTTP (localhost)
┌──────────────────▼──────────────────────────────┐
│              Go HTTP Server                     │
│  Routes │ Templates │ Handlers                  │
└───┬──────────┬──────────────┬───────────────────┘
    │          │              │
┌───▼───┐ ┌───▼───┐   ┌──────▼──────┐
│SQLite │ │claude │   │  gh CLI     │
│  DB   │ │  CLI  │   │(issues)     │
└───────┘ └───┬───┘   └─────────────┘
              │
        ┌─────▼─────┐
        │Cloned Repo│
        │(read-only)│
        └───────────┘
```

### Project Structure

```
prompter/
├── cmd/prompter/
│   └── main.go              # CLI entry point, arg parsing, server startup
├── internal/
│   ├── server/
│   │   ├── server.go        # HTTP server setup, routes
│   │   └── handlers.go      # Request handlers
│   ├── claude/
│   │   └── claude.go        # Claude CLI wrapper (exec, parse JSON)
│   ├── github/
│   │   └── github.go        # gh CLI wrapper (issue create/edit)
│   ├── repo/
│   │   └── repo.go          # Git clone/pull operations
│   ├── db/
│   │   ├── db.go            # SQLite connection, migrations
│   │   └── queries.go       # Data access functions
│   └── models/
│       └── models.go        # Data structs
├── templates/
│   ├── layout.html           # Base layout with head, nav
│   ├── dashboard.html        # Prompt request list
│   ├── conversation.html     # Chat UI
│   └── published.html        # Read-only published view
├── static/
│   ├── tokens.css            # Design tokens (Theme UI spec)
│   ├── style.css             # Styles using tokens
│   └── htmx.min.js           # HTMX library (vendored)
├── docs/
│   ├── brainstorms/
│   └── plans/
├── go.mod
├── go.sum
└── mise.toml
```

### Data Model & SQLite Schema

```sql
-- internal/db/db.go (embedded in Go, run on startup)

CREATE TABLE IF NOT EXISTS repositories (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    url         TEXT NOT NULL UNIQUE,          -- "github.com/owner/repo"
    local_path  TEXT NOT NULL,                 -- "~/.prompter/repos/github.com/owner/repo"
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS prompt_requests (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    repository_id   INTEGER NOT NULL REFERENCES repositories(id),
    title           TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'deleted')),
    session_id      TEXT NOT NULL,             -- UUID for claude CLI --session-id
    issue_number    INTEGER,                   -- GitHub issue number (NULL until first publish)
    issue_url       TEXT,                      -- Full GitHub issue URL
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS messages (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    prompt_request_id INTEGER NOT NULL REFERENCES prompt_requests(id),
    role              TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content           TEXT NOT NULL,            -- Raw user text or AI message text
    raw_response      TEXT,                     -- Full JSON from claude CLI (assistant only)
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS revisions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    prompt_request_id INTEGER NOT NULL REFERENCES prompt_requests(id),
    content           TEXT NOT NULL,            -- The generated prompt
    published_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_prompt_requests_repository ON prompt_requests(repository_id);
CREATE INDEX IF NOT EXISTS idx_prompt_requests_status ON prompt_requests(status);
CREATE INDEX IF NOT EXISTS idx_messages_prompt_request ON messages(prompt_request_id);
CREATE INDEX IF NOT EXISTS idx_revisions_prompt_request ON revisions(prompt_request_id);
```

**State transitions:**
- `draft` → `published` (on first publish)
- `draft` → `deleted` (soft delete)
- `published` stays `published` (re-publish updates issue, creates new revision)

### Claude CLI Integration

**Invocation (via `exec.Command` — no shell injection):**

```go
// internal/claude/claude.go

cmd := exec.CommandContext(ctx, "claude",
    "-p",
    "--session-id", sessionID,
    "--output-format", "json",
    "--json-schema", jsonSchema,
    "--system-prompt", systemPrompt,
    "--allowedTools", "Read,Glob,Grep",
    "--permission-mode", "bypassPermissions",
    userMessage,
)
cmd.Dir = repoLocalPath  // Run in cloned repo directory
```

**JSON Schema for structured output:**

```json
{
  "type": "object",
  "properties": {
    "message": {
      "type": "string",
      "description": "Your response to the contributor"
    },
    "question": {
      "type": "object",
      "properties": {
        "text": {
          "type": "string",
          "description": "A clarifying question to ask"
        },
        "options": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "label": { "type": "string" },
              "description": { "type": "string" }
            },
            "required": ["label", "description"]
          }
        }
      },
      "required": ["text", "options"]
    },
    "prompt_ready": {
      "type": "boolean",
      "description": "True when you have gathered enough context to generate the final prompt"
    },
    "generated_prompt": {
      "type": "string",
      "description": "The complete prompt request, only when prompt_ready is true"
    }
  },
  "required": ["message"]
}
```

**System prompt (built-in):**

```
You are a helpful assistant that guides open source contributors in creating
clear, actionable feature requests for repository maintainers.

You are running inside the repository's codebase. Use your tools (Read, Glob,
Grep) to explore the code and understand the project structure, patterns, and
conventions. This helps you ask informed questions.

Your goal is to gather enough context to generate a well-crafted "prompt
request" — a natural language prompt that a maintainer can feed to their AI
coding agent to implement the feature.

Guidelines:
- Start by understanding what the contributor wants at a high level
- Explore the codebase to understand relevant patterns and architecture
- Ask clarifying questions one at a time using the "question" field with options
- Keep questions simple and non-technical — contributors may not be developers
- When you have enough context, set "prompt_ready" to true and include the
  "generated_prompt" — a clear, detailed prompt that describes what to build,
  where in the codebase, and any relevant context from the code
- The generated prompt should be self-contained: a maintainer reading only the
  prompt (without the conversation) should understand exactly what to implement
- Always include your thinking in "message" so the contributor understands
  what you're doing
```

### GitHub Integration

```go
// internal/github/github.go

// Create new issue
cmd := exec.CommandContext(ctx, "gh", "issue", "create",
    "--repo", repoURL,
    "--title", title,
    "--body", promptContent,
)

// Update existing issue
cmd := exec.CommandContext(ctx, "gh", "issue", "edit",
    strconv.Itoa(issueNumber),
    "--repo", repoURL,
    "--body", promptContent,
)
```

### HTTP Routes & HTMX Interactions

| Method | Path | Handler | HTMX | Description |
|--------|------|---------|------|-------------|
| GET | `/` | Dashboard | Full page | List all prompt requests across repos |
| GET | `/new?repo=<url>` | NewForm | Full page | Create prompt request form |
| POST | `/prompt-requests` | Create | Redirect | Create new prompt request, redirect to conversation |
| GET | `/prompt-requests/{id}` | Show | Full page | Conversation view (draft) or published view |
| POST | `/prompt-requests/{id}/messages` | SendMessage | `hx-swap` | Send message, append AI response to chat |
| POST | `/prompt-requests/{id}/publish` | Publish | `hx-swap` | Publish to GitHub, show success |
| DELETE | `/prompt-requests/{id}` | Delete | `hx-swap` | Soft delete, remove from dashboard |

**HTMX conversation flow:**

```html
<!-- templates/conversation.html -->

<!-- Message form -->
<form hx-post="/prompt-requests/{{.ID}}/messages"
      hx-target="#conversation"
      hx-swap="beforeend"
      hx-indicator="#loading">
  <textarea name="message"></textarea>
  <button type="submit">Send</button>
</form>

<!-- Structured question (rendered when AI asks one) -->
<form hx-post="/prompt-requests/{{.ID}}/messages"
      hx-target="#conversation"
      hx-swap="beforeend"
      hx-indicator="#loading">
  <fieldset>
    <legend>{{.Question.Text}}</legend>
    {{range .Question.Options}}
    <label>
      <input type="radio" name="message" value="{{.Label}}">
      <span>{{.Label}}</span>
      <small>{{.Description}}</small>
    </label>
    {{end}}
  </fieldset>
  <button type="submit">Answer</button>
</form>

<!-- Loading indicator -->
<div id="loading" class="htmx-indicator">Thinking...</div>
```

### Repo Management

**Storage location:** `~/.prompter/repos/<url-path>/`

Example: `github.com/esnunes/prompter` → `~/.prompter/repos/github.com/esnunes/prompter/`

```go
// internal/repo/repo.go

func localPath(repoURL string) string {
    // repoURL is already validated as "github.com/owner/repo"
    return filepath.Join(homeDir, ".prompter", "repos", repoURL)
}
```

**URL validation:** Accept `github.com/owner/repo` format only (MVP). Reject anything else with a clear error message.

**Clone/pull logic:**
1. If local path doesn't exist → `git clone https://<url>.git <local-path>`
2. If local path exists → `cd <local-path> && git pull --ff-only`
3. If pull fails (merge conflict, etc.) → show error, suggest deleting and re-cloning

### CSS Design Tokens

```css
/* static/tokens.css — Theme UI spec */
:root {
  /* Colors */
  --color-text: #1a1a2e;
  --color-background: #ffffff;
  --color-primary: #2563eb;
  --color-secondary: #64748b;
  --color-accent: #8b5cf6;
  --color-muted: #f1f5f9;
  --color-border: #e2e8f0;
  --color-success: #16a34a;
  --color-error: #dc2626;
  --color-warning: #d97706;

  /* Typography */
  --font-body: system-ui, -apple-system, sans-serif;
  --font-mono: ui-monospace, monospace;
  --font-size-sm: 0.875rem;
  --font-size-base: 1rem;
  --font-size-lg: 1.125rem;
  --font-size-xl: 1.25rem;
  --font-size-2xl: 1.5rem;
  --font-weight-normal: 400;
  --font-weight-medium: 500;
  --font-weight-bold: 700;
  --line-height-tight: 1.25;
  --line-height-base: 1.5;

  /* Spacing */
  --space-1: 0.25rem;
  --space-2: 0.5rem;
  --space-3: 0.75rem;
  --space-4: 1rem;
  --space-5: 1.25rem;
  --space-6: 1.5rem;
  --space-8: 2rem;
  --space-10: 2.5rem;
  --space-12: 3rem;
  --space-16: 4rem;

  /* Borders */
  --radius-sm: 0.25rem;
  --radius-md: 0.5rem;
  --radius-lg: 0.75rem;
  --radius-full: 9999px;
  --border-width: 1px;

  /* Shadows */
  --shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.05);
  --shadow-md: 0 4px 6px rgba(0, 0, 0, 0.07);
  --shadow-lg: 0 10px 15px rgba(0, 0, 0, 0.1);

  /* Sizes */
  --size-container: 48rem;

  /* Transitions */
  --transition-fast: 150ms ease;
  --transition-base: 200ms ease;
}
```

### Error Handling Strategy

**Startup checks (in order):**
1. Check `claude` CLI exists → error: "claude CLI not found. Install: https://docs.anthropic.com/en/docs/claude-code"
2. Check `gh` CLI exists → error: "gh CLI not found. Install: https://cli.github.com"
3. Check `gh auth status` → error: "Not authenticated with GitHub. Run: gh auth login"
4. Validate repo URL format → error: "Invalid repository URL. Expected: github.com/owner/repo"
5. Clone or pull repo → error with git output
6. Open/create SQLite DB → error with path info
7. Start HTTP server → print URL to terminal

**Runtime errors:**
- Claude CLI timeout (120s) → "AI is taking too long. Please try again."
- Claude CLI malformed JSON → "AI response was unexpected. Please try again." (log raw response)
- `gh issue create` fails → show gh stderr to user
- Git pull fails → "Could not update repository. Try deleting ~/.prompter/repos/<url> and restarting."

**Security measures:**
- All CLI calls via `exec.Command` with separate args (no shell string concatenation)
- Repo URL validated against `^github\.com/[\w.-]+/[\w.-]+$` regex
- SQLite queries use parameterized statements
- User input never interpolated into shell strings

## Implementation Phases

### Phase 1: Foundation

Set up project structure, CLI entry point, SQLite database, and repo cloning.

**Deliverables:**
- `cmd/prompter/main.go` — CLI arg parsing, startup checks, server launch
- `internal/db/db.go` — SQLite connection, schema migration
- `internal/db/queries.go` — CRUD for all models
- `internal/models/models.go` — Go structs for Repository, PromptRequest, Message, Revision
- `internal/repo/repo.go` — Git clone/pull via `exec.Command`

**Success criteria:**
- `go run ./cmd/prompter github.com/owner/repo` clones repo, creates DB, prints "Server starting..."
- DB has correct schema with all tables

### Phase 2: Web Server & Dashboard

HTTP server, templates, static assets, dashboard page.

**Deliverables:**
- `internal/server/server.go` — HTTP server setup, static file serving, route registration
- `internal/server/handlers.go` — Dashboard handler
- `templates/layout.html` — Base layout
- `templates/dashboard.html` — Prompt request list (empty state + populated)
- `static/tokens.css` — Design tokens
- `static/style.css` — Styles
- `static/htmx.min.js` — Vendored HTMX

**Success criteria:**
- Browser opens to dashboard showing empty state
- Static assets load correctly
- CSS tokens apply throughout

### Phase 3: Claude CLI Integration & Conversation UI

Wire up Claude CLI, build the conversation page with HTMX.

**Deliverables:**
- `internal/claude/claude.go` — Claude CLI wrapper (exec, JSON parsing, timeout)
- `internal/server/handlers.go` — Create prompt request, send message, conversation page handlers
- `templates/conversation.html` — Chat UI with message history, input form, loading indicator
- Structured question rendering (radio buttons)

**Success criteria:**
- User creates a prompt request from dashboard
- Sends messages, sees AI responses rendered in chat
- Structured questions appear as radio buttons
- Loading indicator during AI response
- Conversation persists across page refreshes

### Phase 4: Publishing & GitHub Integration

Publish prompt to GitHub Issues, revision tracking, published view.

**Deliverables:**
- `internal/github/github.go` — `gh` CLI wrapper (issue create/edit)
- `internal/server/handlers.go` — Publish handler, published view handler, delete handler
- `templates/published.html` — Read-only view with revision history

**Success criteria:**
- "Publish" button appears when `prompt_ready: true`
- First publish creates GitHub Issue, stores issue number
- Subsequent publishes update issue description, create new revision
- Published view shows prompt and revision history
- Delete soft-deletes draft from dashboard

### Phase 5: Polish & Edge Cases

Error handling, empty states, startup validation, UX improvements.

**Deliverables:**
- Startup dependency checks (claude, gh, gh auth)
- Error pages/messages for all failure modes
- Empty state on dashboard with call-to-action
- Browser auto-open on server start
- Graceful shutdown on Ctrl+C
- Print server URL to terminal as fallback

**Success criteria:**
- Clear error message if claude/gh missing
- Graceful handling of AI timeouts and malformed responses
- Dashboard empty state guides new users
- Server shuts down cleanly

## Acceptance Criteria

### Functional Requirements

- [x] `prompter github.com/owner/repo` clones repo (or pulls if cached) and opens browser
- [x] Dashboard lists prompt requests across all repos with status indicators
- [x] Create new prompt request starts AI-guided conversation
- [x] AI asks structured questions rendered as radio buttons
- [x] AI explores codebase via Read/Glob/Grep tools for context
- [x] Publish creates/updates GitHub Issue with generated prompt
- [x] Each publish creates a new revision
- [x] Conversation and prompt requests persist in SQLite
- [x] Soft delete removes drafts from dashboard

### Non-Functional Requirements

- [x] No shell injection — all CLI calls via `exec.Command` with separate args
- [x] Repo URL validated against regex before any git operations
- [x] SQLite uses parameterized queries
- [x] Claude CLI limited to read-only tools (`Read,Glob,Grep`)
- [x] 120-second timeout on Claude CLI calls
- [x] Startup checks for `claude` and `gh` CLI presence and auth

### Quality Gates

- [x] All Go code compiles without warnings
- [ ] Manual test: full flow from `prompter <repo>` to published GitHub Issue
- [ ] Manual test: error handling for missing CLI tools
- [ ] Manual test: conversation resume after page refresh

## Dependencies & Prerequisites

- Go 1.25.5 (configured via mise)
- `claude` CLI with `--json-schema` support
- `gh` CLI authenticated with issue create/edit permissions
- Git CLI for clone/pull
- Go SQLite driver: `github.com/mattn/go-sqlite3` (CGO) or `modernc.org/sqlite` (pure Go)

## Risk Analysis & Mitigation

| Risk | Impact | Mitigation |
|------|--------|------------|
| `--json-schema` doesn't enforce output reliably | AI returns unexpected format | Graceful fallback: display raw message, log error, let user retry |
| Claude CLI session persistence breaks across versions | Can't resume conversations | Store messages in SQLite as source of truth, rebuild context if needed |
| Large repos slow to clone | Poor first-run experience | Show progress output from git clone in terminal |
| Claude CLI cold start per message (2-5s) | Slow conversation pace | Show loading indicator, accept for MVP, plan streaming for v2 |
| `gh issue edit --body` overwrites without revision tracking | Lost history on GitHub side | Our SQLite stores all revisions; GitHub issue body is latest only |

## References & Research

### Internal References

- Brainstorm: `docs/brainstorms/2026-02-16-prompter-mvp-brainstorm.md`

### External References

- Claude CLI docs: `claude --help` (flags: `--json-schema`, `--session-id`, `--output-format`, `--allowedTools`, `--system-prompt`)
- GitHub CLI docs: https://cli.github.com/manual/
- HTMX docs: https://htmx.org/docs/
- Theme UI spec: https://theme-ui.com/theme-spec
- Go SQLite (pure Go): https://pkg.go.dev/modernc.org/sqlite
