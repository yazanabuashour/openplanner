package service_test

import (
	"errors"
	"path/filepath"
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

	firstPage, err := svc.ListTasks(domain.PageParams{Limit: 2})
	if err != nil {
		t.Fatalf("ListTasks(first page): %v", err)
	}
	if len(firstPage.Items) != 2 {
		t.Fatalf("first page length = %d, want 2", len(firstPage.Items))
	}
	if firstPage.NextCursor == nil {
		t.Fatal("first page next cursor = nil")
	}

	secondPage, err := svc.ListTasks(domain.PageParams{Cursor: *firstPage.NextCursor, Limit: 2})
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

	firstPage, err := svc.ListTasks(domain.PageParams{Limit: 1})
	if err != nil {
		t.Fatalf("ListTasks(first page): %v", err)
	}
	if firstPage.NextCursor == nil {
		t.Fatal("first page next cursor = nil")
	}
	if err := svc.DeleteTask(firstPage.Items[0].ID); err != nil {
		t.Fatalf("DeleteTask(): %v", err)
	}

	_, err = svc.ListTasks(domain.PageParams{Cursor: *firstPage.NextCursor, Limit: 1})
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
				_, err := svc.ListTasks(domain.PageParams{CalendarID: "not-a-ulid"})
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

func int32Ptr(value int32) *int32 {
	return &value
}

func stringPtr(value string) *string {
	return &value
}

func timePtr(value time.Time) *time.Time {
	return &value
}
