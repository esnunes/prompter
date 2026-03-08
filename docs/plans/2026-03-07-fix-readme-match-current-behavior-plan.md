---
title: "Fix: Update README to match current application behavior"
type: fix
date: 2026-03-07
issue: https://github.com/esnunes/prompter/issues/24
---

# Fix: Update README to match current application behavior

## Overview

The README.md has several inaccuracies compared to the actual application. The usage command is wrong, the workflow description is misleading, configuration options are undocumented, and data storage details are incomplete. This plan fixes all inaccuracies and adds a Configuration section.

## Problem Statement

A first-time user following the current README would:
1. Run `prompter github.com/owner/repo` and get an error (CLI takes no arguments)
2. Expect the browser to open automatically (it doesn't)
3. Not know the default server address to open manually
4. Not know how to change host/port
5. Not know where local data is stored

## Changes

All changes are in a single file: `README.md`.

### 1. Fix the Usage section (line 29)

**Current:**
```bash
prompter github.com/owner/repo
```

**Replace with:**
```bash
prompter
```

### 2. Fix the workflow description (lines 32-39)

**Current** (inaccurate — says it clones repos at startup and opens browser):
```
Prompter will clone the target repo (or pull if already cloned), start a local web server, and open your browser. From the UI you can:

1. Create a new prompt request
2. Have a guided conversation with Claude, which explores the repo and asks clarifying questions
3. Review the generated prompt
4. Publish it as a GitHub issue
```

**Replace with** (accurate — server starts, user navigates to it):
```
This starts a local web server. Open http://localhost:8080 in your browser, enter a GitHub repo URL to get started, and from the UI you can:

1. Create a new prompt request (the repo is cloned automatically)
2. Have a guided conversation with Claude, which explores the repo and asks clarifying questions
3. Review the generated prompt
4. Publish it as a GitHub issue
```

Key corrections:
- Removed claim that CLI takes a repo argument
- Removed claim that browser opens automatically
- Added the default server URL
- Moved repo cloning to step 1 where it actually happens (on prompt request creation)

### 3. Add Configuration section (after Usage, before Data Storage)

Add a new section documenting the two environment variables from `cmd/prompter/main.go:48-55`:

```markdown
## Configuration

| Variable | Default | Description |
|---|---|---|
| `PROMPTER_HOST` | `0.0.0.0` | Address to bind the server to |
| `PROMPTER_PORT` | `8080` | Port to listen on |

Example:

    PROMPTER_PORT=3000 prompter
```

### 4. Expand the data storage note (line 39)

**Current:**
```
Data is stored locally in `~/.cache/prompter/`.
```

**Replace with:**
```
Data is stored locally in `~/.cache/prompter/` (or `$XDG_CACHE_HOME/prompter/`):

- **Database:** `prompter.db` (SQLite)
- **Cloned repos:** `repos/<github.com/owner/repo>/`
```

### 5. Prerequisites — no change needed

`git` was already added to the Prerequisites list (line 12) since the issue was filed.

## Acceptance Criteria

- [x] `prompter` command shown without arguments
- [x] Workflow description matches actual behavior (no auto-browser, no startup cloning)
- [x] Default server URL (`http://localhost:8080`) mentioned
- [x] Configuration section documents `PROMPTER_HOST` and `PROMPTER_PORT` with defaults
- [x] Data storage section lists DB path and repos path
- [x] XDG convention mentioned
- [x] Overall README tone stays brief and to the point

## Verified Against Source Code

| Claim | Source | Confirmed |
|---|---|---|
| CLI takes no arguments | `cmd/prompter/main.go` — no `os.Args` usage | Yes |
| No auto-browser open | `internal/server/server.go` — no `open` command | Yes |
| PROMPTER_HOST default `0.0.0.0` | `cmd/prompter/main.go:49` | Yes |
| PROMPTER_PORT default `8080` | `cmd/prompter/main.go:52` | Yes |
| DB at `prompter.db` | `internal/db/db.go:65` | Yes |
| Repos at `repos/` | `internal/repo/repo.go:28` | Yes |
| XDG_CACHE_HOME respected | `internal/paths/paths.go:10-11` | Yes |
| Dashboard has repo URL input | `templates/dashboard.html:14-21` | Yes |

## References

- Issue: [#24](https://github.com/esnunes/prompter/issues/24)
- `cmd/prompter/main.go` — CLI entry point, env var handling
- `internal/paths/paths.go` — cache directory resolution
- `internal/db/db.go:57-65` — DB path construction
- `internal/repo/repo.go:23-29` — repo local path construction
- `internal/server/server.go:155-162` — server startup output
- `internal/server/templates/dashboard.html:14-21` — repo URL input form
