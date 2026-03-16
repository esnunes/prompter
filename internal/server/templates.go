// internal/server/templates.go
package server

import (
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

func loadTemplates() (*template.Template, error) {
	t := template.New("").Funcs(funcMap)

	dirs := []string{"pages", "pages/dashboard", "pages/repo", "pages/conversation", "hx"}
	for _, dir := range dirs {
		err := fs.WalkDir(contentFS, dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(path) != ".html" {
				return err
			}
			data, readErr := fs.ReadFile(contentFS, path)
			if readErr != nil {
				return readErr
			}
			template.Must(t.New(path).Parse(string(data)))
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return t, nil
}
