---
title: "feat: Render assistant messages as markdown"
type: feat
date: 2026-02-16
---

# feat: Render assistant messages as markdown

Assistant responses from Claude often contain markdown (headings, lists, code blocks, bold/italic). Currently these render as escaped plain text, losing all formatting. Add client-side markdown parsing so assistant message bubbles display rendered HTML.

## Acceptance Criteria

- [x] Assistant message content renders markdown (headings, lists, code blocks, bold, italic, links)
- [x] User message content stays as plain text (no markdown processing)
- [x] Works on full page load (conversation.html, published.html)
- [x] Works on HTMX fragment insertion (message_fragment.html)
- [x] Code blocks use the existing monospace font from `tokens.css` (`--font-mono`)
- [x] Add basic prose styles for rendered markdown inside `.message-bubble` (spacing for paragraphs, lists, code blocks)

## Approach

Client-side rendering with **marked.js** (vendored), matching the existing pattern of vendoring htmx.min.js. This avoids server-side changes and XSS concerns from bypassing Go's template auto-escaping.

### Implementation Steps

#### 1. Vendor marked.min.js

- [x] Download `marked.min.js` into `internal/server/static/marked.min.js`
- [x] Add `<script>` tag in `layout.html` after htmx

#### 2. Add markdown rendering script

- [x] Create inline `<script>` in `layout.html` (or a small `app.js` file) that:
  - Defines a `renderMarkdown()` function targeting `.message-assistant .message-bubble` elements
  - Calls it on `DOMContentLoaded` for initial page load
  - Listens for `htmx:afterSwap` to process newly inserted HTMX fragments
- [x] Use `marked.parse(element.textContent)` and set `element.innerHTML` with the result
- [x] Skip elements already processed (add a `data-md-rendered` attribute)

#### 3. Add prose CSS styles

- [x] Add styles in `style.css` for markdown content inside `.message-bubble`:
  - Paragraph spacing
  - List indentation and markers
  - Inline code (`<code>`) background and padding
  - Code blocks (`<pre><code>`) with monospace font, background, padding, overflow-x
  - Headings (smaller sizes appropriate for chat bubbles)
  - Links styling
  - Blockquote styling

## Context

**Templates that render `{{.Content}}`:**
- `internal/server/templates/conversation.html:15` — full page message loop
- `internal/server/templates/message_fragment.html:3` — HTMX fragment
- `internal/server/templates/published.html:22,29` — published view messages

**Static assets (vendored, embedded via `go:embed`):**
- `internal/server/static/htmx.min.js` — existing vendored JS pattern
- `internal/server/static/tokens.css` — design tokens including `--font-mono`
- `internal/server/static/style.css` — component styles

**Key HTMX event:** `htmx:afterSwap` fires after fragment insertion, ideal for processing new message bubbles.

## References

- marked.js: lightweight markdown parser, ~50KB minified
- Existing vendored JS pattern: `internal/server/static/htmx.min.js`
