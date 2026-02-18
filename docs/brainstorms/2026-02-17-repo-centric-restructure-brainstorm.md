# Brainstorm: Repository-Centric Restructure

**Date:** 2026-02-17
**Status:** Draft

## What We're Building

Restructure the app around repository-specific pages, removing the CLI repository argument and adding real-time download status feedback. The app becomes a multi-repo tool launched with just `prompter` (no arguments), where each repository gets its own page for managing prompt requests.

### Core Changes

1. **CLI simplification** — Remove the required repository argument. `prompter` starts the web server immediately with no args.

2. **Repository pages** — New page at `/github.com/<org>/<repo>/prompt-requests` listing all prompt requests for a specific repo. Verified against GitHub via `gh` CLI (authenticated, no rate limits).

3. **Remove `/new` page** — Prompt requests are created from the repository page, skipping the form since the repo is known from the URL.

4. **Repo-scoped conversation URLs** — Conversation views move from `/prompt-requests/{id}` to `/github.com/<org>/<repo>/prompt-requests/{id}`.

5. **Dashboard updates** — Dashboard continues showing all prompt requests across repos, with clickable repo names linking to repo pages.

6. **Async repo download with status feedback** — Clone/pull runs asynchronously on prompt request creation. HTMX polling shows status ("Cloning...", "Ready!") in the assistant area. User can type and submit messages during download; processing auto-starts when repo is ready.

## Why This Approach

- **`gh` for repo verification** instead of unauthenticated GitHub API avoids the 60 req/hr rate limit. The app already requires `gh` for publishing.
- **In-memory status tracking** (`sync.Map`) avoids DB schema changes for transient state. Status is only relevant during the clone/pull operation — if the server restarts, the goroutine is lost anyway.
- **HTMX polling** (vs SSE) keeps the architecture simple and consistent with existing patterns. No new HTMX extensions or server-side streaming needed.
- **Goroutine on create** (vs on page load) starts the download immediately so it's further along by the time the user lands on the conversation page.
- **Poll-then-auto-send** provides the smoothest UX — the user submits their message, sees status updates, and the Claude response appears automatically when the repo is ready.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Real-time status mechanism | HTMX polling (~2s interval) | Simpler than SSE, consistent with existing patterns |
| Conversation URLs | Repo-scoped (`/github.com/org/repo/prompt-requests/{id}`) | Consistent URL structure, repo context visible in URL |
| Repo verification | `gh` CLI | Authenticated (no rate limits), already a dependency |
| Status state storage | In-memory `sync.Map` on Server | Transient state, no DB changes needed |
| Clone/pull trigger | Goroutine on prompt request creation | Download starts ASAP, before user lands on conversation page |
| Message queue behavior | Poll-then-auto-send | User submits message, it auto-processes when repo ready |

## User Flows

### Flow 1: New repo, first prompt request

1. User navigates to `/github.com/org/repo/prompt-requests`
2. Server verifies repo exists via `gh api repos/org/repo`
3. Repo not in DB yet → empty state page with "Create new prompt request" button and link back to dashboard
4. User clicks "Create new prompt request"
5. POST creates prompt request record, upserts repo in DB, starts async clone goroutine, redirects to conversation page
6. Conversation page shows message input + assistant area with polling status
7. Polling div shows: "Cloning repository..." → "Repository ready!"
8. User types and submits first message (can happen during or after clone)
9. Message saved to DB. If repo not ready, message waits.
10. Once repo ready + pending message exists, Claude is invoked automatically
11. Claude response replaces status messages in assistant area

### Flow 2: Existing repo, new prompt request

1. User navigates to `/github.com/org/repo/prompt-requests`
2. Repo exists in DB → page shows list of existing prompt requests + "Create new prompt request" button
3. User clicks create → POST creates record, starts async pull goroutine, redirects to conversation
4. Same polling/auto-send flow as above (pull is usually faster than clone)

### Flow 3: Dashboard to repo page

1. User visits dashboard (`/`)
2. Sees all prompt requests with clickable repo names
3. Clicks a repo name → navigates to that repo's prompt request list

### Flow 4: Non-existent repo

1. User navigates to `/github.com/org/nonexistent/prompt-requests`
2. Server calls `gh api repos/org/nonexistent` → 404
3. Page shows error: "This repository doesn't exist on GitHub"

## Scope Boundaries

**In scope:**
- CLI arg removal
- `/new` page removal
- Repo page (list + create)
- Repo-scoped conversation URLs
- Dashboard repo links
- Async clone/pull with polling status
- Auto-send queued messages

**Out of scope:**
- Repo settings or configuration pages
- Repo deletion/removal from app
- Multiple GitHub providers (only github.com)
- Auth/permissions for repos
- Repo branch selection (uses default branch)

## Open Questions

_None — all key decisions resolved during brainstorming._
