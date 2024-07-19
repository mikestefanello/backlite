package backlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/mikestefanello/backlite/internal/task"
	"github.com/mikestefanello/backlite/internal/testutil"
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
	m := &mockDispatcher{}
	c.dispatcher = m
	err := c.Install()
	if err != nil {
		t.Fatal(err)
	}

	reset := func() {
		testutil.DeleteTasks(t, db)
		m = &mockDispatcher{}
		c.dispatcher = m
	}

	t.Run("single", func(t *testing.T) {
		defer reset()

		tk := testTask{Val: "a"}
		op := c.Add(tk)
		if err = op.Save(); err != nil {
			t.Fatal(err)
		}

		got := testutil.GetTasks(t, db)
		testutil.Length(t, got, 1)

		testutil.IsTask(t, task.Task{
			Queue:     tk.Config().Name,
			Task:      testutil.Encode(t, tk),
			Attempts:  0,
			CreatedAt: now(),
		}, *got[0])

		testutil.Equal(t, "notified", m.notified, true)
	})

	t.Run("wait", func(t *testing.T) {
		defer reset()

		tk := testTask{Val: "f"}
		op := c.Add(tk).Wait(time.Hour)

		if err = op.Save(); err != nil {
			t.Fatal(err)
		}

		got := testutil.GetTasks(t, db)
		testutil.Length(t, got, 1)

		testutil.IsTask(t, task.Task{
			Queue:     tk.Config().Name,
			Task:      testutil.Encode(t, tk),
			Attempts:  0,
			CreatedAt: now(),
			WaitUntil: testutil.Pointer(now().Add(time.Hour)),
		}, *got[0])
	})

	t.Run("multiple", func(t *testing.T) {
		defer reset()

		task1 := testTask{Val: "b"}
		task2 := testTask{Val: "c"}
		op := c.Add(task1, task2)
		if err = op.Save(); err != nil {
			t.Fatal(err)
		}

		got := testutil.GetTasks(t, db)
		testutil.Length(t, got, 2)

		testutil.IsTask(t, task.Task{
			Queue:     task1.Config().Name,
			Task:      testutil.Encode(t, task1),
			Attempts:  0,
			CreatedAt: now(),
		}, *got[0])

		testutil.IsTask(t, task.Task{
			Queue:     task2.Config().Name,
			Task:      testutil.Encode(t, task2),
			Attempts:  0,
			CreatedAt: now(),
		}, *got[1])
	})

	t.Run("context", func(t *testing.T) {
		defer reset()

		ctx, cancel := context.WithCancel(context.Background())
		tk := testTask{Val: "d"}
		op := c.Add(tk).Ctx(ctx)
		cancel()

		if err = op.Save(); !errors.Is(err, context.Canceled) {
			t.Error("expected context cancel")
		}
	})

	t.Run("transaction", func(t *testing.T) {
		defer reset()

		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}

		if err := c.Install(); err != nil {
			t.Fatal(err)
		}

		tk := testTask{Val: "e"}
		op := c.Add(tk).Tx(tx)

		if err = op.Save(); err != nil {
			t.Fatal(err)
		}

		testutil.Equal(t, "notified", m.notified, false)

		got := testutil.GetTasks(t, db)
		testutil.Length(t, got, 0)

		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}

		got = testutil.GetTasks(t, db)
		testutil.Length(t, got, 1)
	})
}
