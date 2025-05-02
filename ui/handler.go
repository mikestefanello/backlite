package ui

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/mikestefanello/backlite/internal/task"
)

type (
	// Handler handles HTTP requests for the Backlite UI.
	Handler struct {
		// cfg stores the UI configuration.
		cfg Config
	}

	// Config contains configuration for the Handler.
	Config struct {
		// DB is the Backlite database.
		DB *sql.DB

		// BasePath is an optional base path to prepend to all URL paths.
		BasePath string

		// ItemsPerPage is the maximum amount of items to display per page. This defaults to 25.
		ItemsPerPage int

		// ReleaseAfter is the duration after which a task is released back to a queue if it has not finished executing.
		// Ths defaults to one hour, but it should match the value you use in your Backlite client.
		ReleaseAfter time.Duration
	}

	// templateData is a wrapper of data sent to templates for rendering.
	templateData struct {
		// Path is the current request URL path, excluding the base path.
		Path string

		// BasePath is the configured base path.
		BasePath string

		// Content is the data to render.
		Content any

		// Page is the page number.
		Page int
	}

	// handleFunc is an HTTP handle func that returns an error.
	handleFunc func(http.ResponseWriter, *http.Request) error
)

// NewHandler creates a new handler for the Backlite web UI.
func NewHandler(config Config) (*Handler, error) {
	if config.ReleaseAfter == 0 {
		config.ReleaseAfter = time.Hour
	}

	if config.ItemsPerPage == 0 {
		config.ItemsPerPage = 25
	}

	if config.BasePath != "" {
		switch {
		case strings.HasSuffix(config.BasePath, "/"):
			return nil, errors.New("base path must not end with /")
		case !strings.HasPrefix(config.BasePath, "/"):
			return nil, errors.New("base path must start with /")
		}
	}

	switch {
	case config.DB == nil:
		return nil, errors.New("db is required")
	case config.ItemsPerPage <= 0:
		return nil, errors.New("items per page must be greater than zero")
	case config.ReleaseAfter < 0:
		return nil, errors.New("release after must be greater than zero")
	}

	return &Handler{cfg: config}, nil
}

// Register registers all available routes.
func (h *Handler) Register(mux *http.ServeMux) *http.ServeMux {
	path := func(p string) string {
		if h.cfg.BasePath != "" && p == "/" {
			p = ""
		}
		return fmt.Sprintf("GET %s%s", h.cfg.BasePath, p)
	}
	mux.HandleFunc(path("/"), handle(h.Running))
	mux.HandleFunc(path("/upcoming"), handle(h.Upcoming))
	mux.HandleFunc(path("/succeeded"), handle(h.Succeeded))
	mux.HandleFunc(path("/failed"), handle(h.Failed))
	mux.HandleFunc(path("/task/{task}"), handle(h.Task))
	mux.HandleFunc(path("/completed/{task}"), handle(h.TaskCompleted))
	return mux
}

func handle(hf handleFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := hf(w, r); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
			log.Println(err)
		}
	}
}

// Running renders the running tasks.
func (h *Handler) Running(w http.ResponseWriter, req *http.Request) error {
	tasks, err := task.GetTasks(
		req.Context(),
		h.cfg.DB,
		selectRunningTasks,
		h.cfg.ItemsPerPage,
		h.getOffset(req.URL),
	)
	if err != nil {
		return err
	}

	return h.render(req, w, tmplTasksRunning, tasks)
}

// Upcoming renders the upcoming tasks.
func (h *Handler) Upcoming(w http.ResponseWriter, req *http.Request) error {
	tasks, err := task.GetScheduledTasksWithOffset(
		req.Context(),
		h.cfg.DB,
		time.Now().Add(-h.cfg.ReleaseAfter),
		h.cfg.ItemsPerPage,
		h.getOffset(req.URL),
	)
	if err != nil {
		return err
	}

	return h.render(req, w, tmplTasksUpcoming, tasks)
}

// Succeeded renders the completed tasks that have succeeded.
func (h *Handler) Succeeded(w http.ResponseWriter, req *http.Request) error {
	tasks, err := task.GetCompletedTasks(
		req.Context(),
		h.cfg.DB,
		selectCompletedTasks,
		1,
		h.cfg.ItemsPerPage,
		h.getOffset(req.URL),
	)
	if err != nil {
		return err
	}

	return h.render(req, w, tmplTasksCompleted, tasks)
}

// Failed renders the completed tasks that have failed.
func (h *Handler) Failed(w http.ResponseWriter, req *http.Request) error {
	tasks, err := task.GetCompletedTasks(
		req.Context(),
		h.cfg.DB,
		selectCompletedTasks,
		0,
		h.cfg.ItemsPerPage,
		h.getOffset(req.URL),
	)
	if err != nil {
		return err
	}

	return h.render(req, w, tmplTasksCompleted, tasks)
}

// Task renders a task.
func (h *Handler) Task(w http.ResponseWriter, req *http.Request) error {
	id := req.PathValue("task")
	tasks, err := task.GetTasks(req.Context(), h.cfg.DB, selectTask, id)
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
	tasks, err := task.GetCompletedTasks(req.Context(), h.cfg.DB, selectCompletedTask, id)
	if err != nil {
		return err
	}

	if len(tasks) > 0 {
		t = tasks[0]
	}

	return h.render(req, w, tmplTaskCompleted, t)
}

func (h *Handler) render(req *http.Request, w io.Writer, tmpl *template.Template, data any) error {
	path, _ := strings.CutPrefix(req.URL.Path, h.cfg.BasePath)
	if path == "" {
		path = "/"
	}
	return tmpl.ExecuteTemplate(w, "layout.gohtml", templateData{
		Path:     path,
		BasePath: h.cfg.BasePath,
		Content:  data,
		Page:     getPage(req.URL),
	})
}

func (h *Handler) getOffset(u *url.URL) int {
	return (getPage(u) - 1) * h.cfg.ItemsPerPage
}

func getPage(u *url.URL) int {
	if p := u.Query().Get("page"); p != "" {
		if page, err := strconv.Atoi(p); err == nil {
			if page > 0 {
				return page
			}
		}
	}
	return 1
}

// FullPath outputs a given path in full, including the base path.
func (t templateData) FullPath(path string) string {
	if t.BasePath != "" && path == "/" {
		path = ""
	}
	return fmt.Sprintf("%s%s", t.BasePath, path)
}
