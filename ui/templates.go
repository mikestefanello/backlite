package ui

import (
	"embed"
	"fmt"
	"text/template"
	"time"
)

//go:embed templates/*.gohtml
var templates embed.FS

var (
	tmplTasksRunning   = mustParse("running")
	tmplTasksUpcoming  = mustParse("upcoming")
	tmplTasksCompleted = mustParse("completed_tasks")
	tmplTask           = mustParse("task")
	tmplTaskCompleted  = mustParse("completed_task")
)

func mustParse(page string) *template.Template {
	t, err := template.
		New("layout.gohtml").
		Funcs(
			template.FuncMap{
				"bytestring": bytestring,
				"datetime":   datetime,
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

func datetime(t time.Time) string {
	return t.Local().Format("02 Jan 2006 15:04:05 MST")
}
