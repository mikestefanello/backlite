package testutil

import (
	"database/sql"
	"sync"
	"testing"

	"github.com/mikestefanello/backlite/internal/task"
)

func TestDBFail(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	test := func(name string, tester func(t *testing.T)) {
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			st := &testing.T{}
			tester(st)

			if !st.Failed() {
				t.Fatalf("expected %s to fail", name)
			}
		}()
		wg.Wait()
	}

	test("GetTasks", func(t *testing.T) {
		GetTasks(t, db)
	})

	test("InsertTask", func(t *testing.T) {
		InsertTask(t, db, &task.Task{})
	})

	test("DeleteTasks", func(t *testing.T) {
		DeleteTasks(t, db)
	})

	test("GetCompletedTasks", func(t *testing.T) {
		GetCompletedTasks(t, db)
	})

	test("DeleteCompletedTasks", func(t *testing.T) {
		DeleteCompletedTasks(t, db)
	})

	test("InsertCompleted", func(t *testing.T) {
		InsertCompleted(t, db, task.Completed{})
	})
}

func TestWaitForChan(t *testing.T) {
	c := make(chan int, 1)
	st := &testing.T{}
	c <- 1
	WaitForChan(st, c)
	if st.Failed() {
		t.Fatalf("should not have failed")
	}

	WaitForChan(st, c)
	if !st.Failed() {
		t.Fatalf("should have failed")
	}
}

func TestLength(t *testing.T) {
	st := &testing.T{}
	obj := []int{1, 2}
	Length(st, obj, 2)
	if st.Failed() {
		t.Error("should not have failed")
	}
	Length(st, obj, 1)
	if !st.Failed() {
		t.Error("should have failed")
	}
}

func TestEqual(t *testing.T) {
	st := &testing.T{}
	Equal(st, "t", "a", "a")
	if st.Failed() {
		t.Error("should not have failed")
	}
	Equal(st, "t", "a", "b")
	if !st.Failed() {
		t.Error("should have failed")
	}
}

func TestCompleteTaskIDsExist(t *testing.T) {
	st := &testing.T{}
	db := NewDB(t)
	InsertCompleted(t, db, task.Completed{ID: "1"})
	InsertCompleted(t, db, task.Completed{ID: "2"})
	CompleteTaskIDsExist(st, db, []string{"1", "2"})
	if st.Failed() {
		t.Error("should not have failed")
	}
	CompleteTaskIDsExist(st, db, []string{"1", "2", "3"})
	if !st.Failed() {
		t.Error("should have failed")
	}
}

func TestTaskIDsExist(t *testing.T) {
	st := &testing.T{}
	db := NewDB(t)
	InsertTask(t, db, &task.Task{ID: "1", Task: []byte("a")})
	InsertTask(t, db, &task.Task{ID: "2", Task: []byte("a")})
	TaskIDsExist(st, db, []string{"1", "2"})
	if st.Failed() {
		t.Error("should not have failed")
	}
	TaskIDsExist(st, db, []string{"1", "2", "3"})
	if !st.Failed() {
		t.Error("should have failed")
	}
}
