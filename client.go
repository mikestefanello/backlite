package backlite

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/gob"
	"errors"
	"fmt"
	"sync"
	"time"
)

//go:embed schema.sql
var schema string

type (
	// Client is a client used to register queues and add tasks to them for execution.
	Client struct {
		// db stores the database to use for storing tasks.
		db *sql.DB

		// log is the logger.
		log Logger

		// queues stores the registered queues which tasks can be added to.
		queues map[string]Queue

		// queuesLock provides a lock for the queues.
		queueLock sync.RWMutex

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

		// TODO hooks (success, failure, deadletter?)
	}
)

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
		queues: make(map[string]Queue),
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
	c.queueLock.Lock()
	defer c.queueLock.Unlock()

	if _, exists := c.queues[queue.Config().Name]; exists {
		panic(fmt.Sprintf("queue '%s' already registered", queue.Config().Name))
	}

	c.queues[queue.Config().Name] = queue
}

// Add starts an operation to add one or many tasks.
func (c *Client) Add(tasks ...Task) *TaskAddOp {
	return &TaskAddOp{
		client: c,
		tasks:  tasks,
	}
}

// Start starts the dispatcher so queued tasks can automatically be executed in the background.
func (c *Client) Start(ctx context.Context) {
	c.dispatcher.start(ctx)
}

func (c *Client) Shutdown(ctx context.Context) {
	// TODO needed?
}

// Install installs the provided schema in the database.
// TODO provide robust migrations
func (c *Client) Install() error {
	_, err := c.db.Exec(schema)
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

			if err := op.tx.Rollback(); err != nil {
				c.log.Error("failed to rollback task creation transaction",
					"error", err,
				)
			}
		}()
	}

	stm, err := op.tx.PrepareContext(op.ctx, queryInsertTask)
	if err != nil {
		return err
	}

	var wait *int64
	if op.wait != nil {
		m := op.wait.UnixMilli()
		wait = &m
	}

	for _, task := range op.tasks {
		buf.Reset()

		// Encode the task.
		if err = gob.NewEncoder(buf).Encode(task); err != nil {
			return err
		}

		// Insert the task.
		_, err = op.tx.Stmt(stm).ExecContext(
			op.ctx,
			time.Now().UnixMilli(),
			task.Config().Name,
			buf.Bytes(),
			wait,
		)

		if err != nil {
			return err
		}
	}

	// If we created the transaction we'll commit it now.
	if commit {
		if err = op.tx.Commit(); err != nil {
			return err
		}

		c.dispatcher.notify()
	}

	return nil
}

// getQueue loads a queue from the registry by name.
func (c *Client) getQueue(name string) Queue {
	c.queueLock.RLock()
	defer c.queueLock.RUnlock()
	return c.queues[name]
}
