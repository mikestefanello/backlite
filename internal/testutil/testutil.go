package testutil

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"github.com/mikestefanello/backlite/internal/task"
	"testing"
)

func GetTasks(t *testing.T, db *sql.DB) task.Tasks {
	got, err := task.GetTasks(context.Background(), db, `
		SELECT 
			id, queue, task, attempts, wait_until, created_at
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

func DeleteTasks(t *testing.T, db *sql.DB) {
	_, err := db.Exec("DELETE FROM backlite_tasks")
	if err != nil {
		t.Fatal(err)
	}
}

func Equal[T comparable](t *testing.T, name string, a, b T) {
	if a != b {
		t.Errorf("%s not equal; %v != %v", name, a, b)
	}
}

func Length[T any](t *testing.T, obj []T, expectedLength int) {
	if len(obj) != expectedLength {
		t.Errorf("got %d items, expected %d", len(obj), expectedLength)
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
