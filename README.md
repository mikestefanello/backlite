## Backlite: Type-safe, persistent, embedded task queues and background job runner w/ SQLite

[![Go Report Card](https://goreportcard.com/badge/github.com/mikestefanello/backlite)](https://goreportcard.com/report/github.com/mikestefanello/backlite)
[![Test](https://github.com/mikestefanello/backlite/actions/workflows/test.yml/badge.svg)](https://github.com/mikestefanello/backlite/actions/workflows/test.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Reference](https://pkg.go.dev/badge/github.com/mikestefanello/backlite.svg)](https://pkg.go.dev/github.com/mikestefanello/backlite)
[![GoT](https://img.shields.io/badge/Made%20with-Go-1f425f.svg)](https://go.dev)

<p align="center"><img alt="Logo" src="https://raw.githubusercontent.com/mikestefanello/readmeimages/main/backlite/logo.png" /></p>

## Table of Contents
* [Introduction](#introduction)
    * [Overview](#overview)
    * [Origin](#origin)
    * [Screenshots](#screenshots)
    * [Status](#status)
* [Installation](#installation)
* [Features](#features)
    * [Type-safety](#type-safety) 
    * [Persistence with SQLite](#persistence-with-sqlite)
    * [Optional retention](#optional-retention)
    * [Retry & Backoff](#retry--backoff)
    * [Scheduled execution](#scheduled-execution)
    * [Logging](#logging)
    * [Nested tasks](#nested-tasks)
    * [Graceful shutdown](#graceful-shutdown)
    * [Transactions](#transactions)
    * [No database polling](#no-database-polling)
    * [Driver flexibility](#driver-flexibility)
    * [Bulk inserts](#bulk-inserts)
    * [Execution timeout](#execution-timeout)
    * [Background worker pool](#background-worker-pool)
    * [Web UI](#web-ui)
* [Usage](#usage)
    * [Client initialization](#client-initialization)
    * [Schema installation](#schema-installation)
    * [Declaring a Task type](#declaring-a-task-type)
    * [Queue processor](#queue-processor)
    * [Registering a queue](#registering-a-queue)
    * [Adding tasks](#adding-tasks)
    * [Starting the dispatcher](#starting-the-dispatcher)
    * [Shutting down the dispatcher](#shutting-down-the-dispatcher)
* [Roadmap](#roadmap)

## Introduction

### Overview

Backlite provides type-safe, persistent and embedded task queues meant to run within your application as opposed to an external message broker. A task can be of any type and each type declares its own queue along with the configuration for the queue. Tasks are automatically executed in the background via a configurable worker pool.

### Origin

This project started shortly after migrating [Pagoda](https://github.com/mikestefanello/pagoda) to SQLite from Postgres and Redis. Redis was previously used to handle task queues and I wanted to leverage SQLite instead. Originally, [goqite](https://github.com/maragudk/goqite) was chosen, which is a great library and one that I took inspiration from, but I had a lot of different ideas for the overall approach and it lacked a number of features that I felt were needed.

[River](https://github.com/riverqueue/river), an excellent, similar library built for Postgres, was also a major source of inspiration and ideas.

### Screenshots

<img src="https://raw.githubusercontent.com/mikestefanello/readmeimages/main/backlite/failed.png" alt="Failed tasks"/>

<img src="https://raw.githubusercontent.com/mikestefanello/readmeimages/main/backlite/task.png" alt="Task details"/>

<img src="https://raw.githubusercontent.com/mikestefanello/readmeimages/main/backlite/task-failed.png" alt="Task failed"/>

More to come as the web UI is under active development.

### Status

This project is under active development though all features outlined below are available and complete. No significant API or schema changes are expected at this time.

## Installation

Install by simply running: `go get github.com/mikestefanello/backlite`

## Features

### Type-safety

No need to deal with serialization and byte slices. By leveraging generics, tasks and queues are completely type-safe which means that you pass in your task type into a queue and your task processor callbacks will only receive that given type.
 
### Persistence with SQLite

When tasks are added to a queue, they are inserted into a SQLite database table to ensure persistence. 

### Optional retention

Each queue can have completed tasks retained in a separate table for archiving, auditing, monitoring, etc. Options exist to retain all completed tasks or only those that failed all attempts. An option also exists to retain the task data for all tasks or only those that failed.

### Retry & Backoff

Each queue can be configured to retry tasks a certain number of times and to backoff a given amount of time between each attempt.

### Scheduled execution

When adding a task to a queue, you can specify a duration or specific time to wait until executing the task.

### Logging

Optionally log queue operations with a logger of your choice, as long as it implements the simple `Logger` interface, which `log/slog` does.

```
2024/07/21 14:08:13 INFO task processed id=0190d67a-d8da-76d4-8fb8-ded870d69151 queue=example duration=85.101µs attempt=1
```

### Nested tasks

While processing a given task, it's easy to create another task in the same or a different queue, which allows you to nest tasks to create workflows. Use `FromContext()` with the provided context to get your initialized client from within the task processor, and add one or many tasks.

### Graceful shutdown

The task dispatcher, which handles sending tasks to the worker pool for execution, can be shutdown gracefully by calling `Stop()` on the client. That will wait for all workers to finish for as long as the passed in context is not cancelled. The hard-stop the dispatcher, cancel the context passed in when calling `Start()`. See usage below.

### Transactions

Task creation can be added to a given database transaction. If you are using SQLite as your primary database, this provides a simple, robust way to ensure data integrity. For example, using the eCommerce app example, when inserting a new order into your database, the same transaction to be used to add a task to send an order notification email, and they either both succeed or both fail. Use the chained method `Tx()` to provide your transaction when adding one or multiple tasks.

### No database polling

Since SQLite only supports one writer, no continuous database polling is required. The task dispatcher is able to remain aware of new tasks and keep track of when future tasks are scheduled for, and thus only queries the database when it needs to.

### Driver flexibility

Use any SQLite driver that you'd like. This library only includes [go-sqlite3](https://github.com/mattn/go-sqlite3) since it is used in tests.

### Bulk inserts

Insert one or many tasks across one or many queues in a single operation.

### Execution timeout

Each queue can be configured with an execution timeout for processing a given task. The provided context will cancel after the time elapses. If you want to respect that timeout, your processor code will have to listen for the context cancellation.

### Background worker pool

When creating a client, you can specify the amount of goroutines to use to build a worker pool. This pool is created and shutdown via the dispatcher by calling `Start()` and `Stop()` on the client. The worker pool is the only way to process tasks; they cannot be pulled manually.

### Web UI

A simple web UI to monitor running, upcoming, and completed tasks is provided.

To run, pass your `*sql.DB` to `ui.NewHandler()` and register that to an HTTP handler, for example:

```go
mux := http.DefaultServeMux
ui.NewHandler(db).Register(mux)
err := http.ListenAndServe(":9000", mux)
```

Then visit the given port and/or domain in your browser (ie, `localhost:9000`).

The web CSS and JS are provided by [tabler](https://github.com/tabler/tabler).

## Usage

### Client initialization

First, open a connection to your SQLite database using the driver of your choice:

```go
db, err := sql.Open("sqlite3", "data.db?_journal=WAL&_timeout=5000")
```

Second, initialize a client:

```go
client, err := backlite.NewClient(backlite.ClientConfig{
    DB:              db,
    Logger:          slog.Default(),
    ReleaseAfter:    10 * time.Minute,
    NumWorkers:      10,
    CleanupInterval: time.Hour,
})
```

The configuration options are:

* **DB**: The database connection.
* **Logger**: A logger that implements the `Logger` interface. Omit if you do not want to log.
* **ReleaseAfter**: The duration after which tasks claimed and passed for execution should be added back to the queue if a response was never received.
* **NumWorkers**: The amount of goroutines to open which will process queued tasks.
* **CleanupInterval**: How often the completed tasks database table will attempt to remove expired rows.

### Schema installation

Until a more robust system is provided, to install the database schema, call `client.Install()`. This must be done prior to using the client. It is safe to call this if the schema was previously installed. The schema is currently defined in `internal/query/schema.sql`.

### Declaring a Task type

Any type can be a task as long as it implements the `Task` interface, which requires only the `Config() QueueConfig` method, used to provide information about the queue that these tasks will be added to. As an example, this is a task used to send new order email notifications:

```go
type NewOrderEmailTask struct {
    OrderID string
    EmailAddress string
}
```

Then implement the `Task` interface by providing the queue configuration:

```go
func (t NewOrderEmailTask) Config() backlite.QueueConfig {
    return backlite.QueueConfig{
        Name:        "NewOrderEmail",
        MaxAttempts: 5,
        Backoff:     5 * time.Second,
        Timeout:     10 * time.Second,
        Retention: &backlite.Retention{
            Duration:   6 * time.Hour,
            OnlyFailed: false,
            Data: &backlite.RetainData{
                OnlyFailed: true,
            },
        },
    }
}
```

The configuration options are:

* **Name**: The name of the queue. This must be unique otherwise registering the queue will fail.
* **MaxAttempts**: The maximum number of times to try executing this task before it's consider failed and marked as complete.
* **Backoff**: The amount of time to wait before retrying after a failed attempt at processing.
* **Retention**: If provided, completed tasks will be retained in the database in a separate table according to the included options.
    * **Duration**: How long to retain completed tasks in the database for. Omit to never expire.
    * **OnlyFailed**: If true, only failed tasks will be retained.
    * **Data**: If provided, the task data (the serialized task itself) will be retained.
        * **OnlyFailed**: If true, the task data will only be retained for failed tasks.

### Queue processor

The easiest way to implement a queue and define the processor is to use `backlite.NewQueue()`. This leverages generics in order to provide type-safety with a given task type. Using the example above:

```go
processor := func(ctx context.Context, task NewOrderEmailTask) error {
    return email.Send(ctx, task.EmailAddress, fmt.Sprintf("Order %s received", task.OrderID))
}

queue := backlite.NewQueue[NewOrderEmailTask](processor)
```

The parameter is the processor callback which is what will be called by the dispatcher worker pool to execute the task. If no error is returned, the task is considered successfully executed. If the task fails all attempts and the queue has retention enabled, the value of the error will be stored in the database.

The provided context will be set to timeout at the duration set in the queue settings, if provided. To get the client from the context, you can call `client := backlite.FromContext(ctx)`.

### Registering a queue

You must register all queues with the client by calling `client.Register(queue)`. This will panic if duplicate queue names are registered.

### Adding tasks

To add a task to the queue, simply pass one or many into `client.Add()`. You can provide tasks of different types. This returns a chainable operation which contains many options, that can be used as follows:

```go
err := client.
    Add(task1, task2).
    Ctx(ctx).
    Tx(tx).
    At(time.Date(2024, 1, 5, 12, 30, 00)).
    Wait(15 * time.Minute).
    Save()
```

Only `Add()` and `Save()` are required. Don't use `At()` and `Wait()` together as they override each other.

The options are:

* **Ctx**: Provide a context to use for the operation.
* **Tx**: Provide a database transaction to add the tasks to. You must commit this yourself then call `client.Notify()` to tell the dispatcher that the new task(s) were added. This may be improved in the future but for now it is required.
* **At**: Don't execute this task until at least the given date and time.
* **Wait**: Wait at least the given duration before executing the task.

### Starting the dispatcher

To start the dispatcher, which will spin up the worker pool and begin executing tasks in the background, call `client.Start()`. The context you pass in must persist for as long as you want the dispatcher to continue working. If that is ever cancelled, the dispatcher will shutdown. See the next section for more details.

### Shutting down the dispatcher

To gracefully shutdown the dispatcher, which will wait until all tasks currently being executed are finished, call `client.Stop()`. You can provide a context with a given timeout in order to give the shutdown process a set amount of time to gracefully shutdown. For example:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
client.Stop(ctx)
```

This will wait up to 5 seconds for all workers to complete the task they are currently working on.

If you want to hard-stop the dispatcher, cancel the context that was provided when calling `client.Start()`.

## Roadmap

- Finish Web UI
- Hooks
- Expand processor context to store attempt number, other data
- Avoid needing to call Notify() when using transaction
- Queue priority
- Better handling of database schema, migrations
- Store queue stats in a separate table?
- Pause/resume queues
- Benchmarks
- Expand testing