package caldav

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yazanabuashour/openplanner/internal/domain"
	"github.com/yazanabuashour/openplanner/internal/service"
	"github.com/yazanabuashour/openplanner/internal/store"
)

func newTestServer(t *testing.T) (*Server, *service.Service) {
	t.Helper()

	repository, err := store.Open(filepath.Join(t.TempDir(), "openplanner.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := repository.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	svc := service.New(repository)
	return NewServer(svc), svc
}

func createCalendar(t *testing.T, svc *service.Service) domain.Calendar {
	t.Helper()

	calendar, err := svc.CreateCalendar(domain.Calendar{Name: "Work"})
	if err != nil {
		t.Fatalf("CreateCalendar(): %v", err)
	}
	return calendar
}

func TestPropfindDiscoveryAndCollections(t *testing.T) {
	server, svc := newTestServer(t)
	calendar := createCalendar(t, svc)

	root := request(server, "PROPFIND", "/caldav/", `<propfind xmlns="DAV:"><allprop/></propfind>`, nil)
	if root.Code != 207 {
		t.Fatalf("root status = %d, body = %s", root.Code, root.Body.String())
	}
	assertBodyContains(t, root, "calendar-home-set")
	assertBodyContains(t, root, "/caldav/calendars/local/")

	home := request(server, "PROPFIND", "/caldav/calendars/local/", `<propfind xmlns="DAV:"><allprop/></propfind>`, map[string]string{"Depth": "1"})
	if home.Code != 207 {
		t.Fatalf("home status = %d, body = %s", home.Code, home.Body.String())
	}
	assertBodyContains(t, home, calendarHref(calendar.ID))
	assertBodyContains(t, home, "OpenPlanner Calendars")

	collection := request(server, "PROPFIND", calendarHref(calendar.ID), `<propfind xmlns="DAV:"><prop><displayname/><unknown-property/></prop></propfind>`, nil)
	if collection.Code != 207 {
		t.Fatalf("collection status = %d, body = %s", collection.Code, collection.Body.String())
	}
	assertBodyContains(t, collection, "Work")
	assertBodyContains(t, collection, "HTTP/1.1 404 Not Found")
}

func TestGetCalendarObjectReturnsICalendarAndETag(t *testing.T) {
	server, svc := newTestServer(t)
	calendar := createCalendar(t, svc)
	start := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	event, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Planning",
		StartAt:    &start,
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}

	response := request(server, http.MethodGet, objectHref(calendar.ID, event.ID+".ics"), "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("ETag") == "" {
		t.Fatal("GET ETag is empty")
	}
	if !strings.Contains(response.Header().Get("Content-Type"), "text/calendar") {
		t.Fatalf("content-type = %q, want text/calendar", response.Header().Get("Content-Type"))
	}
	assertBodyContains(t, response, "BEGIN:VCALENDAR")
	assertBodyContains(t, response, "SUMMARY:Planning")
}

func TestCalendarQueryReportsObjectsAndEventTimeRange(t *testing.T) {
	server, svc := newTestServer(t)
	calendar := createCalendar(t, svc)
	start := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	event, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "In range",
		StartAt:    &start,
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}
	dueDate := "2026-04-20"
	task, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Task",
		DueDate:    &dueDate,
	})
	if err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}

	all := request(server, "REPORT", calendarHref(calendar.ID), `<calendar-query xmlns="urn:ietf:params:xml:ns:caldav"/>`, nil)
	if all.Code != 207 {
		t.Fatalf("all REPORT status = %d, body = %s", all.Code, all.Body.String())
	}
	assertBodyContains(t, all, event.ID+".ics")
	assertBodyContains(t, all, task.ID+".ics")

	timeRange := strings.Join([]string{
		`<calendar-query xmlns="urn:ietf:params:xml:ns:caldav">`,
		`<filter><comp-filter name="VCALENDAR"><comp-filter name="VEVENT">`,
		`<time-range start="20260420T000000Z" end="20260421T000000Z"/>`,
		`</comp-filter></comp-filter></filter>`,
		`</calendar-query>`,
	}, "")
	filtered := request(server, "REPORT", calendarHref(calendar.ID), timeRange, nil)
	if filtered.Code != 207 {
		t.Fatalf("filtered REPORT status = %d, body = %s", filtered.Code, filtered.Body.String())
	}
	assertBodyContains(t, filtered, event.ID+".ics")
	if strings.Contains(filtered.Body.String(), task.ID+".ics") {
		t.Fatalf("filtered REPORT included task resource: %s", filtered.Body.String())
	}
}

func TestPutCreatesAndUpdatesCalendarObjectByUID(t *testing.T) {
	server, svc := newTestServer(t)
	calendar := createCalendar(t, svc)
	createContent := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:client-event@example.com",
		"SUMMARY:Client event",
		"DTSTART;VALUE=DATE:20260420",
		"END:VEVENT",
		"END:VCALENDAR",
	}, "\r\n")

	created := request(server, http.MethodPut, objectHref(calendar.ID, "client-event@example.com.ics"), createContent, map[string]string{"Content-Type": "text/calendar"})
	if created.Code != http.StatusCreated {
		t.Fatalf("PUT create status = %d, body = %s", created.Code, created.Body.String())
	}

	fetched := request(server, http.MethodGet, objectHref(calendar.ID, "client-event@example.com.ics"), "", nil)
	if fetched.Code != http.StatusOK {
		t.Fatalf("GET created status = %d, body = %s", fetched.Code, fetched.Body.String())
	}
	assertBodyContains(t, fetched, "SUMMARY:Client event")
	assertBodyContains(t, fetched, "UID:client-event@example.com")

	updateContent := strings.Replace(createContent, "SUMMARY:Client event", "SUMMARY:Updated client event", 1)
	updated := request(server, http.MethodPut, objectHref(calendar.ID, "client-event@example.com.ics"), updateContent, map[string]string{"Content-Type": "text/calendar"})
	if updated.Code != http.StatusNoContent {
		t.Fatalf("PUT update status = %d, body = %s", updated.Code, updated.Body.String())
	}

	refetched := request(server, http.MethodGet, objectHref(calendar.ID, "client-event@example.com.ics"), "", nil)
	if refetched.Code != http.StatusOK {
		t.Fatalf("GET updated status = %d, body = %s", refetched.Code, refetched.Body.String())
	}
	assertBodyContains(t, refetched, "SUMMARY:Updated client event")
	assertBodyContains(t, refetched, "UID:client-event@example.com")
}

func TestPutRejectsMultipleCalendarObjects(t *testing.T) {
	server, svc := newTestServer(t)
	calendar := createCalendar(t, svc)
	content := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:first@example.com",
		"SUMMARY:First",
		"DTSTART;VALUE=DATE:20260420",
		"END:VEVENT",
		"BEGIN:VEVENT",
		"UID:second@example.com",
		"SUMMARY:Second",
		"DTSTART;VALUE=DATE:20260421",
		"END:VEVENT",
		"END:VCALENDAR",
	}, "\r\n")

	response := request(server, http.MethodPut, objectHref(calendar.ID, "bad.ics"), content, map[string]string{"Content-Type": "text/calendar"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("PUT status = %d, body = %s", response.Code, response.Body.String())
	}
	assertBodyContains(t, response, "exactly one base VEVENT or VTODO")
}

func TestDeleteCalendarObject(t *testing.T) {
	server, svc := newTestServer(t)
	calendar := createCalendar(t, svc)
	dueDate := "2026-04-20"
	task, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Delete me",
		DueDate:    &dueDate,
	})
	if err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}

	deleted := request(server, http.MethodDelete, objectHref(calendar.ID, task.ID+".ics"), "", nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, body = %s", deleted.Code, deleted.Body.String())
	}

	missing := request(server, http.MethodGet, objectHref(calendar.ID, task.ID+".ics"), "", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("GET deleted status = %d, body = %s", missing.Code, missing.Body.String())
	}
}

func request(server *Server, method string, target string, body string, headers map[string]string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	server.ServeHTTP(recorder, request)
	return recorder
}

func assertBodyContains(t *testing.T, response *httptest.ResponseRecorder, value string) {
	t.Helper()

	if !strings.Contains(response.Body.String(), value) {
		t.Fatalf("body = %s, want %q", response.Body.String(), value)
	}
}
