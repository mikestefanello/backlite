package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mikestefanello/backlite/internal/task"
	"github.com/mikestefanello/backlite/internal/testutil"
)

var (
	h            *Handler
	mux          *http.ServeMux
	now          = time.Now()
	tasksRunning = task.Tasks{
		{
			ID:             "1",
			Queue:          "testqueueR",
			Task:           genTask(1),
			Attempts:       1,
			WaitUntil:      nil,
			CreatedAt:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			LastExecutedAt: ptr(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)),
		},
		{
			ID:             "2",
			Queue:          "testqueueR",
			Task:           genTask(2),
			Attempts:       1,
			WaitUntil:      nil,
			CreatedAt:      time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			LastExecutedAt: nil,
		},
	}
	tasksUpcoming = task.Tasks{
		{
			ID:             "3",
			Queue:          "testqueueU",
			Task:           genTask(3),
			Attempts:       2,
			WaitUntil:      nil,
			CreatedAt:      time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC),
			LastExecutedAt: ptr(time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)),
		},
		{
			ID:             "4",
			Queue:          "testqueueUs",
			Task:           genTask(4),
			Attempts:       0,
			WaitUntil:      ptr(time.Date(2025, 1, 8, 0, 0, 0, 0, time.UTC)),
			CreatedAt:      time.Date(2025, 1, 7, 0, 0, 0, 0, time.UTC),
			LastExecutedAt: nil,
		},
	}
	tasksSucceeded = []*task.Completed{
		{
			ID:             "5",
			Queue:          "testqueueS",
			Task:           genTask(5),
			Attempts:       4,
			Succeeded:      true,
			LastDuration:   5 * time.Second,
			ExpiresAt:      ptr(time.Date(2025, 2, 8, 0, 0, 0, 0, time.UTC)),
			CreatedAt:      time.Date(2025, 3, 9, 0, 0, 0, 0, time.UTC),
			LastExecutedAt: time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:             "6",
			Queue:          "testqueueS",
			Task:           genTask(6),
			Attempts:       3,
			Succeeded:      true,
			LastDuration:   752 * time.Millisecond,
			ExpiresAt:      nil,
			CreatedAt:      time.Date(2025, 2, 11, 0, 0, 0, 0, time.UTC),
			LastExecutedAt: time.Date(2025, 2, 12, 0, 0, 0, 0, time.UTC),
		},
	}
	tasksFailed = []*task.Completed{
		{
			ID:             "7",
			Queue:          "testqueueF",
			Task:           genTask(7),
			Attempts:       2,
			Succeeded:      false,
			LastDuration:   5123 * time.Millisecond,
			ExpiresAt:      ptr(time.Date(2025, 5, 8, 0, 0, 0, 0, time.UTC)),
			CreatedAt:      time.Date(2025, 5, 9, 0, 0, 0, 0, time.UTC),
			LastExecutedAt: time.Date(2025, 5, 10, 0, 0, 0, 0, time.UTC),
			Error:          ptr("bad thing happened"),
		},
		{
			ID:             "8",
			Queue:          "testqueueF",
			Task:           genTask(8),
			Attempts:       6,
			Succeeded:      false,
			LastDuration:   72 * time.Second,
			ExpiresAt:      nil,
			CreatedAt:      time.Date(2025, 4, 11, 0, 0, 0, 0, time.UTC),
			LastExecutedAt: time.Date(2025, 4, 12, 0, 0, 0, 0, time.UTC),
			Error:          ptr("bad thing happened again!"),
		},
	}
)

func TestMain(m *testing.M) {
	t := &testing.T{}
	db := testutil.NewDB(t)
	if t.Failed() {
		panic("failed to open database")
	}
	defer db.Close()

	h = NewHandler(db)
	seedTestData()
	mux = http.NewServeMux()
	h.Register(mux)
	os.Exit(m.Run())
}

func seedTestData() {
	tx, err := h.db.Begin()
	if err != nil {
		panic(err)
	}

	for _, t := range tasksRunning {
		if err := t.InsertTx(context.Background(), tx); err != nil {
			panic(err)
		}
	}

	for _, t := range tasksUpcoming {
		if err := t.InsertTx(context.Background(), tx); err != nil {
			panic(err)
		}
	}

	for _, t := range tasksSucceeded {
		if err := t.InsertTx(context.Background(), tx); err != nil {
			panic(err)
		}
	}

	for _, t := range tasksFailed {
		if err := t.InsertTx(context.Background(), tx); err != nil {
			panic(err)
		}
	}

	if err := tx.Commit(); err != nil {
		panic(err)
	}

	if err := tasksRunning.Claim(context.Background(), h.db); err != nil {
		panic(err)
	}

	for _, t := range tasksRunning {
		// Set a fixed time for when the running tasks were claimed so we can assert it in the UI.
		_, err := h.db.Exec("UPDATE backlite_tasks SET claimed_at = ? WHERE id = ?", now.UnixMilli(), t.ID)
		if err != nil {
			panic(err)
		}
		t.ClaimedAt = ptr(now)

		// Set the last executed time for tasks so we can assert it in the UI.
		if t.LastExecutedAt != nil {
			_, err := h.db.Exec("UPDATE backlite_tasks SET last_executed_at = ? WHERE id = ?", t.LastExecutedAt.UnixMilli(), t.ID)
			if err != nil {
				panic(err)
			}
		}
	}

	for _, t := range tasksUpcoming {
		if t.LastExecutedAt != nil {
			_, err := h.db.Exec(
				"UPDATE backlite_tasks SET attempts = ?, last_executed_at = ? WHERE id = ?",
				t.Attempts,
				t.LastExecutedAt.UnixMilli(),
				t.ID,
			)

			if err != nil {
				panic(err)
			}
		}
	}
}

func ptr[T any](v T) *T {
	return &v
}

func genTask(num int) []byte {
	return []byte(fmt.Sprintf(`{"value": %d}`, num))
}

func request(t *testing.T, hf handleFunc, url string) (*httptest.ResponseRecorder, *goquery.Document) {
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	doc, err := goquery.NewDocumentFromReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	return rec, doc
}
