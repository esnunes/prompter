# Prompter MVP Brainstorm

**Date:** 2026-02-16
**Status:** Draft

## What We're Building

Prompter is a CLI tool that helps open source contributors create **prompt requests** instead of pull requests. Instead of writing code, contributors describe what they want through an AI-guided conversation, and the tool generates a well-crafted prompt that maintainers can feed to their own AI agent workflow.

### User Flow

1. Contributor runs `prompter github.com/owner/repo`
2. Tool clones the repo to a local directory
3. Starts a local web server, opens browser to dashboard
4. Dashboard shows the contributor's prompt requests across all repos
5. Contributor creates a new prompt request — starts an AI-guided conversation
6. The AI (claude CLI running in the cloned repo dir) asks structured questions, rendered as UI elements (radio buttons, checkboxes)
7. Contributor answers questions; AI uses codebase context to ask informed follow-ups
8. When satisfied, contributor **publishes** — tool creates/updates a GitHub Issue via `gh` CLI
9. Each publish updates the issue description (revision tracking)
10. Maintainer receives the issue and feeds the prompt to their AI agent

### Who It Serves

- **Contributors:** Non-technical or technical users who want to request features without writing code. The UI must be simple and approachable.
- **Maintainers:** Receive clean, actionable prompts as GitHub Issues that they can execute with their preferred AI workflow.

## Why This Approach

### Approach A: Per-message Claude CLI (Selected)

Each user message shells out to `claude -p --session-id <id> --output-format json --json-schema <schema> --system-prompt <prompt> --allowedTools "Read,Glob,Grep" "<msg>"` in the cloned repo directory. The `--json-schema` flag forces Claude to respond in a structured format that the Go server parses and renders.

**Structured response format (via --json-schema):**
```json
{
  "message": "string — AI's response text (always present)",
  "question": {
    "text": "string — question to ask the user",
    "options": [{ "label": "string", "description": "string" }]
  },
  "prompt_ready": "boolean — true when AI has enough info",
  "generated_prompt": "string — the final prompt when ready"
}
```

**Why this approach:**
- Simplest implementation — each message is one subprocess call
- Claude CLI handles session persistence natively
- `--json-schema` enforces structured output — no MCP server needed
- `--allowedTools "Read,Glob,Grep"` limits Claude to read-only codebase exploration
- Leverages existing `gh` and `claude` CLI auth — no custom auth needed
- Stateless web server, stateful claude sessions
- HTMX fits naturally (form submit, swap response)

**Trade-offs accepted:**
- No streaming — user waits for full response (acceptable for MVP)
- Cold start per message from claude CLI boot time
- Dependency on two CLI tools (`gh`, `claude`)

### Alternatives Considered

- **Long-running Claude CLI process:** Streaming responses but complex process lifecycle management. Deferred to later.
- **Direct Anthropic API:** Full control but rebuilds what claude CLI provides. Better suited for the SaaS phase.

## Key Decisions

1. **CLI + local web server** — not a pure terminal TUI. Opens browser for the UI.
2. **Per-message claude CLI execution** — shell out per message, not a long-running process.
3. **SQLite for local persistence** — stores draft prompt requests, conversation history, session metadata.
4. **Auth via `gh` + `claude` CLI** — no custom auth system for MVP.
5. **Structured UI for AI questions** — AI questions rendered as radio buttons/checkboxes, not plain text.
6. **Final prompt only in GitHub Issues** — no conversation history in the published issue.
7. **Feature requests first** — bug reports added in a future version.
8. **Built-in system prompt** — maintainer-configurable prompts deferred to later.
9. **Go templates + HTMX + plain HTML/CSS/JS** — no frontend build step.
10. **CSS via Theme UI spec** — `tokens.css` (design tokens/variables) + `style.css`.

## Tech Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.25.5 |
| Web server | Go `net/http` + html/template |
| Interactivity | HTMX |
| CSS | Theme UI spec (tokens.css + style.css) |
| Storage | SQLite |
| AI | `claude` CLI (shelled out per message) |
| GitHub | `gh` CLI |
| Dev tools | Mise |

## Data Model (Conceptual)

- **Repository** — cloned repo metadata (URL, local path, last cloned)
- **PromptRequest** — a draft or published prompt request (title, status, repo, claude session ID, GitHub issue number)
- **Message** — conversation messages (prompt request ID, role, content, timestamp)
- **Revision** — published versions of a prompt request (prompt request ID, content, published at, issue URL)

## Resolved Questions

1. **Claude CLI structured output:** Use `--json-schema` to enforce structured responses. Claude outputs JSON with message, optional question (with options), and optional generated prompt. No MCP needed.
2. **Repo caching:** Git pull the existing clone on subsequent runs. Don't re-clone.
3. **Multiple repos:** Dashboard shows prompt requests across all repos the contributor has used.

## Open Questions

1. **System prompt design:** What should the built-in system prompt look like to effectively guide feature request conversations? (Will be designed during implementation.)

## Future Considerations (Not MVP)

- Bug report support
- Maintainer-configurable conversation templates (`.prompter.yml`)
- Web SaaS version (hosted, no CLI install needed)
- Direct Anthropic API integration (for SaaS, streaming)
- GitHub OAuth (for SaaS auth)
- Streaming responses
- Multiple AI provider support
