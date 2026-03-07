---
title: "feat: Visual refresh with Catppuccin Latte theme and modern polish"
type: feat
date: 2026-03-07
issue: 26
---

# Visual Refresh: Catppuccin Latte Theme with Modern Polish

## Overview

Redesign Prompter's UI with a Minimal & Modern aesthetic using the Catppuccin Latte color palette. Replace the current generic blue/white scheme and system fonts with a cohesive, intentional design language across all pages. Add subtle CSS animations for a polished feel.

This is a CSS-focused change. No Go code or template structure changes are needed — only `tokens.css`, `style.css`, and minor `app.js` additions.

## Problem Statement

Prompter currently uses generic system fonts, standard blue/white colors, and no animations. Contributors spend meaningful time in the chat and dashboard views, and the experience should feel refined. A visual refresh with the Catppuccin Latte palette and subtle motion makes Prompter feel more trustworthy and pleasant.

## Proposed Solution

A phased CSS migration: first replace color tokens, then fix hardcoded colors, then refine components, then add animations. Light-only — no dark mode.

## Technical Approach

### Files to Modify

| File | Changes |
|------|---------|
| `internal/server/static/tokens.css` | Replace all color token values; add new tokens |
| `internal/server/static/style.css` | Fix hardcoded colors; refine components; add animations |
| `internal/server/static/app.js` | Add smooth scroll for revision anchors |

No template changes. No Go code changes.

### Complete Catppuccin Latte Palette Reference

| Color | Hex | Role |
|-------|-----|------|
| Rosewater | #dc8a78 | Highlight details (reserved) |
| Flamingo | #dd7878 | — |
| Pink | #ea76cb | — |
| Mauve | #8839ef | — |
| Red | #d20f39 | Errors, danger |
| Maroon | #e64553 | Danger hover |
| Peach | #fe640b | — |
| Yellow | #df8e1d | Warnings |
| Green | #40a02b | Success |
| Teal | #179299 | — |
| Sky | #04a5e5 | — |
| Sapphire | #209fb5 | — |
| Blue | #1e66f5 | Primary accent |
| Lavender | #7287fd | Focus rings, active borders |
| Text | #4c4f69 | Body text |
| Subtext 1 | #5c5f77 | — |
| Subtext 0 | #6c6f85 | Secondary text |
| Overlay 2 | #7c7f93 | Selection bg (at 20-30% opacity) |
| Overlay 1 | #8c8fa1 | Muted/overlay text |
| Overlay 0 | #9ca0b0 | — |
| Surface 2 | #acb0be | — |
| Surface 1 | #bcc0cc | — |
| Surface 0 | #ccd0da | Borders |
| Base | #eff1f5 | Primary backgrounds, cards, header |
| Mantle | #e6e9ef | Body background, assistant bubbles |
| Crust | #dce0e8 | Code backgrounds, hover highlights |

### Implementation Phases

---

#### Phase 1: Token Migration

Replace all values in `tokens.css`:

```css
/* tokens.css — Complete replacement */
:root {
  /* Colors — Catppuccin Latte */
  --color-text: #4c4f69;           /* Text */
  --color-text-secondary: #6c6f85; /* Subtext0 */
  --color-background: #eff1f5;     /* Base — cards, header, elevated surfaces */
  --color-surface: #e6e9ef;        /* Mantle — body bg, assistant bubbles */
  --color-primary: #1e66f5;        /* Blue */
  --color-primary-hover: #1a5ad8;  /* Blue darkened ~10% */
  --color-secondary: #6c6f85;      /* Subtext0 */
  --color-accent: #7287fd;         /* Lavender — focus rings, active borders */
  --color-muted: #dce0e8;          /* Crust — code bg, hover highlights */
  --color-border: #ccd0da;         /* Surface0 */
  --color-success: #40a02b;        /* Green */
  --color-success-bg: #e2f0dc;     /* Green at ~12% over Base */
  --color-error: #d20f39;          /* Red */
  --color-error-bg: #f5dfe3;       /* Red at ~12% over Base */
  --color-warning: #df8e1d;        /* Yellow */
  --color-warning-bg: #f3ebda;     /* Yellow at ~12% over Base */

  /* Typography — unchanged */
  /* ... keep all existing typography tokens ... */

  /* Spacing — unchanged */
  /* ... keep all existing spacing tokens ... */

  /* Borders — unchanged */
  /* ... keep all existing border tokens ... */

  /* Shadows — soften slightly for Latte palette */
  --shadow-sm: 0 1px 2px rgba(76, 79, 105, 0.06);
  --shadow-md: 0 4px 6px rgba(76, 79, 105, 0.08);
  --shadow-lg: 0 10px 15px rgba(76, 79, 105, 0.10);

  /* Sizes — unchanged */
  /* ... keep all existing size tokens ... */

  /* Transitions — unchanged */
  /* ... keep all existing transition tokens ... */
}
```

**Key decisions:**
- `--color-background` (Base) remains the "elevated" surface (cards, header). `--color-surface` (Mantle) is the body bg. This preserves the current semantic meaning where cards appear brighter than the page background.
- Shadows use Text color (#4c4f69) at low opacity instead of pure black, creating warmer, more integrated shadows.
- Semantic background tints (success-bg, error-bg, warning-bg) are computed as ~12% of the accent color over Base.

---

#### Phase 2: Hardcoded Color Fixes

Replace all hardcoded color values in `style.css`. There are **10 instances** that won't automatically update when tokens change.

**2a. Replace `rgba(37, 99, 235, ...)` → `rgba(30, 102, 245, ...)`**

All 9 instances (Blue rgb = 30, 102, 245):

| Line | Context | Old | New |
|------|---------|-----|-----|
| 262 | Input focus ring | `rgba(37, 99, 235, 0.1)` | `rgba(114, 135, 253, 0.15)` (Lavender) |
| 293 | Option item hover | `rgba(37, 99, 235, 0.02)` | `rgba(30, 102, 245, 0.04)` |
| 298 | Option item checked | `rgba(37, 99, 235, 0.05)` | `rgba(30, 102, 245, 0.08)` |
| 437 | Active sidebar item | `rgba(37, 99, 235, 0.08)` | `rgba(30, 102, 245, 0.10)` |
| 478 | Processing badge bg | `rgba(37, 99, 235, 0.1)` | `rgba(30, 102, 245, 0.12)` |
| 536 | Highlight flash keyframe | `rgba(37, 99, 235, 0.1)` | `rgba(30, 102, 245, 0.12)` |
| 770 | Question header bg | `rgba(37, 99, 235, 0.08)` | `rgba(30, 102, 245, 0.10)` |
| 792 | Other-input focus ring | `rgba(37, 99, 235, 0.1)` | `rgba(114, 135, 253, 0.15)` (Lavender) |

Note: Focus rings (lines 262, 792) switch to Lavender per spec ("Lavender for active borders and focus rings"). Other interactive states stay Blue.

**2b. Replace hardcoded hex**

| Line | Context | Old | New |
|------|---------|-----|-----|
| 236 | `.btn-danger:hover` | `#b91c1c` | `#e64553` (Maroon — Catppuccin's danger hover) |

**2c. Replace `white` on user bubbles**

| Line | Context | Old | New |
|------|---------|-----|-----|
| 609 | `.message-user .message-bubble color` | `white` | `#eff1f5` (Base — per spec) |

Keep `white` on `.btn-primary` and `.btn-danger` for maximum contrast on colored backgrounds.

---

#### Phase 3: Component Refinements

**3a. Assistant message bubbles — switch to Mantle**

```css
/* style.css — assistant bubbles */
.message-assistant .message-bubble {
  background: var(--color-surface);   /* Mantle — was --color-background */
  border: var(--border-width) solid var(--color-border);
  border-bottom-left-radius: var(--radius-sm);
}
```

Update nested code block backgrounds to use Crust for contrast against Mantle:

```css
.message-assistant .message-bubble code {
  background: var(--color-muted);  /* Crust — already correct token */
}

.message-assistant .message-bubble pre {
  background: var(--color-muted);  /* Crust — was --color-surface */
  border: var(--border-width) solid var(--color-border);
}
```

**3b. Cards — cleaner edges, refined shadows**

```css
.card {
  background: var(--color-background);
  border: var(--border-width) solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--space-5);
  box-shadow: var(--shadow-sm);
  transition: box-shadow var(--transition-base), transform var(--transition-base);
}

.card:hover {
  box-shadow: var(--shadow-md);
  transform: translateY(-1px);  /* Subtle lift on hover */
}
```

**3c. Empty state — more welcoming**

```css
.empty-state {
  text-align: center;
  padding: var(--space-16) var(--space-4);
}

.empty-state h2 {
  font-size: var(--font-size-2xl);
  margin-bottom: var(--space-3);
  color: var(--color-text);
}

.empty-state p {
  color: var(--color-text-secondary);
  margin-bottom: var(--space-6);
  max-width: 28rem;
  margin-left: auto;
  margin-right: auto;
  line-height: var(--line-height-relaxed);
}
```

**3d. Question block — better integration with chat**

```css
.question-block {
  margin-top: var(--space-4);
  padding: var(--space-4);
  background: var(--color-background);   /* Base — lighter than chat bg */
  border-radius: var(--radius-lg);       /* Rounder corners */
  border: var(--border-width) solid var(--color-border);
  box-shadow: var(--shadow-sm);          /* Add subtle shadow */
}
```

**3e. Focus rings — switch to Lavender**

```css
.card-action:focus-visible {
  outline: 2px solid var(--color-accent);  /* Lavender */
  outline-offset: 2px;
}

textarea:focus,
input[type="text"]:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px rgba(114, 135, 253, 0.15);  /* Lavender focus ring */
}
```

**3f. Selection styling (new)**

```css
::selection {
  background: rgba(124, 127, 147, 0.25);  /* Overlay2 at 25% */
  color: var(--color-text);
}
```

**3g. Submission markers — align with new palette**

The `highlight-flash` keyframe already uses a hardcoded blue; fix in Phase 2. The `submission-marker-text` uses `--color-surface` and `--color-border` which will update automatically.

**3h. Revision sidebar — no CSS changes needed**

Uses tokens (`--color-border`, `--color-text-secondary`, `--color-primary`) that auto-update.

---

#### Phase 4: Animations & Transitions

All CSS-only. Add to end of `style.css`.

**4a. Fade-in for new chat messages**

```css
/* New messages fade in when HTMX appends them */
@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* htmx-added is applied by HTMX during swap settling */
.message.htmx-added,
.question-block.htmx-added,
.prompt-ready.htmx-added,
.repo-status.htmx-added {
  animation: fadeIn 0.3s ease-out;
}
```

Note: HTMX applies the `htmx-added` class to newly inserted elements during `beforeend` swaps, then removes it after the settle period. This ensures only NEW messages animate, not existing ones on page load.

**4b. Question block appear/disappear**

The question block is inserted via HTMX swap (`beforeend`), so `htmx-added` handles the appear animation (covered by 4a). Removal is handled by `this.closest('.question-block').remove()` in JS, which is instant. A CSS exit animation is not feasible with `remove()` — acceptable trade-off.

**4c. Refined loading spinner**

Replace the simple border-spin with a smoother pulsing dot animation:

```css
.spinner {
  width: 1rem;
  height: 1rem;
  border: 2px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s ease-in-out infinite;  /* Slower, eased rotation */
}
```

Change from `0.6s linear` to `0.8s ease-in-out` for a more organic feel. Keep the border-spinner design as it's recognizable.

**4d. Smooth hover transitions on all interactive elements**

Add transitions where missing:

```css
a {
  transition: color var(--transition-fast);
}

.badge {
  transition: background var(--transition-fast), color var(--transition-fast);
}

.revision-link {
  transition: color var(--transition-fast);
}
```

**4e. Smooth scroll for revision anchors**

Add to `style.css`:

```css
.chat-messages {
  scroll-behavior: smooth;
}
```

This handles both `scrollIntoView` calls (already using `behavior: "smooth"`) and native anchor navigation (`<a href="#revision-N">`).

---

#### Phase 5: Accessibility Verification

After implementation, verify these contrast ratios meet WCAG AA (4.5:1 for normal text, 3:1 for large text):

| Combination | Expected Ratio | Status |
|-------------|---------------|--------|
| Text (#4c4f69) on Base (#eff1f5) | ~7.5:1 | Pass AAA |
| Base (#eff1f5) on Blue (#1e66f5) | ~5.7:1 | Pass AA |
| Blue (#1e66f5) on Base (#eff1f5) | ~5.7:1 | Pass AA |
| Subtext0 (#6c6f85) on Base (#eff1f5) | ~4.7:1 | Pass AA |
| Green (#40a02b) on success-bg | ~4.8:1 | Pass AA (verify) |
| Red (#d20f39) on error-bg | ~5.5:1 | Pass AA (verify) |
| Yellow (#df8e1d) on warning-bg | ~4.2:1 | Borderline — may need darker bg |

**Action:** If Yellow on warning-bg fails, darken `--color-warning-bg` slightly or use Peach (#fe640b) which has better contrast properties.

## Acceptance Criteria

### Functional Requirements

- [x] All 18 color tokens in `tokens.css` updated to Catppuccin Latte values
- [x] All 9 hardcoded `rgba(37, 99, 235, ...)` instances replaced
- [x] Hardcoded `#b91c1c` danger hover replaced with Maroon (#e64553)
- [x] User message bubbles use Base (#eff1f5) text on Blue background
- [x] Assistant message bubbles use Mantle (#e6e9ef) background
- [x] Cards have subtle lift on hover (`translateY(-1px)`)
- [x] New chat messages fade in via `htmx-added` class animation
- [x] Spinner uses smoother `0.8s ease-in-out` animation
- [x] `.chat-messages` has `scroll-behavior: smooth`
- [x] Focus rings use Lavender (#7287fd) instead of Blue
- [x] `::selection` uses Overlay2 at 25% opacity
- [x] All shadow tokens use tinted shadows (Text color at low opacity)
- [x] No dark mode — light-only theme

### Non-Functional Requirements

- [x] All text-on-background combinations pass WCAG AA (4.5:1)
- [x] No layout shifts — spacing and sizing tokens unchanged
- [x] Mobile responsive breakpoint (768px) continues working
- [x] Animations are CSS-only (except scroll behavior)
- [x] No new JavaScript dependencies
- [x] No template changes required
- [x] No Go code changes required

### Quality Gates

- [ ] Visual inspection on dashboard, repo page, and conversation page
- [ ] Test all interactive states: hover, focus, active, selected
- [ ] Test question block with options (radio, checkbox, other input)
- [ ] Test processing/cancelled/error/ready status indicators
- [ ] Test archive/unarchive flow on both list pages and conversation
- [ ] Test mobile viewport (< 768px)
- [ ] Verify Markdown rendering in assistant bubbles (headings, code, lists, blockquotes, links)
- [ ] Verify revision content rendering in expandable submission markers

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Yellow warning text fails contrast | Darken warning-bg or switch to Peach accent |
| `htmx-added` class timing inconsistent | Fallback: animate all `.message` elements with `animation-delay` based on load state |
| Hardcoded color missed | Search for `#2563eb`, `#1d4ed8`, `#b91c1c`, `rgb(37`, `rgba(37` |
| Inline styles in templates use old colors | Audit templates — current templates use only CSS classes and token vars in inline styles |

## Out of Scope

- Dark mode toggle
- Font changes (keep system-ui)
- Template/layout restructuring
- Native `confirm()` dialog replacement (follow-up)
- Favicon or logo changes
- Markdown table/image styling (follow-up)

## References

### Internal References

- `internal/server/static/tokens.css` — all design tokens
- `internal/server/static/style.css` — all component styles (~1045 lines)
- `internal/server/static/app.js` — client-side JS (scroll, markdown, timers)
- `internal/server/templates/` — all 7 HTML templates

### External References

- [Catppuccin Latte Palette](https://catppuccin.com/palette/)
- [Catppuccin Style Guide](https://github.com/catppuccin/catppuccin/blob/main/docs/style-guide.md)
- [WCAG Contrast Requirements](https://www.w3.org/WAI/WCAG21/Understanding/contrast-minimum.html)
- [HTMX CSS Transitions](https://htmx.org/docs/#css_transitions)

### Related Work

- Issue: #26
