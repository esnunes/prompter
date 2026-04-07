---
title: "feat: Change primary color theme from blue to Catppuccin Latte Peach"
type: feat
date: 2026-04-07
---

# Change Primary Color to Catppuccin Latte Peach

## Overview

Replace the primary blue color (`#1e66f5`) with Catppuccin Latte Peach (`#fe640b`) across the application by updating CSS custom properties in `tokens.css` and fixing one hardcoded gradient in `style.css`.

## Problem Statement

The current primary color is Catppuccin Latte Blue (`#1e66f5`). The desired primary color is Catppuccin Latte Peach (`#fe640b`) to give the application a warm orange identity.

## Proposed Solution

Update the three `--color-primary-*` CSS custom properties in `tokens.css` and fix the one hardcoded blue reference in `style.css`.

## Technical Approach

### Files to Modify

| File | Changes |
|------|---------|
| `internal/server/static/tokens.css` | Update 3 primary color token values |
| `internal/server/static/style.css` | Update 1 hardcoded gradient color |

No Go code changes. No template changes.

### Step 1: Update `tokens.css` primary color tokens

In `internal/server/static/tokens.css`, change lines 10–12:

| Token | Old Value | New Value |
|-------|-----------|-----------|
| `--color-primary` | `#1e66f5` | `#fe640b` |
| `--color-primary-hover` | `#1a5ad8` | `#e55a0a` |
| `--color-primary-subtle` | `rgba(30, 102, 245, 0.08)` | `rgba(254, 100, 11, 0.08)` |

The hover shade (`#e55a0a`) is derived by darkening `#fe640b` by ~10%, matching the existing pattern where `#1a5ad8` is ~10% darker than `#1e66f5`.

### Step 2: Fix hardcoded gradient in `style.css`

In `internal/server/static/style.css` line 710, update the gradient:

```css
/* Before */
background: linear-gradient(135deg, #1e66f5 0%, #395ced 100%);

/* After */
background: linear-gradient(135deg, #fe640b 0%, #e55a0a 100%);
```

The gradient end color uses the same darker Peach shade for a subtle warm gradient effect.

### Tokens NOT changed

All other tokens remain unchanged:
- `--color-accent: #7287fd` (Lavender)
- `--color-secondary: #6c6f85` (Overlay 0)
- `--color-success`, `--color-error`, `--color-warning`
- All spacing, typography, shadow, border tokens

## Verification

1. `go build ./...` — confirms no build issues (CSS is embedded via `go:embed`)
2. Visual check: buttons, links, and interactive elements should appear orange instead of blue
3. Hover states should darken slightly
4. Subtle backgrounds (e.g., selected items) should have a faint orange tint

## Risks

- **Low risk.** Only CSS color values change. No logic, no templates, no Go code.
- All UI elements referencing `var(--color-primary)` will automatically pick up the new color.
- The one hardcoded gradient is explicitly addressed.
