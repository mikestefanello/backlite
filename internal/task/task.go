package task

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/google/uuid"
	"github.com/mikestefanello/backlite/internal/query"
	"time"
)

type Task struct {
	ID             string
	Queue          string
	Task           []byte
	Attempts       int
	WaitUntil      *time.Time
	CreatedAt      time.Time
	LastExecutedAt *time.Time
}

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

func (t *Task) DeleteTx(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, query.DeleteTask, t.ID)
	return err
}

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
