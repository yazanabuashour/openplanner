package caldav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestValidateAddrAcceptsLoopback(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{
		"127.0.0.1:0",
		"127.0.0.2:8080",
		"[::1]:8080",
		"localhost:8080",
	} {
		addr := addr
		t.Run(addr, func(t *testing.T) {
			t.Parallel()

			if err := ValidateAddr(addr); err != nil {
				t.Fatalf("ValidateAddr(%q): %v", addr, err)
			}
		})
	}
}

func TestValidateAddrRejectsNonLoopback(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{
		":8080",
		"0.0.0.0:8080",
		"[::]:8080",
		"192.168.1.10:8080",
		"8.8.8.8:8080",
		"calendar.local:8080",
	} {
		addr := addr
		t.Run(addr, func(t *testing.T) {
			t.Parallel()

			err := ValidateAddr(addr)
			if err == nil {
				t.Fatalf("ValidateAddr(%q) succeeded, want rejection", addr)
			}
			if !strings.Contains(err.Error(), "loopback") {
				t.Fatalf("ValidateAddr(%q) error = %q, want loopback rejection", addr, err)
			}
		})
	}
}

func TestListenAndServeRejectsNonLoopbackBeforeOpeningDatabase(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "caldav.db")
	err := ListenAndServe(context.Background(), Options{Addr: "0.0.0.0:8080", DatabasePath: databasePath})
	if err == nil {
		t.Fatal("ListenAndServe succeeded, want non-loopback rejection")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("ListenAndServe error = %q, want loopback rejection", err)
	}
	if _, statErr := os.Stat(databasePath); !os.IsNotExist(statErr) {
		t.Fatalf("database path exists after rejected addr: %v", statErr)
	}
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

func TestPropfindParserHardening(t *testing.T) {
	server, svc := newTestServer(t)
	calendar := createCalendar(t, svc)

	tests := []struct {
		name        string
		path        string
		body        string
		headers     map[string]string
		wantStatus  int
		contains    []string
		notContains []string
	}{
		{
			name:       "empty body defaults to allprop",
			path:       "/caldav/",
			wantStatus: 207,
			contains:   []string{"calendar-home-set", "/caldav/calendars/local/"},
		},
		{
			name:       "malformed xml is rejected",
			path:       "/caldav/",
			body:       `<propfind xmlns="DAV:"><prop><displayname/>`,
			wantStatus: http.StatusBadRequest,
			contains:   []string{"invalid PROPFIND body"},
		},
		{
			name:       "oversized body is rejected",
			path:       "/caldav/",
			body:       strings.Repeat("x", maxCalDAVRequestBodyBytes+1),
			wantStatus: http.StatusBadRequest,
			contains:   []string{"invalid PROPFIND body"},
		},
		{
			name:       "deeply nested xml is rejected",
			path:       "/caldav/",
			body:       `<propfind xmlns="DAV:">` + nestedXML("x", maxCalDAVXMLDepth+1, "") + `</propfind>`,
			wantStatus: http.StatusBadRequest,
			contains:   []string{"invalid PROPFIND body"},
		},
		{
			name:       "unknown property is isolated to not found propstat",
			path:       calendarHref(calendar.ID),
			body:       `<propfind xmlns="DAV:"><prop><displayname/><unknown-property/></prop></propfind>`,
			wantStatus: 207,
			contains:   []string{"Work", "unknown-property", "HTTP/1.1 404 Not Found"},
		},
		{
			name:       "namespace prefixes are accepted",
			path:       calendarHref(calendar.ID),
			body:       `<D:propfind xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:prop><D:displayname/><C:supported-calendar-component-set/></D:prop></D:propfind>`,
			wantStatus: 207,
			contains:   []string{"Work", "supported-calendar-component-set", "VEVENT", "VTODO"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := request(server, "PROPFIND", test.path, test.body, test.headers)
			if response.Code != test.wantStatus {
				t.Fatalf("PROPFIND status = %d, want %d, body = %s", response.Code, test.wantStatus, response.Body.String())
			}
			for _, value := range test.contains {
				assertBodyContains(t, response, value)
			}
			for _, value := range test.notContains {
				assertBodyNotContains(t, response, value)
			}
		})
	}
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

func TestCalendarMultigetReturnsOnlyRequestedObjects(t *testing.T) {
	server, svc := newTestServer(t)
	calendar := createCalendar(t, svc)
	start := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	event, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Requested event",
		StartAt:    &start,
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}
	dueDate := "2026-04-20"
	task, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Requested task",
		DueDate:    &dueDate,
	})
	if err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}
	extra, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Not requested",
		StartAt:    &start,
	})
	if err != nil {
		t.Fatalf("CreateEvent(extra): %v", err)
	}

	body := calendarMultigetBody(
		objectHref(calendar.ID, event.ID+".ics"),
		objectHref(calendar.ID, task.ID+".ics"),
	)
	response := request(server, "REPORT", calendarHref(calendar.ID), body, nil)
	if response.Code != 207 {
		t.Fatalf("calendar-multiget status = %d, body = %s", response.Code, response.Body.String())
	}
	assertBodyContains(t, response, event.ID+".ics")
	assertBodyContains(t, response, task.ID+".ics")
	assertBodyContains(t, response, "SUMMARY:Requested event")
	assertBodyContains(t, response, "SUMMARY:Requested task")
	assertBodyNotContains(t, response, extra.ID+".ics")
	assertBodyNotContains(t, response, "SUMMARY:Not requested")
}

func TestCalendarMultigetSupportsNestedCalendarDataAndObjectProps(t *testing.T) {
	server, svc := newTestServer(t)
	calendar := createCalendar(t, svc)
	start := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	event, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Prop request",
		StartAt:    &start,
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}

	body := strings.Join([]string{
		`<C:calendar-multiget xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">`,
		`<D:prop>`,
		`<D:getetag/>`,
		`<D:getcontenttype/>`,
		`<D:getcontentlength/>`,
		`<C:calendar-data><C:comp name="VCALENDAR"><C:comp name="VEVENT"/></C:comp></C:calendar-data>`,
		`</D:prop>`,
		`<D:href>`, objectHref(calendar.ID, event.ID+".ics"), `</D:href>`,
		`</C:calendar-multiget>`,
	}, "")
	response := request(server, "REPORT", calendarHref(calendar.ID), body, nil)
	if response.Code != 207 {
		t.Fatalf("calendar-multiget status = %d, body = %s", response.Code, response.Body.String())
	}
	assertBodyContains(t, response, "getcontenttype")
	assertBodyContains(t, response, "text/calendar")
	assertBodyContains(t, response, "getcontentlength")
	assertBodyContains(t, response, "SUMMARY:Prop request")
	assertBodyNotContains(t, response, "HTTP/1.1 404 Not Found")
	assertBodyNotContains(t, response, "<comp")
}

func TestCalendarMultigetReportsMissingAndCrossCalendarObjectsAsNotFound(t *testing.T) {
	server, svc := newTestServer(t)
	calendar := createCalendar(t, svc)
	otherCalendar, err := svc.CreateCalendar(domain.Calendar{Name: "Other"})
	if err != nil {
		t.Fatalf("CreateCalendar(other): %v", err)
	}
	start := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	otherEvent, err := svc.CreateEvent(domain.Event{
		CalendarID: otherCalendar.ID,
		Title:      "Other event",
		StartAt:    &start,
	})
	if err != nil {
		t.Fatalf("CreateEvent(other): %v", err)
	}

	missing := objectHref(calendar.ID, "missing.ics")
	crossCalendar := objectHref(otherCalendar.ID, otherEvent.ID+".ics")
	body := calendarMultigetBody(missing, crossCalendar)
	response := request(server, "REPORT", calendarHref(calendar.ID), body, nil)
	if response.Code != 207 {
		t.Fatalf("calendar-multiget status = %d, body = %s", response.Code, response.Body.String())
	}
	assertBodyContains(t, response, missing)
	assertBodyContains(t, response, crossCalendar)
	if strings.Count(response.Body.String(), "HTTP/1.1 404 Not Found") != 2 {
		t.Fatalf("body = %s, want two 404 propstats", response.Body.String())
	}
	assertBodyNotContains(t, response, "SUMMARY:Other event")
}

func TestReportParserHardening(t *testing.T) {
	server, svc := newTestServer(t)
	calendar := createCalendar(t, svc)
	otherCalendar, err := svc.CreateCalendar(domain.Calendar{Name: "Other"})
	if err != nil {
		t.Fatalf("CreateCalendar(other): %v", err)
	}
	start := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	event, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Report event",
		StartAt:    &start,
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}
	otherEvent, err := svc.CreateEvent(domain.Event{
		CalendarID: otherCalendar.ID,
		Title:      "Other event",
		StartAt:    &start,
	})
	if err != nil {
		t.Fatalf("CreateEvent(other): %v", err)
	}
	object := objectHref(calendar.ID, event.ID+".ics")
	missing := objectHref(calendar.ID, "missing.ics")
	crossCalendar := objectHref(otherCalendar.ID, otherEvent.ID+".ics")

	tests := []struct {
		name        string
		body        string
		wantStatus  int
		contains    []string
		notContains []string
	}{
		{
			name:       "malformed xml is rejected",
			body:       `<calendar-query xmlns="urn:ietf:params:xml:ns:caldav"><filter>`,
			wantStatus: http.StatusBadRequest,
			contains:   []string{"invalid REPORT"},
		},
		{
			name: "malformed time range is rejected",
			body: strings.Join([]string{
				`<calendar-query xmlns="urn:ietf:params:xml:ns:caldav">`,
				`<filter><comp-filter name="VCALENDAR"><comp-filter name="VEVENT">`,
				`<time-range start="not-a-date" end="20260421T000000Z"/>`,
				`</comp-filter></comp-filter></filter>`,
				`</calendar-query>`,
			}, ""),
			wantStatus: http.StatusBadRequest,
			contains:   []string{"invalid REPORT"},
		},
		{
			name:       "oversized body is rejected",
			body:       strings.Repeat("x", maxCalDAVRequestBodyBytes+1),
			wantStatus: http.StatusBadRequest,
			contains:   []string{"invalid REPORT"},
		},
		{
			name:       "deeply nested filter is rejected",
			body:       `<calendar-query xmlns="urn:ietf:params:xml:ns:caldav">` + nestedXML("comp-filter", maxCalDAVXMLDepth+1, "") + `</calendar-query>`,
			wantStatus: http.StatusBadRequest,
			contains:   []string{"invalid REPORT"},
		},
		{
			name:       "unknown requested property is isolated to not found propstat",
			body:       `<C:calendar-multiget xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav" xmlns:X="urn:test"><D:prop><D:getetag/><X:unknown-property/></D:prop><D:href>` + object + `</D:href></C:calendar-multiget>`,
			wantStatus: 207,
			contains:   []string{object, "unknown-property", "HTTP/1.1 404 Not Found"},
		},
		{
			name:       "empty calendar query is safe",
			body:       `<calendar-query xmlns="urn:ietf:params:xml:ns:caldav"/>`,
			wantStatus: 207,
			contains:   []string{event.ID + ".ics", "SUMMARY:Report event"},
		},
		{
			name:       "calendar multiget trims href text",
			body:       `<C:calendar-multiget xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:prop><D:getetag/><C:calendar-data/></D:prop><D:href> ` + object + "\n" + `</D:href></C:calendar-multiget>`,
			wantStatus: 207,
			contains:   []string{object, "SUMMARY:Report event"},
		},
		{
			name:        "calendar multiget with no hrefs returns empty multistatus",
			body:        `<C:calendar-multiget xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:prop><D:getetag/><C:calendar-data/></D:prop></C:calendar-multiget>`,
			wantStatus:  207,
			contains:    []string{"multistatus"},
			notContains: []string{event.ID + ".ics", "SUMMARY:Report event"},
		},
		{
			name:        "missing and cross-calendar hrefs are not found",
			body:        calendarMultigetBody(missing, crossCalendar),
			wantStatus:  207,
			contains:    []string{missing, crossCalendar, "HTTP/1.1 404 Not Found"},
			notContains: []string{"SUMMARY:Other event"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := request(server, "REPORT", calendarHref(calendar.ID), test.body, nil)
			if response.Code != test.wantStatus {
				t.Fatalf("REPORT status = %d, want %d, body = %s", response.Code, test.wantStatus, response.Body.String())
			}
			for _, value := range test.contains {
				assertBodyContains(t, response, value)
			}
			for _, value := range test.notContains {
				assertBodyNotContains(t, response, value)
			}
		})
	}
}

func TestCalendarObjectETagsAreStableAcrossUnchangedSyncReads(t *testing.T) {
	server, svc := newTestServer(t)
	calendar := createCalendar(t, svc)
	start := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	event, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Stable",
		StartAt:    &start,
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}
	object := objectHref(calendar.ID, event.ID+".ics")

	firstGet := request(server, http.MethodGet, object, "", nil)
	if firstGet.Code != http.StatusOK {
		t.Fatalf("first GET status = %d, body = %s", firstGet.Code, firstGet.Body.String())
	}
	time.Sleep(1100 * time.Millisecond)
	secondGet := request(server, http.MethodGet, object, "", nil)
	if secondGet.Code != http.StatusOK {
		t.Fatalf("second GET status = %d, body = %s", secondGet.Code, secondGet.Body.String())
	}
	if firstGet.Header().Get("ETag") != secondGet.Header().Get("ETag") || firstGet.Body.String() != secondGet.Body.String() {
		t.Fatalf("unchanged GET was unstable: first etag %q, second etag %q", firstGet.Header().Get("ETag"), secondGet.Header().Get("ETag"))
	}

	propfindBody := `<propfind xmlns="DAV:"><prop><getetag/></prop></propfind>`
	firstPropfind := request(server, "PROPFIND", calendarHref(calendar.ID), propfindBody, map[string]string{"Depth": "1"})
	time.Sleep(1100 * time.Millisecond)
	secondPropfind := request(server, "PROPFIND", calendarHref(calendar.ID), propfindBody, map[string]string{"Depth": "1"})
	if firstPropfind.Body.String() != secondPropfind.Body.String() {
		t.Fatalf("unchanged PROPFIND body changed:\nfirst=%s\nsecond=%s", firstPropfind.Body.String(), secondPropfind.Body.String())
	}

	multigetBody := calendarMultigetBody(object)
	firstMultiget := request(server, "REPORT", calendarHref(calendar.ID), multigetBody, nil)
	time.Sleep(1100 * time.Millisecond)
	secondMultiget := request(server, "REPORT", calendarHref(calendar.ID), multigetBody, nil)
	if firstMultiget.Body.String() != secondMultiget.Body.String() {
		t.Fatalf("unchanged calendar-multiget body changed:\nfirst=%s\nsecond=%s", firstMultiget.Body.String(), secondMultiget.Body.String())
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
	if created.Header().Get("Location") != objectHref(calendar.ID, "client-event@example.com.ics") {
		t.Fatalf("PUT create Location = %q, want client resource href", created.Header().Get("Location"))
	}
	if created.Header().Get("ETag") == "" {
		t.Fatal("PUT create ETag is empty")
	}

	fetched := request(server, http.MethodGet, objectHref(calendar.ID, "client-event@example.com.ics"), "", nil)
	if fetched.Code != http.StatusOK {
		t.Fatalf("GET created status = %d, body = %s", fetched.Code, fetched.Body.String())
	}
	assertBodyContains(t, fetched, "SUMMARY:Client event")
	assertBodyContains(t, fetched, "UID:client-event@example.com")

	collection := request(server, "PROPFIND", calendarHref(calendar.ID), `<propfind xmlns="DAV:"><prop><getetag/></prop></propfind>`, map[string]string{"Depth": "1"})
	if collection.Code != 207 {
		t.Fatalf("collection PROPFIND status = %d, body = %s", collection.Code, collection.Body.String())
	}
	assertBodyContains(t, collection, objectHref(calendar.ID, "client-event@example.com.ics"))

	updateContent := strings.Replace(createContent, "SUMMARY:Client event", "SUMMARY:Updated client event", 1)
	updated := request(server, http.MethodPut, objectHref(calendar.ID, "client-event@example.com.ics"), updateContent, map[string]string{"Content-Type": "text/calendar"})
	if updated.Code != http.StatusNoContent {
		t.Fatalf("PUT update status = %d, body = %s", updated.Code, updated.Body.String())
	}
	if updated.Header().Get("ETag") == "" {
		t.Fatal("PUT update ETag is empty")
	}

	refetched := request(server, http.MethodGet, objectHref(calendar.ID, "client-event@example.com.ics"), "", nil)
	if refetched.Code != http.StatusOK {
		t.Fatalf("GET updated status = %d, body = %s", refetched.Code, refetched.Body.String())
	}
	assertBodyContains(t, refetched, "SUMMARY:Updated client event")
	assertBodyContains(t, refetched, "UID:client-event@example.com")
}

func TestPutCalendarObjectWithSlashUIDUsesRoutableResource(t *testing.T) {
	server, svc := newTestServer(t)
	calendar := createCalendar(t, svc)
	createContent := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:client/event@example.com",
		"SUMMARY:Slash UID event",
		"DTSTART;VALUE=DATE:20260420",
		"END:VEVENT",
		"END:VCALENDAR",
	}, "\r\n")

	created := request(server, http.MethodPut, objectHref(calendar.ID, "random-client-resource.ics"), createContent, map[string]string{"Content-Type": "text/calendar"})
	if created.Code != http.StatusCreated {
		t.Fatalf("PUT create status = %d, body = %s", created.Code, created.Body.String())
	}
	location := created.Header().Get("Location")
	wantLocation := objectHref(calendar.ID, pathEscape("client/event@example.com")+".ics")
	if location != wantLocation {
		t.Fatalf("PUT create Location = %q, want routable escaped UID resource %q", location, wantLocation)
	}

	fetched := request(server, http.MethodGet, location, "", nil)
	if fetched.Code != http.StatusOK {
		t.Fatalf("GET slash UID status = %d, body = %s", fetched.Code, fetched.Body.String())
	}
	assertBodyContains(t, fetched, "SUMMARY:Slash UID event")
	assertBodyContains(t, fetched, "UID:client/event@example.com")

	collection := request(server, "PROPFIND", calendarHref(calendar.ID), `<propfind xmlns="DAV:"><prop><getetag/></prop></propfind>`, map[string]string{"Depth": "1"})
	if collection.Code != 207 {
		t.Fatalf("collection PROPFIND status = %d, body = %s", collection.Code, collection.Body.String())
	}
	assertBodyContains(t, collection, location)

	multiget := request(server, "REPORT", calendarHref(calendar.ID), calendarMultigetBody(location), nil)
	if multiget.Code != 207 {
		t.Fatalf("calendar-multiget status = %d, body = %s", multiget.Code, multiget.Body.String())
	}
	assertBodyContains(t, multiget, "SUMMARY:Slash UID event")

	deleted := request(server, http.MethodDelete, location, "", nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("DELETE slash UID status = %d, body = %s", deleted.Code, deleted.Body.String())
	}
}

func TestPutTaskCompletionUpdatesETagAndExport(t *testing.T) {
	server, svc := newTestServer(t)
	calendar := createCalendar(t, svc)
	createContent := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VTODO",
		"UID:client-task@example.com",
		"SUMMARY:Client task",
		"DUE;VALUE=DATE:20260420",
		"STATUS:NEEDS-ACTION",
		"END:VTODO",
		"END:VCALENDAR",
	}, "\r\n")

	created := request(server, http.MethodPut, objectHref(calendar.ID, "random-client-resource.ics"), createContent, map[string]string{"Content-Type": "text/calendar"})
	if created.Code != http.StatusCreated {
		t.Fatalf("PUT create status = %d, body = %s", created.Code, created.Body.String())
	}
	if created.Header().Get("Location") != objectHref(calendar.ID, "client-task@example.com.ics") {
		t.Fatalf("PUT create Location = %q, want canonical UID resource href", created.Header().Get("Location"))
	}
	if created.Header().Get("ETag") == "" {
		t.Fatal("PUT create ETag is empty")
	}
	before := request(server, http.MethodGet, objectHref(calendar.ID, "client-task@example.com.ics"), "", nil)
	if before.Code != http.StatusOK {
		t.Fatalf("GET before status = %d, body = %s", before.Code, before.Body.String())
	}

	updateContent := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VTODO",
		"UID:client-task@example.com",
		"SUMMARY:Client task",
		"DUE;VALUE=DATE:20260420",
		"STATUS:COMPLETED",
		"COMPLETED:20260420T120000Z",
		"END:VTODO",
		"END:VCALENDAR",
	}, "\r\n")
	updated := request(server, http.MethodPut, objectHref(calendar.ID, "client-task@example.com.ics"), updateContent, map[string]string{"Content-Type": "text/calendar"})
	if updated.Code != http.StatusNoContent {
		t.Fatalf("PUT update status = %d, body = %s", updated.Code, updated.Body.String())
	}

	after := request(server, http.MethodGet, objectHref(calendar.ID, "client-task@example.com.ics"), "", nil)
	if after.Code != http.StatusOK {
		t.Fatalf("GET after status = %d, body = %s", after.Code, after.Body.String())
	}
	if before.Header().Get("ETag") == after.Header().Get("ETag") {
		t.Fatalf("ETag did not change after task completion update: %s", after.Header().Get("ETag"))
	}
	assertBodyContains(t, after, "STATUS:COMPLETED")
	assertBodyContains(t, after, "COMPLETED:20260420T120000Z")
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

func TestPutRejectsOversizedBodyWithoutCreatingObject(t *testing.T) {
	server, svc := newTestServer(t)
	calendar := createCalendar(t, svc)

	response := request(server, http.MethodPut, objectHref(calendar.ID, "oversized.ics"), strings.Repeat("x", maxCalDAVRequestBodyBytes+1), map[string]string{"Content-Type": "text/calendar"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("PUT status = %d, body = %s", response.Code, response.Body.String())
	}
	assertBodyContains(t, response, "read request body")
	assertBodyContains(t, response, "exceeds")

	fetched := request(server, http.MethodGet, objectHref(calendar.ID, "oversized.ics"), "", nil)
	if fetched.Code != http.StatusNotFound {
		t.Fatalf("GET oversized rejected object status = %d, body = %s", fetched.Code, fetched.Body.String())
	}
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

func calendarMultigetBody(hrefs ...string) string {
	var builder strings.Builder
	builder.WriteString(`<C:calendar-multiget xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:prop><D:getetag/><C:calendar-data/></D:prop>`)
	for _, href := range hrefs {
		builder.WriteString(`<D:href>`)
		builder.WriteString(href)
		builder.WriteString(`</D:href>`)
	}
	builder.WriteString(`</C:calendar-multiget>`)
	return builder.String()
}

func FuzzParsePropfind(f *testing.F) {
	for _, seed := range []string{
		"",
		`<propfind xmlns="DAV:"><allprop/></propfind>`,
		`<propfind xmlns="DAV:"><prop><displayname/><unknown-property/></prop></propfind>`,
		`<D:propfind xmlns:D="DAV:"><D:prop><D:getetag/></D:prop></D:propfind>`,
		`<propfind xmlns="DAV:"><prop><displayname/>`,
		`<propfind xmlns="DAV:">` + nestedXML("x", 8, "") + `</propfind>`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, content string) {
		allProperties, requested, err := parsePropfind(strings.NewReader(content))
		if err != nil {
			return
		}
		if allProperties && len(requested) != 0 {
			t.Fatalf("allProperties with requested properties: %#v", requested)
		}
		if !allProperties && len(requested) == 0 {
			t.Fatal("specific propfind returned no requested properties")
		}
		for _, name := range requested {
			if strings.TrimSpace(name.Local) == "" {
				t.Fatalf("requested property has empty local name: %#v", name)
			}
		}
	})
}

func FuzzParseReport(f *testing.F) {
	for _, seed := range []string{
		"",
		`<calendar-query xmlns="urn:ietf:params:xml:ns:caldav"/>`,
		`<calendar-query xmlns="urn:ietf:params:xml:ns:caldav"><filter><comp-filter name="VCALENDAR"><comp-filter name="VEVENT"><time-range start="20260420T000000Z" end="20260421T000000Z"/></comp-filter></comp-filter></filter></calendar-query>`,
		`<C:calendar-multiget xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav"><D:prop><D:getetag/><C:calendar-data/></D:prop><D:href>/caldav/calendars/local/calendar/event.ics</D:href></C:calendar-multiget>`,
		`<calendar-query xmlns="urn:ietf:params:xml:ns:caldav"><filter>`,
		`<calendar-query xmlns="urn:ietf:params:xml:ns:caldav">` + nestedXML("comp-filter", 8, "") + `</calendar-query>`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, content string) {
		report, err := parseReport(strings.NewReader(content))
		if err != nil {
			return
		}
		if report.kind != reportCalendarQuery && report.kind != reportCalendarMultiget {
			t.Fatalf("unknown report kind: %v", report.kind)
		}
		for _, name := range report.requested {
			if strings.TrimSpace(name.Local) == "" {
				t.Fatalf("requested property has empty local name: %#v", name)
			}
		}
		for _, href := range report.hrefs {
			if strings.TrimSpace(href) == "" {
				t.Fatalf("empty href survived parsing: %#v", report.hrefs)
			}
		}
		if report.query.hasTimeRange && !report.query.to.After(report.query.from) {
			t.Fatalf("invalid time range: from=%s to=%s", report.query.from, report.query.to)
		}
	})
}

func nestedXML(name string, depth int, leaf string) string {
	var builder strings.Builder
	for range depth {
		builder.WriteString("<")
		builder.WriteString(name)
		builder.WriteString(">")
	}
	builder.WriteString(leaf)
	for range depth {
		builder.WriteString("</")
		builder.WriteString(name)
		builder.WriteString(">")
	}
	return builder.String()
}

func assertBodyContains(t *testing.T, response *httptest.ResponseRecorder, value string) {
	t.Helper()

	if !strings.Contains(response.Body.String(), value) {
		t.Fatalf("body = %s, want %q", response.Body.String(), value)
	}
}

func assertBodyNotContains(t *testing.T, response *httptest.ResponseRecorder, value string) {
	t.Helper()

	if strings.Contains(response.Body.String(), value) {
		t.Fatalf("body = %s, did not want %q", response.Body.String(), value)
	}
}
