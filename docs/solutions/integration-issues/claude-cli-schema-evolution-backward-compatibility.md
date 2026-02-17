---
title: "Schema evolution from singular question to questions array with backward-compatible parsing"
date: 2026-02-17
module: claude-integration
severity: high
symptoms:
  - "Old sessions with singular 'question' field fail to render interactive question blocks after schema upgrade"
  - "extractQuestionFromRaw returns nil for new questions array format"
  - "Page reload loses question context for pre-upgrade sessions"
root_cause: "JSON schema field renamed from singular 'question' object to 'questions' array. Go's json.Unmarshal silently skips unknown fields, so old data parsed with the new struct produces empty Questions slice instead of an error."
tags: [json-schema, backward-compatibility, schema-evolution, sqlite, raw-response, json-parsing]
---

# Schema Evolution: Singular Object to Array with Backward-Compatible Parsing

## Problem

The Claude response schema evolved from a singular `"question"` object to a `"questions"` array to support batched multi-select questions. After this change, existing assistant messages stored in SQLite's `raw_response` column still contained the old shape:

```json
{"structured_output": {"question": {"text": "...", "options": [...]}}}
```

The updated `claude.Response` struct only unmarshals `"questions"` (the array). When `json.Unmarshal` processes old data with `"question"` (singular), it silently ignores the unrecognized key and leaves `Questions` as nil. No error is raised — the regression is silent.

## Root Cause

Go's `encoding/json` decoder skips unknown fields by default. When the struct field binding changed from `Question *Question` to `Questions []Question`, old stored JSON with `"question"` was silently ignored rather than failing loudly.

## Solution

Two-pass parse strategy in `extractQuestionsFromRaw`:

1. Parse with the current `claude.Response` struct (via `parseRawResponse`)
2. If `resp.Questions` is empty, fall back to `extractLegacyQuestion` which targets the old schema using an anonymous inline struct

```go
func extractQuestionsFromRaw(rawJSON string) ([]questionData, bool) {
    resp := parseRawResponse(rawJSON)
    if resp == nil {
        return nil, false
    }

    if len(resp.Questions) == 0 {
        // Try the old singular "question" field for backward compat
        questions := extractLegacyQuestion(rawJSON)
        return questions, resp.PromptReady
    }

    var questions []questionData
    for i, q := range resp.Questions {
        qd := questionData{
            Header:      q.Header,
            Text:        q.Text,
            MultiSelect: q.MultiSelect,
            Index:       i,
        }
        for _, opt := range q.Options {
            qd.Options = append(qd.Options, optionData{
                Label: opt.Label, Description: opt.Description,
            })
        }
        questions = append(questions, qd)
    }
    return questions, resp.PromptReady
}
```

The legacy fallback uses an anonymous struct isolated from current types:

```go
func extractLegacyQuestion(rawJSON string) []questionData {
    var legacy struct {
        StructuredOutput *struct {
            Question *struct {
                Text    string `json:"text"`
                Options []struct {
                    Label       string `json:"label"`
                    Description string `json:"description"`
                } `json:"options"`
            } `json:"question"`
        } `json:"structured_output"`
    }
    if err := json.Unmarshal([]byte(rawJSON), &legacy); err != nil {
        return nil
    }
    if legacy.StructuredOutput == nil || legacy.StructuredOutput.Question == nil {
        return nil
    }

    q := legacy.StructuredOutput.Question
    qd := questionData{Text: q.Text, Index: 0}
    for _, opt := range q.Options {
        qd.Options = append(qd.Options, optionData{
            Label: opt.Label, Description: opt.Description,
        })
    }
    return []questionData{qd}
}
```

Note: `PromptReady` is taken from the primary parse, not from the legacy struct. The `prompt_ready` field existed in both schemas unchanged, so the primary parse handles it in all cases.

## Key Insight

When evolving a JSON schema stored in a database, the safe pattern is: **primary parse with the current struct, check for empty/zero values, then re-parse the same raw bytes with a purpose-built legacy struct targeting only the old field names.** This keeps legacy code isolated in a single function, avoids migration of existing rows, and `len(resp.Questions) == 0` acts as a natural discriminator between old and new data.

## Prevention

- **When renaming or restructuring JSON fields**: always check if old data is persisted anywhere (database, files, caches). If so, implement a fallback parser before deploying the new schema.
- **Test against stored data**: create test fixtures with old format JSON and verify parsing still works after schema changes.
- **Go's silent field-skipping is the core hazard**: `json.Unmarshal` won't error on unknown fields. Add explicit checks for empty/zero values after parsing to detect when the expected fields weren't found.
- **Keep the three-tier parsing chain maintained**: this codebase has three locations that parse Claude CLI output (`claude.go:parseResponse`, `handlers.go:parseRawResponse`, `queries.go:extractGeneratedContent`). All must be updated together.

## Related

- [Claude CLI structured output parsing](claude-cli-structured-output-parsing.md) — the three-tier fallback chain (structured_output -> result string -> direct parse)
- [Claude CLI session resume flag](claude-cli-session-resume-flag.md) — `--session-id` vs `--resume` flag handling
- `internal/server/handlers.go:extractQuestionsFromRaw` + `extractLegacyQuestion`
- `internal/claude/claude.go:Response` struct
