package gotk

import (
	"bytes"
	"html/template"
)

// ViewContext is available to view event handlers. It provides the full
// instruction builder and template rendering. This is the only context
// that can produce DOM instructions.
type ViewContext struct {
	instructions []Instruction
	templates    *template.Template
}

// NewViewContext creates a new ViewContext with the given templates.
func NewViewContext(templates *template.Template) *ViewContext {
	return &ViewContext{templates: templates}
}

// Instructions returns all instructions accumulated since the last Reset.
func (c *ViewContext) Instructions() []Instruction {
	return c.instructions
}

// Reset clears accumulated instructions. Called by the framework after
// flushing instructions to the client.
func (c *ViewContext) Reset() {
	c.instructions = c.instructions[:0]
}

// HTML produces an html instruction.
func (c *ViewContext) HTML(target, html string, mode ...string) {
	m := Replace
	if len(mode) > 0 && mode[0] != "" {
		m = mode[0]
	}
	c.instructions = append(c.instructions, Instruction{
		Op:     "html",
		Target: target,
		HTML:   html,
		Mode:   m,
	})
}

// Remove produces an html instruction with mode "remove".
func (c *ViewContext) Remove(target string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "html",
		Target: target,
		Mode:   Remove,
	})
}

// Template produces a template instruction.
func (c *ViewContext) Template(source, target string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "template",
		Source:  source,
		Target: target,
	})
}

// Populate produces a populate instruction.
func (c *ViewContext) Populate(target string, data map[string]any) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "populate",
		Target: target,
		Data:   data,
	})
}

// Navigate produces a navigate instruction.
func (c *ViewContext) Navigate(url string, targetAndHTML ...string) {
	ins := Instruction{Op: "navigate", URL: url}
	if len(targetAndHTML) >= 1 {
		ins.Target = targetAndHTML[0]
	}
	if len(targetAndHTML) >= 2 {
		ins.HTML = targetAndHTML[1]
	}
	c.instructions = append(c.instructions, ins)
}

// AttrSet produces an attr-set instruction.
func (c *ViewContext) AttrSet(target, attr string, value ...string) {
	v := ""
	if len(value) > 0 {
		v = value[0]
	}
	c.instructions = append(c.instructions, Instruction{
		Op:     "attr-set",
		Target: target,
		Attr:   attr,
		Value:  v,
	})
}

// AttrRemove produces an attr-remove instruction.
func (c *ViewContext) AttrRemove(target, attr string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "attr-remove",
		Target: target,
		Attr:   attr,
	})
}

// SetValue produces a set-value instruction.
func (c *ViewContext) SetValue(target, value string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "set-value",
		Target: target,
		Value:  value,
	})
}

// Dispatch produces a dispatch instruction (dispatches a DOM CustomEvent).
func (c *ViewContext) Dispatch(target, event string, detail ...map[string]any) {
	ins := Instruction{Op: "dispatch", Target: target, Event: event}
	if len(detail) > 0 {
		ins.Detail = detail[0]
	}
	c.instructions = append(c.instructions, ins)
}

// Focus produces a focus instruction.
func (c *ViewContext) Focus(target string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "focus",
		Target: target,
	})
}

// Exec produces an exec instruction.
func (c *ViewContext) Exec(name string, args ...map[string]any) {
	ins := Instruction{Op: "exec", Name: name}
	if len(args) > 0 {
		ins.Args = args[0]
	}
	c.instructions = append(c.instructions, ins)
}

// Error produces an html instruction with a gotk-error div.
func (c *ViewContext) Error(target, message string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "html",
		Target: target,
		HTML:   `<div class="gotk-error">` + htmlEscape(message) + `</div>`,
	})
}

// Render renders a Go template by name and returns the HTML string.
func (c *ViewContext) Render(name string, data any) string {
	if c.templates == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := c.templates.ExecuteTemplate(&buf, name, data); err != nil {
		return ""
	}
	return buf.String()
}
