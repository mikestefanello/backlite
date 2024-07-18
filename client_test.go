package backlite

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/mikestefanello/backlite/internal/testutil"

	_ "github.com/mattn/go-sqlite3"
)

var (
	db *sql.DB
)

type testTask struct {
	Val string
}

func (t testTask) Config() QueueConfig {
	return QueueConfig{
		Name:        "test",
		MaxAttempts: 1,
	}
}

func TestMain(m *testing.M) {
	var err error

	db, err = sql.Open("sqlite3", ":memory:?_journal=WAL&_timeout=1000")
	if err != nil {
		panic(err)
	}

	n := time.Now().Round(time.Second)
	now = func() time.Time {
		return n
	}

	os.Exit(m.Run())
}

func TestNewClient(t *testing.T) {
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

	if c.db != db {
		t.Errorf("db not set")
	}

	if c.log != slog.Default() {
		t.Errorf("logger not set")
	}

	if c.dispatcher == nil {
		t.Fatal("dispatcher is nil")
	}

	if c.dispatcher.client != c {
		t.Error("dispatcher client not set")
	}

	if c.dispatcher.log != c.log {
		t.Error("dispatcher log not set")
	}

	if c.dispatcher.numWorkers != 2 {
		t.Error("dispatcher numWorkers not set")
	}

	if c.dispatcher.releaseAfter != time.Second {
		t.Error("dispatcher releaseAfter not set")
	}

	if c.dispatcher.cleanupInterval != time.Hour {
		t.Error("dispatcher cleanupInterval not set")
	}
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
}

func TestClient_Install(t *testing.T) {
	c := mustNewClient(t)

	err := c.Install()
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("SELECT 1 FROM backlite_tasks")
	if err != nil {
		t.Error("table backlite_tasks not created")
	}

	_, err = db.Exec("SELECT 1 FROM backlite_tasks_completed")
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
	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)
	testutil.Equal(t, "ctx", ctx, c.dispatcher.ctx)
	testutil.Equal(t, "tasks channel", cap(c.dispatcher.tasks), c.dispatcher.numWorkers)
	testutil.Equal(t, "ready channel", cap(c.dispatcher.ready), 1000)
	testutil.Equal(t, "trigger channel", cap(c.dispatcher.trigger), 10)
	testutil.Equal(t, "available workers channel", cap(c.dispatcher.availableWorkers), c.dispatcher.numWorkers)
	testutil.Equal(t, "available workers channel length", len(c.dispatcher.availableWorkers), c.dispatcher.numWorkers)
	testutil.Equal(t, "running", c.dispatcher.running.Load(), true)

	cancel()
	// TODO need to wait...
	testutil.Equal(t, "running", c.dispatcher.running.Load(), false)
}

func TestClient_Stop(t *testing.T) {
	c := mustNewClient(t)
	ctx := context.Background()
	c.Start(ctx)
	got := c.Stop(ctx)
	testutil.Equal(t, "", got, true)

	c = mustNewClient(t)
	c.Start(ctx)
	<-c.dispatcher.availableWorkers
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
	defer cancel()
	got = c.Stop(ctx)
	testutil.Equal(t, "", got, false)

	// TODO somehow this is failing randomly
}

func TestClient_Notify(t *testing.T) {
	c := mustNewClient(t)

	c.dispatcher.running.Store(true)
	c.dispatcher.ready = make(chan struct{}, 1)
	c.Notify()

	select {
	case <-c.dispatcher.ready:
	default:
		t.Error("ready signal not sent")
	}
}

func TestClient_FromContext(t *testing.T) {
	got := FromContext(context.Background())
	testutil.Equal(t, "client", got, nil)

	c := &Client{}
	ctx := context.WithValue(context.Background(), ctxKeyClient{}, c)
	got = FromContext(ctx)
	testutil.Equal(t, "client", got, c)
}

func mustNewClient(t *testing.T) *Client {
	client, err := NewClient(ClientConfig{
		DB:              db,
		NumWorkers:      1,
		ReleaseAfter:    time.Hour,
		CleanupInterval: 6 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	return client
}

func newDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:?_journal=WAL&_timeout=1000")
	if err != nil {
		t.Fatal(err)
	}
	return db
}
