package task

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mikestefanello/backlite/internal/query"
)

// Task is a task that is queued for execution.
type Task struct {
	// ID is the Task ID.
	ID string

	// Queue is the name of the queue this Task belongs to.
	Queue string

	// Task is the task data.
	Task []byte

	// Attempts are the amount of times this Task was executed.
	Attempts int

	// WaitUntil is the time the task should not be executed until.
	WaitUntil *time.Time

	// CreatedAt is when the Task was originally created.
	CreatedAt time.Time

	// LastExecutedAt is the last time this Task executed.
	LastExecutedAt *time.Time

	// ClaimedAt is the time this Task was claimed for execution.
	ClaimedAt *time.Time
}

// InsertTx inserts a task as part of a database transaction.
func (t *Task) InsertTx(ctx context.Context, tx *sql.Tx) error {
	if len(t.ID) == 0 {
		// UUID is used because it's faster and more reliable than having the DB generate a random string.
		// And since it's time-sortable, we avoid needing a separate index on the created time.
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("unable to generate task ID: %w", err)
		}
		t.ID = id.String()
	}

	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}

	var wait *int64
	if t.WaitUntil != nil {
		v := t.WaitUntil.UnixMilli()
		wait = &v
	}

	_, err := tx.ExecContext(
		ctx,
		query.InsertTask,
		t.ID,
		t.CreatedAt.UnixMilli(),
		t.Queue,
		t.Task,
		wait,
	)

	return err
}

// DeleteTx deletes a task as part of a database transaction.
func (t *Task) DeleteTx(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, query.DeleteTask, t.ID)
	return err
}

// Fail marks a task as failed in the database and queues it to be executed again.
func (t *Task) Fail(ctx context.Context, db *sql.DB, waitUntil time.Time) error {
	_, err := db.ExecContext(
		ctx,
		query.TaskFailed,
		waitUntil.UnixMilli(),
		t.LastExecutedAt.UnixMilli(),
		t.ID,
	)
	return err
}
