package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const timeout = 120 * time.Second

const systemPrompt = `You are a helpful assistant that guides open source contributors in creating clear, actionable feature requests for repository maintainers.

You are running inside the repository's codebase. Use your tools (Read, Glob, Grep) to explore the code and understand the project structure, patterns, and conventions. This helps you ask informed questions.

Your goal is to gather enough context to generate a well-crafted "prompt request" — a natural language prompt that a maintainer can feed to their AI coding agent to implement the feature.

Guidelines:
- Start by understanding what the contributor wants at a high level
- Explore the codebase to understand relevant patterns and architecture
- Ask clarifying questions one at a time using the "question" field with options
- Keep questions simple and non-technical — contributors may not be developers
- When you have enough context, set "prompt_ready" to true and include the "generated_prompt" — a clear, detailed prompt that describes what to build, where in the codebase, and any relevant context from the code
- The generated prompt should be self-contained: a maintainer reading only the prompt (without the conversation) should understand exactly what to implement
- Always include your thinking in "message" so the contributor understands what you're doing`

const jsonSchema = `{
  "type": "object",
  "properties": {
    "message": {
      "type": "string",
      "description": "Your response to the contributor"
    },
    "question": {
      "type": "object",
      "properties": {
        "text": {
          "type": "string",
          "description": "A clarifying question to ask"
        },
        "options": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "label": { "type": "string" },
              "description": { "type": "string" }
            },
            "required": ["label", "description"]
          }
        }
      },
      "required": ["text", "options"]
    },
    "prompt_ready": {
      "type": "boolean",
      "description": "True when you have gathered enough context to generate the final prompt"
    },
    "generated_prompt": {
      "type": "string",
      "description": "The complete prompt request, only when prompt_ready is true"
    }
  },
  "required": ["message"]
}`

type Response struct {
	Message         string    `json:"message"`
	Question        *Question `json:"question,omitempty"`
	PromptReady     bool      `json:"prompt_ready,omitempty"`
	GeneratedPrompt string    `json:"generated_prompt,omitempty"`
}

type Question struct {
	Text    string   `json:"text"`
	Options []Option `json:"options"`
}

type Option struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

func SendMessage(ctx context.Context, sessionID, repoDir, userMessage string, resume bool) (*Response, string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"-p"}
	if resume {
		// Continue an existing session.
		args = append(args, "--resume", sessionID)
	} else {
		// First message — create a new session with this ID.
		args = append(args, "--session-id", sessionID)
	}
	args = append(args,
		"--output-format", "json",
		"--json-schema", jsonSchema,
		"--system-prompt", systemPrompt,
		"--allowedTools", "Read,Glob,Grep",
		"--permission-mode", "bypassPermissions",
		userMessage,
	)

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = repoDir
	cmd.Env = envWithout("CLAUDECODE")
	// Send SIGTERM on context cancellation so Claude CLI can clean up its
	// session lock before exiting. Fall back to SIGKILL after 5 seconds.
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = 5 * time.Second

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, "", fmt.Errorf("AI is taking too long, please try again")
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, "", fmt.Errorf("claude error: %s", string(exitErr.Stderr))
		}
		return nil, "", fmt.Errorf("running claude: %w", err)
	}

	rawJSON := string(output)
	resp, err := parseResponse(output)
	if err != nil {
		return &Response{Message: rawJSON}, rawJSON, nil
	}
	return resp, rawJSON, nil
}

func parseResponse(output []byte) (*Response, error) {
	// claude -p --output-format json returns:
	// {"type":"result", "structured_output": {...}, "result": "", ...}
	var wrapper struct {
		StructuredOutput *Response `json:"structured_output"`
		Result           string    `json:"result"`
	}
	if err := json.Unmarshal(output, &wrapper); err == nil {
		if wrapper.StructuredOutput != nil {
			return wrapper.StructuredOutput, nil
		}
		if wrapper.Result != "" {
			var resp Response
			if err := json.Unmarshal([]byte(wrapper.Result), &resp); err != nil {
				return &Response{Message: wrapper.Result}, nil
			}
			return &resp, nil
		}
	}

	// Try parsing directly as our schema
	var resp Response
	if err := json.Unmarshal(output, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func envWithout(key string) []string {
	prefix := key + "="
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, prefix) {
			env = append(env, e)
		}
	}
	return env
}
