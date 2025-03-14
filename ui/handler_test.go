package ui

import (
	"database/sql"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mikestefanello/backlite/internal/task"
	"github.com/mikestefanello/backlite/internal/testutil"
)

func TestHandler_Running(t *testing.T) {
	url := "/"
	_, doc := request(t, h.Running, url)
	assertNav(t, doc, url)

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
	url := "/upcoming"
	_, doc := request(t, h.Upcoming, url)
	assertNav(t, doc, url)

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
	url := "/succeeded"
	_, doc := request(t, h.Succeeded, url)
	assertNav(t, doc, url)

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
	url := "/failed"
	_, doc := request(t, h.Failed, url)
	assertNav(t, doc, url)

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
			url := fmt.Sprintf("/task/%s", tk.ID)
			_, doc := request(t, h.Task, url)
			assertNav(t, doc, url)
			assertTask(t, doc, tk)
		})
	}
}

func TestHandler_Task__Upcoming(t *testing.T) {
	for n, tk := range tasksUpcoming {
		t.Run(fmt.Sprintf("task_%d", n+1), func(t *testing.T) {
			url := fmt.Sprintf("/task/%s", tk.ID)
			_, doc := request(t, h.Task, url)
			assertNav(t, doc, url)
			assertTask(t, doc, tk)
		})
	}
}

func TestHandler_TaskCompleted__Succeeded(t *testing.T) {
	for n, tk := range tasksSucceeded {
		t.Run(fmt.Sprintf("task_%d", n+1), func(t *testing.T) {
			url := fmt.Sprintf("/completed/%s", tk.ID)
			_, doc := request(t, h.TaskCompleted, url)
			assertNav(t, doc, url)
			assertTaskCompleted(t, doc, tk)
		})
	}
}

func TestHandler_TaskCompleted__Failed(t *testing.T) {
	for n, tk := range tasksFailed {
		t.Run(fmt.Sprintf("task_%d", n+1), func(t *testing.T) {
			url := fmt.Sprintf("/completed/%s", tk.ID)
			_, doc := request(t, h.TaskCompleted, url)
			assertNav(t, doc, url)
			assertTaskCompleted(t, doc, tk)
		})
	}
}

func TestHandler_Task__IsCompleted(t *testing.T) {
	url := fmt.Sprintf("/task/%s", tasksSucceeded[0].ID)
	// A completed task can be passed in to the task route and it should redirect.
	_, doc := request(t, h.Task, url)
	assertNav(t, doc, url)
	assertTaskCompleted(t, doc, tasksSucceeded[0])
}

func TestHandler_Task__NotFound(t *testing.T) {
	url := fmt.Sprintf("/task/abcd")
	_, doc := request(t, h.Task, url)
	assertNav(t, doc, url)
	text := doc.Find("div.page-body").Text()
	testutil.Equal(t, "body", "Task not found!", strings.TrimSpace(text))
}

func TestHandler_TaskCompleted__NotFound(t *testing.T) {
	url := fmt.Sprintf("/completed/abcd")
	_, doc := request(t, h.Task, url)
	assertNav(t, doc, url)
	text := doc.Find("div.page-body").Text()
	testutil.Equal(t, "body", "Task not found!", strings.TrimSpace(text))
}

func assertNav(t *testing.T, doc *goquery.Document, url string) {
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
			testutil.Equal(t, "active", url == "/", sel.HasClass("active"))
		case 1:
			testutil.Equal(t, "nav", "Upcoming", strings.TrimSpace(link.Text()))
			testutil.Equal(t, "href", "/upcoming", href)
			testutil.Equal(t, "active", url == "/upcoming", sel.HasClass("active"))
		case 2:
			testutil.Equal(t, "nav", "Succeeded", strings.TrimSpace(link.Text()))
			testutil.Equal(t, "href", "/succeeded", href)
			testutil.Equal(t, "active", url == "/succeeded", sel.HasClass("active"))
		case 3:
			testutil.Equal(t, "nav", "Failed", strings.TrimSpace(link.Text()))
			testutil.Equal(t, "href", "/failed", href)
			testutil.Equal(t, "active", url == "/failed", sel.HasClass("active"))
		}
	})
}

func TestHandler_DBFailures(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := NewHandler(db)

	assert := func(t *testing.T, hf handleFunc, url string) {
		req := httptest.NewRequest(http.MethodGet, url, nil)
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
