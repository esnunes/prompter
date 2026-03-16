package hx

import (
	"html/template"

	"github.com/esnunes/prompter/internal/db"
)

type Handler struct {
	tmpl          *template.Template
	queries       *db.Queries
	getRepoStatus func(int64) string
}

func New(tmpl *template.Template, queries *db.Queries, getRepoStatus func(int64) string) *Handler {
	return &Handler{tmpl: tmpl, queries: queries, getRepoStatus: getRepoStatus}
}
