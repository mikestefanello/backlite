package ui

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"text/template"
	"time"

	"github.com/mikestefanello/backlite"

	"github.com/mikestefanello/backlite/internal/task"
)

type (
	Handler struct {
		db  *sql.DB
		cli backlite.Client
	}

	TemplateData struct {
		Path    string
		Content any
	}
)

func NewHandler(db *sql.DB) *http.ServeMux {
	h := &Handler{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", h.Running)
	mux.HandleFunc("GET /upcoming", h.Upcoming)
	mux.HandleFunc("GET /succeeded", h.Succeeded)
	mux.HandleFunc("GET /failed", h.Failed)
	return mux
}

func (h *Handler) Running(w http.ResponseWriter, req *http.Request) {
	if err := h.running(w, req); err != nil {
		h.error(w, err)
	}
}

func (h *Handler) Upcoming(w http.ResponseWriter, req *http.Request) {
	if err := h.upcoming(w, req); err != nil {
		h.error(w, err)
	}
}

func (h *Handler) Succeeded(w http.ResponseWriter, req *http.Request) {
	if err := h.succeeded(w, req); err != nil {
		h.error(w, err)
	}
}

func (h *Handler) Failed(w http.ResponseWriter, req *http.Request) {
	if err := h.failed(w, req); err != nil {
		h.error(w, err)
	}
}

func (h *Handler) running(w http.ResponseWriter, req *http.Request) error {
	tasks, err := task.GetTasks(req.Context(), h.db, selectRunningTasks, itemLimit)
	if err != nil {
		return err
	}

	return h.render(req, w, tmplTasks, tasks)
}

func (h *Handler) upcoming(w http.ResponseWriter, req *http.Request) error {
	// TODO use actual time from the client
	tasks, err := task.GetScheduledTasks(req.Context(), h.db, time.Now().Add(time.Hour), itemLimit)
	if err != nil {
		return err
	}

	return h.render(req, w, tmplTasks, tasks)
}

func (h *Handler) succeeded(w http.ResponseWriter, req *http.Request) error {
	tasks, err := task.GetCompletedTasks(req.Context(), h.db, selectCompletedTasks, 1, itemLimit)
	if err != nil {
		return err
	}

	return h.render(req, w, tmplCompletedTasks, tasks)
}

func (h *Handler) failed(w http.ResponseWriter, req *http.Request) error {
	tasks, err := task.GetCompletedTasks(req.Context(), h.db, selectCompletedTasks, 0, itemLimit)
	if err != nil {
		return err
	}

	return h.render(req, w, tmplCompletedTasks, tasks)
}

func (h *Handler) error(w http.ResponseWriter, err error) {
	fmt.Fprintf(w, err.Error())
	log.Println(err)
	w.WriteHeader(http.StatusInternalServerError)
}

func (h *Handler) render(req *http.Request, w io.Writer, tmpl *template.Template, data any) error {
	return tmpl.ExecuteTemplate(w, "layout.gohtml", TemplateData{
		Path:    req.URL.Path,
		Content: data,
	})
}
