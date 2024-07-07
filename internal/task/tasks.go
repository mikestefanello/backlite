package task

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/mikestefanello/backlite/internal/query"
)

type Tasks []*Task

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

func GetScheduledTasks(ctx context.Context, db *sql.DB, deadline time.Time, limit int) (Tasks, error) {
	return GetTasks(
		ctx,
		db,
		query.SelectScheduledTasks,
		deadline.UnixMilli(),
		limit,
	)
}
