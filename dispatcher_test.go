package backlite

import (
	"context"
	"testing"
	"time"

	"github.com/mikestefanello/backlite/internal/task"
	"github.com/mikestefanello/backlite/internal/testutil"
)

func TestDispatcher_Notify(t *testing.T) {
	d := dispatcher{
		ready: make(chan struct{}, 1),
	}

	d.Notify()
	select {
	case <-d.ready:
		t.Error("ready message was sent")
	default:
	}

	d.running.Store(true)
	d.Notify()
	select {
	case <-d.ready:
	default:
		t.Error("ready message was not sent")
	}
}

func TestDispatcher_Start(t *testing.T) {
	d := newDispatcher(t)

	// Start while already started.
	d.running.Store(true)
	d.Start(context.Background())
	testutil.Equal(t, "ctx", nil, d.ctx)
	testutil.Equal(t, "tasks channel", 0, cap(d.tasks))
	testutil.Equal(t, "ready channel", 0, cap(d.ready))
	testutil.Equal(t, "trigger channel", 0, cap(d.trigger))
	testutil.Equal(t, "available workers channel", 0, cap(d.availableWorkers))
	testutil.Equal(t, "available workers channel length", 0, len(d.availableWorkers))
	testutil.Equal(t, "running", true, d.running.Load())

	// Start when not yet started.
	d.running.Store(false)
	ctx, cancel := context.WithCancel(context.Background())
	d.Start(ctx)
	testutil.Equal(t, "ctx", ctx, d.ctx)
	testutil.Equal(t, "tasks channel", d.numWorkers, cap(d.tasks))
	testutil.Equal(t, "ready channel", 1000, cap(d.ready))
	testutil.Equal(t, "trigger channel", 10, cap(d.trigger))
	testutil.Equal(t, "available workers channel", d.numWorkers, cap(d.availableWorkers))
	testutil.Equal(t, "available workers channel length", d.numWorkers, len(d.availableWorkers))
	testutil.Equal(t, "running", true, d.running.Load())

	// Context cancel should shut down.
	cancel()
	testutil.Wait()
	testutil.Equal(t, "running", false, d.running.Load())

	// Check that the ready signal was sent by forcing the goroutines to close.
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	d.Start(ctx)
	testutil.WaitForChan(t, d.ready)
}

func TestDispatcher_Stop(t *testing.T) {
	d := newDispatcher(t)
	ctx := context.Background()

	// Not running.
	got := d.Stop(ctx)
	testutil.Equal(t, "shutdown", true, got)
	testutil.Equal(t, "running", false, d.running.Load())

	// All workers are free.
	d.Start(ctx)
	got = d.Stop(ctx)
	testutil.Wait()
	testutil.Equal(t, "shutdown", true, got)
	testutil.Equal(t, "running", false, d.running.Load())
	select {
	case <-d.shutdownCtx.Done():
	default:
		t.Error("shutdown context was not cancelled")
	}

	// One worker is not free.
	d.Start(ctx)
	<-d.availableWorkers
	ctx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	got = d.Stop(ctx)
	testutil.Wait()
	testutil.Equal(t, "shutdown", false, got)
	testutil.Equal(t, "running", false, d.running.Load())
}

func TestDispatcher_Triggerer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	d := &dispatcher{
		ready:       make(chan struct{}, 5),
		trigger:     make(chan struct{}, 5),
		shutdownCtx: context.Background(),
		ctx:         ctx,
	}
	go d.triggerer()

	// Send one and expect one trigger.
	d.ready <- struct{}{}
	testutil.WaitForChan(t, d.trigger)

	d.triggered.Store(false)

	// Send multiple and still expect only one.
	d.ready <- struct{}{}
	d.ready <- struct{}{}
	d.ready <- struct{}{}
	testutil.Wait()
	if len(d.trigger) != 1 {
		t.Fatalf("trigger contains %d, not 1", len(d.trigger))
	}

	<-d.trigger
	d.triggered.Store(false)

	// Shutdown main context and expect nothing.
	cancel()
	d.ready <- struct{}{}
	testutil.Wait()
	if len(d.trigger) != 0 {
		t.Fatalf("trigger contains %d, not 0", len(d.trigger))
	}

	// Shutdown graceful context and expect nothing.
	ctx, cancel = context.WithCancel(context.Background())
	d = &dispatcher{
		ready:       make(chan struct{}, 5),
		trigger:     make(chan struct{}, 5),
		shutdownCtx: ctx,
		ctx:         context.Background(),
	}
	go d.triggerer()

	cancel()
	testutil.Wait()
	d.ready <- struct{}{}
	testutil.Wait()
	if len(d.trigger) != 0 {
		t.Fatalf("trigger contains %d, not 0", len(d.trigger))
	}
}

func TestDispatcher_Cleaner(t *testing.T) {
	db := testutil.NewDB(t)
	defer db.Close()

	d := &dispatcher{
		numWorkers: 1,
		client:     &Client{db: db},
		log:        &noLogger{},
	}

	idsExist := func(ids []string) {
		idMap := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			idMap[id] = struct{}{}
		}
		for _, tc := range testutil.GetCompletedTasks(t, db) {
			delete(idMap, tc.ID)
		}

		if len(idMap) != 0 {
			t.Errorf("ids do not exist: %v", idMap)
		}
	}

	tc := task.Completed{
		Queue:          "test",
		Task:           nil,
		Attempts:       1,
		Succeeded:      false,
		LastDuration:   0,
		CreatedAt:      time.Now(),
		LastExecutedAt: time.Now(),
		Error:          nil,
	}

	// Disabled.
	d.Start(context.Background())
	tc.ID = "1"
	tc.ExpiresAt = testutil.Pointer(time.Now())
	testutil.InsertCompleted(t, db, tc)
	testutil.Wait()
	testutil.Equal(t, "", len(testutil.GetCompletedTasks(t, db)), 1)
	d.Stop(context.Background())
	testutil.Wait()

	// Enabled.
	d.cleanupInterval = 2 * time.Millisecond
	d.Start(context.Background())
	testutil.Wait()
	testutil.Equal(t, "", len(testutil.GetCompletedTasks(t, db)), 0)
	d.Stop(context.Background())
	testutil.Wait()

	// Enable again but with different expiration conditions.
	tc.ID = "2"
	tc.ExpiresAt = nil
	testutil.InsertCompleted(t, db, tc)
	tc.ID = "3"
	tc.ExpiresAt = testutil.Pointer(time.Now().Add(time.Hour))
	testutil.InsertCompleted(t, db, tc)
	tc.ID = "4"
	tc.ExpiresAt = testutil.Pointer(time.Now().Add(time.Millisecond))
	testutil.InsertCompleted(t, db, tc)
	tc.ID = "5"
	tc.ExpiresAt = testutil.Pointer(time.Now().Add(150 * time.Millisecond))
	testutil.InsertCompleted(t, db, tc)
	d.Start(context.Background())
	testutil.Wait()
	idsExist([]string{"2", "3", "5"})
	testutil.Wait()
	idsExist([]string{"2", "3"})
	d.Stop(context.Background())
}

func newDispatcher(t *testing.T) *dispatcher {
	return &dispatcher{
		numWorkers: 3,
		log:        &noLogger{},
		client:     mustNewClient(t),
	}
}
