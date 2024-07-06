package backlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/mikestefanello/backlite/internal/query"
	"github.com/mikestefanello/backlite/internal/task"
	"log/slog"
	"sync/atomic"
	"time"
)

// dispatcher handles automatically pulling queued tasks and executing them via queue processors.
type dispatcher struct {
	// client is the Client that this dispatcher belongs to.
	client *Client

	// log is the logger.
	log Logger

	// ctx stores the context used to start the dispatcher.
	ctx context.Context

	// numWorkers is the amount of goroutines opened to execute tasks.
	numWorkers int

	// releaseAfter is the duration to reclaim a task for execution if it has not completed.
	releaseAfter time.Duration

	// CleanupInterval is how often to run cleanup operations on the database in order to remove expired completed
	// tasks.
	cleanupInterval time.Duration

	// availableWorkers tracks the amount of workers available to receive a task to execute.
	availableWorkers atomic.Int32

	// running indicates if the dispatching is currently running.
	running atomic.Bool

	// tasks transmits tasks to the workers.
	tasks chan *task.Task

	// ticker will fetch tasks from the database if the next task is delayed.
	ticker *time.Ticker

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

// start starts the dispatcher.
// To stop, cancel the provided context.
func (d *dispatcher) start(ctx context.Context) {
	// Abort if the dispatcher is already running
	if d.running.Load() {
		return
	}

	d.running.Store(true)
	d.ctx = ctx
	d.tasks = make(chan *task.Task, d.numWorkers)
	d.ticker = time.NewTicker(time.Second)
	d.ticker.Stop()                     // No need to tick yet
	d.ready = make(chan struct{}, 1000) // Prevent blocking task creation
	d.trigger = make(chan struct{}, 10) // Should never need more than 1 but just in case
	d.availableWorkers.Store(int32(d.numWorkers))

	for range d.numWorkers {
		go d.worker()
	}

	if d.cleanupInterval > 0 {
		go d.cleaner()
	}

	go d.triggerer()
	go d.fetcher()

	d.ready <- struct{}{}
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

		case <-d.ctx.Done():
			return
		}
	}
}

// fetcher fetches tasks from the database to be executed either when the ticker ticks or when the trigger signal
// is sent by the triggerer.
func (d *dispatcher) fetcher() {
	for {
		select {
		case <-d.ticker.C:
			d.ticker.Stop()
			d.fetch()

		case <-d.trigger:
			d.fetch()

		case <-d.ctx.Done():
			d.running.Store(false)
			d.ticker.Stop()
			close(d.tasks)
			return
		}
	}
}

// worker processes incoming tasks.
func (d *dispatcher) worker() {
	for {
		select {
		case row := <-d.tasks:
			d.availableWorkers.Add(-1)
			d.processTask(row)
			d.availableWorkers.Add(1)

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
			_, err := d.client.db.ExecContext(
				d.ctx,
				query.DeleteExpiredCompletedTasks,
				time.Now().UnixMilli(),
			)

			if err != nil {
				d.log.Error("failed to delete expired completed tasks",
					"error", err,
				)
			}

		case <-d.ctx.Done():
			ticker.Stop()
			return
		}
	}
}

// acquireWorkers waits until at least one worker is available to execute a task and returns the number that are
// available.
func (d *dispatcher) acquireWorkers() int32 {
	for {
		if w := d.availableWorkers.Load(); w > 0 {
			return w
		}
		time.Sleep(100 * time.Millisecond) // TODO use channel instead?
	}
}

// fetch fetches tasks from the database to be executed and/or coordinate the dispatcher, so it is aware of when it
// needs to fetch again.
func (d *dispatcher) fetch() {
	var success bool

	// If we failed at any point, we need to tell the dispatcher to try again.
	defer func() {
		if !success {
			// Wait and try again
			time.Sleep(100 * time.Millisecond)
			d.ready <- struct{}{}
		}
	}()

	// Indicate that incoming task additions from this point on should trigger another fetch.
	d.triggered.Store(false)

	// Determine how many workers are available, so we only fetch that many tasks.
	workers := d.acquireWorkers()

	// Fetch tasks for each available worker plus the next upcoming task so the scheduler knows when to
	// query the database again without having to continually poll.
	tasks, err := task.GetScheduledTasks(
		d.ctx,
		d.client.db,
		time.Now().Add(-d.releaseAfter),
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
		if int32(i+1) > workers {
			nextUp(i)
			break
		}

		// Check if this task is not ready yet.
		if tasks[i].WaitUntil != nil {
			if tasks[i].WaitUntil.After(time.Now()) {
				nextUp(i)
				break
			}
		}
	}

	slog.Info("fetched tasks", "ready", len(tasks), "next", next != nil) // TODO remove

	// Claim the tasks that are ready to be processed.
	if err := tasks.Claim(d.ctx, d.client.db); err != nil {
		d.log.Error("failed to claim tasks",
			"error", err,
		)
		return
	}

	// Send the ready tasks to the workers.
	for i := range tasks {
		tasks[i].Attempts++
		d.tasks <- tasks[i]
	}

	success = true

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

		dur := time.Until(*t.WaitUntil)
		if dur < 0 {
			d.ready <- struct{}{}
			return
		}

		d.ticker.Reset(dur)
	}
}

func (d *dispatcher) processTask(t *task.Task) {
	q := d.client.getQueue(t.Queue)
	cfg := q.Config()

	var ctx context.Context
	var cancel context.CancelFunc

	// Set a context timeout, if desired.
	if cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()
	} else {
		ctx = context.Background()
	}

	// Store the client in the context so the processor can use it.
	ctx = context.WithValue(ctx, clientCtxKey{}, d.client)

	start := time.Now()

	// Recover from panics from within the task processor.
	defer func() {
		if rec := recover(); rec != nil {
			d.log.Error("panic processing task",
				"id", t.ID,
				"queue", t.Queue,
				"error", rec,
			)

			d.taskFailure(q, t, start, time.Since(start), fmt.Errorf("%v", rec))
		}
	}()

	// Process the task and measure the execution duration.
	err := q.Receive(ctx, t.Task)
	duration := time.Since(start)

	if err != nil {
		d.taskFailure(q, t, start, duration, err)
	} else {
		d.taskSuccess(q, t, start, duration)
	}
}

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

			// TODO what do we do now?
		}
	}()

	d.log.Info("task successfully processed",
		"id", t.ID,
		"queue", t.Queue,
		"duration", dur,
		"attempt", t.Attempts,
	)

	tx, err = d.client.db.Begin()
	if err != nil {
		return
	}

	err = t.DeleteTx(d.ctx, tx) // TODO use this context?
	if err != nil {
		return
	}

	if ret := q.Config().Retention; ret != nil {
		c := task.Completed{
			ID:             t.ID,
			Queue:          t.Queue,
			Attempts:       t.Attempts,
			Succeeded:      true,
			LastDuration:   dur,
			CreatedAt:      t.CreatedAt,
			LastExecutedAt: started,
		}

		if ret.Duration != 0 {
			v := time.Now().Add(ret.Duration)
			c.ExpiresAt = &v
		}

		if ret.Data != nil && !ret.Data.OnlyFailed {
			c.Task = t.Task
		}

		err = c.InsertTx(d.ctx, tx) // TODO use this context?
		if err != nil {
			return
		}
	}

	err = tx.Commit()
}

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

				// TODO what do we do now?
			}
		}()

		tx, err = d.client.db.Begin()
		if err != nil {
			return
		}

		err = t.DeleteTx(d.ctx, tx) // TODO use this context?
		if err != nil {
			return
		}

		if ret := q.Config().Retention; ret != nil {
			errStr := taskErr.Error()

			c := task.Completed{
				ID:             t.ID,
				Queue:          t.Queue,
				Attempts:       t.Attempts,
				Succeeded:      false,
				LastDuration:   dur,
				CreatedAt:      t.CreatedAt,
				LastExecutedAt: started,
				Error:          &errStr,
			}

			if ret.Duration != 0 {
				v := time.Now().Add(ret.Duration)
				c.ExpiresAt = &v
			}

			if ret.Data != nil {
				c.Task = t.Task
			}

			err = c.InsertTx(d.ctx, tx) // TODO use this context?
			if err != nil {
				return
			}
		}

		err = tx.Commit()
	} else {
		t.LastExecutedAt = &started

		err := t.Fail(
			d.ctx,
			d.client.db,
			time.Now().Add(q.Config().Backoff),
		) // TODO use this context?

		if err != nil {
			d.log.Error("failed to update task failure",
				"id", t.ID,
				"queue", t.Queue,
				"error", err,
			)
		}

		// TODO schedule or just poll?
		d.ready <- struct{}{}
	}
}

// notify is used by the client to notify the dispatcher that a new task was added.
func (d *dispatcher) notify() {
	if d.running.Load() {
		d.ready <- struct{}{}
	}
}
