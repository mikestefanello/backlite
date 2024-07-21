## Backlite: Type-safe, persistent, embedded task queues and background job runner w/ SQLite

[![Go Report Card](https://goreportcard.com/badge/github.com/mikestefanello/backlite)](https://goreportcard.com/report/github.com/mikestefanello/backlite)
[![Test](https://github.com/mikestefanello/backlite/actions/workflows/test.yml/badge.svg)](https://github.com/mikestefanello/backlite/actions/workflows/test.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Reference](https://pkg.go.dev/badge/github.com/mikestefanello/backlite.svg)](https://pkg.go.dev/github.com/mikestefanello/backlite)
[![GoT](https://img.shields.io/badge/Made%20with-Go-1f425f.svg)](https://go.dev)

<p align="center"><img alt="Logo" src="todo" height="200px"/></p>

## Table of Contents
* [Introduction](#introduction)
    * [Overview](#overview)
    * [Origin](#origin)
    * [Screenshots](#screenshots)
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
    * [Declaring a Task type](#declaring-a-task-type)
    * [Queue processor](#queue-processor)
    * [Registering a queue](#registering-a-queue)
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

Coming soon.

## Installation

Install by simply running: `go get github.com/mikestefanello/backlite`

## Features

### Type-safety

No need to deal with serialization and byte slices. By leveraging generics, tasks and queues are completely type-safe which means that you pass in your task type in to a queue and your task processor callbacks will only receive that given type.
 
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

The task dispatcher, which handles sending tasks to the worker pool for execution, can be shut-down gracefully by calling `Stop()` on the client. That will wait for all workers to finish for as long as the passed in context is not cancelled. The hard-stop the dispatcher, cancel the context passed in when calling `Start()`. See usage below.

### Transactions

Task creation can be added to a given database transaction. If you are using SQLite as your primary database, this provides a simple, robust way to ensure data integrity. For example, using the eCommerce app example, when inserting a new order in to your database, the same transaction to be used to add a task to send an order notification email, and they either both succeed or both fail. Use the chained method `Tx()` to provide your transaction when adding one or multiple tasks.

### No database polling

Since SQLite only supports one writer, no continuous database polling is required. The task dispatcher is able to remain aware of new tasks and keep track of when future tasks are scheduled for, and thus only queries the database when it needs to.

### Driver flexibility

Use any SQLite driver that you'd like. This library only include [go-sqlite3](https://github.com/mattn/go-sqlite3) since it is used in tests.

### Bulk inserts

Insert one or many tasks across one or many queues in a single operation.

### Execution timeout

Each queue can be configured with an execution timeout for processing a given task. The provided context will cancel after the time elapses. If you want to respect that timeout, your processor code will have to listen for the context cancellation.

### Background worker pool

When creating a client, you can specify the amount of goroutines to use to build a worker pool. This pool is created and shutdown via the dispatcher by calling `Start()` and `Stop()` on the client. The worker pool is the only way to process tasks; they cannot be pulled manually.

### Web UI

A simple web UI to monitor running, upcoming, and completed tasks is provided but under active development.

## Usage


### Client initialization



### Declaring a Task type



### Queue processor



### Registering a queue



### Starting the dispatcher



### Shutting down the dispatcher



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