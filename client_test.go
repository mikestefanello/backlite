package backlite

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/mikestefanello/backlite/internal/task"
	"github.com/mikestefanello/backlite/internal/testutil"
)

type mockDispatcher struct {
	started      bool
	stopped      bool
	gracefulStop bool
	notified     bool
}

func (d *mockDispatcher) Start(_ context.Context) {
	d.started = true
}

func (d *mockDispatcher) Stop(_ context.Context) bool {
	d.stopped = true
	return d.gracefulStop
}

func (d *mockDispatcher) Notify() {
	d.notified = true
}

func TestMain(m *testing.M) {
	n := time.Now().Round(time.Millisecond)
	now = func() time.Time {
		return n
	}

	os.Exit(m.Run())
}

func TestNewClient(t *testing.T) {
	db := testutil.NewDB(t)
	defer db.Close()

	c, err := NewClient(ClientConfig{
		DB:              db,
		Logger:          slog.Default(),
		NumWorkers:      2,
		ReleaseAfter:    time.Second,
		CleanupInterval: time.Hour,
	})

	if err != nil {
		t.Fatal(err)
	}

	if c.dispatcher == nil {
		t.Fatal("dispatcher is nil")
	}

	d, ok := c.dispatcher.(*dispatcher)
	if !ok {
		t.Fatal("dispatcher not set")
	}

	if c.log != slog.Default() {
		t.Error("log wrong value")
	}

	testutil.Equal(t, "client", c, d.client)
	testutil.Equal(t, "db", db, c.db)
	testutil.Equal(t, "log", d.log, c.log)
	testutil.Equal(t, "workers", 2, d.numWorkers)
	testutil.Equal(t, "release after", time.Second, d.releaseAfter)
	testutil.Equal(t, "cleanup interval", time.Hour, d.cleanupInterval)
}

func TestNewClient__DefaultLogger(t *testing.T) {
	c := mustNewClient(t)

	if c.log == nil {
		t.Fatal("log is nil")
	}

	_, ok := c.log.(*noLogger)
	if !ok {
		t.Error("log not set to noLogger")
	}
}

func TestNewClient__Validation(t *testing.T) {
	db := testutil.NewDB(t)
	defer db.Close()

	_, err := NewClient(ClientConfig{
		DB:           nil,
		NumWorkers:   1,
		ReleaseAfter: time.Second,
	})
	if err == nil {
		t.Error("expected error, got none")
	}

	_, err = NewClient(ClientConfig{
		DB:           db,
		NumWorkers:   0,
		ReleaseAfter: time.Second,
	})
	if err == nil {
		t.Error("expected error, got none")
	}

	_, err = NewClient(ClientConfig{
		DB:           db,
		NumWorkers:   1,
		ReleaseAfter: time.Duration(0),
	})
	if err == nil {
		t.Error("expected error, got none")
	}
}

func TestClient_Register(t *testing.T) {
	c := mustNewClient(t)

	q := NewQueue[testTask](func(_ context.Context, _ testTask) error {
		return nil
	})
	c.Register(q)

	var panicked bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		c.Register(q)
	}()

	if !panicked {
		t.Error("expected panic")
	}

	q = NewQueue[testTaskNoName](func(_ context.Context, _ testTaskNoName) error {
		return nil
	})
	panicked = false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		c.Register(q)
	}()

	if !panicked {
		t.Error("expected panic")
	}
}

func TestClient_Install(t *testing.T) {
	c := mustNewClient(t)

	err := c.Install()
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.db.Exec("SELECT 1 FROM backlite_tasks")
	if err != nil {
		t.Error("table backlite_tasks not created")
	}

	_, err = c.db.Exec("SELECT 1 FROM backlite_tasks_completed")
	if err != nil {
		t.Error("table backlite_tasks_completed not created")
	}
}

func TestClient_Add(t *testing.T) {
	c := mustNewClient(t)

	t1, t2 := testTask{}, testTask{}
	op := c.Add(t1, t2)

	if op.client != c {
		t.Error("client not set")
	}

	if len(op.tasks) != 2 {
		t.Error("tasks not set")
	} else {
		if op.tasks[0] != t1 || op.tasks[1] != t2 {
			t.Error("tasks do not match")
		}
	}
}

func TestClient_Start(t *testing.T) {
	c := mustNewClient(t)
	m := &mockDispatcher{}
	c.dispatcher = m

	c.Start(context.Background())
	testutil.Equal(t, "started", true, m.started)
}

func TestClient_Stop(t *testing.T) {
	c := mustNewClient(t)
	m := &mockDispatcher{}
	c.dispatcher = m

	c.Stop(context.Background())
	testutil.Equal(t, "stopped", true, m.stopped)
	testutil.Equal(t, "graceful", false, m.gracefulStop)

	m.stopped = false
	m.gracefulStop = true
	c.Stop(context.Background())
	testutil.Equal(t, "stopped", true, m.stopped)
	testutil.Equal(t, "graceful", true, m.gracefulStop)
}

func TestClient_Notify(t *testing.T) {
	c := mustNewClient(t)
	m := &mockDispatcher{}
	c.dispatcher = m

	c.Notify()
	testutil.Equal(t, "notified", true, m.notified)
}

func TestClient_FromContext(t *testing.T) {
	got := FromContext(context.Background())
	testutil.Equal(t, "client", got, nil)

	c := &Client{}
	ctx := context.WithValue(context.Background(), ctxKeyClient{}, c)
	got = FromContext(ctx)
	testutil.Equal(t, "client", got, c)
}

func TestClient_Status(t *testing.T) {
	c := mustNewClient(t)

	tk := &task.Task{
		ID:    "a",
		Queue: "test",
		Task:  []byte("test"),
	}

	s, err := c.Status(context.Background(), tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Equal(t, "status", TaskStatusNotFound, s)

	testutil.InsertTask(t, c.db, tk)
	s, err = c.Status(context.Background(), tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Equal(t, "status", TaskStatusPending, s)

	err = task.Tasks{tk}.Claim(context.Background(), c.db)
	if err != nil {
		t.Fatal(err)
	}
	s, err = c.Status(context.Background(), tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Equal(t, "status", TaskStatusRunning, s)

	ctk := task.Completed{
		ID:    "b",
		Queue: "test",
	}
	testutil.InsertCompleted(t, c.db, ctk)
	s, err = c.Status(context.Background(), ctk.ID)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Equal(t, "status", TaskStatusSuccess, s)

	errStr := "err"
	ctk = task.Completed{
		ID:    "c",
		Queue: "test",
		Error: &errStr,
	}
	testutil.InsertCompleted(t, c.db, ctk)
	s, err = c.Status(context.Background(), ctk.ID)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Equal(t, "status", TaskStatusFailure, s)
}

func mustNewClient(t *testing.T) *Client {
	client, err := NewClient(ClientConfig{
		DB:              testutil.NewDB(t),
		NumWorkers:      1,
		ReleaseAfter:    time.Hour,
		CleanupInterval: 6 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	return client
}
