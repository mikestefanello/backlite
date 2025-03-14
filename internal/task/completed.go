package task

import (
	"context"
	"database/sql"
	"time"

	"github.com/mikestefanello/backlite/internal/query"
)

type (
	// Completed is a completed task.
	Completed struct {
		// ID is the Task ID
		ID string

		// Queue is the name of the queue this Task belongs to.
		Queue string

		// Task is the task data.
		Task []byte

		// Attempts are the amount of times this Task was executed.
		Attempts int

		// Succeeded indicates if the Task execution was a success.
		Succeeded bool

		// LastDuration is the last execution duration.
		LastDuration time.Duration

		// ExpiresAt is when this record should be removed from the database.
		// If omitted, the record should not be removed.
		ExpiresAt *time.Time

		// CreatedAt is when the Task was originally created.
		CreatedAt time.Time

		// LastExecutedAt is the last time this Task executed.
		LastExecutedAt time.Time

		// Error is the error message provided by the Task processor.
		Error *string
	}

	// CompletedTasks contains multiple completed tasks.
	CompletedTasks []*Completed
)

// InsertTx inserts a completed task as part of a database transaction.
func (c *Completed) InsertTx(ctx context.Context, tx *sql.Tx) error {
	var expiresAt *int64
	if c.ExpiresAt != nil {
		v := c.ExpiresAt.UnixMilli()
		expiresAt = &v
	}

	_, err := tx.ExecContext(
		ctx,
		query.InsertCompletedTask,
		c.ID,
		c.CreatedAt.UnixMilli(),
		c.Queue,
		c.LastExecutedAt.UnixMilli(),
		c.Attempts,
		c.LastDuration.Microseconds(),
		c.Succeeded,
		c.Task,
		expiresAt,
		c.Error,
	)
	return err
}

// GetCompletedTasks loads completed tasks from the database using a given query and arguments.
func GetCompletedTasks(ctx context.Context, db *sql.DB, query string, args ...any) (CompletedTasks, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	tasks := make(CompletedTasks, 0)

	for rows.Next() {
		var task Completed
		var lastExecutedAt, createdAt int64
		var expiresAt *int64

		err = rows.Scan(
			&task.ID,
			&createdAt,
			&task.Queue,
			&lastExecutedAt,
			&task.Attempts,
			&task.LastDuration,
			&task.Succeeded,
			&task.Task,
			&expiresAt,
			&task.Error,
		)

		if err != nil {
			return nil, err
		}

		task.LastExecutedAt = time.UnixMilli(lastExecutedAt)
		task.CreatedAt = time.UnixMilli(createdAt)
		task.LastDuration *= 1000

		if expiresAt != nil {
			v := time.UnixMilli(*expiresAt)
			task.ExpiresAt = &v
		}

		tasks = append(tasks, &task)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

// DeleteExpiredCompleted deletes completed tasks that have an expiration date in the past.
func DeleteExpiredCompleted(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(
		ctx,
		query.DeleteExpiredCompletedTasks,
		time.Now().UnixMilli(),
	)
	return err
}
