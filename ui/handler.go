package ui

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"text/template"
	"time"

	"github.com/mikestefanello/backlite/internal/task"
)

// itemLimit is the limit of items to fetch from the database.
// TODO allow this to be configurable via the UI.
const itemLimit = 25

type (
	// Handler handles HTTP requests for the backlite UI.
	Handler struct {
		// db stores the backlite database.
		db *sql.DB
	}

	// templateData is a wrapper of data sent to templates for rendering.
	templateData struct {
		// Path is the current request URL path.
		Path string

		// Content is the data to render.
		Content any
	}

	// handleFunc is an HTTP handle func that returns an error.
	handleFunc func(http.ResponseWriter, *http.Request) error
)

// NewHandler creates a new handler for the Backlite web UI.
func NewHandler(db *sql.DB) *Handler {
	return &Handler{db: db}
}

// Register registers all available routes.
func (h *Handler) Register(mux *http.ServeMux) *http.ServeMux {
	mux.HandleFunc("GET /", handle(h.Running))
	mux.HandleFunc("GET /upcoming", handle(h.Upcoming))
	mux.HandleFunc("GET /succeeded", handle(h.Succeeded))
	mux.HandleFunc("GET /failed", handle(h.Failed))
	mux.HandleFunc("GET /task/{task}", handle(h.Task))
	mux.HandleFunc("GET /completed/{task}", handle(h.TaskCompleted))
	return mux
}

func handle(hf handleFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := hf(w, r); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
			log.Println(err)
			return
		}
	}
}

// Running renders the running tasks.
func (h *Handler) Running(w http.ResponseWriter, req *http.Request) error {
	tasks, err := task.GetTasks(req.Context(), h.db, selectRunningTasks, itemLimit)
	if err != nil {
		return err
	}

	return h.render(req, w, tmplTasksRunning, tasks)
}

// Upcoming renders the upcoming tasks.
func (h *Handler) Upcoming(w http.ResponseWriter, req *http.Request) error {
	// TODO use actual time from the client
	tasks, err := task.GetScheduledTasks(req.Context(), h.db, time.Now().Add(time.Hour), itemLimit)
	if err != nil {
		return err
	}

	return h.render(req, w, tmplTasksUpcoming, tasks)
}

// Succeeded renders the completed tasks that have succeeded.
func (h *Handler) Succeeded(w http.ResponseWriter, req *http.Request) error {
	tasks, err := task.GetCompletedTasks(req.Context(), h.db, selectCompletedTasks, 1, itemLimit)
	if err != nil {
		return err
	}

	return h.render(req, w, tmplTasksCompleted, tasks)
}

// Failed renders the completed tasks that have failed.
func (h *Handler) Failed(w http.ResponseWriter, req *http.Request) error {
	tasks, err := task.GetCompletedTasks(req.Context(), h.db, selectCompletedTasks, 0, itemLimit)
	if err != nil {
		return err
	}

	return h.render(req, w, tmplTasksCompleted, tasks)
}

// Task renders a task.
func (h *Handler) Task(w http.ResponseWriter, req *http.Request) error {
	id := req.PathValue("task")
	tasks, err := task.GetTasks(req.Context(), h.db, selectTask, id)
	if err != nil {
		return err
	}

	if len(tasks) > 0 {
		return h.render(req, w, tmplTask, tasks[0])
	}

	// If no task found, try the same ID as a completed task.
	return h.TaskCompleted(w, req)
}

// TaskCompleted renders a completed task.
func (h *Handler) TaskCompleted(w http.ResponseWriter, req *http.Request) error {
	var t *task.Completed
	id := req.PathValue("task")
	tasks, err := task.GetCompletedTasks(req.Context(), h.db, selectCompletedTask, id)
	if err != nil {
		return err
	}

	if len(tasks) > 0 {
		t = tasks[0]
	}

	return h.render(req, w, tmplTaskCompleted, t)
}

func (h *Handler) render(req *http.Request, w io.Writer, tmpl *template.Template, data any) error {
	return tmpl.ExecuteTemplate(w, "layout.gohtml", templateData{
		Path:    req.URL.Path,
		Content: data,
	})
}
