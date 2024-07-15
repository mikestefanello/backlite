package backlite

import (
	"bytes"
	"context"
	"database/sql"

	"github.com/mikestefanello/backlite/internal/query"

	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/mikestefanello/backlite/internal/task"
)

// now returns the current time in a way that tests can override.
var now = func() time.Time { return time.Now() }

type (
	// Client is a client used to register queues and add tasks to them for execution.
	Client struct {
		// db stores the database to use for storing tasks.
		db *sql.DB

		// log is the logger.
		log Logger

		// queues stores the registered queues which tasks can be added to.
		queues queues

		// buffers is a pool of byte buffers for more efficient encoding.
		buffers sync.Pool

		// dispatcher is used to fetch and dispatch queued tasks to the workers for execution.
		dispatcher *dispatcher
	}

	// ClientConfig contains configuration for the Client.
	ClientConfig struct {
		// DB is the open database connection used for storing tasks.
		DB *sql.DB

		// Logger is the logger used to log task execution.
		Logger Logger

		// NumWorkers is the number of goroutines to open to use for executing queued tasks concurrently.
		NumWorkers int

		// ReleaseAfter is the duration after which a task is released back to a queue if it has not finished executing.
		// This value should be much higher than the timeout setting used for each queue and exists as a fail-safe
		// just in case tasks become stuck.
		ReleaseAfter time.Duration

		// CleanupInterval is how often to run cleanup operations on the database in order to remove expired completed
		// tasks. If omitted, no cleanup operations will be performed and the task retention duration will be ignored.
		CleanupInterval time.Duration
	}

	// ctxKeyClient is used to store a Client in a context.
	ctxKeyClient struct{}
)

// FromContext returns a Client from a context which is set for queue processor callbacks, so they can access
// the client in order to create additional tasks.
func FromContext(ctx context.Context) *Client {
	if c, ok := ctx.Value(ctxKeyClient{}).(*Client); ok {
		return c
	}
	return nil
}

// NewClient initializes a new Client
func NewClient(cfg ClientConfig) (*Client, error) {
	switch {
	case cfg.DB == nil:
		return nil, errors.New("missing database")

	case cfg.NumWorkers < 1:
		return nil, errors.New("at least one worker required")

	case cfg.ReleaseAfter <= 0:
		return nil, errors.New("release duration must be greater than zero")
	}

	if cfg.Logger == nil {
		cfg.Logger = &noLogger{}
	}

	c := &Client{
		db:     cfg.DB,
		log:    cfg.Logger,
		queues: queues{registry: make(map[string]Queue)},
		buffers: sync.Pool{
			New: func() any {
				return bytes.NewBuffer(nil)
			},
		},
	}

	c.dispatcher = &dispatcher{
		client:          c,
		log:             cfg.Logger,
		numWorkers:      cfg.NumWorkers,
		releaseAfter:    cfg.ReleaseAfter,
		cleanupInterval: cfg.CleanupInterval,
	}

	return c, nil
}

// Register registers a new Queue so tasks can be added to it.
// This will panic if the name of the queue provided has already been registered.
func (c *Client) Register(queue Queue) {
	c.queues.add(queue)
}

// Add starts an operation to add one or many tasks.
func (c *Client) Add(tasks ...Task) *TaskAddOp {
	return &TaskAddOp{
		client: c,
		tasks:  tasks,
	}
}

// Start starts the dispatcher so queued tasks can automatically be executed in the background.
// To gracefully shut down the dispatcher, call Stop(), or to hard-stop, cancel the provided context.
func (c *Client) Start(ctx context.Context) {
	c.dispatcher.start(ctx)
}

// Stop attempts to gracefully shut down the dispatcher before the provided context is cancelled.
// True is returned if all workers were able to complete their tasks prior to shutting down.
func (c *Client) Stop(ctx context.Context) bool {
	return c.dispatcher.stop(ctx)
}

// Install installs the provided schema in the database.
// TODO provide migrations
func (c *Client) Install() error {
	_, err := c.db.Exec(query.Schema)
	return err
}

// Notify notifies the dispatcher that a new task has been added.
// This is only needed and required if you supply a database transaction when adding a task.
// See TaskAddOp.Tx().
func (c *Client) Notify() {
	c.dispatcher.notify()
}

// save saves a task add operation.
func (c *Client) save(op *TaskAddOp) error {
	var commit bool
	var err error

	// Get a buffer for the encoding.
	buf := c.buffers.Get().(*bytes.Buffer)

	// Put the buffer back in the pool for re-use.
	defer func() {
		buf.Reset()
		c.buffers.Put(buf)
	}()

	if op.ctx == nil {
		op.ctx = context.Background()
	}

	// Start a transaction if one isn't provided.
	if op.tx == nil {
		op.tx, err = c.db.BeginTx(op.ctx, nil)
		if err != nil {
			return err
		}
		commit = true

		defer func() {
			if err == nil {
				return
			}

			if err = op.tx.Rollback(); err != nil {
				c.log.Error("failed to rollback task creation transaction",
					"error", err,
				)
			}
		}()
	}

	// Insert the tasks.
	for _, t := range op.tasks {
		buf.Reset()

		if err = json.NewEncoder(buf).Encode(t); err != nil {
			return err
		}

		m := task.Task{
			Queue:     t.Config().Name,
			Task:      buf.Bytes(),
			WaitUntil: op.wait,
		}

		if err = m.InsertTx(op.ctx, op.tx); err != nil {
			return err
		}
	}

	// If we created the transaction we'll commit it now.
	if commit {
		if err = op.tx.Commit(); err != nil {
			return err
		}

		// Tell the dispatcher that a new task has been added.
		c.dispatcher.notify()
	}

	return nil
}
