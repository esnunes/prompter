package gotk

import (
	"html/template"
	"log"
	"net/http"
)

// RenderPage renders a full server-side page using templates.
// It executes the named template with the given data.
func RenderPage(w http.ResponseWriter, tmpl *template.Template, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("gotk: render error (%s): %v", name, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// RenderFragment renders a named template fragment to the response.
func RenderFragment(w http.ResponseWriter, tmpl *template.Template, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("gotk: render error (%s): %v", name, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// RenderString renders a named template to a string.
func RenderString(tmpl *template.Template, name string, data any) string {
	if tmpl == nil {
		return ""
	}
	var w stringWriter
	if err := tmpl.ExecuteTemplate(&w, name, data); err != nil {
		return ""
	}
	return w.String()
}

type stringWriter struct {
	buf []byte
}

func (w *stringWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *stringWriter) String() string {
	return string(w.buf)
}
