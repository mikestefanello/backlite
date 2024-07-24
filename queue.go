package backlite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type (
	// Queue represents a queue which contains tasks to be executed.
	Queue interface {
		// Config returns the configuration for the queue.
		Config() *QueueConfig

		// Process processes the Task.
		Process(ctx context.Context, payload []byte) error
	}

	// QueueConfig is the configuration options for a queue.
	QueueConfig struct {
		// Name is the name of the queue and must be unique.
		Name string

		// MaxAttempts are the maximum number of attempts to execute this task before it's marked as completed.
		MaxAttempts int

		// Timeout is the duration set on the context while executing a given task.
		Timeout time.Duration

		// Backoff is the duration a failed task will be held in the queue until being retried.
		Backoff time.Duration

		// Retention dictates if and how completed tasks will be retained in the database.
		// If nil, no completed tasks will be retained.
		Retention *Retention
	}

	// Retention is the policy for how completed tasks will be retained in the database.
	Retention struct {
		// Duration is the amount of time to retain a task for after completion.
		// If omitted, the task will be retained forever.
		Duration time.Duration

		// OnlyFailed indicates if only failed tasks should be retained.
		OnlyFailed bool

		// Data provides options for retaining Task payload data.
		// If nil, no task payload data will be retained.
		Data *RetainData
	}

	// RetainData is the policy for how Task payload data will be retained in the database after the task is complete.
	RetainData struct {
		// OnlyFailed indicates if Task payload data should only be retained for failed tasks.
		OnlyFailed bool
	}

	// queue provides a type-safe implementation of Queue
	queue[T Task] struct {
		config    *QueueConfig
		processor QueueProcessor[T]
	}

	// QueueProcessor is a generic processor callback for a given queue to process Tasks
	QueueProcessor[T Task] func(context.Context, T) error

	// queues stores a registry of queues.
	queues struct {
		registry map[string]Queue
		sync.RWMutex
	}
)

// NewQueue creates a new type-safe Queue of a given Task type
func NewQueue[T Task](processor QueueProcessor[T]) Queue {
	var task T
	cfg := task.Config() // TODO fix this?

	q := &queue[T]{
		config:    &cfg,
		processor: processor,
	}

	return q
}

func (q *queue[T]) Config() *QueueConfig {
	return q.config
}

func (q *queue[T]) Process(ctx context.Context, payload []byte) error {
	var obj T

	err := json.
		NewDecoder(bytes.NewReader(payload)).
		Decode(&obj)

	if err != nil {
		return err
	}

	return q.processor(ctx, obj)
}

// add adds a queue to the registry and will panic if the name has already been registered.
func (q *queues) add(queue Queue) {
	if len(queue.Config().Name) == 0 {
		panic("queue name is missing")
	}

	q.Lock()
	defer q.Unlock()

	if _, exists := q.registry[queue.Config().Name]; exists {
		panic(fmt.Sprintf("queue '%s' already registered", queue.Config().Name))
	}

	q.registry[queue.Config().Name] = queue
}

// get loads a queue from the registry by name.
func (q *queues) get(name string) Queue {
	q.RLock()
	defer q.RUnlock()
	return q.registry[name]
}
