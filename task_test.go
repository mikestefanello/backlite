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

func TestTaskAddOp_Save__Single(t *testing.T) {
	c := mustNewClient(t)
	m := &mockDispatcher{}
	c.dispatcher = m
	defer c.db.Close()

	tk := testTask{Val: "a"}
	op := c.Add(tk)
	if err := op.Save(); err != nil {
		t.Fatal(err)
	}

	got := testutil.GetTasks(t, c.db)
	testutil.Length(t, got, 1)

	testutil.IsTask(t, task.Task{
		Queue:     tk.Config().Name,
		Task:      testutil.Encode(t, tk),
		Attempts:  0,
		CreatedAt: now(),
	}, *got[0])

	testutil.Equal(t, "notified", true, m.notified)
}

func TestTaskAddOp_Save__Wait(t *testing.T) {
	c := mustNewClient(t)
	m := &mockDispatcher{}
	c.dispatcher = m
	defer c.db.Close()

	tk := testTask{Val: "f"}
	op := c.Add(tk).Wait(time.Hour)

	if err := op.Save(); err != nil {
		t.Fatal(err)
	}

	got := testutil.GetTasks(t, c.db)
	testutil.Length(t, got, 1)

	testutil.IsTask(t, task.Task{
		Queue:     tk.Config().Name,
		Task:      testutil.Encode(t, tk),
		Attempts:  0,
		CreatedAt: now(),
		WaitUntil: testutil.Pointer(now().Add(time.Hour)),
	}, *got[0])
}

func TestTaskAddOp_Save__Multiple(t *testing.T) {
	c := mustNewClient(t)
	m := &mockDispatcher{}
	c.dispatcher = m
	defer c.db.Close()

	task1 := testTask{Val: "b"}
	task2 := testTask{Val: "c"}
	op := c.Add(task1, task2)
	if err := op.Save(); err != nil {
		t.Fatal(err)
	}

	got := testutil.GetTasks(t, c.db)
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
}

func TestTaskAddOp_Save__Context(t *testing.T) {
	c := mustNewClient(t)
	m := &mockDispatcher{}
	c.dispatcher = m
	defer c.db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	tk := testTask{Val: "d"}
	op := c.Add(tk).Ctx(ctx)
	cancel()

	if err := op.Save(); !errors.Is(err, context.Canceled) {
		t.Error("expected context cancel")
	}
}

func TestTaskAddOp_Save__Transaction(t *testing.T) {
	c := mustNewClient(t)
	m := &mockDispatcher{}
	c.dispatcher = m
	defer c.db.Close()

	tx, err := c.db.Begin()
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

	testutil.Equal(t, "notified", false, m.notified)

	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	got := testutil.GetTasks(t, c.db)
	testutil.Length(t, got, 1)
}

func TestTaskAddOp_Save__EncodeFailure(t *testing.T) {
	c := mustNewClient(t)
	m := &mockDispatcher{}
	c.dispatcher = m
	defer c.db.Close()

	tx, err := c.db.Begin()
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Install(); err != nil {
		t.Fatal(err)
	}

	tk := testTaskEncodeFail{Val: make(chan int)}
	op := c.Add(tk).Tx(tx)

	if err = op.Save(); err == nil {
		t.Error("expected error")
	}
}
