package backlite

import (
	"context"
	"database/sql"
	"time"
)

type (
	// Task represents a task that will be placed in to a queue for execution.
	Task interface {
		// Config returns the configuration options for the queue that this Task will be placed in.
		Config() QueueConfig
	}

	// TaskAddOp facilitates adding Tasks to the queue.
	TaskAddOp struct {
		client *Client
		ctx    context.Context
		tasks  []Task
		wait   *time.Time
		tx     *sql.Tx
	}
)

// Ctx sets the request context.
func (t *TaskAddOp) Ctx(ctx context.Context) *TaskAddOp {
	t.ctx = ctx
	return t
}

// At sets the time the task should not be executed until.
func (t *TaskAddOp) At(processAt time.Time) *TaskAddOp {
	t.wait = &processAt
	return t
}

// Wait instructs the task to wait a given duration before it is executed.
func (t *TaskAddOp) Wait(duration time.Duration) *TaskAddOp {
	t.At(time.Now().Add(duration))
	return t
}

// Tx will include the task as part of a given database transaction.
// When using this, it is critical that after you commit the transaction that you call Notify() on the
// client so the dispatcher is aware that a new task has been created, otherwise it may not be executed.
// This is necessary because there is, unfortunately, no way for outsiders to know if or when a transaction
// is committed and since the dispatcher avoids continuous polling, it needs to know when tasks are added.
func (t *TaskAddOp) Tx(tx *sql.Tx) *TaskAddOp {
	t.tx = tx
	return t
}

// Save saves the task, so it can be queued for execution.
func (t *TaskAddOp) Save() error {
	return t.client.save(t)
}
