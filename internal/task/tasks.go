package task

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/mikestefanello/backlite/internal/query"
)

// Tasks are a slice of tasks.
type Tasks []*Task

// Claim updates a Task in the database to indicate that it has been claimed by a processor to be executed.
func (t Tasks) Claim(ctx context.Context, db *sql.DB) error {
	if len(t) == 0 {
		return nil
	}

	params := make([]any, 0, len(t)+1)
	params = append(params, time.Now().UnixMilli())

	for _, task := range t {
		params = append(params, task.ID)
	}

	_, err := db.ExecContext(
		ctx,
		query.ClaimTasks(len(t)),
		params...,
	)

	slog.Info("claimed tasks", "len", len(t)) // TODO remove
	return err
}

// GetTasks loads tasks from the database using a given query and arguments.
func GetTasks(ctx context.Context, db *sql.DB, query string, args ...any) (Tasks, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	tasks := make(Tasks, 0)

	for rows.Next() {
		var task Task
		var createdAt int64
		var waitUntil *int64

		err = rows.Scan(
			&task.ID,
			&task.Queue,
			&task.Task,
			&task.Attempts,
			&waitUntil,
			&createdAt,
		)

		if err != nil {
			return nil, err
		}

		task.CreatedAt = time.UnixMilli(createdAt)
		if waitUntil != nil {
			v := time.UnixMilli(*waitUntil)
			task.WaitUntil = &v
		}

		tasks = append(tasks, &task)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

// GetScheduledTasks loads the tasks that are next up to be executed in order of execution time.
// It's important to note that this does not filter out tasks that are not yet ready based on their wait time.
// The deadline provided is used to include tasks that have been claimed if that given amount of time has elapsed.
func GetScheduledTasks(ctx context.Context, db *sql.DB, deadline time.Time, limit int) (Tasks, error) {
	return GetTasks(
		ctx,
		db,
		query.SelectScheduledTasks,
		deadline.UnixMilli(),
		limit,
	)
}
