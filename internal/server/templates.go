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

// loadTemplates builds per-page template sets. Each page gets its own
// independent template with the layout, shared hx fragments, and
// page-specific templates. This prevents {{define}} block conflicts
// between pages (e.g., multiple pages defining "title" or "content").
func loadTemplates() (map[string]*template.Template, error) {
	pageDirs := []string{"dashboard", "repo", "conversation"}
	tmpls := make(map[string]*template.Template, len(pageDirs))

	for _, dir := range pageDirs {
		t := template.New("")
		t.Funcs(funcMap)

		// Parse layout (pages/*.html only, not subdirs), shared hx
		// fragments, and this page's specific templates.
		dirs := []string{"pages", "hx", "pages/" + dir}
		for _, d := range dirs {
			if err := fs.WalkDir(contentFS, d, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				// For "pages", only parse top-level files (e.g. base.html),
				// skip subdirectories to avoid loading other pages' templates.
				if entry.IsDir() {
					if d == "pages" && path != "pages" {
						return fs.SkipDir
					}
					return nil
				}
				if filepath.Ext(path) != ".html" {
					return nil
				}
				data, readErr := fs.ReadFile(contentFS, path)
				if readErr != nil {
					return readErr
				}
				template.Must(t.New(path).Parse(string(data)))
				return nil
			}); err != nil {
				return nil, fmt.Errorf("parsing %s templates: %w", d, err)
			}
		}

		tmpls[dir] = t
	}

	return tmpls, nil
}
