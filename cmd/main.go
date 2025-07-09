package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/mikestefanello/backlite"
	"github.com/mikestefanello/backlite/ui"

	_ "github.com/mattn/go-sqlite3"
)

type NewOrderEmailTask struct {
	OrderID      string
	EmailAddress string
}

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

func main() {
	db, err := sql.Open("sqlite3", "data.db?_journal=WAL&_timeout=5000")
	if err != nil {
		panic(err)
	}

	client, err := backlite.NewClient(backlite.ClientConfig{
		DB:              db,
		Logger:          slog.Default(),
		ReleaseAfter:    10 * time.Minute,
		NumWorkers:      10,
		CleanupInterval: time.Hour,
	})

	ctx := context.Background()
	if err = client.Install(); err != nil {
		panic(err)
	}
	client.Start(ctx)
	defer client.Stop(ctx)

	task := NewOrderEmailTask{OrderID: "123", EmailAddress: "a@b.com"}
	taskIDs, err := client.
		Add(task).
		Ctx(ctx).
		Wait(1 * time.Second).
		Save()
	if err != nil {
		panic(err)
	}
	fmt.Println("Task IDs:", taskIDs)
	status, completion, err := client.Query(taskIDs[0])
	fmt.Println(status, backlite.IsInvalid(completion), err)

	processor := func(ctx context.Context, task NewOrderEmailTask) error {
		fmt.Printf("running task for email %s", task.EmailAddress)
		return nil
	}

	queue := backlite.NewQueue[NewOrderEmailTask](processor)

	client.Register(queue)

	mux := http.DefaultServeMux
	h, err := ui.NewHandler(ui.Config{
		DB: db,
	})
	h.Register(mux)
	err = http.ListenAndServe(":9000", mux)
	if err != nil {
		panic(err)
	}
}
