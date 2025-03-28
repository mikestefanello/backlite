package task

import (
	"context"
	"database/sql"
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

	toTime := func(ms *int64) *time.Time {
		if ms == nil {
			return nil
		}
		v := time.UnixMilli(*ms)
		return &v
	}

	for rows.Next() {
		var task Task
		var createdAt int64
		var waitUntil, lastExecutedAt, claimedAt *int64

		err = rows.Scan(
			&task.ID,
			&task.Queue,
			&task.Task,
			&task.Attempts,
			&waitUntil,
			&createdAt,
			&lastExecutedAt,
			&claimedAt,
		)

		if err != nil {
			return nil, err
		}

		task.CreatedAt = time.UnixMilli(createdAt)
		task.WaitUntil = toTime(waitUntil)
		task.LastExecutedAt = toTime(lastExecutedAt)
		task.ClaimedAt = toTime(claimedAt)

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
	return GetScheduledTasksWithOffset(
		ctx,
		db,
		deadline,
		limit,
		0,
	)
}

// GetScheduledTasksWithOffset is the same as GetScheduledTasks but with an offset for paging.
func GetScheduledTasksWithOffset(
	ctx context.Context,
	db *sql.DB,
	deadline time.Time,
	limit,
	offset int) (Tasks, error) {
	return GetTasks(
		ctx,
		db,
		query.SelectScheduledTasks,
		deadline.UnixMilli(),
		limit,
		offset,
	)
}
