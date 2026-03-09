package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const LabelName = "prompter"

type Issue struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

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

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("creating issue: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("creating issue: %w", err)
	}

	// gh issue create outputs the issue URL
	issueURL := strings.TrimSpace(string(output))

	// Extract issue number from URL (e.g., https://github.com/owner/repo/issues/42)
	number, err := extractIssueNumber(issueURL)
	if err != nil {
		return nil, err
	}

	return &Issue{Number: number, URL: issueURL}, nil
}

func EditIssue(ctx context.Context, repoURL string, issueNumber int, body string) error {
	ghRepo := toGHRepo(repoURL)

	cmd := exec.CommandContext(ctx, "gh", "issue", "edit",
		strconv.Itoa(issueNumber),
		"--repo", ghRepo,
		"--body", body,
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("editing issue: %s", string(output))
	}
	return nil
}

// VerifyRepo checks if a repository exists on GitHub using the gh CLI.
func VerifyRepo(ctx context.Context, org, repo string) error {
	cmd := exec.CommandContext(ctx, "gh", "api", fmt.Sprintf("repos/%s/%s", org, repo), "--silent")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("repository not found: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func CheckAuth(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "gh", "auth", "status")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("not authenticated with GitHub: %s\nRun: gh auth login", string(output))
	}
	return nil
}

func toGHRepo(repoURL string) string {
	// Convert "github.com/owner/repo" to "owner/repo"
	return strings.TrimPrefix(repoURL, "github.com/")
}

func extractIssueNumber(issueURL string) (int, error) {
	parts := strings.Split(issueURL, "/")
	if len(parts) == 0 {
		return 0, fmt.Errorf("unexpected issue URL format: %s", issueURL)
	}
	num, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		// Try parsing as JSON (gh might return JSON in some configs)
		var issue Issue
		if jsonErr := json.Unmarshal([]byte(issueURL), &issue); jsonErr == nil && issue.Number > 0 {
			return issue.Number, nil
		}
		return 0, fmt.Errorf("extracting issue number from %q: %w", issueURL, err)
	}
	return num, nil
}
