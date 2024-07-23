package ui

import (
	"embed"
	"fmt"
	"text/template"
)

//go:embed templates/*.gohtml
var templates embed.FS

var (
	tmplTasks          = mustParse("tasks")
	tmplCompletedTasks = mustParse("completed_tasks")
)

func mustParse(page string) *template.Template {
	t, err := template.ParseFS(
		templates,
		"templates/layout.gohtml",
		fmt.Sprintf("templates/%s.gohtml", page),
	)
	if err != nil {
		panic(err)
	}
	return t
}
