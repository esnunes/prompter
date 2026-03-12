package gotk

import "encoding/json"

// CommandRegistry holds frontend command handlers for WASM compilation.
// Commands registered here are discoverable by the thin client via
// listCommands() and executable via execCommand().
type CommandRegistry struct {
	handlers map[string]HandlerFunc
	names    []string
}

// NewCommandRegistry creates a new CommandRegistry.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		handlers: make(map[string]HandlerFunc),
	}
}

// Register adds a command handler to the registry.
func (r *CommandRegistry) Register(name string, handler HandlerFunc) {
	if _, exists := r.handlers[name]; !exists {
		r.names = append(r.names, name)
	}
	r.handlers[name] = handler
}

// ListCommandsJSON returns a JSON array of registered command names.
// Called once by the thin client at init to build the local command set.
func (r *CommandRegistry) ListCommandsJSON() string {
	if len(r.names) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(r.names)
	return string(data)
}

// execResult is the JSON shape returned by ExecCommandJSON.
type execResult struct {
	Ins   []Instruction `json:"ins"`
	Async []AsyncCall   `json:"async,omitempty"`
}

// ExecCommandJSON dispatches a command by name with a JSON payload.
// Returns a JSON string: {"ins": [...], "async": [...]}.
func (r *CommandRegistry) ExecCommandJSON(cmd, payloadJSON string) string {
	var payload map[string]any
	if payloadJSON != "" {
		_ = json.Unmarshal([]byte(payloadJSON), &payload)
	}

	handler, ok := r.handlers[cmd]
	if !ok {
		return marshalResult(execResult{
			Ins: []Instruction{{
				Op:   "exec",
				Name: "console.warn",
				Args: map[string]any{"message": "unknown command: " + cmd},
			}},
		})
	}

	ctx := &Context{Payload: NewPayload(payload)}
	if err := handler(ctx); err != nil {
		return marshalResult(execResult{
			Ins: []Instruction{{
				Op:   "exec",
				Name: "console.warn",
				Args: map[string]any{"message": "command error: " + err.Error()},
			}},
		})
	}

	result := execResult{
		Ins:   ctx.instructions,
		Async: ctx.asyncCalls,
	}
	if result.Ins == nil {
		result.Ins = []Instruction{}
	}
	return marshalResult(result)
}

func marshalResult(r execResult) string {
	data, _ := json.Marshal(r)
	return string(data)
}
