---
title: "fix: Wider Other input and scroll to first question"
type: fix
date: 2026-03-06
---

# fix: Wider Other input and scroll to first question

Two UI improvements to the questionnaire: make the "Other" text input match the
width of option items, and scroll to the first question when AI responds instead
of scrolling to the bottom.

## Acceptance Criteria

- [x] When "Other" is selected, the text input is the same width as the option items above it
- [x] When the AI responds with questions, the view scrolls to the first question (the `#question-form` element)
- [x] Scroll-to-question works for all three scroll triggers: question form submit, message form submit, and background auto-send response
- [x] Both templates (`conversation.html` and `message_fragment.html`) are updated consistently

## 1. Wider "Other" text input

**Problem:** The `.other-input` is nested inside a `<div>` within the
`.option-item` label, alongside the radio/checkbox. Because it's a flex child
sibling to the radio button, it's narrower than the full option row width.

**Current structure** (`message_fragment.html:31-39`, `conversation.html:60-68`):

```html
<label class="option-item other-option">
  <input type="radio" ...>     <!-- takes ~16px + gap -->
  <div>
    <div class="option-label">Other</div>
    <input type="text" class="other-input" ...>  <!-- 100% of this div, NOT the option-item -->
  </div>
</label>
```

**Solution:** Move the `<input type="text">` out of the `<label>` and place it
after the `.options-list` div, as a direct child of `.question-group`. This way
it naturally spans the full width of the question group, matching the option
items.

**New structure:**

```html
<div class="question-group">
  ...
  <div class="options-list">
    ...
    <label class="option-item other-option">
      <input type="radio" name="q_0" value="__other__">
      <div>
        <div class="option-label">Other</div>
      </div>
    </label>
  </div>
  <input type="text" name="q_0_other" class="other-input" placeholder="Type your answer..." maxlength="500">
</div>
```

### Files to change

**`internal/server/templates/message_fragment.html`** — Move `<input type="text"
class="other-input">` from inside the `.other-option` label to after the
`.options-list` closing `</div>`, still inside `.question-group`.

**`internal/server/templates/conversation.html`** — Same change in the
`.LastQuestions` block (lines 60-68).

**`internal/server/static/style.css`** — Update the CSS `:has()` selector from:

```css
.other-option:has(input:checked) .other-input {
  display: block;
}
```

to:

```css
.question-group:has(.other-option input:checked) > .other-input {
  display: block;
}
```

No other CSS changes needed — `.other-input` already has `width: 100%`.

## 2. Scroll to first question

**Problem:** All three scroll triggers use `scrollTop = scrollHeight`, which
jumps to the bottom. When questions appear, the user may not see the first
question.

**Solution:** Add a `scrollConversation()` helper in `app.js` that checks for
`#question-form`. If present, scroll to it with `scrollIntoView()`. Otherwise,
scroll to bottom as before.

### Files to change

**`internal/server/static/app.js`** — Add helper function and call it from
`htmx:afterSwap`:

```javascript
function scrollConversation() {
  var c = document.getElementById('conversation');
  var q = document.getElementById('question-form');
  if (q) {
    q.scrollIntoView({ behavior: 'smooth', block: 'start' });
  } else if (c) {
    c.scrollTop = c.scrollHeight;
  }
}
```

Call `scrollConversation()` at the end of the `htmx:afterSwap` handler (after
`renderMarkdown` and `updateMessageFormVisibility`).

**`internal/server/templates/conversation.html`** (line 42) — Replace inline
scroll in `hx-on::after-request`:

```
document.getElementById('conversation').scrollTop = document.getElementById('conversation').scrollHeight;
```

with:

```
if(typeof scrollConversation==='function')scrollConversation();
```

**`internal/server/templates/message_fragment.html`** (line 13) — Same
replacement in `hx-on::after-request`.

**`internal/server/templates/conversation.html`** (line 110) — Same replacement
in the message form's `hx-on::after-request`.

**`internal/server/handlers.go`** (line 582) — Replace `c.scrollTop=c.scrollHeight`
in the inline script with `if(typeof scrollConversation==='function')scrollConversation();`.

**Note:** `scrollConversation` must be a global function (defined on `window` or
outside the IIFE), since it's called from inline `hx-on` attributes and inline
`<script>` tags. Either expose it on `window` or move it outside the IIFE.

## References

- `internal/server/static/style.css:207-242` — option-item styles
- `internal/server/static/style.css:618-639` — other-input styles
- `internal/server/static/app.js:47-53` — updateMessageFormVisibility pattern
- `internal/server/handlers.go:582` — inline scroll script
