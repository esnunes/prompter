package gotk

import (
	"bytes"
	"html/template"
)

// AsyncCall represents a server command scheduled by ctx.Async.
type AsyncCall struct {
	Cmd     string
	Payload map[string]any
}

// Context is passed to every command handler.
type Context struct {
	Payload Payload

	instructions []Instruction
	asyncCalls   []AsyncCall
	templates    *template.Template
}

// HTML produces an html instruction.
func (c *Context) HTML(target, html string, mode ...string) {
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
func (c *Context) Remove(target string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "html",
		Target: target,
		Mode:   Remove,
	})
}

// Template produces a template instruction.
func (c *Context) Template(source, target string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "template",
		Source:  source,
		Target: target,
	})
}

// Populate produces a populate instruction.
func (c *Context) Populate(target string, data map[string]any) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "populate",
		Target: target,
		Data:   data,
	})
}

// Navigate produces a navigate instruction.
func (c *Context) Navigate(url string, targetAndHTML ...string) {
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
func (c *Context) AttrSet(target, attr string, value ...string) {
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
func (c *Context) AttrRemove(target, attr string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "attr-remove",
		Target: target,
		Attr:   attr,
	})
}

// SetValue produces a set-value instruction.
func (c *Context) SetValue(target, value string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "set-value",
		Target: target,
		Value:  value,
	})
}

// Dispatch produces a dispatch instruction.
func (c *Context) Dispatch(target, event string, detail ...map[string]any) {
	ins := Instruction{Op: "dispatch", Target: target, Event: event}
	if len(detail) > 0 {
		ins.Detail = detail[0]
	}
	c.instructions = append(c.instructions, ins)
}

// Focus produces a focus instruction.
func (c *Context) Focus(target string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "focus",
		Target: target,
	})
}

// Exec produces an exec instruction.
func (c *Context) Exec(name string, args ...map[string]any) {
	ins := Instruction{Op: "exec", Name: name}
	if len(args) > 0 {
		ins.Args = args[0]
	}
	c.instructions = append(c.instructions, ins)
}

// Async schedules a server command from a frontend command.
func (c *Context) Async(cmd string, payload map[string]any) {
	c.asyncCalls = append(c.asyncCalls, AsyncCall{Cmd: cmd, Payload: payload})
	c.instructions = append(c.instructions, Instruction{
		Op:      "cmd",
		Cmd:     cmd,
		Payload: payload,
	})
}

// Error produces an html instruction with a gotk-error div.
func (c *Context) Error(target, message string) {
	c.instructions = append(c.instructions, Instruction{
		Op:     "html",
		Target: target,
		HTML:   `<div class="gotk-error">` + template.HTMLEscapeString(message) + `</div>`,
	})
}

// Render renders a Go template by name and returns the HTML string.
func (c *Context) Render(name string, data any) string {
	if c.templates == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := c.templates.ExecuteTemplate(&buf, name, data); err != nil {
		return ""
	}
	return buf.String()
}

// setTemplates configures the template engine for ctx.Render.
func (c *Context) setTemplates(t *template.Template) {
	c.templates = t
}
