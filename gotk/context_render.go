//go:build !tinygo

package gotk

import (
	"bytes"
	"html/template"
)

// Render renders a Go template by name and returns the HTML string.
// Not available in TinyGo/WASM builds.
func (c *Context) Render(name string, data any) string {
	if c.templates == nil {
		return ""
	}
	tmpl, ok := c.templates.(*template.Template)
	if !ok {
		return ""
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return ""
	}
	return buf.String()
}

// setTemplates configures the template engine for ctx.Render.
func (c *Context) setTemplates(t *template.Template) {
	c.templates = t
}
