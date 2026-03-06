---
title: Configurable Server Startup
type: feat
date: 2026-03-06
---

# feat: Configurable Server Startup

## Overview

Make Prompter's HTTP server configurable via environment variables (`PROMPTER_HOST`, `PROMPTER_PORT`) and remove automatic browser opening on startup.

## Current Behavior

- **Bind address:** `127.0.0.1:0` — localhost only, OS-assigned random port (`server.go:125`)
- **Browser opening:** `openBrowser()` called between `Listen()` and `Serve()` (`main.go:52`)
- **Startup log:** `Prompter running at http://<addr>` + `Press Ctrl+C to stop.` (`server.go:141-142`)
- **Two-phase startup:** `Listen()` binds the port, `Serve()` starts accepting — split exists so `Addr()` is available for `openBrowser()`

## Proposed Changes

### 1. Environment variable support

Read in `main.go:run()`, pass as argument to `server.Listen(addr string)`:

| Env Var | Default | Description |
|---------|---------|-------------|
| `PROMPTER_HOST` | `0.0.0.0` | Network interface to bind to |
| `PROMPTER_PORT` | `8080` | TCP port to listen on |

Construct `addr` as `host:port` in `main.go` and pass to `Listen()`. No validation beyond what `net.Listen` provides — invalid values produce clear OS-level errors (e.g., `address abc: invalid port`).

### 2. Remove automatic browser opening

- Delete `openBrowser()` function from `main.go` (lines 78-91)
- Remove the call to `openBrowser()` from `run()` (line 52)
- Remove the `os/exec` and `runtime` imports if they become unused

### 3. Update startup log

Change the log message in `Serve()` from:
```
Prompter running at http://<addr>
Press Ctrl+C to stop.
```
To:
```
Listening on http://<addr>
Press Ctrl+C to stop.
```

Use `s.addr` (resolved from `ln.Addr().String()`) so the logged address reflects the actual bound address.

### 4. Simplify Listen() signature

Change `Listen()` to accept an `addr string` parameter:

```go
// internal/server/server.go
func (s *Server) Listen(addr string) error {
    ln, err := net.Listen("tcp", addr)
    if err != nil {
        return fmt.Errorf("binding port: %w", err)
    }
    s.ln = ln
    s.addr = ln.Addr().String()
    return nil
}
```

Keep the two-phase `Listen`/`Serve` split — it's clean separation and `Addr()` is still used for logging.

## Acceptance Criteria

- [x] `PROMPTER_PORT=9090 prompter` listens on port 9090
- [x] `PROMPTER_HOST=127.0.0.1 prompter` binds to localhost only
- [x] Running `prompter` with no env vars binds to `0.0.0.0:8080`
- [x] Browser does NOT open automatically on startup
- [x] Startup prints `Listening on http://0.0.0.0:8080` (or configured address)
- [x] `Press Ctrl+C to stop.` line is preserved
- [x] Invalid port values produce a clear error and exit

## Files to Modify

1. **`cmd/prompter/main.go`** — read env vars, remove `openBrowser`, pass addr to `Listen()`
2. **`internal/server/server.go`** — update `Listen()` signature, update log message in `Serve()`
