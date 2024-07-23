package backlite

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/mikestefanello/backlite/internal/task"
)

type (
	// Dispatcher handles automatically pulling queued tasks and executing them via queue processors.
	Dispatcher interface {
		// Start starts the dispatcher.
		Start(context.Context)

		// Stop stops the dispatcher.
		Stop(context.Context) bool

		// Notify notifies the dispatcher that a new task has been added.
		Notify()
	}

	// dispatcher implements Dispatcher.
	dispatcher struct {
		// client is the Client that this dispatcher belongs to.
		client *Client

		// log is the logger.
		log Logger

		// ctx stores the context used to start the dispatcher.
		ctx context.Context

		// shutdownCtx stores an internal context that is used when attempting to gracefully shut down the dispatcher.
		shutdownCtx context.Context

		// shutdown is the cancel function for cancelling shutdownCtx.
		shutdown context.CancelFunc

		// numWorkers is the amount of goroutines opened to execute tasks.
		numWorkers int

		// releaseAfter is the duration to reclaim a task for execution if it has not completed.
		releaseAfter time.Duration

		// CleanupInterval is how often to run cleanup operations on the database in order to remove expired completed
		// tasks.
		cleanupInterval time.Duration

		// running indicates if the dispatching is currently running.
		running atomic.Bool

		// ticker will fetch tasks from the database if the next task is delayed.
		ticker *time.Ticker

		// tasks transmits tasks to the workers.
		tasks chan *task.Task

		// availableWorkers tracks the amount of workers available to receive a task to execute.
		availableWorkers chan struct{}

		// ready tells the dispatcher that fetching tasks from the database is required.
		ready chan struct{}

		// trigger instructs the dispatcher to fetch tasks from the database now.
		trigger chan struct{}

		// triggered indicates that a trigger was sent but not yet received.
		// This is used to allow multiple calls to ready, which will happen whenever a task is added,
		// but only 1 database fetch since that is all that is needed for the dispatcher to be aware of the
		// current state of the queues.
		triggered atomic.Bool
	}
)

// Start starts the dispatcher.
// To hard-stop, cancel the provided context. To gracefully stop, call stop().
func (d *dispatcher) Start(ctx context.Context) {
	// Abort if the dispatcher is already running.
	if d.running.Load() {
		return
	}

	d.ctx = ctx
	d.shutdownCtx, d.shutdown = context.WithCancel(context.Background())
	d.tasks = make(chan *task.Task, d.numWorkers)
	d.ticker = time.NewTicker(time.Second)
	d.ticker.Stop()                     // No need to tick yet
	d.ready = make(chan struct{}, 1000) // Prevent blocking task creation
	d.trigger = make(chan struct{}, 10) // Should never need more than 1 but just in case
	d.availableWorkers = make(chan struct{}, d.numWorkers)
	d.running.Store(true)

	for range d.numWorkers {
		go d.worker()
		d.availableWorkers <- struct{}{}
	}

	if d.cleanupInterval > 0 {
		go d.cleaner()
	}

	go d.triggerer()
	go d.fetcher()

	d.ready <- struct{}{}

	d.log.Info("task dispatcher started")
}

// Stop attempts to gracefully shut down the dispatcher by blocking until either the context is cancelled or all
// workers are done with their task. If all workers are able to complete, true will be returned.
func (d *dispatcher) Stop(ctx context.Context) bool {
	if !d.running.Load() {
		return true
	}

	// Call the internal shutdown to gracefully close all goroutines.
	d.shutdown()

	var count int

	for {
		select {
		case <-ctx.Done():
			return false

		case <-d.availableWorkers:
			count++

			if count == d.numWorkers {
				return true
			}
		}
	}
}

// triggerer listens to the ready channel and sends a trigger to the fetcher only when it is needed which is
// controlled by the triggered lock. This allows the dispatcher to track database fetches and when one is made,
// it can account for all incoming tasks that sent a signal to the ready channel before it, rather than fetching
// from the database every single time a new task is added.
func (d *dispatcher) triggerer() {
	for {
		select {
		case <-d.ready:
			if d.triggered.CompareAndSwap(false, true) {
				d.trigger <- struct{}{}
			}

		case <-d.shutdownCtx.Done():
			return

		case <-d.ctx.Done():
			return
		}
	}
}

// fetcher fetches tasks from the database to be executed either when the ticker ticks or when the trigger signal
// is sent by the triggerer.
func (d *dispatcher) fetcher() {
	defer func() {
		d.running.Store(false)
		d.ticker.Stop()
		close(d.tasks)
		d.log.Info("shutting down dispatcher")
	}()

	for {
		select {
		case <-d.ticker.C:
			d.ticker.Stop()
			d.fetch()

		case <-d.trigger:
			d.fetch()

		case <-d.shutdownCtx.Done():
			return

		case <-d.ctx.Done():
			return
		}
	}
}

// worker processes incoming tasks.
func (d *dispatcher) worker() {
	for {
		select {
		case row := <-d.tasks:
			if row == nil {
				break
			}
			d.processTask(row)
			d.availableWorkers <- struct{}{}

		case <-d.shutdownCtx.Done():
			return

		case <-d.ctx.Done():
			return
		}
	}
}

// cleaner periodically deletes expired completed tasks from the database.
func (d *dispatcher) cleaner() {
	ticker := time.NewTicker(d.cleanupInterval)

	for {
		select {
		case <-ticker.C:
			if err := task.DeleteExpiredCompleted(d.ctx, d.client.db); err != nil {
				d.log.Error("failed to delete expired completed tasks",
					"error", err,
				)
			}

		case <-d.shutdownCtx.Done():
			return

		case <-d.ctx.Done():
			ticker.Stop()
			return
		}
	}
}

// waitForWorkers waits until at least one worker is available to execute a task and returns the number that are
// available.
func (d *dispatcher) waitForWorkers() int {
	for {
		if w := len(d.availableWorkers); w > 0 {
			return w
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// fetch fetches tasks from the database to be executed and/or coordinate the dispatcher, so it is aware of when it
// needs to fetch again.
func (d *dispatcher) fetch() {
	var err error

	// If we failed at any point, we need to tell the dispatcher to try again.
	defer func() {
		if err != nil {
			// Wait and try again.
			time.Sleep(100 * time.Millisecond)
			d.ready <- struct{}{}
		}
	}()

	// Indicate that incoming task additions from this point on should trigger another fetch.
	d.triggered.Store(false)

	// Determine how many workers are available, so we only fetch that many tasks.
	workers := d.waitForWorkers()

	// Fetch tasks for each available worker plus the next upcoming task so the scheduler knows when to
	// query the database again without having to continually poll.
	tasks, err := task.GetScheduledTasks(
		d.ctx,
		d.client.db,
		now().Add(-d.releaseAfter),
		int(workers)+1,
	)

	if err != nil {
		d.log.Error("fetch tasks query failed",
			"error", err,
		)
		return
	}

	var next *task.Task
	nextUp := func(i int) {
		next = tasks[i]
		tasks = tasks[:i]
	}

	for i := range tasks {
		// Check if the workers are full.
		if (i + 1) > workers {
			nextUp(i)
			break
		}

		// Check if this task is not ready yet.
		if tasks[i].WaitUntil != nil {
			if tasks[i].WaitUntil.After(now()) {
				nextUp(i)
				break
			}
		}
	}

	// Claim the tasks that are ready to be processed.
	if err = tasks.Claim(d.ctx, d.client.db); err != nil {
		d.log.Error("failed to claim tasks",
			"error", err,
		)
		return
	}

	// Send the ready tasks to the workers.
	for i := range tasks {
		tasks[i].Attempts++
		<-d.availableWorkers
		d.tasks <- tasks[i]
	}

	// Adjust the schedule based on the next up task.
	d.schedule(next)
}

// schedule handles scheduling the dispatcher based on the next up task provided by the fetcher.
func (d *dispatcher) schedule(t *task.Task) {
	d.ticker.Stop()

	if t != nil {
		if t.WaitUntil == nil {
			d.ready <- struct{}{}
			return
		}

		dur := t.WaitUntil.Sub(now())
		if dur < 0 {
			d.ready <- struct{}{}
			return
		}

		d.ticker.Reset(dur)
	}
}

// processTask attempts to execute a given task.
func (d *dispatcher) processTask(t *task.Task) {
	var err error
	var ctx context.Context
	var cancel context.CancelFunc

	q := d.client.queues.get(t.Queue)
	cfg := q.Config()

	// Set a context timeout, if desired.
	if cfg.Timeout > 0 {
		ctx, cancel = context.WithDeadline(d.ctx, now().Add(cfg.Timeout))
		defer cancel()
	} else {
		ctx = d.ctx
	}

	// Store the client in the context so the processor can use it.
	// TODO include the attempt number
	ctx = context.WithValue(ctx, ctxKeyClient{}, d.client)

	start := now()

	defer func() {
		// Recover from panics from within the task processor.
		if rec := recover(); rec != nil {
			d.log.Error("panic processing task",
				"id", t.ID,
				"queue", t.Queue,
				"error", rec,
			)

			err = fmt.Errorf("%v", rec)
		}

		// If panic or error, handle the task as a failure.
		if err != nil {
			d.taskFailure(q, t, start, time.Since(start), err)
		}
	}()

	// Process the task.
	if err = q.Receive(ctx, t.Task); err == nil {
		d.taskSuccess(q, t, start, time.Since(start))
	}
}

// taskSuccess handles post successful execution of a given task by removing it from the task table and optionally
// retaining it in the completed tasks table if the queue settings have retention enabled.
func (d *dispatcher) taskSuccess(q Queue, t *task.Task, started time.Time, dur time.Duration) {
	var tx *sql.Tx
	var err error

	defer func() {
		if err != nil {
			d.log.Error("failed to update task success",
				"id", t.ID,
				"queue", t.Queue,
				"error", err,
			)

			if tx != nil {
				if err := tx.Rollback(); err != nil {
					d.log.Error("failed to rollback task success",
						"id", t.ID,
						"queue", t.Queue,
						"error", err,
					)
				}
			}
		}
	}()

	d.log.Info("task processed",
		"id", t.ID,
		"queue", t.Queue,
		"duration", dur,
		"attempt", t.Attempts,
	)

	tx, err = d.client.db.Begin()
	if err != nil {
		return
	}

	err = t.DeleteTx(d.ctx, tx)
	if err != nil {
		return
	}

	if err = d.taskComplete(tx, q, t, started, dur, nil); err != nil {
		return
	}

	err = tx.Commit()
}

// taskFailure handles post failed execution of a given task by either releasing it back to the queue, if the maximum
// amount of attempts haven't been reached, or by deleting it from the task table and optionally moving to the completed
// task table if the queue has retention enabled.
func (d *dispatcher) taskFailure(q Queue, t *task.Task, started time.Time, dur time.Duration, taskErr error) {
	remaining := q.Config().MaxAttempts - t.Attempts

	d.log.Error("task processing failed",
		"id", t.ID,
		"queue", t.Queue,
		"duration", dur,
		"attempt", t.Attempts,
		"remaining", remaining,
	)

	if remaining < 1 {
		var tx *sql.Tx
		var err error

		defer func() {
			if err != nil {
				d.log.Error("failed to update task failure",
					"id", t.ID,
					"queue", t.Queue,
					"error", err,
				)

				if tx != nil {
					if err := tx.Rollback(); err != nil {
						d.log.Error("failed to rollback task failure",
							"id", t.ID,
							"queue", t.Queue,
							"error", err,
						)
					}
				}
			}
		}()

		tx, err = d.client.db.Begin()
		if err != nil {
			return
		}

		err = t.DeleteTx(d.ctx, tx)
		if err != nil {
			return
		}

		if err = d.taskComplete(tx, q, t, started, dur, taskErr); err != nil {
			return
		}

		err = tx.Commit()
	} else {
		t.LastExecutedAt = &started

		err := t.Fail(
			d.ctx,
			d.client.db,
			now().Add(q.Config().Backoff),
		)

		if err != nil {
			d.log.Error("failed to update task failure",
				"id", t.ID,
				"queue", t.Queue,
				"error", err,
			)
		}

		d.ready <- struct{}{}
	}
}

// taskComplete creates a completed task from a given task.
func (d *dispatcher) taskComplete(
	tx *sql.Tx,
	q Queue,
	t *task.Task,
	started time.Time,
	dur time.Duration,
	taskErr error) error {
	ret := q.Config().Retention
	if ret == nil {
		return nil
	}

	if taskErr == nil && ret.OnlyFailed {
		return nil
	}

	c := task.Completed{
		ID:             t.ID,
		Queue:          t.Queue,
		Attempts:       t.Attempts,
		Succeeded:      taskErr == nil,
		LastDuration:   dur,
		CreatedAt:      t.CreatedAt,
		LastExecutedAt: started,
	}

	if taskErr != nil {
		errStr := taskErr.Error()
		c.Error = &errStr
	}

	if ret.Duration != 0 {
		v := now().Add(ret.Duration)
		c.ExpiresAt = &v
	}

	if ret.Data != nil {
		if !ret.Data.OnlyFailed || taskErr != nil {
			c.Task = t.Task
		}
	}

	return c.InsertTx(d.ctx, tx)
}

// Notify is used by the client to notify the dispatcher that a new task was added.
func (d *dispatcher) Notify() {
	if d.running.Load() {
		d.ready <- struct{}{}
	}
}
