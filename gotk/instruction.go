package gotk

// Mode constants for the html instruction.
const (
	Replace = "replace"
	Append  = "append"
	Prepend = "prepend"
	Remove  = "remove"
)

// Instruction is a single DOM operation. All fields are optional except Op.
// Only the fields relevant to each Op are serialized to JSON.
type Instruction struct {
	Op      string         `json:"op"`
	Target  string         `json:"target,omitempty"`
	HTML    string         `json:"html,omitempty"`
	Mode    string         `json:"mode,omitempty"`
	Source  string         `json:"source,omitempty"`
	Attr    string         `json:"attr,omitempty"`
	Value   string         `json:"value,omitempty"`
	Event   string         `json:"event,omitempty"`
	Detail  map[string]any `json:"detail,omitempty"`
	URL     string         `json:"url,omitempty"`
	Name string         `json:"name,omitempty"`
	Args map[string]any `json:"args,omitempty"`
	Data map[string]any `json:"data,omitempty"`
}

// HandlerFunc is the signature for all command handlers.
type HandlerFunc func(ctx *Context) error
