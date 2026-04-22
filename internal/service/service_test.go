package service_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yazanabuashour/openplanner/internal/domain"
	"github.com/yazanabuashour/openplanner/internal/service"
	"github.com/yazanabuashour/openplanner/internal/store"
)

func newTestService(t *testing.T) *service.Service {
	t.Helper()

	repository, err := store.Open(filepath.Join(t.TempDir(), "openplanner.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := repository.Close(); closeErr != nil {
			t.Fatalf("close store: %v", closeErr)
		}
	})

	return service.New(repository)
}

func createCalendar(t *testing.T, svc *service.Service) domain.Calendar {
	t.Helper()

	calendar, err := svc.CreateCalendar(domain.Calendar{Name: "Primary"})
	if err != nil {
		t.Fatalf("create calendar: %v", err)
	}

	return calendar
}

func TestCreateEventRejectsMixedTiming(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	startAt := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)
	startDate := "2026-04-16"

	_, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Mixed timing",
		StartAt:    &startAt,
		StartDate:  &startDate,
	})
	if err == nil {
		t.Fatal("CreateEvent() error = nil, want validation error")
	}

	var validationErr *service.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("CreateEvent() error = %T, want ValidationError", err)
	}
}

func TestListTasksPaginatesWithCursor(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)

	for _, title := range []string{"one", "two", "three"} {
		if _, err := svc.CreateTask(domain.Task{
			CalendarID: calendar.ID,
			Title:      title,
			DueDate:    stringPtr("2026-04-16"),
		}); err != nil {
			t.Fatalf("CreateTask(%q): %v", title, err)
		}
	}

	firstPage, err := svc.ListTasks(domain.TaskListParams{PageParams: domain.PageParams{Limit: 2}})
	if err != nil {
		t.Fatalf("ListTasks(first page): %v", err)
	}
	if len(firstPage.Items) != 2 {
		t.Fatalf("first page length = %d, want 2", len(firstPage.Items))
	}
	if firstPage.NextCursor == nil {
		t.Fatal("first page next cursor = nil")
	}

	secondPage, err := svc.ListTasks(domain.TaskListParams{PageParams: domain.PageParams{Cursor: *firstPage.NextCursor, Limit: 2}})
	if err != nil {
		t.Fatalf("ListTasks(second page): %v", err)
	}
	if len(secondPage.Items) != 1 {
		t.Fatalf("second page length = %d, want 1", len(secondPage.Items))
	}
}

func TestListTasksRejectsStaleCursor(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)

	for _, title := range []string{"one", "two", "three"} {
		if _, err := svc.CreateTask(domain.Task{
			CalendarID: calendar.ID,
			Title:      title,
			DueDate:    stringPtr("2026-04-16"),
		}); err != nil {
			t.Fatalf("CreateTask(%q): %v", title, err)
		}
	}

	firstPage, err := svc.ListTasks(domain.TaskListParams{PageParams: domain.PageParams{Limit: 1}})
	if err != nil {
		t.Fatalf("ListTasks(first page): %v", err)
	}
	if firstPage.NextCursor == nil {
		t.Fatal("first page next cursor = nil")
	}
	if err := svc.DeleteTask(firstPage.Items[0].ID); err != nil {
		t.Fatalf("DeleteTask(): %v", err)
	}

	_, err = svc.ListTasks(domain.TaskListParams{PageParams: domain.PageParams{Cursor: *firstPage.NextCursor, Limit: 1}})
	if err == nil {
		t.Fatal("ListTasks(stale cursor) error = nil, want validation error")
	}

	var validationErr *service.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("ListTasks(stale cursor) error = %T, want ValidationError", err)
	}
}

func TestListAgendaRejectsInvalidCursor(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	startAt := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)

	if _, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Planning",
		StartAt:    &startAt,
	}); err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}

	_, err := svc.ListAgenda(domain.AgendaParams{
		From:   time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC),
		To:     time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC),
		Cursor: "not-a-valid-cursor",
		Limit:  10,
	})
	if err == nil {
		t.Fatal("ListAgenda(invalid cursor) error = nil, want validation error")
	}

	var validationErr *service.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("ListAgenda(invalid cursor) error = %T, want ValidationError", err)
	}
}

func TestEventTaskLinksCreateListDeleteAndAgendaExposure(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)

	event, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Planning",
		StartDate:  stringPtr("2026-04-16"),
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}
	task, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Prep notes",
		DueDate:    stringPtr("2026-04-16"),
	})
	if err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}

	link, err := svc.CreateEventTaskLink(event.ID, task.ID)
	if err != nil {
		t.Fatalf("CreateEventTaskLink(): %v", err)
	}
	if link.EventID != event.ID || link.TaskID != task.ID {
		t.Fatalf("link = %#v, want event/task ids", link)
	}

	eventLinks, err := svc.ListEventTaskLinks(domain.EventTaskLinkFilter{EventID: event.ID})
	if err != nil {
		t.Fatalf("ListEventTaskLinks(event): %v", err)
	}
	if len(eventLinks) != 1 || eventLinks[0].TaskID != task.ID {
		t.Fatalf("event links = %#v, want linked task", eventLinks)
	}
	taskLinks, err := svc.ListEventTaskLinks(domain.EventTaskLinkFilter{TaskID: task.ID})
	if err != nil {
		t.Fatalf("ListEventTaskLinks(task): %v", err)
	}
	if len(taskLinks) != 1 || taskLinks[0].EventID != event.ID {
		t.Fatalf("task links = %#v, want linked event", taskLinks)
	}

	storedEvent, err := svc.GetEvent(event.ID)
	if err != nil {
		t.Fatalf("GetEvent(): %v", err)
	}
	if !slices.Equal(storedEvent.LinkedTaskIDs, []string{task.ID}) {
		t.Fatalf("event linked tasks = %v, want %v", storedEvent.LinkedTaskIDs, []string{task.ID})
	}
	storedTask, err := svc.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask(): %v", err)
	}
	if !slices.Equal(storedTask.LinkedEventIDs, []string{event.ID}) {
		t.Fatalf("task linked events = %v, want %v", storedTask.LinkedEventIDs, []string{event.ID})
	}

	agenda, err := svc.ListAgenda(domain.AgendaParams{
		From:  time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListAgenda(): %v", err)
	}
	if len(agenda.Items) != 2 {
		t.Fatalf("agenda = %#v, want two linked items", agenda.Items)
	}
	for _, item := range agenda.Items {
		switch item.Kind {
		case domain.AgendaItemKindEvent:
			if !slices.Equal(item.LinkedTaskIDs, []string{task.ID}) {
				t.Fatalf("event agenda linked tasks = %v, want %v", item.LinkedTaskIDs, []string{task.ID})
			}
		case domain.AgendaItemKindTask:
			if !slices.Equal(item.LinkedEventIDs, []string{event.ID}) {
				t.Fatalf("task agenda linked events = %v, want %v", item.LinkedEventIDs, []string{event.ID})
			}
		}
	}

	if err := svc.DeleteEventTaskLink(event.ID, task.ID); err != nil {
		t.Fatalf("DeleteEventTaskLink(): %v", err)
	}
	linksAfterDelete, err := svc.ListEventTaskLinks(domain.EventTaskLinkFilter{})
	if err != nil {
		t.Fatalf("ListEventTaskLinks(after delete): %v", err)
	}
	if len(linksAfterDelete) != 0 {
		t.Fatalf("links after delete = %#v, want none", linksAfterDelete)
	}
}

func TestEventTaskLinkValidationRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	event, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Planning",
		StartDate:  stringPtr("2026-04-16"),
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}
	task, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Prep",
		DueDate:    stringPtr("2026-04-16"),
	})
	if err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}

	_, err = svc.CreateEventTaskLink("not-a-ulid", task.ID)
	if err == nil {
		t.Fatal("CreateEventTaskLink(invalid event) error = nil, want validation error")
	}
	var validationErr *service.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("CreateEventTaskLink(invalid event) error = %T, want ValidationError", err)
	}

	_, err = svc.CreateEventTaskLink(event.ID, "not-a-ulid")
	if err == nil {
		t.Fatal("CreateEventTaskLink(invalid task) error = nil, want validation error")
	}
	if !errors.As(err, &validationErr) {
		t.Fatalf("CreateEventTaskLink(invalid task) error = %T, want ValidationError", err)
	}

	missingID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	_, err = svc.CreateEventTaskLink(missingID, task.ID)
	if err == nil {
		t.Fatal("CreateEventTaskLink(missing event) error = nil, want not found")
	}
	var notFoundErr *service.NotFoundError
	if !errors.As(err, &notFoundErr) {
		t.Fatalf("CreateEventTaskLink(missing event) error = %T, want NotFoundError", err)
	}

	_, err = svc.CreateEventTaskLink(event.ID, task.ID)
	if err != nil {
		t.Fatalf("CreateEventTaskLink(): %v", err)
	}
	_, err = svc.CreateEventTaskLink(event.ID, task.ID)
	if err == nil {
		t.Fatal("CreateEventTaskLink(duplicate) error = nil, want conflict")
	}
	var conflictErr *service.ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("CreateEventTaskLink(duplicate) error = %T, want ConflictError", err)
	}

	if err := svc.DeleteEventTaskLink(event.ID, missingID); err == nil {
		t.Fatal("DeleteEventTaskLink(missing task) error = nil, want not found")
	} else if !errors.As(err, &notFoundErr) {
		t.Fatalf("DeleteEventTaskLink(missing task) error = %T, want NotFoundError", err)
	}
	if err := svc.DeleteEventTaskLink(event.ID, task.ID); err != nil {
		t.Fatalf("DeleteEventTaskLink(): %v", err)
	}
	if err := svc.DeleteEventTaskLink(event.ID, task.ID); err == nil {
		t.Fatal("DeleteEventTaskLink(missing link) error = nil, want not found")
	} else if !errors.As(err, &notFoundErr) {
		t.Fatalf("DeleteEventTaskLink(missing link) error = %T, want NotFoundError", err)
	}
}

func TestListEndpointsRejectInvalidCalendarFilter(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "events",
			call: func() error {
				_, err := svc.ListEvents(domain.PageParams{CalendarID: "not-a-ulid"})
				return err
			},
		},
		{
			name: "tasks",
			call: func() error {
				_, err := svc.ListTasks(domain.TaskListParams{PageParams: domain.PageParams{CalendarID: "not-a-ulid"}})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if err == nil {
				t.Fatal("error = nil, want validation error")
			}

			var validationErr *service.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T, want ValidationError", err)
			}
		})
	}
}

func TestCompleteRecurringTaskPerOccurrence(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)

	task, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Recurring",
		DueDate:    stringPtr("2026-04-16"),
		Recurrence: &domain.RecurrenceRule{
			Frequency: domain.RecurrenceFrequencyDaily,
			Count:     int32Ptr(3),
		},
	})
	if err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}

	firstCompletion, err := svc.CompleteTask(task.ID, domain.TaskCompletionRequest{
		OccurrenceDate: stringPtr("2026-04-16"),
	})
	if err != nil {
		t.Fatalf("CompleteTask(first): %v", err)
	}
	if firstCompletion.OccurrenceDate == nil || *firstCompletion.OccurrenceDate != "2026-04-16" {
		t.Fatalf("first completion date = %v", firstCompletion.OccurrenceDate)
	}

	_, err = svc.CompleteTask(task.ID, domain.TaskCompletionRequest{
		OccurrenceDate: stringPtr("2026-04-16"),
	})
	if err == nil {
		t.Fatal("CompleteTask(duplicate) error = nil, want conflict")
	}
	var conflictErr *service.ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("CompleteTask(duplicate) error = %T, want ConflictError", err)
	}

	secondCompletion, err := svc.CompleteTask(task.ID, domain.TaskCompletionRequest{
		OccurrenceDate: stringPtr("2026-04-17"),
	})
	if err != nil {
		t.Fatalf("CompleteTask(second occurrence): %v", err)
	}
	if secondCompletion.OccurrenceDate == nil || *secondCompletion.OccurrenceDate != "2026-04-17" {
		t.Fatalf("second completion date = %v", secondCompletion.OccurrenceDate)
	}
}

func TestRecurringEventOccurrenceCancellationAndRescheduleAffectAgenda(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	startAt := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)
	endAt := startAt.Add(30 * time.Minute)
	count := int32(3)

	event, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Standup",
		StartAt:    &startAt,
		EndAt:      &endAt,
		Recurrence: &domain.RecurrenceRule{
			Frequency: domain.RecurrenceFrequencyDaily,
			Count:     &count,
		},
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}

	_, err = svc.CancelEventOccurrence(event.ID, domain.OccurrenceMutationRequest{
		OccurrenceAt: timePtr(time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("CancelEventOccurrence(): %v", err)
	}
	moved, err := svc.RescheduleEventOccurrence(event.ID, domain.OccurrenceMutationRequest{
		OccurrenceAt:  timePtr(time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)),
		ReplacementAt: timePtr(time.Date(2026, 4, 19, 11, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("RescheduleEventOccurrence(): %v", err)
	}

	page, err := svc.ListAgenda(domain.AgendaParams{
		From:  time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListAgenda(): %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("agenda items = %#v, want original and moved occurrence", page.Items)
	}
	if page.Items[0].StartAt == nil || page.Items[0].StartAt.Format(time.RFC3339) != "2026-04-16T09:00:00Z" {
		t.Fatalf("first item = %#v, want original first occurrence", page.Items[0])
	}
	if page.Items[1].OccurrenceKey != moved.OccurrenceKey || page.Items[1].StartAt == nil || page.Items[1].StartAt.Format(time.RFC3339) != "2026-04-19T11:00:00Z" {
		t.Fatalf("moved item = %#v, want stable key %q at replacement time", page.Items[1], moved.OccurrenceKey)
	}
}

func TestTimezoneAwareRecurringEventKeepsLocalTimeAcrossSpringDST(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	timeZone := "America/New_York"
	startAt := time.Date(2026, 3, 3, 9, 0, 0, 0, time.FixedZone("", -5*60*60))
	count := int32(2)

	_, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Weekly sync",
		StartAt:    &startAt,
		TimeZone:   &timeZone,
		Recurrence: &domain.RecurrenceRule{
			Frequency: domain.RecurrenceFrequencyWeekly,
			Count:     &count,
		},
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}

	page, err := svc.ListAgenda(domain.AgendaParams{
		From:  time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListAgenda(): %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("agenda items = %#v, want two occurrences", page.Items)
	}
	if page.Items[0].StartAt == nil || page.Items[0].StartAt.Format(time.RFC3339) != "2026-03-03T09:00:00-05:00" {
		t.Fatalf("first occurrence = %#v, want 09:00 EST", page.Items[0])
	}
	if page.Items[1].StartAt == nil || page.Items[1].StartAt.Format(time.RFC3339) != "2026-03-10T09:00:00-04:00" {
		t.Fatalf("second occurrence = %#v, want 09:00 EDT", page.Items[1])
	}
	if page.Items[1].TimeZone == nil || *page.Items[1].TimeZone != timeZone {
		t.Fatalf("agenda timezone = %#v, want %q", page.Items[1].TimeZone, timeZone)
	}
}

func TestTimezoneAwareRecurringEventKeepsLocalTimeAcrossFallDST(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	timeZone := "America/New_York"
	startAt := time.Date(2026, 10, 27, 9, 0, 0, 0, time.FixedZone("", -4*60*60))
	count := int32(2)

	_, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Weekly sync",
		StartAt:    &startAt,
		TimeZone:   &timeZone,
		Recurrence: &domain.RecurrenceRule{
			Frequency: domain.RecurrenceFrequencyWeekly,
			Count:     &count,
		},
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}

	page, err := svc.ListAgenda(domain.AgendaParams{
		From:  time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListAgenda(): %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("agenda items = %#v, want two occurrences", page.Items)
	}
	if page.Items[0].StartAt == nil || page.Items[0].StartAt.Format(time.RFC3339) != "2026-10-27T09:00:00-04:00" {
		t.Fatalf("first occurrence = %#v, want 09:00 EDT", page.Items[0])
	}
	if page.Items[1].StartAt == nil || page.Items[1].StartAt.Format(time.RFC3339) != "2026-11-03T09:00:00-05:00" {
		t.Fatalf("second occurrence = %#v, want 09:00 EST", page.Items[1])
	}
}

func TestTimezoneAwareEventValidationAndOccurrenceMutation(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	timeZone := "America/New_York"
	startAt := time.Date(2026, 3, 3, 9, 0, 0, 0, time.FixedZone("", -5*60*60))
	count := int32(2)

	_, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "All day with zone",
		StartDate:  stringPtr("2026-03-03"),
		TimeZone:   &timeZone,
	})
	if err == nil {
		t.Fatal("CreateEvent(all-day timezone) error = nil, want validation error")
	}
	var validationErr *service.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("CreateEvent(all-day timezone) error = %T, want ValidationError", err)
	}

	mismatched := time.Date(2026, 3, 10, 9, 0, 0, 0, time.FixedZone("", -5*60*60))
	_, err = svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Mismatch",
		StartAt:    &mismatched,
		TimeZone:   &timeZone,
	})
	if err == nil {
		t.Fatal("CreateEvent(offset mismatch) error = nil, want validation error")
	}
	if !errors.As(err, &validationErr) {
		t.Fatalf("CreateEvent(offset mismatch) error = %T, want ValidationError", err)
	}

	event, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Weekly sync",
		StartAt:    &startAt,
		TimeZone:   &timeZone,
		Recurrence: &domain.RecurrenceRule{
			Frequency: domain.RecurrenceFrequencyWeekly,
			Count:     &count,
		},
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}

	moved, err := svc.RescheduleEventOccurrence(event.ID, domain.OccurrenceMutationRequest{
		OccurrenceAt:  timePtr(time.Date(2026, 3, 10, 9, 0, 0, 0, time.FixedZone("", -4*60*60))),
		ReplacementAt: timePtr(time.Date(2026, 3, 11, 10, 0, 0, 0, time.FixedZone("", -4*60*60))),
	})
	if err != nil {
		t.Fatalf("RescheduleEventOccurrence(): %v", err)
	}

	page, err := svc.ListAgenda(domain.AgendaParams{
		From:  time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListAgenda(): %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("agenda items = %#v, want original and moved occurrence", page.Items)
	}
	if page.Items[1].OccurrenceKey != moved.OccurrenceKey || page.Items[1].StartAt == nil || page.Items[1].StartAt.Format(time.RFC3339) != "2026-03-11T10:00:00-04:00" {
		t.Fatalf("moved occurrence = %#v, want stable key at replacement time", page.Items[1])
	}
}

func TestRecurringTaskOccurrenceStateAndCompletionByKey(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	count := int32(3)

	task, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Review",
		DueDate:    stringPtr("2026-04-16"),
		Recurrence: &domain.RecurrenceRule{
			Frequency: domain.RecurrenceFrequencyDaily,
			Count:     &count,
		},
	})
	if err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}

	_, err = svc.CancelTaskOccurrence(task.ID, domain.OccurrenceMutationRequest{
		OccurrenceDate: stringPtr("2026-04-17"),
	})
	if err != nil {
		t.Fatalf("CancelTaskOccurrence(): %v", err)
	}
	_, err = svc.CompleteTask(task.ID, domain.TaskCompletionRequest{OccurrenceDate: stringPtr("2026-04-17")})
	if err == nil {
		t.Fatal("CompleteTask(canceled) error = nil, want conflict")
	}
	var conflictErr *service.ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("CompleteTask(canceled) error = %T, want ConflictError", err)
	}

	moved, err := svc.RescheduleTaskOccurrence(task.ID, domain.OccurrenceMutationRequest{
		OccurrenceDate:  stringPtr("2026-04-18"),
		ReplacementDate: stringPtr("2026-04-19"),
	})
	if err != nil {
		t.Fatalf("RescheduleTaskOccurrence(): %v", err)
	}
	completion, err := svc.CompleteTask(task.ID, domain.TaskCompletionRequest{OccurrenceKey: moved.OccurrenceKey})
	if err != nil {
		t.Fatalf("CompleteTask(occurrence key): %v", err)
	}
	if completion.OccurrenceKey != moved.OccurrenceKey || completion.OccurrenceDate == nil || *completion.OccurrenceDate != "2026-04-18" {
		t.Fatalf("completion = %#v, want stable original occurrence key/date", completion)
	}

	page, err := svc.ListAgenda(domain.AgendaParams{
		From:  time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListAgenda(): %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("agenda items = %#v, want first and moved task occurrences", page.Items)
	}
	if page.Items[1].OccurrenceKey != moved.OccurrenceKey || page.Items[1].DueDate == nil || *page.Items[1].DueDate != "2026-04-19" || page.Items[1].CompletedAt == nil {
		t.Fatalf("moved agenda item = %#v, want replacement date with completion", page.Items[1])
	}
}

func TestRescheduledEventOccurrenceIgnoredAfterRecurrenceUpdate(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	startAt := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)
	count := int32(3)

	event, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Standup",
		StartAt:    &startAt,
		Recurrence: &domain.RecurrenceRule{
			Frequency: domain.RecurrenceFrequencyDaily,
			Count:     &count,
		},
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}
	moved, err := svc.RescheduleEventOccurrence(event.ID, domain.OccurrenceMutationRequest{
		OccurrenceAt:  timePtr(time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)),
		ReplacementAt: timePtr(time.Date(2026, 4, 20, 11, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("RescheduleEventOccurrence(): %v", err)
	}

	reducedCount := int32(2)
	_, err = svc.UpdateEvent(event.ID, domain.EventPatch{
		Recurrence: domain.SetPatch(domain.RecurrenceRule{
			Frequency: domain.RecurrenceFrequencyDaily,
			Count:     &reducedCount,
		}),
	})
	if err != nil {
		t.Fatalf("UpdateEvent(): %v", err)
	}

	page, err := svc.ListAgenda(domain.AgendaParams{
		From:  time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListAgenda(): %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("agenda items = %#v, want only current scheduled occurrences", page.Items)
	}
	for _, item := range page.Items {
		if item.OccurrenceKey == moved.OccurrenceKey {
			t.Fatalf("agenda item = %#v, want stale moved occurrence omitted", item)
		}
	}
}

func TestRescheduledTaskOccurrenceIgnoredAfterRecurrenceUpdate(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	count := int32(3)

	task, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Review",
		DueDate:    stringPtr("2026-04-16"),
		Recurrence: &domain.RecurrenceRule{
			Frequency: domain.RecurrenceFrequencyDaily,
			Count:     &count,
		},
		Reminders: []domain.ReminderRule{{BeforeMinutes: 60}},
	})
	if err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}
	moved, err := svc.RescheduleTaskOccurrence(task.ID, domain.OccurrenceMutationRequest{
		OccurrenceDate:  stringPtr("2026-04-18"),
		ReplacementDate: stringPtr("2026-04-20"),
	})
	if err != nil {
		t.Fatalf("RescheduleTaskOccurrence(): %v", err)
	}

	reducedCount := int32(2)
	_, err = svc.UpdateTask(task.ID, domain.TaskPatch{
		Recurrence: domain.SetPatch(domain.RecurrenceRule{
			Frequency: domain.RecurrenceFrequencyDaily,
			Count:     &reducedCount,
		}),
	})
	if err != nil {
		t.Fatalf("UpdateTask(): %v", err)
	}

	page, err := svc.ListAgenda(domain.AgendaParams{
		From:  time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListAgenda(): %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("agenda items = %#v, want only current scheduled occurrences", page.Items)
	}
	for _, item := range page.Items {
		if item.OccurrenceKey == moved.OccurrenceKey {
			t.Fatalf("agenda item = %#v, want stale moved occurrence omitted", item)
		}
	}

	reminders, err := svc.ListPendingReminders(domain.ReminderQueryParams{
		From:  time.Date(2026, 4, 19, 22, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 4, 20, 1, 0, 0, 0, time.UTC),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListPendingReminders(): %v", err)
	}
	if len(reminders.Items) != 0 {
		t.Fatalf("pending reminders = %#v, want stale moved reminder omitted", reminders.Items)
	}
}

func TestRecurringTimedUntilDateStopsAtEndOfDay(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	dueAt := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	untilDate := "2026-04-15"

	task, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Recurring midnight task",
		DueAt:      &dueAt,
		Recurrence: &domain.RecurrenceRule{
			Frequency: domain.RecurrenceFrequencyDaily,
			UntilDate: &untilDate,
		},
	})
	if err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}

	page, err := svc.ListAgenda(domain.AgendaParams{
		From:  time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListAgenda(): %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("ListAgenda() returned %d items, want 1", len(page.Items))
	}

	_, err = svc.CompleteTask(task.ID, domain.TaskCompletionRequest{
		OccurrenceAt: timePtr(time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)),
	})
	if err == nil {
		t.Fatal("CompleteTask() error = nil, want validation error")
	}

	var validationErr *service.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("CompleteTask() error = %T, want ValidationError", err)
	}
}

func TestUpdateCalendarPatchSetOmitAndClear(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar, err := svc.CreateCalendar(domain.Calendar{
		Name:        "Work",
		Description: stringPtr("Planning"),
		Color:       stringPtr("#112233"),
	})
	if err != nil {
		t.Fatalf("CreateCalendar(): %v", err)
	}

	updated, err := svc.UpdateCalendar(calendar.ID, domain.CalendarPatch{
		Description: domain.SetPatch("Delivery"),
	})
	if err != nil {
		t.Fatalf("UpdateCalendar(set): %v", err)
	}
	if updated.Description == nil || *updated.Description != "Delivery" {
		t.Fatalf("description = %#v, want Delivery", updated.Description)
	}
	if updated.Color == nil || *updated.Color != "#112233" {
		t.Fatalf("color = %#v, want omitted field preserved", updated.Color)
	}

	cleared, err := svc.UpdateCalendar(calendar.ID, domain.CalendarPatch{
		Color: domain.ClearPatch[string](),
	})
	if err != nil {
		t.Fatalf("UpdateCalendar(clear): %v", err)
	}
	if cleared.Color != nil {
		t.Fatalf("color = %#v, want cleared", cleared.Color)
	}

	_, err = svc.UpdateCalendar(calendar.ID, domain.CalendarPatch{
		Name: domain.ClearPatch[string](),
	})
	if err == nil {
		t.Fatal("UpdateCalendar(clear name) error = nil, want validation error")
	}
	var validationErr *service.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("UpdateCalendar(clear name) error = %T, want ValidationError", err)
	}
}

func TestUpdateEventPatchClearRecurrenceAndSwitchTiming(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	startAt := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)
	endAt := startAt.Add(time.Hour)
	timeZone := "UTC"
	event, err := svc.CreateEvent(domain.Event{
		CalendarID:  calendar.ID,
		Title:       "Standup",
		Description: stringPtr("Daily sync"),
		StartAt:     &startAt,
		EndAt:       &endAt,
		TimeZone:    &timeZone,
		Recurrence: &domain.RecurrenceRule{
			Frequency: domain.RecurrenceFrequencyDaily,
			Count:     int32Ptr(3),
		},
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}

	renamed, err := svc.UpdateEvent(event.ID, domain.EventPatch{
		Title:       domain.SetPatch("Team standup"),
		Description: domain.ClearPatch[string](),
		Recurrence:  domain.ClearPatch[domain.RecurrenceRule](),
	})
	if err != nil {
		t.Fatalf("UpdateEvent(clear): %v", err)
	}
	if renamed.Title != "Team standup" || renamed.Description != nil || renamed.Recurrence != nil {
		t.Fatalf("updated event = %#v, want title set and optional fields cleared", renamed)
	}
	if renamed.StartAt == nil || renamed.EndAt == nil {
		t.Fatalf("timing = %#v/%#v, want omitted fields preserved", renamed.StartAt, renamed.EndAt)
	}

	startDate := "2026-04-17"
	allDay, err := svc.UpdateEvent(event.ID, domain.EventPatch{
		StartAt:   domain.ClearPatch[time.Time](),
		EndAt:     domain.ClearPatch[time.Time](),
		StartDate: domain.SetPatch(startDate),
	})
	if err != nil {
		t.Fatalf("UpdateEvent(switch all-day): %v", err)
	}
	if allDay.StartAt != nil || allDay.EndAt != nil || allDay.TimeZone != nil || allDay.StartDate == nil || *allDay.StartDate != startDate {
		t.Fatalf("all-day event = %#v, want timed fields and timezone cleared with start_date set", allDay)
	}

	_, err = svc.UpdateEvent(event.ID, domain.EventPatch{
		Title: domain.ClearPatch[string](),
	})
	if err == nil {
		t.Fatal("UpdateEvent(clear title) error = nil, want validation error")
	}
	var validationErr *service.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("UpdateEvent(clear title) error = %T, want ValidationError", err)
	}
}

func TestUpdateTaskPatchClearAndSwitchDueMode(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	dueDate := "2026-04-16"
	task, err := svc.CreateTask(domain.Task{
		CalendarID:  calendar.ID,
		Title:       "Review",
		Description: stringPtr("Notes"),
		DueDate:     &dueDate,
		Recurrence: &domain.RecurrenceRule{
			Frequency: domain.RecurrenceFrequencyDaily,
			Count:     int32Ptr(2),
		},
	})
	if err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}

	dueAt := time.Date(2026, 4, 16, 11, 0, 0, 0, time.UTC)
	updated, err := svc.UpdateTask(task.ID, domain.TaskPatch{
		Description: domain.ClearPatch[string](),
		DueDate:     domain.ClearPatch[string](),
		DueAt:       domain.SetPatch(dueAt),
		Recurrence:  domain.ClearPatch[domain.RecurrenceRule](),
	})
	if err != nil {
		t.Fatalf("UpdateTask(): %v", err)
	}
	if updated.Description != nil || updated.DueDate != nil || updated.DueAt == nil || !updated.DueAt.Equal(dueAt) || updated.Recurrence != nil {
		t.Fatalf("updated task = %#v, want clear and due mode switch", updated)
	}

	_, err = svc.UpdateTask(task.ID, domain.TaskPatch{
		Title: domain.ClearPatch[string](),
	})
	if err == nil {
		t.Fatal("UpdateTask(clear title) error = nil, want validation error")
	}
	var validationErr *service.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("UpdateTask(clear title) error = %T, want ValidationError", err)
	}
}

func TestTaskMetadataDefaultsUpdateFiltersAndValidation(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	dueDate := "2026-04-16"

	defaulted, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Default metadata",
		DueDate:    &dueDate,
	})
	if err != nil {
		t.Fatalf("CreateTask(defaulted): %v", err)
	}
	if defaulted.Priority != domain.TaskPriorityMedium || defaulted.Status != domain.TaskStatusTodo || len(defaulted.Tags) != 0 {
		t.Fatalf("default metadata = priority:%q status:%q tags:%v, want medium/todo/[]", defaulted.Priority, defaulted.Status, defaulted.Tags)
	}

	task, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Tagged review",
		DueDate:    &dueDate,
		Priority:   domain.TaskPriorityHigh,
		Status:     domain.TaskStatusInProgress,
		Tags:       []string{" Planning ", "review"},
	})
	if err != nil {
		t.Fatalf("CreateTask(metadata): %v", err)
	}
	if task.Priority != domain.TaskPriorityHigh || task.Status != domain.TaskStatusInProgress || !slices.Equal(task.Tags, []string{"planning", "review"}) {
		t.Fatalf("task metadata = priority:%q status:%q tags:%v", task.Priority, task.Status, task.Tags)
	}

	filtered, err := svc.ListTasks(domain.TaskListParams{
		PageParams: domain.PageParams{CalendarID: calendar.ID, Limit: 10},
		Priority:   domain.TaskPriorityHigh,
		Status:     domain.TaskStatusInProgress,
		Tags:       []string{"review", "planning"},
	})
	if err != nil {
		t.Fatalf("ListTasks(filtered): %v", err)
	}
	if len(filtered.Items) != 1 || filtered.Items[0].ID != task.ID {
		t.Fatalf("filtered tasks = %#v, want tagged review only", filtered.Items)
	}

	updated, err := svc.UpdateTask(task.ID, domain.TaskPatch{
		Priority: domain.SetPatch(domain.TaskPriorityLow),
		Tags:     domain.SetPatch([]string{"later"}),
	})
	if err != nil {
		t.Fatalf("UpdateTask(metadata): %v", err)
	}
	if updated.Priority != domain.TaskPriorityLow || !slices.Equal(updated.Tags, []string{"later"}) {
		t.Fatalf("updated metadata = priority:%q tags:%v, want low/[later]", updated.Priority, updated.Tags)
	}

	cleared, err := svc.UpdateTask(task.ID, domain.TaskPatch{Tags: domain.ClearPatch[[]string]()})
	if err != nil {
		t.Fatalf("UpdateTask(clear tags): %v", err)
	}
	if len(cleared.Tags) != 0 {
		t.Fatalf("cleared tags = %v, want []", cleared.Tags)
	}

	invalidUpdates := []struct {
		name  string
		patch domain.TaskPatch
	}{
		{
			name:  "empty priority",
			patch: domain.TaskPatch{Priority: domain.SetPatch(domain.TaskPriority(""))},
		},
		{
			name:  "empty status",
			patch: domain.TaskPatch{Status: domain.SetPatch(domain.TaskStatus(""))},
		},
	}
	for _, test := range invalidUpdates {
		t.Run("update "+test.name, func(t *testing.T) {
			_, err := svc.UpdateTask(task.ID, test.patch)
			if err == nil {
				t.Fatal("UpdateTask() error = nil, want validation error")
			}
			var validationErr *service.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("UpdateTask() error = %T, want ValidationError", err)
			}
		})
	}

	invalidCases := []struct {
		name string
		task domain.Task
	}{
		{
			name: "priority",
			task: domain.Task{CalendarID: calendar.ID, Title: "Bad priority", Priority: domain.TaskPriority("urgent")},
		},
		{
			name: "status",
			task: domain.Task{CalendarID: calendar.ID, Title: "Bad status", Status: domain.TaskStatus("blocked")},
		},
		{
			name: "tag",
			task: domain.Task{CalendarID: calendar.ID, Title: "Bad tag", Tags: []string{"needs review"}},
		},
	}
	for _, test := range invalidCases {
		t.Run(test.name, func(t *testing.T) {
			_, err := svc.CreateTask(test.task)
			if err == nil {
				t.Fatal("CreateTask() error = nil, want validation error")
			}
			var validationErr *service.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("CreateTask() error = %T, want ValidationError", err)
			}
		})
	}
}

func TestTaskStatusDoneCompletionInvariants(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	dueDate := "2026-04-16"

	task, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Review",
		DueDate:    &dueDate,
	})
	if err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}
	completed, err := svc.CompleteTask(task.ID, domain.TaskCompletionRequest{})
	if err != nil {
		t.Fatalf("CompleteTask(): %v", err)
	}
	if completed.CompletedAt.IsZero() {
		t.Fatal("completion completed_at is zero")
	}
	stored, err := svc.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask(): %v", err)
	}
	if stored.Status != domain.TaskStatusDone || stored.CompletedAt == nil {
		t.Fatalf("stored task = %#v, want done with completed_at", stored)
	}

	_, err = svc.UpdateTask(task.ID, domain.TaskPatch{Status: domain.SetPatch(domain.TaskStatusTodo)})
	if err == nil {
		t.Fatal("UpdateTask(reopen) error = nil, want validation error")
	}
	var validationErr *service.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("UpdateTask(reopen) error = %T, want ValidationError", err)
	}

	recurring, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Recurring",
		DueDate:    &dueDate,
		Recurrence: &domain.RecurrenceRule{Frequency: domain.RecurrenceFrequencyDaily, Count: int32Ptr(2)},
	})
	if err != nil {
		t.Fatalf("CreateTask(recurring): %v", err)
	}
	_, err = svc.UpdateTask(recurring.ID, domain.TaskPatch{Status: domain.SetPatch(domain.TaskStatusDone)})
	if err == nil {
		t.Fatal("UpdateTask(recurring done) error = nil, want validation error")
	}
	if !errors.As(err, &validationErr) {
		t.Fatalf("UpdateTask(recurring done) error = %T, want ValidationError", err)
	}
}

func TestImportICalendarClonesOpenPlannerExportIntoRequestedCalendar(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	sourceCalendar := createCalendar(t, svc)
	targetCalendar, err := svc.CreateCalendar(domain.Calendar{Name: "Target"})
	if err != nil {
		t.Fatalf("CreateCalendar(target): %v", err)
	}
	startDate := "2026-04-16"
	if _, err := svc.CreateEvent(domain.Event{
		CalendarID: sourceCalendar.ID,
		Title:      "Source event",
		StartDate:  &startDate,
	}); err != nil {
		t.Fatalf("CreateEvent(source): %v", err)
	}

	exported, err := svc.ExportICalendar(sourceCalendar.ID)
	if err != nil {
		t.Fatalf("ExportICalendar(): %v", err)
	}
	content := strings.Replace(exported.Content, "SUMMARY:Source event", "SUMMARY:Imported clone", 1)
	if _, err := svc.ImportICalendar(domain.ICalendarImportRequest{
		CalendarID: targetCalendar.ID,
		Content:    content,
	}); err != nil {
		t.Fatalf("ImportICalendar(): %v", err)
	}

	sourceEvents, err := svc.ListEvents(domain.PageParams{CalendarID: sourceCalendar.ID})
	if err != nil {
		t.Fatalf("ListEvents(source): %v", err)
	}
	if len(sourceEvents.Items) != 1 || sourceEvents.Items[0].Title != "Source event" {
		t.Fatalf("source events = %#v, want unchanged source event", sourceEvents.Items)
	}
	targetEvents, err := svc.ListEvents(domain.PageParams{CalendarID: targetCalendar.ID})
	if err != nil {
		t.Fatalf("ListEvents(target): %v", err)
	}
	if len(targetEvents.Items) != 1 || targetEvents.Items[0].Title != "Imported clone" {
		t.Fatalf("target events = %#v, want imported clone in target calendar", targetEvents.Items)
	}
}

func TestImportICalendarReimportRemovesStaleOccurrenceState(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	withExdate := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:event-stale@example.com",
		"SUMMARY:Daily focus",
		"DTSTART;VALUE=DATE:20260416",
		"RRULE:FREQ=DAILY;COUNT=2",
		"EXDATE;VALUE=DATE:20260417",
		"END:VEVENT",
		"END:VCALENDAR",
	}, "\r\n")
	withoutExdate := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:event-stale@example.com",
		"SUMMARY:Daily focus",
		"DTSTART;VALUE=DATE:20260416",
		"RRULE:FREQ=DAILY;COUNT=2",
		"END:VEVENT",
		"END:VCALENDAR",
	}, "\r\n")

	if _, err := svc.ImportICalendar(domain.ICalendarImportRequest{
		CalendarID: calendar.ID,
		Content:    withExdate,
	}); err != nil {
		t.Fatalf("ImportICalendar(withExdate): %v", err)
	}
	firstAgenda, err := svc.ListAgenda(domain.AgendaParams{
		From: time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ListAgenda(first): %v", err)
	}
	if len(firstAgenda.Items) != 1 {
		t.Fatalf("first agenda = %#v, want EXDATE to suppress one occurrence", firstAgenda.Items)
	}

	if _, err := svc.ImportICalendar(domain.ICalendarImportRequest{
		CalendarID: calendar.ID,
		Content:    withoutExdate,
	}); err != nil {
		t.Fatalf("ImportICalendar(withoutExdate): %v", err)
	}
	secondAgenda, err := svc.ListAgenda(domain.AgendaParams{
		From: time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ListAgenda(second): %v", err)
	}
	if len(secondAgenda.Items) != 2 {
		t.Fatalf("second agenda = %#v, want reimport to restore removed occurrence", secondAgenda.Items)
	}
}

func TestReminderPendingQueriesAndDismissal(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	otherCalendar, err := svc.CreateCalendar(domain.Calendar{Name: "Other"})
	if err != nil {
		t.Fatalf("CreateCalendar(other): %v", err)
	}

	eventStart := time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC)
	event, err := svc.CreateEvent(domain.Event{
		CalendarID: calendar.ID,
		Title:      "Standup",
		StartAt:    &eventStart,
		Reminders:  []domain.ReminderRule{{BeforeMinutes: 60}},
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}
	if len(event.Reminders) != 1 || event.Reminders[0].ID == "" {
		t.Fatalf("event reminders = %#v, want generated reminder id", event.Reminders)
	}

	dueDate := "2026-04-16"
	if _, err := svc.CreateTask(domain.Task{
		CalendarID: otherCalendar.ID,
		Title:      "Filtered out",
		DueDate:    &dueDate,
		Reminders:  []domain.ReminderRule{{BeforeMinutes: 30}},
	}); err != nil {
		t.Fatalf("CreateTask(other): %v", err)
	}

	dueAt := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	recurring, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Daily review",
		DueAt:      &dueAt,
		Recurrence: &domain.RecurrenceRule{Frequency: domain.RecurrenceFrequencyDaily, Count: int32Ptr(2)},
		Reminders:  []domain.ReminderRule{{BeforeMinutes: 15}},
	})
	if err != nil {
		t.Fatalf("CreateTask(recurring): %v", err)
	}
	if len(recurring.Reminders) != 1 || recurring.Reminders[0].ID == "" {
		t.Fatalf("recurring reminders = %#v, want generated reminder id", recurring.Reminders)
	}

	dateTask, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Medicine",
		DueDate:    &dueDate,
		Reminders:  []domain.ReminderRule{{BeforeMinutes: 30}},
	})
	if err != nil {
		t.Fatalf("CreateTask(date reminder): %v", err)
	}

	page, err := svc.ListPendingReminders(domain.ReminderQueryParams{
		From:       time.Date(2026, 4, 15, 23, 0, 0, 0, time.UTC),
		To:         time.Date(2026, 4, 17, 13, 0, 0, 0, time.UTC),
		CalendarID: calendar.ID,
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("ListPendingReminders(): %v", err)
	}
	if len(page.Items) != 4 {
		t.Fatalf("pending reminders length = %d, want 4: %#v", len(page.Items), page.Items)
	}
	if page.Items[0].Title != "Medicine" || !page.Items[0].RemindAt.Equal(time.Date(2026, 4, 15, 23, 30, 0, 0, time.UTC)) || page.Items[0].DueDate == nil || *page.Items[0].DueDate != dueDate {
		t.Fatalf("first pending reminder = %#v, want date task at UTC midnight minus offset", page.Items[0])
	}
	if page.Items[1].Title != "Standup" || !page.Items[1].RemindAt.Equal(time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("second pending reminder = %#v, want Standup at 09:00", page.Items[1])
	}
	if page.Items[2].Title != "Daily review" || page.Items[3].Title != "Daily review" {
		t.Fatalf("recurring reminders = %#v/%#v, want two Daily review occurrences", page.Items[2], page.Items[3])
	}
	if page.Items[0].ID == "" || page.Items[0].ReminderID != dateTask.Reminders[0].ID {
		t.Fatalf("pending reminder id fields = %#v", page.Items[0])
	}

	firstPage, err := svc.ListPendingReminders(domain.ReminderQueryParams{
		From:       time.Date(2026, 4, 15, 23, 0, 0, 0, time.UTC),
		To:         time.Date(2026, 4, 17, 13, 0, 0, 0, time.UTC),
		CalendarID: calendar.ID,
		Limit:      2,
	})
	if err != nil {
		t.Fatalf("ListPendingReminders(first page): %v", err)
	}
	if len(firstPage.Items) != 2 || firstPage.NextCursor == nil {
		t.Fatalf("first page = %#v, want two items and next cursor", firstPage)
	}
	secondPage, err := svc.ListPendingReminders(domain.ReminderQueryParams{
		From:       time.Date(2026, 4, 15, 23, 0, 0, 0, time.UTC),
		To:         time.Date(2026, 4, 17, 13, 0, 0, 0, time.UTC),
		CalendarID: calendar.ID,
		Cursor:     *firstPage.NextCursor,
		Limit:      2,
	})
	if err != nil {
		t.Fatalf("ListPendingReminders(second page): %v", err)
	}
	if len(secondPage.Items) != 2 {
		t.Fatalf("second page length = %d, want 2", len(secondPage.Items))
	}

	dismissal, err := svc.DismissReminderOccurrence(page.Items[0].ID)
	if err != nil {
		t.Fatalf("DismissReminderOccurrence(first): %v", err)
	}
	if dismissal.AlreadyDismissed {
		t.Fatalf("first dismissal = %#v, want newly dismissed", dismissal)
	}
	repeated, err := svc.DismissReminderOccurrence(page.Items[0].ID)
	if err != nil {
		t.Fatalf("DismissReminderOccurrence(second): %v", err)
	}
	if !repeated.AlreadyDismissed {
		t.Fatalf("second dismissal = %#v, want already dismissed", repeated)
	}
	afterDismiss, err := svc.ListPendingReminders(domain.ReminderQueryParams{
		From:       time.Date(2026, 4, 15, 23, 0, 0, 0, time.UTC),
		To:         time.Date(2026, 4, 17, 13, 0, 0, 0, time.UTC),
		CalendarID: calendar.ID,
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("ListPendingReminders(after dismiss): %v", err)
	}
	if len(afterDismiss.Items) != 3 || afterDismiss.Items[0].Title == "Medicine" {
		t.Fatalf("after dismiss = %#v, want dismissed occurrence omitted", afterDismiss.Items)
	}
}

func TestPendingRemindersUseOccurrenceState(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	dueAt := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)

	task, err := svc.CreateTask(domain.Task{
		CalendarID: calendar.ID,
		Title:      "Daily review",
		DueAt:      &dueAt,
		Recurrence: &domain.RecurrenceRule{Frequency: domain.RecurrenceFrequencyDaily, Count: int32Ptr(2)},
		Reminders:  []domain.ReminderRule{{BeforeMinutes: 15}},
	})
	if err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}
	moved, err := svc.RescheduleTaskOccurrence(task.ID, domain.OccurrenceMutationRequest{
		OccurrenceAt:  timePtr(time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)),
		ReplacementAt: timePtr(time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("RescheduleTaskOccurrence(): %v", err)
	}

	page, err := svc.ListPendingReminders(domain.ReminderQueryParams{
		From:  time.Date(2026, 4, 17, 11, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListPendingReminders(): %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].OccurrenceKey != moved.OccurrenceKey || !page.Items[0].RemindAt.Equal(time.Date(2026, 4, 18, 8, 45, 0, 0, time.UTC)) {
		t.Fatalf("pending reminders = %#v, want moved reminder with stable key", page.Items)
	}
}

func TestReminderValidationRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	calendar := createCalendar(t, svc)
	dueDate := "2026-04-16"

	invalidCases := []struct {
		name string
		task domain.Task
	}{
		{
			name: "non-positive offset",
			task: domain.Task{CalendarID: calendar.ID, Title: "Bad reminder", DueDate: &dueDate, Reminders: []domain.ReminderRule{{BeforeMinutes: 0}}},
		},
		{
			name: "duplicate offset",
			task: domain.Task{CalendarID: calendar.ID, Title: "Duplicate reminder", DueDate: &dueDate, Reminders: []domain.ReminderRule{{BeforeMinutes: 30}, {BeforeMinutes: 30}}},
		},
		{
			name: "missing due",
			task: domain.Task{CalendarID: calendar.ID, Title: "No due", Reminders: []domain.ReminderRule{{BeforeMinutes: 30}}},
		},
	}
	for _, test := range invalidCases {
		t.Run(test.name, func(t *testing.T) {
			_, err := svc.CreateTask(test.task)
			if err == nil {
				t.Fatal("CreateTask() error = nil, want validation error")
			}
			var validationErr *service.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("CreateTask() error = %T, want ValidationError", err)
			}
		})
	}

	_, err := svc.ListPendingReminders(domain.ReminderQueryParams{
		From: time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("ListPendingReminders(invalid range) error = nil, want validation error")
	}
	var validationErr *service.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("ListPendingReminders(invalid range) error = %T, want ValidationError", err)
	}

	_, err = svc.DismissReminderOccurrence("not-valid")
	if err == nil {
		t.Fatal("DismissReminderOccurrence(invalid id) error = nil, want validation error")
	}
	if !errors.As(err, &validationErr) {
		t.Fatalf("DismissReminderOccurrence(invalid id) error = %T, want ValidationError", err)
	}
}

func int32Ptr(value int32) *int32 {
	return &value
}

func stringPtr(value string) *string {
	return &value
}

func timePtr(value time.Time) *time.Time {
	return &value
}
