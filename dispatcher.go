package backlite

import (
	"context"
	"database/sql"
	"log/slog"
	"sync/atomic"
	"time"
)

type (
	// dispatcher handles automatically pulling queued tasks and executing them via queue processors.
	dispatcher struct {
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
		tasks chan taskRow

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

	taskRow struct {
		id        string
		queue     string
		task      []byte
		attempts  int
		waitUntil *int64
		createdAt int64
	}
)

// start starts the dispatcher.
// To stop, cancel the provided context.
func (d *dispatcher) start(ctx context.Context) {
	// Abort if the dispatcher is already running
	if d.running.Load() {
		return
	}

	d.running.Store(true)
	d.ctx = ctx
	d.tasks = make(chan taskRow, d.numWorkers)
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
			d.ticker.Stop()
			close(d.tasks)
			d.running.Store(false)
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
			_, err := d.client.db.Exec(queryDeleteExpiredCompletedTasks, time.Now().UnixMilli())
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
		time.Sleep(100 * time.Millisecond)
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
	rows, err := d.client.db.QueryContext(
		d.ctx,
		querySelectTasks,
		time.Now().Add(-d.releaseAfter).UnixMilli(),
		workers+1,
	)

	if err != nil {
		d.log.Error("fetch tasks query failed",
			"error", err,
		)
		return
	}

	defer func() {
		if err := rows.Close(); err != nil {
			d.log.Error("fetch tasks row close failed",
				"error", err,
			)
		}
	}()

	tasks := make([]taskRow, 0, workers)
	var next *taskRow
	var rowCount int

	for rows.Next() {
		var row taskRow
		err := rows.Scan(&row.id, &row.queue, &row.task, &row.attempts, &row.waitUntil, &row.createdAt)

		if err != nil {
			d.log.Error("failed scanning task",
				"error", err,
			)
			continue
		}

		rowCount++

		// Check if the workers are full
		if int32(rowCount) > workers {
			next = &row
			break
		}

		// Check if this task is not ready yet
		if row.waitUntil != nil {
			if time.UnixMilli(*row.waitUntil).After(time.Now()) {
				next = &row
				break
			}
		}

		tasks = append(tasks, row)
	}

	if rows.Err() != nil {
		d.log.Error("iterating tasks failed",
			"error", err,
		)
		return
	}

	slog.Info("fetched tasks", "ready", len(tasks), "next", next != nil) // TODO remove

	// Claim the tasks that are ready to be processed.
	if err := d.claimTasks(d.ctx, tasks); err != nil {
		d.log.Error("failed to claim tasks",
			"error", err,
		)
		return
	}

	// Send the ready tasks to the workers.
	for _, row := range tasks {
		row.attempts++
		d.tasks <- row
	}

	success = true

	// Adjust the schedule based on the next up task.
	d.schedule(next)
}

func (d *dispatcher) claimTasks(ctx context.Context, rows []taskRow) error {
	if len(rows) == 0 {
		return nil
	}

	params := make([]any, 0, len(rows)+1)
	params = append(params, time.Now().UnixMilli())

	for _, row := range rows {
		params = append(params, row.id)
	}

	_, err := d.client.db.ExecContext(
		ctx,
		queryClaimTasks(len(rows)),
		params...,
	)
	slog.Info("claimed tasks", "len", len(rows)) // TODO remove
	return err
}

// schedule handles scheduling the dispatcher based on the next up task provided by the fetcher.
func (d *dispatcher) schedule(row *taskRow) {
	d.ticker.Stop()

	if row != nil {
		if row.waitUntil == nil {
			d.ready <- struct{}{}
			return
		}

		dur := time.Until(time.UnixMilli(*row.waitUntil))
		if dur < 0 {
			d.ready <- struct{}{}
			return
		}

		d.ticker.Reset(dur)
	}
}

func (d *dispatcher) processTask(row taskRow) {
	defer func() {
		if rec := recover(); rec != nil {
			d.log.Error("panic processing task",
				"id", row.id,
				"queue", row.queue,
				"error", rec,
			)
		}
		// TODO update the db
	}()

	q := d.client.getQueue(row.queue)
	cfg := q.Config()

	var ctx context.Context
	var cancel context.CancelFunc

	if cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()
	} else {
		ctx = context.Background()
	}

	start := time.Now()
	err := q.Receive(ctx, row.task) // TODO goroutine in order to timeout?
	duration := time.Since(start)

	if err != nil {
		d.taskFailure(q, row, start, duration, err)
	} else {
		d.taskSuccess(q, row, start, duration)
	}
}

func (d *dispatcher) taskSuccess(q Queue, row taskRow, started time.Time, dur time.Duration) {
	var tx *sql.Tx
	var err error

	defer func() {
		if err != nil {
			d.log.Error("failed to update task success",
				"id", row.id,
				"queue", row.queue,
				"error", err,
			)

			if tx != nil {
				if err := tx.Rollback(); err != nil {
					d.log.Error("failed to rollback task success",
						"id", row.id,
						"queue", row.queue,
						"error", err,
					)
				}
			}

			// TODO what do we do now?
		}
	}()

	d.log.Info("task successfully processed",
		"id", row.id,
		"queue", row.queue,
		"duration", dur,
		"attempt", row.attempts,
	)

	tx, err = d.client.db.Begin()
	if err != nil {
		return
	}

	_, err = tx.Exec(queryDeleteTask, row.id)
	if err != nil {
		return
	}

	if ret := q.Config().Retention; ret != nil {
		var expiresAt *int64
		var payload []byte

		if ret.Duration != 0 {
			at := time.Now().Add(ret.Duration).UnixMilli()
			expiresAt = &at
		}

		if ret.Data != nil && !ret.Data.OnlyFailed {
			payload = row.task
		}

		_, err = tx.Exec(
			queryInsertCompletedTask,
			row.id,
			row.createdAt,
			q.Config().Name,
			started.UnixMilli(),
			row.attempts,
			dur.Microseconds(),
			1,
			payload,
			expiresAt,
			nil,
		)
		if err != nil {
			return
		}
	}

	err = tx.Commit()
}

func (d *dispatcher) taskFailure(q Queue, row taskRow, started time.Time, dur time.Duration, taskErr error) {
	d.log.Error("task processing failed",
		"id", row.id,
		"queue", row.queue,
		"duration", dur,
		"attempt", row.attempts,
		"remaining", q.Config().MaxAttempts-row.attempts,
	)

	if row.attempts >= q.Config().MaxAttempts {
		var tx *sql.Tx
		var err error

		defer func() {
			if err != nil {
				d.log.Error("failed to update task failure",
					"id", row.id,
					"queue", row.queue,
					"error", err,
				)

				if tx != nil {
					if err := tx.Rollback(); err != nil {
						d.log.Error("failed to rollback task failure",
							"id", row.id,
							"queue", row.queue,
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

		_, err = tx.Exec(queryDeleteTask, row.id)
		if err != nil {
			return
		}

		if ret := q.Config().Retention; ret != nil {
			var expiresAt *int64
			var payload []byte

			if ret.Duration != 0 {
				at := time.Now().Add(ret.Duration).UnixMilli()
				expiresAt = &at
			}

			if ret.Data != nil {
				payload = row.task
			}

			_, err = tx.Exec(
				queryInsertCompletedTask,
				row.id,
				row.createdAt,
				q.Config().Name,
				started.UnixMilli(),
				row.attempts,
				dur.Microseconds(),
				0,
				payload,
				expiresAt,
				taskErr.Error(),
			)
			if err != nil {
				return
			}
		}

		err = tx.Commit()
	} else {
		_, err := d.client.db.Exec(
			queryTaskFailed,
			time.Now().Add(q.Config().Backoff).UnixMilli(),
			started.UnixMilli(),
			row.id,
		)

		if err != nil {
			d.log.Error("failed to update task failure",
				"id", row.id,
				"queue", row.queue,
				"error", err,
			)
		}

		// TODO schedule?
		d.ready <- struct{}{}
	}
}

// notify is used by the client to notify the dispatcher that a new task was added.
func (d *dispatcher) notify() {
	if d.running.Load() {
		d.ready <- struct{}{}
	}
}
