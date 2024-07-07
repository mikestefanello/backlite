package task

import (
	"context"
	"database/sql"
	"github.com/mikestefanello/backlite/internal/query"
	"time"
)

// Completed is a completed task.
type Completed struct {
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

// DeleteExpiredCompleted deletes completed tasks that have an expiration date in the past.
func DeleteExpiredCompleted(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(
		ctx,
		query.DeleteExpiredCompletedTasks,
		time.Now().UnixMilli(),
	)
	return err
}
