---
title: "fix: Reduce implementation detail in generated prompt requests"
type: fix
date: 2026-02-17
---

# fix: Reduce implementation detail in generated prompt requests

Generated prompt requests (published as GitHub Issues) contain too much implementation detail — specific routes, file paths, "Key files to modify" tables, and "Existing patterns to follow" sections. These belong in an implementation plan, not a prompt request.

A prompt request should describe the feature from a user/product perspective: what to build, how it should work, and UX suggestions. The AI coding agent receiving the prompt should decide the implementation details itself.

## Problem

Current system prompt (line 27 of `internal/claude/claude.go`) instructs:

> "describes what to build, **where in the codebase**, and any relevant context from the code"

This causes Claude to include:
- Specific routes (`GET /repositories/new`, `POST /repositories`)
- Template filenames (`templates/add_repository.html`)
- "Key Files to Modify" tables
- "Existing Patterns to Follow" sections
- Step-by-step implementation instructions

## Acceptance Criteria

- [x] Update the system prompt to instruct Claude to generate conceptual feature descriptions, not implementation plans
- [x] The generated prompt should describe: what to build, how it should work for users, suggested UX/navigation
- [x] The generated prompt should NOT include: specific file paths, routes, code patterns, or "files to modify" lists
- [x] Claude should still explore the codebase (to ask informed questions), but not leak those details into the final prompt

## Context

The system prompt is at `internal/claude/claude.go:15-28`. Only the prompt text needs to change — no structural code changes required.

Example of current (too detailed) output: https://github.com/esnunes/prompter/issues/3
