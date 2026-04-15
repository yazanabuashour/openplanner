package sdk_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/yazanabuashour/openplanner/sdk"
	"github.com/yazanabuashour/openplanner/sdk/generated"
)

func TestOpenLocalGeneratedClientCRUDAndAgenda(t *testing.T) {
	t.Parallel()

	client, err := sdk.OpenLocal(sdk.Options{
		DatabasePath: filepath.Join(t.TempDir(), "openplanner.db"),
	})
	if err != nil {
		t.Fatalf("OpenLocal(): %v", err)
	}
	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			t.Fatalf("Close(): %v", closeErr)
		}
	}()

	ctx := context.Background()
	calendar, _, err := client.CalendarsAPI.CreateCalendar(ctx).CreateCalendarRequest(generated.CreateCalendarRequest{
		Name: "Personal",
	}).Execute()
	if err != nil {
		t.Fatalf("CreateCalendar(): %v", err)
	}

	startAt := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)
	endAt := startAt.Add(time.Hour)
	count := int32(2)

	event, _, err := client.EventsAPI.CreateEvent(ctx).CreateEventRequest(generated.CreateEventRequest{
		CalendarId: calendar.Id,
		Title:      "Standup",
		StartAt:    &startAt,
		EndAt:      &endAt,
		Recurrence: &generated.RecurrenceRule{
			Frequency: generated.RECURRENCEFREQUENCY_DAILY,
			Count:     &count,
		},
	}).Execute()
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}

	dueDate := "2026-04-16"
	task, _, err := client.TasksAPI.CreateTask(ctx).CreateTaskRequest(generated.CreateTaskRequest{
		CalendarId: calendar.Id,
		Title:      "Review notes",
		DueDate:    &dueDate,
		Recurrence: &generated.RecurrenceRule{
			Frequency: generated.RECURRENCEFREQUENCY_DAILY,
			Count:     &count,
		},
	}).Execute()
	if err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}

	_, _, err = client.TasksAPI.CompleteTask(ctx, task.Id).CompleteTaskRequest(generated.CompleteTaskRequest{
		OccurrenceDate: &dueDate,
	}).Execute()
	if err != nil {
		t.Fatalf("CompleteTask(): %v", err)
	}

	from := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC)
	agenda, _, err := client.AgendaAPI.ListAgenda(ctx).From(from).To(to).Execute()
	if err != nil {
		t.Fatalf("ListAgenda(): %v", err)
	}
	if len(agenda.Items) != 4 {
		t.Fatalf("agenda item count = %d, want 4", len(agenda.Items))
	}

	updateTitle := "Standup (updated)"
	updatedEvent, _, err := client.EventsAPI.UpdateEvent(ctx, event.Id).UpdateEventRequest(generated.UpdateEventRequest{
		Title: &updateTitle,
	}).Execute()
	if err != nil {
		t.Fatalf("UpdateEvent(): %v", err)
	}
	if updatedEvent.Title != updateTitle {
		t.Fatalf("updated title = %q, want %q", updatedEvent.Title, updateTitle)
	}

	if _, err := client.TasksAPI.DeleteTask(ctx, task.Id).Execute(); err != nil {
		t.Fatalf("DeleteTask(): %v", err)
	}
	if _, err := client.EventsAPI.DeleteEvent(ctx, event.Id).Execute(); err != nil {
		t.Fatalf("DeleteEvent(): %v", err)
	}

	tasksPage, _, err := client.TasksAPI.ListTasks(ctx).Execute()
	if err != nil {
		t.Fatalf("ListTasks(): %v", err)
	}
	if len(tasksPage.Items) != 0 {
		t.Fatalf("task count after delete = %d, want 0", len(tasksPage.Items))
	}
}
