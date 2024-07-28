package ui

import (
	"embed"
	"fmt"
	"text/template"
)

//go:embed templates/*.gohtml
var templates embed.FS

var (
	tmplTasksRunning   = mustParse("running")
	tmplTasksUpcoming  = mustParse("upcoming")
	tmplTasksCompleted = mustParse("completed_tasks")
	tmplTask           = mustParse("task")
)

func mustParse(page string) *template.Template {
	t, err := template.
		New("layout.gohtml").
		Funcs(
			template.FuncMap{
				"bytestring": bytestring,
			}).
		ParseFS(
			templates,
			"templates/layout.gohtml",
			fmt.Sprintf("templates/%s.gohtml", page),
		)

	if err != nil {
		panic(err)
	}
	return t
}

func bytestring(b []byte) string {
	return string(b)
}
