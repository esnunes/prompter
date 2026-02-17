---
title: "feat: Enhanced question schema"
type: feat
date: 2026-02-17
---

# Enhanced Question Schema

## Overview

Evolve the question schema from a single-select `question` object to a `questions` array supporting batched independent questions, multi-select, header tags, and an always-available "Other" freeform option.

Brainstorm: `docs/brainstorms/2026-02-17-enhanced-questions-brainstorm.md`

## Problem Statement / Motivation

Contributors currently answer one radio-button question at a time with no way to provide freeform input. This creates unnecessarily long back-and-forth conversations when Claude has multiple independent questions, and forces contributors to pick from a fixed list even when their answer doesn't match any option.

## Proposed Solution

Replace the singular `question` field with a `questions` array in the Claude structured output schema. Each question gains `header`, `multiSelect`, and an implicit "Other" option rendered by the UI. All questions in a batch are submitted together as a single form.

## Technical Considerations

### Answer format sent to Claude

One line per question, prefixed by header (or truncated question text if no header):

```
Auth method: OAuth
Features: REST API, WebSocket, Other: gRPC support
```

For single-question responses, just the answer without prefix:

```
OAuth
```

### Edge case resolutions

| Edge case | Behavior |
|---|---|
| "Other" selected, text empty | Block submission with inline validation |
| Nothing selected on multi-select | Block submission, require at least one selection |
| `prompt_ready: true` + `questions` in same response | Show publish button only, suppress questions |
| Old session with singular `question` in raw_response | No question block renders, conversation continues via textarea |
| Free-text textarea while question block is active | Hide textarea while question block is visible |
| Question with 0 options | Schema enforces `minItems: 1` on options array |

### "Other" reveal mechanism

CSS-only using `:has()` pseudo-class (already used in the codebase for `.option-item:has(input:checked)`):

```css
.other-input { display: none; }
.other-option:has(input:checked) .other-input { display: block; }
```

## Acceptance Criteria

- [x] Claude can return multiple independent questions in a single response
- [x] Each question can be single-select (radio) or multi-select (checkbox)
- [x] Each question has an optional header rendered as a tag/chip
- [x] Every question has an "Other" option with inline text input
- [x] All questions in a batch are submitted via a single form
- [x] Page reload re-hydrates questions from stored raw_response
- [x] Old sessions with singular `question` degrade gracefully
- [x] Textarea is hidden while a question block is active
- [x] Form validation prevents empty submissions

## Implementation Plan

### Phase 1: Schema and Go types

**File: `internal/claude/claude.go`**

1. Update `jsonSchema` constant:
   - Replace `"question"` object with `"questions"` array
   - Add `header` (string, optional), `multiSelect` (boolean, optional) to each item
   - Add `"minItems": 1` on `options` array
   - Keep `"questions"` optional at the top level (not in `required`)

2. Update Go structs:
   ```go
   type Response struct {
       Message             string     `json:"message"`
       Questions           []Question `json:"questions,omitempty"`
       PromptReady         bool       `json:"prompt_ready,omitempty"`
       GeneratedTitle      string     `json:"generated_title,omitempty"`
       GeneratedMotivation string     `json:"generated_motivation,omitempty"`
       GeneratedPrompt     string     `json:"generated_prompt,omitempty"`
   }

   type Question struct {
       Header      string   `json:"header,omitempty"`
       Text        string   `json:"text"`
       MultiSelect bool     `json:"multiSelect,omitempty"`
       Options     []Option `json:"options"`
   }
   ```

3. Update `systemPrompt` constant:
   - Replace "ask one at a time" with guidance on batching independent questions
   - Add instructions for `header`, `multiSelect` usage
   - Instruct Claude to never create dependencies between batched questions
   - Keep recommendation of 2-4 options per question

### Phase 2: Handler updates

**File: `internal/server/handlers.go`**

1. Update `questionData` struct:
   ```go
   type questionData struct {
       Header      string
       Text        string
       MultiSelect bool
       Options     []optionData
       Index       int  // position in the batch, for form field naming
   }
   ```

2. Update `conversationData`:
   - `LastQuestion *questionData` → `LastQuestions []questionData`

3. Update `messageFragmentData`:
   - `Question *questionData` → `Questions []questionData`

4. Update `extractQuestionFromRaw`:
   - Return `[]questionData` instead of `*questionData`
   - Try `questions` array first, fall back to singular `question` for old sessions
   - Set `Index` on each question for template form field naming

5. Update `handleShow`:
   - Use `LastQuestions` slice instead of `LastQuestion` pointer

6. Update `handleSendMessage`:
   - Map `resp.Questions` to `fragment.Questions` slice
   - Read form fields `q_0`, `q_0_other`, `q_1`, `q_1_other`, etc.
   - Format combined answer string:
     - Single question: just the answer value
     - Multiple questions: one line per question, `Header: answer` format
   - Handle "Other" values: if `q_N` is `"__other__"`, use `q_N_other` text

### Phase 3: Template updates

**File: `internal/server/templates/conversation.html`**

Replace `{{if .LastQuestion}}` block with:
- `{{if .LastQuestions}}` wrapping a single `<form>`
- Loop `{{range .LastQuestions}}` rendering each question block
- Conditional `type="radio"` vs `type="checkbox"` based on `.MultiSelect`
- Form field names: `name="q_{{.Index}}"` for options, `name="q_{{.Index}}_other"` for text
- "Other" option as last item with inline text input (hidden by default)
- Optional `.Header` rendered as `<span class="question-header">` above question text
- Single submit button after all questions

**File: `internal/server/templates/message_fragment.html`**

Same changes as conversation.html but using `.Questions` instead of `.LastQuestions` and `.PromptRequestID` instead of `.PromptRequest.ID`.

### Phase 4: CSS and JS

**File: `internal/server/static/style.css`**

- Add `.question-header` style (small uppercase tag, subtle background)
- Add checkbox styling for `.option-item:has(input[type="checkbox"]:checked)`
- Add `.other-option` and `.other-input` styles for the reveal mechanism
- Add `.question-group` style for spacing between questions in a batch

**File: `internal/server/static/app.js`**

- Add form submission handler that:
  - Validates at least one option selected per question (or "Other" with text)
  - Validates "Other" text is non-empty when "Other" is checked
  - Assembles the combined answer into a hidden `message` field before HTMX submit
- Add logic to hide/show the textarea when question blocks appear/disappear

### Phase 5: Textarea visibility

**File: `internal/server/templates/conversation.html`**

- Add `id="message-form"` to the textarea form
- Hide it when `.LastQuestions` is present (server-side, via template conditional)

**File: `internal/server/static/app.js`**

- After HTMX swaps in a question block fragment, hide the textarea form
- After question block is removed (on submit), show the textarea form again

## Dependencies & Risks

- **Three-tier parsing must be maintained**: `parseResponse` in `claude.go` and `parseRawResponse` in `handlers.go` must both handle the new `questions` field. The documented solution in `docs/solutions/integration-issues/claude-cli-structured-output-parsing.md` applies.
- **Both templates must stay in sync**: `conversation.html` and `message_fragment.html` render the same question block structure with different data field names.
- **CSS `:has()` browser support**: Already used in the codebase (`.option-item:has(input:checked)`), so this is a non-issue.

## References & Research

- Brainstorm: `docs/brainstorms/2026-02-17-enhanced-questions-brainstorm.md`
- AskUserQuestion pattern: `https://gist.github.com/bgauryy/0cdb9aa337d01ae5bd0c803943aa36bd`
- Current schema: `internal/claude/claude.go:40-86`
- Current templates: `internal/server/templates/conversation.html:19-44`, `internal/server/templates/message_fragment.html:7-32`
- Current handler: `internal/server/handlers.go:76-91`, `257-267`, `364-380`
- Parsing solution: `docs/solutions/integration-issues/claude-cli-structured-output-parsing.md`
