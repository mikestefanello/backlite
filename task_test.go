package backlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/mikestefanello/backlite/internal/task"
)

func TestTaskAddOp_Ctx(t *testing.T) {
	op := &TaskAddOp{}
	ctx := context.Background()
	op.Ctx(ctx)

	switch {
	case op.ctx == nil:
		t.Errorf("ctx is nil")
	case op.ctx != ctx:
		t.Error("ctx wrong value")
	}
}

func TestTaskAddOp_At(t *testing.T) {
	op := &TaskAddOp{}
	at := time.Now()
	op.At(at)

	switch {
	case op.wait == nil:
		t.Error("wait is nil")
	case *op.wait != at:
		t.Error("wait wrong value")
	}
}

func TestTaskAddOp_Wait(t *testing.T) {
	op := &TaskAddOp{}
	wait := time.Hour
	op.Wait(wait)

	switch {
	case op.wait == nil:
		t.Error("wait is nil")
	case !op.wait.Equal(now().Add(time.Hour)):
		t.Error("wait wrong value")
	}
}

func TestTaskAddOp_Tx(t *testing.T) {
	op := &TaskAddOp{}
	tx := &sql.Tx{}
	op.Tx(tx)

	switch {
	case op.tx == nil:
		t.Error("tx is nil")
	case op.tx != tx:
		t.Error("tx wrong value")
	}
}

func TestTaskAddOp_Save(t *testing.T) {
	c := mustNewClient(t)
	err := c.Install()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("single", func(t *testing.T) {
		tt := testTask{Val: "a"}
		op := c.Add(tt)
		if err = op.Save(); err != nil {
			t.Fatal(err)
		}

		got := getTasks(t)

		if len(got) != 1 {
			t.Fatalf("expected 1 task, got %d", len(got))
		}

	})

	t.Run("multiple", func(t *testing.T) {
		task1 := testTask{Val: "b"}
		task2 := testTask{Val: "c"}
		op := c.Add(task1, task2)
		if err = op.Save(); err != nil {
			t.Fatal(err)
		}

		// TODO
	})

	t.Run("context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		task := testTask{Val: "d"}
		op := c.Add(task).Ctx(ctx)
		cancel()

		if err = op.Save(); !errors.Is(err, context.Canceled) {
			t.Error("expected context cancel")
		}
	})

	t.Run("transaction", func(t *testing.T) {
		tx, err := c.db.Begin()
		if err != nil {
			t.Fatal(err)
		}

		task := testTask{Val: "e"}
		op := c.Add(task).Tx(tx)

		if err = op.Save(); err != nil {
			t.Fatal(err)
		}

		// TODO expect nothing yet
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
		// TODO
	})

	t.Run("wait", func(t *testing.T) {
		task := testTask{Val: "f"}
		op := c.Add(task).Wait(time.Hour)

		if err = op.Save(); err != nil {
			t.Fatal(err)
		}

		// TODO
	})
}

func getTasks(t *testing.T) task.Tasks {
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
