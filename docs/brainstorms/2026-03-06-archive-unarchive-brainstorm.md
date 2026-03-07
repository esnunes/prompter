# Brainstorm: Archive and Unarchive Prompt Requests

**Date:** 2026-03-06
**Status:** Draft
**Issue:** [#27](https://github.com/esnunes/prompter/issues/27)

## What We're Building

Add the ability to archive and unarchive prompt requests so users can declutter their prompt list without permanently deleting anything. Archived prompts disappear from the default view but remain accessible through a toggle.

### Core Changes

1. **Archive action** — Available from two places: the prompt list pages (dashboard and repo page) and the revision sidebar in the conversation view. Uses native `confirm()` dialog. If the prompt has been published to GitHub, the dialog warns that the GitHub issue remains open.

2. **Archived prompt filtering** — A compact "Show archived" toggle switch on dashboard and repo list pages. When toggled on, shows only archived prompts. When toggled off (default), shows only active prompts.

3. **Sidebar behavior** — The left prompt list sidebar always shows active prompts only. No archive toggle in the sidebar.

4. **Unarchive action** — When viewing archived prompts, each prompt has an unarchive action that restores it to its previous state (draft or published). No confirmation dialog needed.

5. **Database change** — New boolean `archived` column (default `false`) on `prompt_requests` table. Status field (draft/published) stays unchanged, enabling seamless restore on unarchive.

## Why This Approach

- **Boolean column over status change** — Keeping `archived` separate from `status` means unarchiving simply flips the boolean back. No need to track "previous status" in a separate field.
- **Native `confirm()`** — YAGNI. A browser-native dialog is simple, accessible, and avoids building a custom modal component.
- **Toggle switch over tabs** — More compact, doesn't change the page layout. Fits naturally as a small control in the list page header.
- **Sidebar stays active-only** — The sidebar is a quick navigation tool. Cluttering it with an archive toggle adds complexity for minimal benefit. Users can manage archived prompts from the main list pages.
- **Archive in revision sidebar** — The conversation view already has a right sidebar for prompt-level actions (revisions, GitHub issue link). Adding archive here is natural and doesn't require a new UI region.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Schema approach | Boolean `archived` column | Preserves status (draft/published) for restore on unarchive |
| Confirmation dialog | Native `confirm()` | YAGNI — simple, accessible, no custom modal needed |
| List page filter | Toggle switch ("Show archived") | Compact, doesn't change layout |
| Sidebar behavior | Active prompts only | Sidebar is for quick navigation, not archive management |
| Conversation archive location | Revision sidebar (right panel) | Natural home for prompt-level actions |
| Unarchive confirmation | None required | Low-risk action, easily reversible |
| Post-archive behavior (conversation) | Stay on page with archived banner | User may want to unarchive immediately; no jarring redirect |
| Archive action on list pages | Icon button on each row (always visible) | Direct, one-click access without extra interaction |

## Resolved Questions

1. **Post-archive redirect** — Stay on the conversation page and show an "archived" banner with an unarchive option. No redirect.
2. **Archive on list pages** — Icon button visible on each prompt row (always visible, not hover-only or behind a menu).
