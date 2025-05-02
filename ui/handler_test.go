package ui

import (
	"context"
	"database/sql"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mikestefanello/backlite/internal/task"
	"github.com/mikestefanello/backlite/internal/testutil"
)

func TestHandler_Validation(t *testing.T) {
	db := testutil.NewDB(t)

	t.Run("release after", func(t *testing.T) {
		t.Run("invalid", func(t *testing.T) {
			_, err := NewHandler(Config{
				DB:           db,
				ReleaseAfter: time.Second * -5,
			})
			if err == nil {
				t.Fatal("expected error")
			}
		})

		t.Run("default", func(t *testing.T) {
			h, err := NewHandler(Config{
				DB: db,
			})
			if err != nil {
				t.Fatal("unexpected error")
			}
			testutil.Equal(t, "ReleaseAfter", time.Hour, h.cfg.ReleaseAfter)
		})
	})

	t.Run("base path invalid", func(t *testing.T) {
		_, err := NewHandler(Config{
			DB:       db,
			BasePath: "abc",
		})
		if err == nil {
			t.Fatal("expected error")
		}

		_, err = NewHandler(Config{
			DB:       db,
			BasePath: "abc/",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("db missing", func(t *testing.T) {
		_, err := NewHandler(Config{})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("items per page", func(t *testing.T) {
		t.Run("invalid", func(t *testing.T) {
			_, err := NewHandler(Config{
				DB:           db,
				ItemsPerPage: -2,
			})
			if err == nil {
				t.Fatal("expected error")
			}
		})

		t.Run("default", func(t *testing.T) {
			h, err := NewHandler(Config{
				DB: db,
			})
			if err != nil {
				t.Fatal("unexpected error")
			}
			testutil.Equal(t, "ItemsPerPage", 25, h.cfg.ItemsPerPage)
		})
	})

	t.Run("valid", func(t *testing.T) {
		_, err := NewHandler(Config{
			DB:           db,
			ItemsPerPage: 10,
			BasePath:     "/dashboard",
			ReleaseAfter: time.Minute * 5,
		})
		if err != nil {
			t.Fatal("unexpected error")
		}
	})
}

func TestHandler_Running(t *testing.T) {
	path := "/"
	_, doc := request(t, path)
	assertNav(t, doc, path)
	assertPager(t, doc, path)

	headers := doc.Find("tr th")
	testutil.Equal(t, "headers", 6, headers.Length())
	headers.Each(func(i int, sel *goquery.Selection) {
		switch i {
		case 0, 5:
			testutil.Equal(t, "header", "", sel.Text())
		case 1:
			testutil.Equal(t, "header", "Queue", sel.Text())
		case 2:
			testutil.Equal(t, "header", "Attempt", sel.Text())
		case 3:
			testutil.Equal(t, "header", "Created at", sel.Text())
		case 4:
			testutil.Equal(t, "header", "Started", sel.Text())
		}
	})

	rows := doc.Find("tbody tr")
	testutil.Equal(t, "rows", 2, rows.Length())
	rows.Each(func(rowIndex int, sel *goquery.Selection) {
		cells := sel.Find("td")
		testutil.Equal(t, "cells", 6, cells.Length())
		cells.Each(func(cellIndex int, sel *goquery.Selection) {
			switch cellIndex {
			case 0:
				testutil.Equal(t, "cell", `<span class="status-dot status-dot-animated status-yellow"></span>`, toHTML(t, sel))
			case 1:
				testutil.Equal(t, "cell", tasksRunning[rowIndex].Queue, sel.Text())
			case 2:
				testutil.Equal(t, "cell", fmt.Sprint(tasksRunning[rowIndex].Attempts), sel.Text())
			case 3:
				testutil.Equal(t, "cell", datetime(tasksRunning[rowIndex].CreatedAt), sel.Text())
			case 4:
				testutil.Equal(t, "cell", datetime(*tasksRunning[rowIndex].ClaimedAt), sel.Text())
			case 5:
				testutil.Equal(t, "cell", fmt.Sprintf(`<a href="/task/%s">View</a>`, tasksRunning[rowIndex].ID), toHTML(t, sel))
			}
		})
	})
}

func TestHandler_Upcoming(t *testing.T) {
	path := "/upcoming"
	_, doc := request(t, path)
	assertNav(t, doc, path)
	assertPager(t, doc, path)

	headers := doc.Find("tr th")
	testutil.Equal(t, "headers", 6, headers.Length())
	headers.Each(func(i int, sel *goquery.Selection) {
		switch i {
		case 0, 5:
			testutil.Equal(t, "header", "", sel.Text())
		case 1:
			testutil.Equal(t, "header", "Queue", sel.Text())
		case 2:
			testutil.Equal(t, "header", "Attempts", sel.Text())
		case 3:
			testutil.Equal(t, "header", "Created at", sel.Text())
		case 4:
			testutil.Equal(t, "header", "Last executed at", sel.Text())
		}
	})

	rows := doc.Find("tbody tr")
	testutil.Equal(t, "rows", 2, rows.Length())
	rows.Each(func(rowIndex int, sel *goquery.Selection) {
		cells := sel.Find("td")
		testutil.Equal(t, "cells", 6, cells.Length())
		cells.Each(func(cellIndex int, sel *goquery.Selection) {
			switch cellIndex {
			case 0:
				testutil.Equal(t, "cell", `<span class="status-dot status-azure"></span>`, toHTML(t, sel))
			case 1:
				testutil.Equal(t, "cell", tasksUpcoming[rowIndex].Queue, sel.Text())
			case 2:
				testutil.Equal(t, "cell", fmt.Sprint(tasksUpcoming[rowIndex].Attempts), sel.Text())
			case 3:
				testutil.Equal(t, "cell", datetime(tasksUpcoming[rowIndex].CreatedAt), sel.Text())
			case 4:
				if tasksUpcoming[rowIndex].LastExecutedAt == nil {
					testutil.Equal(t, "cell", "Never", strings.TrimSpace(sel.Text()))
				} else {
					testutil.Equal(t, "cell", datetime(*tasksUpcoming[rowIndex].LastExecutedAt), strings.TrimSpace(sel.Text()))
				}
			case 5:
				testutil.Equal(t, "cell", fmt.Sprintf(`<a href="/task/%s">View</a>`, tasksUpcoming[rowIndex].ID), toHTML(t, sel))
			}
		})
	})
}

func TestHandler_Succeeded(t *testing.T) {
	path := "/succeeded"
	_, doc := request(t, path)
	assertNav(t, doc, path)
	assertPager(t, doc, path)

	headers := doc.Find("tr th")
	testutil.Equal(t, "headers", 7, headers.Length())
	headers.Each(func(i int, sel *goquery.Selection) {
		switch i {
		case 0, 6:
			testutil.Equal(t, "header", "", sel.Text())
		case 1:
			testutil.Equal(t, "header", "Queue", sel.Text())
		case 2:
			testutil.Equal(t, "header", "Attempts", sel.Text())
		case 3:
			testutil.Equal(t, "header", "Created at", sel.Text())
		case 4:
			testutil.Equal(t, "header", "Last executed at", sel.Text())
		case 5:
			testutil.Equal(t, "header", "Last duration", sel.Text())
		}
	})

	rows := doc.Find("tbody tr")
	testutil.Equal(t, "rows", 2, rows.Length())
	rows.Each(func(rowIndex int, sel *goquery.Selection) {
		cells := sel.Find("td")
		testutil.Equal(t, "cells", 7, cells.Length())
		cells.Each(func(cellIndex int, sel *goquery.Selection) {
			switch cellIndex {
			case 0:
				testutil.Equal(t, "cell", `<span class="status-dot status-green"></span>`, toHTML(t, sel))
			case 1:
				testutil.Equal(t, "cell", tasksSucceeded[rowIndex].Queue, sel.Text())
			case 2:
				testutil.Equal(t, "cell", fmt.Sprint(tasksSucceeded[rowIndex].Attempts), sel.Text())
			case 3:
				testutil.Equal(t, "cell", datetime(tasksSucceeded[rowIndex].CreatedAt), sel.Text())
			case 4:
				testutil.Equal(t, "cell", datetime(tasksSucceeded[rowIndex].LastExecutedAt), strings.TrimSpace(sel.Text()))
			case 5:
				testutil.Equal(t, "cell", tasksSucceeded[rowIndex].LastDuration.String(), strings.TrimSpace(sel.Text()))
			case 6:
				testutil.Equal(t, "cell", fmt.Sprintf(`<a href="/completed/%s">View</a>`, tasksSucceeded[rowIndex].ID), toHTML(t, sel))
			}
		})
	})
}

func TestHandler_Failed(t *testing.T) {
	path := "/failed"
	_, doc := request(t, path)
	assertNav(t, doc, path)
	assertPager(t, doc, path)

	headers := doc.Find("tr th")
	testutil.Equal(t, "headers", 7, headers.Length())
	headers.Each(func(i int, sel *goquery.Selection) {
		switch i {
		case 0, 6:
			testutil.Equal(t, "header", "", sel.Text())
		case 1:
			testutil.Equal(t, "header", "Queue", sel.Text())
		case 2:
			testutil.Equal(t, "header", "Attempts", sel.Text())
		case 3:
			testutil.Equal(t, "header", "Created at", sel.Text())
		case 4:
			testutil.Equal(t, "header", "Last executed at", sel.Text())
		case 5:
			testutil.Equal(t, "header", "Last duration", sel.Text())
		}
	})

	rows := doc.Find("tbody tr")
	testutil.Equal(t, "rows", 2, rows.Length())
	rows.Each(func(rowIndex int, sel *goquery.Selection) {
		cells := sel.Find("td")
		testutil.Equal(t, "cells", 7, cells.Length())
		cells.Each(func(cellIndex int, sel *goquery.Selection) {
			switch cellIndex {
			case 0:
				testutil.Equal(t, "cell", `<span class="status-dot status-red"></span>`, toHTML(t, sel))
			case 1:
				testutil.Equal(t, "cell", tasksFailed[rowIndex].Queue, sel.Text())
			case 2:
				testutil.Equal(t, "cell", fmt.Sprint(tasksFailed[rowIndex].Attempts), sel.Text())
			case 3:
				testutil.Equal(t, "cell", datetime(tasksFailed[rowIndex].CreatedAt), sel.Text())
			case 4:
				testutil.Equal(t, "cell", datetime(tasksFailed[rowIndex].LastExecutedAt), strings.TrimSpace(sel.Text()))
			case 5:
				testutil.Equal(t, "cell", tasksFailed[rowIndex].LastDuration.String(), strings.TrimSpace(sel.Text()))
			case 6:
				testutil.Equal(t, "cell", fmt.Sprintf(`<a href="/completed/%s">View</a>`, tasksFailed[rowIndex].ID), toHTML(t, sel))
			}
		})
	})
}

func TestHandler_Task__Running(t *testing.T) {
	for n, tk := range tasksRunning {
		t.Run(fmt.Sprintf("task_%d", n+1), func(t *testing.T) {
			path := fmt.Sprintf("/task/%s", tk.ID)
			_, doc := request(t, path)
			assertNav(t, doc, path)
			assertTask(t, doc, tk)
		})
	}
}

func TestHandler_Task__Upcoming(t *testing.T) {
	for n, tk := range tasksUpcoming {
		t.Run(fmt.Sprintf("task_%d", n+1), func(t *testing.T) {
			path := fmt.Sprintf("/task/%s", tk.ID)
			_, doc := request(t, path)
			assertNav(t, doc, path)
			assertTask(t, doc, tk)
		})
	}
}

func TestHandler_TaskCompleted__Succeeded(t *testing.T) {
	for n, tk := range tasksSucceeded {
		t.Run(fmt.Sprintf("task_%d", n+1), func(t *testing.T) {
			path := fmt.Sprintf("/completed/%s", tk.ID)
			_, doc := request(t, path)
			assertNav(t, doc, path)
			assertTaskCompleted(t, doc, tk)
		})
	}
}

func TestHandler_TaskCompleted__Failed(t *testing.T) {
	for n, tk := range tasksFailed {
		t.Run(fmt.Sprintf("task_%d", n+1), func(t *testing.T) {
			path := fmt.Sprintf("/completed/%s", tk.ID)
			_, doc := request(t, path)
			assertNav(t, doc, path)
			assertTaskCompleted(t, doc, tk)
		})
	}
}

func TestHandler_Task__IsCompleted(t *testing.T) {
	path := fmt.Sprintf("/task/%s", tasksSucceeded[0].ID)
	// A completed task can be passed in to the task route and it should redirect.
	_, doc := request(t, path)
	assertNav(t, doc, path)
	assertTaskCompleted(t, doc, tasksSucceeded[0])
}

func TestHandler_Task__NotFound(t *testing.T) {
	path := fmt.Sprintf("/task/abcd")
	_, doc := request(t, path)
	assertNav(t, doc, path)
	text := doc.Find("div.page-body").Text()
	testutil.Equal(t, "body", "Task not found!", strings.TrimSpace(text))
}

func TestHandler_TaskCompleted__NotFound(t *testing.T) {
	path := fmt.Sprintf("/completed/abcd")
	_, doc := request(t, path)
	assertNav(t, doc, path)
	text := doc.Find("div.page-body").Text()
	testutil.Equal(t, "body", "Task not found!", strings.TrimSpace(text))
}

func assertNav(t *testing.T, doc *goquery.Document, path string) {
	brand := doc.Find(".navbar-brand")
	testutil.Equal(t, "brand", "Backlite", strings.TrimSpace(brand.Text()))

	links := doc.Find("header ul.navbar-nav li.nav-item")
	testutil.Equal(t, "nav", 4, links.Length())
	links.Each(func(i int, sel *goquery.Selection) {
		link := sel.Find("a.nav-link")
		href, found := link.Attr("href")
		testutil.Equal(t, "href", true, found)

		switch i {
		case 0:
			testutil.Equal(t, "nav", "Running", strings.TrimSpace(link.Text()))
			testutil.Equal(t, "href", "/", href)
			testutil.Equal(t, "active", path == "/", sel.HasClass("active"))
		case 1:
			testutil.Equal(t, "nav", "Upcoming", strings.TrimSpace(link.Text()))
			testutil.Equal(t, "href", "/upcoming", href)
			testutil.Equal(t, "active", path == "/upcoming", sel.HasClass("active"))
		case 2:
			testutil.Equal(t, "nav", "Succeeded", strings.TrimSpace(link.Text()))
			testutil.Equal(t, "href", "/succeeded", href)
			testutil.Equal(t, "active", path == "/succeeded", sel.HasClass("active"))
		case 3:
			testutil.Equal(t, "nav", "Failed", strings.TrimSpace(link.Text()))
			testutil.Equal(t, "href", "/failed", href)
			testutil.Equal(t, "active", path == "/failed", sel.HasClass("active"))
		}
	})
}

func assertPager(t *testing.T, doc *goquery.Document, path string) {
	pager := doc.Find("div.card-footer ul.pagination li.page-item")
	testutil.Equal(t, "pagination", 2, pager.Length())
	u, err := url.Parse(path)
	if err != nil {
		t.Fatal("failed to parse path")
	}
	page := getPage(u)

	assertHref := func(sel *goquery.Selection, pageNum int) {
		href, exists := sel.Attr("href")
		testutil.Equal(t, "href", true, exists)
		testutil.Equal(t, "href", fmt.Sprintf("%s?page=%d", u.Path, pageNum), href)
	}

	pager.Each(func(i int, sel *goquery.Selection) {
		link := sel.Find("a.page-link")
		switch i {
		case 0:
			if page == 1 {
				testutil.Equal(t, "prev", true, sel.HasClass("disabled"))
			}
			testutil.Equal(t, "prev", "prev", strings.TrimSpace(link.Text()))
			assertHref(link, page-1)
		case 1:
			testutil.Equal(t, "next", "next", strings.TrimSpace(link.Text()))
			assertHref(link, page+1)
		}
	})
}

func TestHandler_DBFailures(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}

	assert := func(t *testing.T, hf handleFunc, path string) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handle(hf)(rec, req)

		testutil.Equal(t, "code", http.StatusInternalServerError, rec.Code)
		if !strings.Contains(rec.Body.String(), "no such table") {
			t.Error("error message not found")
		}
	}

	t.Run("running", func(t *testing.T) {
		assert(t, h.Running, "/")
	})

	t.Run("upcoming", func(t *testing.T) {
		assert(t, h.Upcoming, "/upcoming")
	})

	t.Run("succeeded", func(t *testing.T) {
		assert(t, h.Succeeded, "/succeeded")
	})

	t.Run("failed", func(t *testing.T) {
		assert(t, h.Failed, "/failed")
	})

	t.Run("task", func(t *testing.T) {
		assert(t, h.Task, "/task/1")
	})

	t.Run("completed", func(t *testing.T) {
		assert(t, h.TaskCompleted, "/completed/1")
	})
}

func TestHandler_NoTasks(t *testing.T) {
	db := testutil.NewDB(t)
	defer db.Close()
	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}

	assert := func(t *testing.T, hf handleFunc, path string) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handle(hf)(rec, req)

		doc, err := goquery.NewDocumentFromReader(rec.Body)
		if err != nil {
			t.Fatal(err)
		}

		assertNav(t, doc, path)
		assertPager(t, doc, path)
		body := doc.Find("div.table-responsive div.card-body")
		testutil.Equal(t, "message", "No tasks to display.", strings.TrimSpace(body.Text()))
	}

	t.Run("running", func(t *testing.T) {
		assert(t, h.Running, "/")
	})

	t.Run("upcoming", func(t *testing.T) {
		assert(t, h.Upcoming, "/upcoming")
	})

	t.Run("succeeded", func(t *testing.T) {
		assert(t, h.Succeeded, "/succeeded")
	})

	t.Run("failed", func(t *testing.T) {
		assert(t, h.Failed, "/failed")
	})
}

func TestHandler_Paging(t *testing.T) {
	db := testutil.NewDB(t)
	defer db.Close()
	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}

	toClaim := make(task.Tasks, 0, 30)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 60; i++ {
		tk := &task.Task{
			ID:        fmt.Sprint(i),
			Queue:     "a",
			Task:      genTask(1),
			CreatedAt: time.Now(),
		}
		err := tk.InsertTx(context.Background(), tx)
		if err != nil {
			t.Fatal(err)
		}
		if i >= 30 {
			toClaim = append(toClaim, tk)
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	err = toClaim.Claim(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}

	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 60; i++ {
		tk := &task.Completed{
			ID:       fmt.Sprint(i),
			Queue:    "a",
			Attempts: 1,
			Succeeded: func() bool {
				if i%2 == 0 {
					return true
				}
				return false
			}(),
			CreatedAt:      time.Now(),
			LastExecutedAt: time.Now(),
		}

		err := tk.InsertTx(context.Background(), tx)
		if err != nil {
			t.Fatal(err)
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	assert := func(t *testing.T, hf handleFunc, path string) {
		var pagerPath string
		var expectedRows int

		for i := 1; i <= 2; i++ {
			switch i {
			case 1:
				expectedRows = h.cfg.ItemsPerPage
				pagerPath = path
			case 2:
				expectedRows = 5
				pagerPath = fmt.Sprintf("%s?page=%d", path, i)
			}

			req := httptest.NewRequest(http.MethodGet, pagerPath, nil)
			rec := httptest.NewRecorder()
			handle(hf)(rec, req)

			doc, err := goquery.NewDocumentFromReader(rec.Body)
			if err != nil {
				t.Fatal(err)
			}

			assertPager(t, doc, pagerPath)
			rows := doc.Find("tbody tr")
			testutil.Equal(t, "rows", expectedRows, rows.Length())
		}
	}

	t.Run("running", func(t *testing.T) {
		assert(t, h.Running, "/")
	})

	t.Run("upcoming", func(t *testing.T) {
		assert(t, h.Upcoming, "/upcoming")
	})

	t.Run("succeeded", func(t *testing.T) {
		assert(t, h.Succeeded, "/succeeded")
	})

	t.Run("failed", func(t *testing.T) {
		assert(t, h.Failed, "/failed")
	})
}

func assertTask(t *testing.T, doc *goquery.Document, tk *task.Task) {
	expectedTitles := []string{
		"Status",
		"Queue",
		"ID",
		"Created at",
		"Started",
		"Wait until",
		"Attempts",
		"Last executed at",
		"Data",
	}

	expectedContent := []string{
		func() string {
			if tk.ClaimedAt == nil {
				return formatStatus("azure", "Upcoming", false)
			}
			return formatStatus("yellow", "Running", true)
		}(),
		tk.Queue,
		tk.ID,
		datetime(tk.CreatedAt),
		timeOrDash(tk.ClaimedAt),
		timeOrDash(tk.WaitUntil),
		fmt.Sprint(tk.Attempts),
		timeOrDash(tk.LastExecutedAt),
		formatJSON(tk.Task),
	}

	boxes := doc.Find("div.datagrid-item")
	testutil.Equal(t, "boxes", 9, boxes.Length())
	boxes.Each(func(i int, sel *goquery.Selection) {
		title := sel.Find(".datagrid-title").Text()
		testutil.Equal(t, "title", expectedTitles[i], title)

		content := toHTML(t, sel.Find(".datagrid-content"))
		testutil.Equal(t, "content", expectedContent[i], content)
	})
}

func assertTaskCompleted(t *testing.T, doc *goquery.Document, tk *task.Completed) {
	expectedTitles := []string{
		"Status",
		"Queue",
		"ID",
		"Created at",
		"Last executed at",
		"Last duration",
		"Attempts",
		"Expires at",
	}

	expectedContent := []string{
		func() string {
			if tk.Error != nil {
				return formatStatus("red", "Failed", false)
			}
			return formatStatus("green", "Succeeded", false)
		}(),
		tk.Queue,
		tk.ID,
		datetime(tk.CreatedAt),
		datetime(tk.LastExecutedAt),
		tk.LastDuration.String(),
		fmt.Sprint(tk.Attempts),
		func() string {
			if tk.ExpiresAt != nil {
				return datetime(*tk.ExpiresAt)
			}
			return "Never"
		}(),
	}

	if tk.Error != nil {
		expectedTitles = append(expectedTitles, "Error")
		errorTmpl := `<div class="alert alert-important alert-danger" role="alert">%s</div>`
		expectedContent = append(expectedContent, fmt.Sprintf(errorTmpl, *tk.Error))
	}

	expectedTitles = append(expectedTitles, "Data")
	expectedContent = append(expectedContent, formatJSON(tk.Task))

	boxes := doc.Find("div.datagrid-item")
	testutil.Equal(t, "boxes", len(expectedTitles), boxes.Length())
	boxes.Each(func(i int, sel *goquery.Selection) {
		title := sel.Find(".datagrid-title").Text()
		testutil.Equal(t, "title", expectedTitles[i], title)

		content := toHTML(t, sel.Find(".datagrid-content"))
		testutil.Equal(t, "content", expectedContent[i], content)
	})
}

func timeOrDash(t *time.Time) string {
	if t != nil {
		return datetime(*t)
	}
	return "-"
}

func formatStatus(color, label string, animated bool) string {
	var class string
	if animated {
		class = " status-dot-animated"
	}
	tag := `<span class="status status-%s"><span class="status-dot%s"></span>%s</span>`
	return fmt.Sprintf(tag, color, class, label)
}

func formatJSON(b []byte) string {
	return "<kbd>" + html.EscapeString(bytestring(b)) + "</kbd>"
}

func toHTML(t *testing.T, sel *goquery.Selection) string {
	output, err := sel.Html()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(output)
}
