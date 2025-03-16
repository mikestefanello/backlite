package testutil

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	"github.com/mikestefanello/backlite/internal/query"
	"github.com/mikestefanello/backlite/internal/task"
)

func GetTasks(t *testing.T, db *sql.DB) task.Tasks {
	got, err := task.GetTasks(context.Background(), db, `
		SELECT 
			id, queue, task, attempts, wait_until, created_at, last_executed_at, claimed_at
		FROM 
			backlite_tasks
		ORDER BY
			id ASC
	`)

	if err != nil {
		t.Fatal(err)
	}

	return got
}

func InsertTask(t *testing.T, db *sql.DB, tk *task.Task) {
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}

	if err := tk.InsertTx(context.Background(), tx); err != nil {
		t.Fatal(err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TaskIDsExist(t *testing.T, db *sql.DB, ids []string) {
	idMap := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idMap[id] = struct{}{}
	}
	for _, tc := range GetTasks(t, db) {
		delete(idMap, tc.ID)
	}

	if len(idMap) != 0 {
		t.Errorf("ids do not exist: %v", idMap)
	}
}

func DeleteTasks(t *testing.T, db *sql.DB) {
	_, err := db.Exec("DELETE FROM backlite_tasks")
	if err != nil {
		t.Fatal(err)
	}
}

func GetCompletedTasks(t *testing.T, db *sql.DB) task.CompletedTasks {
	got, err := task.GetCompletedTasks(context.Background(), db, `
		SELECT 
			*
		FROM 
			backlite_tasks_completed
		ORDER BY
			id ASC
	`)

	if err != nil {
		t.Fatal(err)
	}

	return got
}

func DeleteCompletedTasks(t *testing.T, db *sql.DB) {
	_, err := db.Exec("DELETE FROM backlite_tasks_completed")
	if err != nil {
		t.Fatal(err)
	}
}

func InsertCompleted(t *testing.T, db *sql.DB, completed task.Completed) {
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}

	if err := completed.InsertTx(context.Background(), tx); err != nil {
		t.Fatal(err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func CompleteTaskIDsExist(t *testing.T, db *sql.DB, ids []string) {
	idMap := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idMap[id] = struct{}{}
	}
	for _, tc := range GetCompletedTasks(t, db) {
		delete(idMap, tc.ID)
	}

	if len(idMap) != 0 {
		t.Errorf("ids do not exist: %v", idMap)
	}
}

func Equal[T comparable](t *testing.T, name string, expected, got T) {
	if expected != got {
		t.Errorf("%s; expected %v, got %v", name, expected, got)
	}
}

func Length[T any](t *testing.T, obj []T, expectedLength int) {
	if len(obj) != expectedLength {
		t.Errorf("expected %d items, got %d", expectedLength, len(obj))
	}
}

func IsTask(t *testing.T, expected, got task.Task) {
	Equal(t, "Queue", expected.Queue, got.Queue)
	Equal(t, "Attempts", expected.Attempts, got.Attempts)
	Equal(t, "CreatedAt", expected.CreatedAt, got.CreatedAt)

	if !bytes.Equal(expected.Task, got.Task) {
		t.Error("Task bytes not equal")
	}

	switch {
	case expected.WaitUntil == nil && got.WaitUntil == nil:
	case expected.WaitUntil != nil && got.WaitUntil != nil:
		Equal(t, "WaitUntil", *expected.WaitUntil, *got.WaitUntil)
	default:
		t.Error("WaitUntil not equal")
	}

	switch {
	case expected.LastExecutedAt == nil && got.LastExecutedAt == nil:
	case expected.LastExecutedAt != nil && got.LastExecutedAt != nil:
		Equal(t, "LastExecutedAt", *expected.LastExecutedAt, *got.LastExecutedAt)
	default:
		t.Error("LastExecutedAt not equal")
	}

	switch {
	case expected.ClaimedAt == nil && got.ClaimedAt == nil:
	case expected.ClaimedAt != nil && got.ClaimedAt != nil:
		Equal(t, "ClaimedAt", *expected.ClaimedAt, *got.ClaimedAt)
	default:
		t.Error("ClaimedAt not equal")
	}
}

func Encode(t *testing.T, v any) []byte {
	b := bytes.NewBuffer(nil)
	err := json.NewEncoder(b).Encode(v)
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func Pointer[T any](v T) *T {
	return &v
}

func Wait() {
	time.Sleep(100 * time.Millisecond)
}

func NewDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:/%s?vfs=memdb&_timeout=1000", uuid.New().String()))
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(query.Schema)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func WaitForChan[T any](t *testing.T, signal chan T) {
	select {
	case <-signal:
	case <-time.After(500 * time.Millisecond):
		t.Error("signal not received")
	}
}
