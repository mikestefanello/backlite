package backlite

import (
	"bytes"
	"context"
	"errors"
	"strconv"
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

func TestDispatcher_ProcessTask__Context(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var innerCtx context.Context
	d := newDispatcher(t)
	d.ctx = ctx
	var called bool

	d.client.Register(NewQueue[testTask](func(ctx context.Context, _ testTask) error {
		called = true
		innerCtx = ctx
		deadline, ok := ctx.Deadline()
		testutil.Equal(t, "deadline set", true, ok)
		testutil.Equal(t, "client", d.client, FromContext(ctx))

		if deadline.Sub(now()) != time.Second {
			t.Error("ctx deadline too large")
		}
		return nil
	}))

	d.processTask(&task.Task{
		ID:        "1",
		Queue:     "test",
		Task:      testutil.Encode(t, &testTask{Val: "1"}),
		Attempts:  1,
		CreatedAt: time.Now(),
	})
	testutil.Equal(t, "called", true, called)

	cancel()
	select {
	case <-innerCtx.Done():
	default:
		t.Error("cancel did not cancel inner context")
	}
}

func TestDispatcher_ProcessTask__Success(t *testing.T) {
	d := newDispatcher(t)
	d.ready = make(chan struct{}, 1)
	d.ctx = context.Background()
	var called bool

	d.client.Register(NewQueue[testTask](func(ctx context.Context, tk testTask) error {
		called = true
		testutil.Equal(t, "task val", "1", tk.Val)
		return nil
	}))

	tk := &task.Task{
		ID:        "4",
		Queue:     "test",
		Task:      testutil.Encode(t, &testTask{Val: "1"}),
		Attempts:  1,
		CreatedAt: now(),
	}
	testutil.InsertTask(t, d.client.db, tk)

	d.processTask(tk)
	testutil.Equal(t, "called", true, called)

	got := testutil.GetTasks(t, d.client.db)
	testutil.Length(t, got, 0)
	testutil.Equal(t, "ready", 0, len(d.ready))

	ct := testutil.GetCompletedTasks(t, d.client.db)
	testutil.Length(t, ct, 1)

	testutil.Equal(t, "id", "4", ct[0].ID)
	testutil.Equal(t, "queue", "test", ct[0].Queue)
	testutil.Equal(t, "attempts", 1, ct[0].Attempts)
	testutil.Equal(t, "succeeded", true, ct[0].Succeeded)
	testutil.Equal(t, "created at", now(), ct[0].CreatedAt)
	testutil.Equal(t, "last executed at", now(), ct[0].LastExecutedAt)
	testutil.Equal(t, "expires at", now().Add(time.Hour), *ct[0].ExpiresAt)
	testutil.Equal(t, "error", nil, ct[0].Error)

	if !bytes.Equal(ct[0].Task, tk.Task) {
		t.Error("task does not match")
	}

	if ct[0].LastDuration <= 0 {
		t.Error("last duration not set")
	}
}

func TestDispatcher_ProcessTask__NoRention(t *testing.T) {
	d := newDispatcher(t)
	d.ready = make(chan struct{}, 1)
	d.ctx = context.Background()

	d.client.Register(NewQueue[testTaskNoRention](func(ctx context.Context, _ testTaskNoRention) error {
		return nil
	}))

	tk := &task.Task{
		ID:        "5",
		Queue:     "test-noret",
		Task:      testutil.Encode(t, &testTaskNoRention{Val: "1"}),
		Attempts:  1,
		CreatedAt: now(),
	}
	testutil.InsertTask(t, d.client.db, tk)

	d.processTask(tk)

	got := testutil.GetTasks(t, d.client.db)
	testutil.Length(t, got, 0)

	ct := testutil.GetCompletedTasks(t, d.client.db)
	testutil.Length(t, ct, 0)
}

func TestDispatcher_ProcessTask__RetainNoData(t *testing.T) {
	d := newDispatcher(t)
	d.ready = make(chan struct{}, 1)
	d.ctx = context.Background()

	d.client.Register(NewQueue[testTaskRetainNoData](func(ctx context.Context, _ testTaskRetainNoData) error {
		return nil
	}))

	tk := &task.Task{
		ID:        "5",
		Queue:     "test-retainnodata",
		Task:      testutil.Encode(t, &testTaskRetainNoData{Val: "1"}),
		Attempts:  1,
		CreatedAt: now(),
	}
	testutil.InsertTask(t, d.client.db, tk)

	d.processTask(tk)

	got := testutil.GetTasks(t, d.client.db)
	testutil.Length(t, got, 0)

	ct := testutil.GetCompletedTasks(t, d.client.db)
	testutil.Length(t, ct, 1)

	if ct[0].Task != nil {
		t.Error("task data shouldn't have been retained")
	}
}

func TestDispatcher_ProcessTask__RetainForever(t *testing.T) {
	d := newDispatcher(t)
	d.ready = make(chan struct{}, 1)
	d.ctx = context.Background()

	d.client.Register(NewQueue[testTaskRentainForever](func(ctx context.Context, _ testTaskRentainForever) error {
		return nil
	}))

	tk := &task.Task{
		ID:        "5",
		Queue:     "test-retainforever",
		Task:      testutil.Encode(t, &testTaskRentainForever{Val: "1"}),
		Attempts:  1,
		CreatedAt: now(),
	}
	testutil.InsertTask(t, d.client.db, tk)

	d.processTask(tk)

	got := testutil.GetTasks(t, d.client.db)
	testutil.Length(t, got, 0)

	ct := testutil.GetCompletedTasks(t, d.client.db)
	testutil.Length(t, ct, 1)
	testutil.Equal(t, "expires at", nil, ct[0].ExpiresAt)
}

func TestDispatcher_ProcessTask__RetainDataFailed(t *testing.T) {
	d := newDispatcher(t)
	d.ready = make(chan struct{}, 1)
	d.ctx = context.Background()
	var succeed bool

	d.client.Register(NewQueue[testTaskRetainDataFailed](func(ctx context.Context, _ testTaskRetainDataFailed) error {
		if succeed {
			return nil
		}
		return errors.New("fail")
	}))

	for _, b := range []bool{true, false} {
		succeed = b
		tk := &task.Task{
			ID:        strconv.FormatBool(succeed),
			Queue:     "test-retaindatafailed",
			Task:      testutil.Encode(t, &testTaskRetainDataFailed{Val: "1"}),
			Attempts:  2,
			CreatedAt: now(),
		}
		testutil.InsertTask(t, d.client.db, tk)

		d.processTask(tk)

		got := testutil.GetTasks(t, d.client.db)
		testutil.Length(t, got, 0)

		ct := testutil.GetCompletedTasks(t, d.client.db)
		testutil.Length(t, ct, 1)

		if succeed {
			if ct[0].Task != nil {
				t.Error("task data shouldn't have been retained")
			}
		} else {
			if ct[0].Task == nil {
				t.Error("task data should have been retained")
			}
		}

		testutil.DeleteCompletedTasks(t, d.client.db)
	}
}

func TestDispatcher_ProcessTask__RetainFailed(t *testing.T) {
	d := newDispatcher(t)
	d.ready = make(chan struct{}, 1)
	d.ctx = context.Background()
	var succeed bool

	d.client.Register(NewQueue[testTaskRetainFailed](func(ctx context.Context, _ testTaskRetainFailed) error {
		if succeed {
			return nil
		}
		return errors.New("fail")
	}))

	for _, b := range []bool{true, false} {
		succeed = b
		tk := &task.Task{
			ID:        strconv.FormatBool(succeed),
			Queue:     "test-retainfailed",
			Task:      testutil.Encode(t, &testTaskRetainFailed{Val: "1"}),
			Attempts:  2,
			CreatedAt: now(),
		}
		testutil.InsertTask(t, d.client.db, tk)

		d.processTask(tk)

		got := testutil.GetTasks(t, d.client.db)
		testutil.Length(t, got, 0)

		ct := testutil.GetCompletedTasks(t, d.client.db)

		if succeed {
			testutil.Length(t, ct, 0)
		} else {
			testutil.Length(t, ct, 1)
		}
	}
}

func TestDispatcher_ProcessTask__Panic(t *testing.T) {
	d := newDispatcher(t)
	d.ready = make(chan struct{}, 1)
	d.ctx = context.Background()
	var called bool

	d.client.Register(NewQueue[testTask](func(ctx context.Context, _ testTask) error {
		called = true
		panic("panic called")
		return nil
	}))

	tk := &task.Task{
		ID:        "2",
		Queue:     "test",
		Task:      testutil.Encode(t, &testTask{Val: "1"}),
		CreatedAt: now(),
	}
	testutil.InsertTask(t, d.client.db, tk)

	d.processTask(tk)
	testutil.Equal(t, "called", true, called)
	testutil.WaitForChan(t, d.ready)

	got := testutil.GetTasks(t, d.client.db)
	testutil.Length(t, got, 1)
	testutil.Equal(t, "last executed at", now(), *got[0].LastExecutedAt)
	testutil.Equal(t, "wait until", now().Add(5*time.Millisecond), *got[0].WaitUntil)
}

func TestDispatcher_ProcessTask__Failure(t *testing.T) {
	d := newDispatcher(t)
	d.ready = make(chan struct{}, 1)
	d.ctx = context.Background()
	var called bool

	d.client.Register(NewQueue[testTask](func(ctx context.Context, _ testTask) error {
		called = true
		return errors.New("failure error")
	}))

	tk := &task.Task{
		ID:        "3",
		Queue:     "test",
		Task:      testutil.Encode(t, &testTask{Val: "1"}),
		Attempts:  1,
		CreatedAt: now(),
	}
	testutil.InsertTask(t, d.client.db, tk)

	// First attempt.
	d.processTask(tk)
	testutil.Equal(t, "called", true, called)
	testutil.WaitForChan(t, d.ready)

	got := testutil.GetTasks(t, d.client.db)
	testutil.Length(t, got, 1)
	testutil.Equal(t, "last executed at", now(), *got[0].LastExecutedAt)
	testutil.Equal(t, "wait until", now().Add(5*time.Millisecond), *got[0].WaitUntil)

	// Final attempt.
	called = false
	tk.Attempts++
	d.processTask(tk)
	testutil.Equal(t, "called", true, called)
	testutil.Equal(t, "ready", 0, len(d.ready))
	got = testutil.GetTasks(t, d.client.db)
	testutil.Length(t, got, 0)

	ct := testutil.GetCompletedTasks(t, d.client.db)
	testutil.Length(t, ct, 1)

	testutil.Equal(t, "id", "3", ct[0].ID)
	testutil.Equal(t, "queue", "test", ct[0].Queue)
	testutil.Equal(t, "attempts", 2, ct[0].Attempts)
	testutil.Equal(t, "succeeded", false, ct[0].Succeeded)
	testutil.Equal(t, "created at", now(), ct[0].CreatedAt)
	testutil.Equal(t, "last executed at", now(), ct[0].LastExecutedAt)
	testutil.Equal(t, "expires at", now().Add(time.Hour), *ct[0].ExpiresAt)
	testutil.Equal(t, "error", "failure error", *ct[0].Error)

	if !bytes.Equal(ct[0].Task, tk.Task) {
		t.Error("task does not match")
	}

	if ct[0].LastDuration <= 0 {
		t.Error("last duration not set")
	}
}

func newDispatcher(t *testing.T) *dispatcher {
	return &dispatcher{
		numWorkers: 3,
		log:        &noLogger{},
		client:     mustNewClient(t),
	}
}
