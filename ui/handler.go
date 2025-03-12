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
)

func NewHandler(db *sql.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) Register(mux *http.ServeMux) *http.ServeMux {
	mux.HandleFunc("GET /", h.Running)
	mux.HandleFunc("GET /upcoming", h.Upcoming)
	mux.HandleFunc("GET /succeeded", h.Succeeded)
	mux.HandleFunc("GET /failed", h.Failed)
	mux.HandleFunc("GET /task/{task}", h.Task)
	mux.HandleFunc("GET /completed/{task}", h.TaskCompleted)
	return mux
}

func (h *Handler) Running(w http.ResponseWriter, req *http.Request) {
	err := func() error {
		tasks, err := task.GetTasks(req.Context(), h.db, selectRunningTasks, itemLimit)
		if err != nil {
			return err
		}

		return h.render(req, w, tmplTasksRunning, tasks)
	}()

	if err != nil {
		h.error(w, err)
	}
}

func (h *Handler) Upcoming(w http.ResponseWriter, req *http.Request) {
	err := func() error {
		// TODO use actual time from the client
		tasks, err := task.GetScheduledTasks(req.Context(), h.db, time.Now().Add(time.Hour), itemLimit)
		if err != nil {
			return err
		}

		return h.render(req, w, tmplTasksUpcoming, tasks)
	}()

	if err != nil {
		h.error(w, err)
	}
}

func (h *Handler) Succeeded(w http.ResponseWriter, req *http.Request) {
	err := func() error {
		tasks, err := task.GetCompletedTasks(req.Context(), h.db, selectCompletedTasks, 1, itemLimit)
		if err != nil {
			return err
		}

		return h.render(req, w, tmplTasksCompleted, tasks)
	}()

	if err != nil {
		h.error(w, err)
	}
}

func (h *Handler) Failed(w http.ResponseWriter, req *http.Request) {
	err := func() error {
		tasks, err := task.GetCompletedTasks(req.Context(), h.db, selectCompletedTasks, 0, itemLimit)
		if err != nil {
			return err
		}

		return h.render(req, w, tmplTasksCompleted, tasks)
	}()

	if err != nil {
		h.error(w, err)
	}
}

func (h *Handler) Task(w http.ResponseWriter, req *http.Request) {
	err := func() error {
		id := req.PathValue("task")
		tasks, err := task.GetTasks(req.Context(), h.db, selectTask, id)
		if err != nil {
			return err
		}

		if len(tasks) > 0 {
			return h.render(req, w, tmplTask, tasks[0])
		}

		// If no task found, try the same ID as a completed task.
		h.TaskCompleted(w, req)
		return nil
	}()

	if err != nil {
		h.error(w, err)
	}
}

func (h *Handler) TaskCompleted(w http.ResponseWriter, req *http.Request) {
	err := func() error {
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
	}()

	if err != nil {
		h.error(w, err)
	}
}

func (h *Handler) error(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprint(w, err)
	log.Println(err)
}

func (h *Handler) render(req *http.Request, w io.Writer, tmpl *template.Template, data any) error {
	return tmpl.ExecuteTemplate(w, "layout.gohtml", templateData{
		Path:    req.URL.Path,
		Content: data,
	})
}
