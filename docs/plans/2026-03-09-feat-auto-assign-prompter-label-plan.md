---
title: "feat: Auto-assign prompter label to published GitHub issues"
type: feat
date: 2026-03-09
issue: 47
---

# feat: Auto-assign "prompter" label to published GitHub issues

## Overview

When a prompt request is published as a GitHub issue for the first time, Prompter automatically assigns a "prompter" label. This lets maintainers filter and identify prompt request issues in their repositories.

## Acceptance Criteria

- [x] On first publish (`pr.IssueNumber == nil`), the "prompter" label is assigned to the created issue
- [x] If the label does not exist in the repo, it is created first (default color, no description)
- [x] If the label already exists, it is reused without modification
- [x] Label failure (creation or assignment) does not block issue publish
- [x] Label failure is logged as a warning server-side
- [x] On re-publish (update), no label logic runs (label already present from first publish)

## Approach

Pass `--label prompter` at `gh issue create` time (atomic, single API call) rather than applying it post-creation. Ensure the label exists beforehand with a separate `gh label create` call that treats "already exists" as success.

If the ensure step fails, fall back to creating the issue without the label.

## MVP

### 1. Add `EnsureLabel` to `internal/github/github.go`

```go
const LabelName = "prompter"

// EnsureLabel creates a label in the repository if it does not already exist.
// Returns nil if the label was created or already exists.
func EnsureLabel(ctx context.Context, repoURL, name string) error {
	ghRepo := toGHRepo(repoURL)
	cmd := exec.CommandContext(ctx, "gh", "label", "create", name, "--repo", ghRepo)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "already exists") {
			return nil
		}
		return fmt.Errorf("ensuring label %q: %s", name, strings.TrimSpace(string(output)))
	}
	return nil
}
```

### 2. Add `labels` parameter to `CreateIssue` in `internal/github/github.go`

```go
func CreateIssue(ctx context.Context, repoURL, title, body string, labels []string) (*Issue, error) {
	ghRepo := toGHRepo(repoURL)

	args := []string{"issue", "create",
		"--repo", ghRepo,
		"--title", title,
		"--body", body,
	}
	for _, l := range labels {
		args = append(args, "--label", l)
	}

	cmd := exec.CommandContext(ctx, "gh", args...)
	// ... rest unchanged
}
```

### 3. Update call site in `internal/server/handlers.go`

In the first-publish branch of `handlePublish` (around line 437):

```go
} else {
	// Ensure "prompter" label exists (best-effort)
	var labels []string
	if err := github.EnsureLabel(r.Context(), pr.RepoURL, github.LabelName); err != nil {
		log.Printf("warning: ensuring label %q: %v", github.LabelName, err)
	} else {
		labels = []string{github.LabelName}
	}

	// Create new issue
	issue, err := github.CreateIssue(r.Context(), pr.RepoURL, issueTitle, body, labels)
	if err != nil {
		log.Printf("creating issue: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create GitHub issue: %v", err), http.StatusInternalServerError)
		return
	}
	if err := s.queries.UpdatePromptRequestIssue(id, issue.Number, issue.URL); err != nil {
		log.Printf("updating issue info: %v", err)
	}
}
```

## Edge Cases

- **"already exists" detection**: Parse `gh label create` stderr for "already exists" substring; treat as success
- **Concurrent publishes to same repo**: Both race on label creation; one gets "already exists" which is handled as success
- **Insufficient label permissions but sufficient issue permissions**: Label ensure fails, issue is created without label, warning logged
- **Network down**: Label ensure fails, issue creation also fails (existing error handling covers this)
- **Re-publish after manual label removal**: Label is not re-applied (matches spec: "only on initial publish")

## Out of Scope

- UI warning/flash message for label failure (log-only, following existing patterns)
- Configurable label name per repository
- Label color or description customization
- Re-applying label on re-publish/update

## References

- Issue: #47
- GitHub client: `internal/github/github.go`
- Publish handler: `internal/server/handlers.go:387-473`
- Existing error handling pattern: `log.Printf` for non-blocking errors (handlers.go:446, 456)
