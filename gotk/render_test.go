package gotk

import (
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderPage(t *testing.T) {
	tmpl := template.Must(template.New("layout").Parse(`<html><body>{{block "content" .}}{{end}}</body></html>`))
	template.Must(tmpl.New("content").Parse(`<h1>{{.Title}}</h1>`))

	w := httptest.NewRecorder()
	RenderPage(w, tmpl, "layout", struct{ Title string }{"Hello"})

	body := w.Body.String()
	if !strings.Contains(body, "<h1>Hello</h1>") {
		t.Errorf("body = %q", body)
	}
	if w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("content-type = %q", w.Header().Get("Content-Type"))
	}
}

func TestRenderString(t *testing.T) {
	tmpl := template.Must(template.New("item").Parse(`<li>{{.}}</li>`))
	got := RenderString(tmpl, "item", "test")
	if got != "<li>test</li>" {
		t.Errorf("got %q", got)
	}
}

func TestRenderString_Nil(t *testing.T) {
	got := RenderString(nil, "x", nil)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
