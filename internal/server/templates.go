package server

import (
	"fmt"
	"html/template"
	"io/fs"
	"path/filepath"
)

var funcMap = template.FuncMap{
	"deref": func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	},
}

// loadTemplates builds per-page template sets. Each page gets a clone of the
// base template (layout + shared hx fragments) with its own page-specific
// templates added. This prevents {{define}} block conflicts between pages
// (e.g., multiple pages defining "title" or "content").
func loadTemplates() (map[string]*template.Template, error) {
	// Build base template with shared fragments
	base := template.New("").Funcs(funcMap)

	// Parse pages/base.html (layout)
	data, err := fs.ReadFile(contentFS, "pages/base.html")
	if err != nil {
		return nil, fmt.Errorf("reading pages/base.html: %w", err)
	}
	template.Must(base.New("pages/base.html").Parse(string(data)))

	// Parse all hx/*.html (shared fragments)
	if err := fs.WalkDir(contentFS, "hx", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".html" {
			return err
		}
		data, readErr := fs.ReadFile(contentFS, path)
		if readErr != nil {
			return readErr
		}
		template.Must(base.New(path).Parse(string(data)))
		return nil
	}); err != nil {
		return nil, fmt.Errorf("parsing hx templates: %w", err)
	}

	// Build per-page template sets by cloning base and adding page-specific templates
	pageDirs := []string{"dashboard", "repo", "conversation"}
	tmpls := make(map[string]*template.Template, len(pageDirs))

	for _, dir := range pageDirs {
		t, err := base.Clone()
		if err != nil {
			return nil, fmt.Errorf("cloning base for %s: %w", dir, err)
		}

		pageDir := "pages/" + dir
		if err := fs.WalkDir(contentFS, pageDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(path) != ".html" {
				return err
			}
			data, readErr := fs.ReadFile(contentFS, path)
			if readErr != nil {
				return readErr
			}
			template.Must(t.New(path).Parse(string(data)))
			return nil
		}); err != nil {
			return nil, fmt.Errorf("parsing %s templates: %w", dir, err)
		}

		tmpls[dir] = t
	}

	return tmpls, nil
}
