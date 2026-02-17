# Enhanced Question Schema

**Date:** 2026-02-17
**Status:** Draft

## What We're Building

Evolve the question schema in Prompter to support richer interactions between Claude and contributors. Currently, questions are single-select radio buttons with no freeform fallback. The new schema adds:

- **Multiple questions per response** — Claude can batch independent questions together
- **Multi-select** — checkboxes instead of radio buttons when multiple answers apply
- **Header/tag** — short label displayed above each question for visual scanning
- **"Other" freeform option** — always available on every question as an inline text input

This is modeled after Claude Code's AskUserQuestion tool pattern.

## Why This Approach

The flexible schema evolution (Approach 1) was chosen over strict spec matching or backward-compatible extension because:

- Prompter is a local tool — no need for strict validation guardrails
- The LLM can be guided via system prompt to batch responsibly
- Simplest to implement, fewest code paths

## Key Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Questions per response | Unlimited (LLM-guided) | System prompt instructs Claude not to create dependencies between batched questions |
| Header field | Yes, added | Visual structure when scanning multiple questions |
| Multi-select | Per-question boolean | Checkboxes when true, radio buttons when false |
| "Other" option | Always available | Every question gets an "Other" option with inline text input — no schema field needed |
| Multi-select answer format | Comma-separated labels | e.g. "REST API, WebSocket" — human-readable, easy for Claude to parse |
| Schema migration | Replace `question` with `questions` array | Clean break, no backward-compat shim needed |

## Schema Design

### New JSON Schema (Claude structured output)

```json
{
  "questions": {
    "type": "array",
    "items": {
      "type": "object",
      "properties": {
        "header": {
          "type": "string",
          "description": "Short label displayed as a tag above the question"
        },
        "text": {
          "type": "string",
          "description": "The question to ask the contributor"
        },
        "multiSelect": {
          "type": "boolean",
          "description": "Allow selecting multiple options"
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
    }
  }
}
```

`header` and `multiSelect` are optional — defaults to no header and single-select.

### UI Behavior

- **Single-select (multiSelect=false):** Radio buttons (current behavior) + "Other" with inline text input
- **Multi-select (multiSelect=true):** Checkboxes + "Other" with inline text input
- **Header:** Rendered as a small tag/chip above the question text
- **"Other" selection:** Reveals an inline `<input type="text">` below the options list
- **Multiple questions:** Each question rendered as its own block within a single form; one submit button for all

### Answer Format Sent to Claude

All answers are joined into a single message string, one answer per line:

```
Header: selected label
Header: label1, label2
Header: Other: user typed text
Header: label1, Other: user typed text
```

When no header is present, the question text (truncated) is used as prefix. This keeps answers human-readable for Claude and avoids ambiguity when multiple questions are answered at once.

### System Prompt Changes

Add instructions for Claude to:
- Batch independent questions when it makes sense
- Never create dependencies between batched questions
- Use `multiSelect: true` when multiple options can apply simultaneously
- Provide a short `header` for each question
- Keep options concise (2-4 per question recommended, not enforced)

### Old Session Compatibility

After the schema change, old assistant messages stored with `question` (singular) in `raw_response` won't re-hydrate on page reload. This is acceptable — old sessions can still be viewed (message text is preserved), but the last question block won't render interactively. No migration needed.

## Open Questions

None — all design decisions resolved through brainstorming.
